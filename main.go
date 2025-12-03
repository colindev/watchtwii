package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// --- 設定區 (建議透過環境變數注入) ---
const (

	// 這裡填入您實際要抓取的網站與 XPath
	// 範例：Yahoo 股市 (僅供參考，XPath 需隨網頁結構更新)
	SpotURL   = "https://tw.stock.yahoo.com/quote/%5ETWII" // 加權指數
	SpotXPath = "//*[@id='main-0-QuoteHeader-Proxy']/div/div[2]/div[1]/div/span[1]"

	FutureURL   = "https://tw.stock.yahoo.com/future/futures.html?fumr=futurefull" // 台指近一 (需確認網址是否為連續月)
	FutureXPath = "/html/body/div[1]/div/div/div/div/div[4]/div[1]/div/div/div[2]/div[3]/div[2]/div/div/ul/li[2]/div/div[4]/span"
)

func ScrapeData() (float64, float64, error) {
	// 1. 取得加權指數
	rawSpot, err := FetchValueString(SpotURL, SpotXPath)
	if err != nil {
		return 0, 0, fmt.Errorf("抓取加權指數失敗: %w", err)
	}
	spotVal, err := ParseToFloat(rawSpot)
	if err != nil {
		return 0, 0, fmt.Errorf("解析加權指數失敗: %w", err)
	}

	// 2. 取得台指期
	rawFuture, err := FetchValueString(FutureURL, FutureXPath)
	if err != nil {
		return 0, 0, fmt.Errorf("抓取台指期失敗: %w", err)
	}
	futureVal, err := ParseToFloat(rawFuture)
	if err != nil {
		return 0, 0, fmt.Errorf("解析台指期失敗: %w", err)
	}

	return spotVal, futureVal, nil
}

// 環境變數中的 Key
var (
	TelegramToken       = os.Getenv("TELEGRAM_TOKEN")
	TelegramChatIDs     = os.Getenv("TELEGRAM_CHAT_IDS") // 預期格式: "123456,789012"
	ThresholdEnv        = os.Getenv("THRESHOLD")
	ThresholdChangedEnv = os.Getenv("THRESHOLD_CHANGED")

	DebugEnv = os.Getenv("DEBUG")
)

func main() {
	fmt.Println("啟動排程檢查...")

	var threshold float64 = 100 // 預設值
	var err error
	// --- 讀取並驗證閾值 ---
	if ThresholdEnv == "" {
		fmt.Printf("❌ 錯誤: THRESHOLD 環境變數未設定。使用預設監控閾值: %.2f 點\n", threshold)
	} else {
		threshold, err = ParseToFloat(ThresholdEnv)
		if err != nil {
			fmt.Printf("❌ 錯誤: 無法解析 THRESHOLD 環境變數 '%s' 為浮點數: %v。使用預設監控閾值: %.2f 點\n", ThresholdEnv, err, threshold)
		} else {
			fmt.Printf("✅ 使用監控閾值: %.2f 點\n", threshold)
		}
	}

	thresholdChanged, err := ParseToFloat(ThresholdChangedEnv)
	if err != nil {
		fmt.Println("沒有設定價差抑制幅度 THRESHOLD_CHANGED, 預設使用10點")
		thresholdChanged = 10
	}

	// --- 判斷盤別 ---
	session, isTrading := GetSessionType()
	fmt.Printf("目前時段: %s, 是否交易中: %v\n", session, isTrading)

	if !isTrading {
		fmt.Println("目前非監控時段，結束程式。")
		return
	}

	// 從 Firestore 讀取上次被通知時的價差
	d, err := GetLastNotifiedData()
	if err != nil {
		// 進入此處代表發生了「初始化客戶端失敗」或「讀取文件失敗（非不存在）」
		// 這是無法運行業務邏輯的致命錯誤 (配置、權限、網路連線等)
		log.Fatalf("❌ Firestore 狀態讀取發生致命錯誤，請檢查配置與權限: %v", err)
	}

	if DebugEnv == "1" || strings.ToUpper(DebugEnv) == "TRUE" {
		fmt.Printf("%+v\n", d.Map()) // DEBUG
	}

	// --- 執行爬蟲與錯誤狀態管理 ---
	spotVal, futureVal, scrapeErr := ScrapeData()
	if scrapeErr != nil && spotVal == 0 && (IsTaipexPreOpen() || session == SessionNight) {
		if futureVal == 0 {
			fmt.Println("點數爬取錯誤") // 有機率為0
			return
		}
		spotVal = d.LastTWIIValue
		scrapeErr = nil
	}

	// 🎯 核心：使用 CheckErrorState 處理狀態變化 (正常<->失敗)
	shouldAlertError, errorMsg := d.CheckErrorState(scrapeErr)

	if shouldAlertError {
		fmt.Println("狀態改變，發送系統通知...")
		SendAlert(errorMsg)
	}

	// 發生錯誤後的處理：儲存錯誤狀態並退出
	if scrapeErr != nil {
		log.Printf("執行失敗: %v (Count: %d)", scrapeErr, d.ErrorCount)
		// ⚠️ 重要：即使失敗也要儲存，這樣下次才知道 ErrorCount > 0
		if err := SaveCurrentData(d); err != nil {
			log.Printf("❌ 無法儲存錯誤狀態: %v", err)
		}
		return // 結束程式
	}

	// --- 以下為成功抓取後的正常業務邏輯 ---
	// 此時 d.ErrorCount 已經被 CheckErrorState 重置為 0

	fmt.Printf("📊 加權指數: %.2f | 台指期: %.2f\n", spotVal, futureVal)

	msg, err := NewMessage(session)
	if err != nil {
		log.Fatalf("❌ 無法判斷開盤階段%s", session)
	}

	alertMsg, shouldNotify := msg.Build(d, spotVal, futureVal, threshold, thresholdChanged)

	// 判斷是否為關鍵時間
	specificAlterMsg, isSpecificTime := CheckSpecificTimeAlert()
	if isSpecificTime {
		shouldNotify = true
		// 如果沒有符合觸發條件, 但是特定時間點依然發送, 要補上訊息
		if alertMsg == "" {
			alertMsg = msg.Info(specificAlterMsg, spotVal, futureVal)
		}
	}

	shouldSave := d.UpdateDailyHighLow(spotVal, futureVal)
	// 🎯 儲存當前價差，用於下次比較
	d.LastTWIIValue = spotVal
	d.LastDiffValue = spotVal - futureVal

	// --- 發送 ---
	if shouldNotify {
		fmt.Println("觸發條件，發送 Telegram 通知...")
		SendAlert(alertMsg)
		if err := SaveCurrentData(d); err != nil {
			log.Printf("❌ 儲存當前價差失敗: %v\n", err)
		} else {
			fmt.Println("✅ 已儲存當前數據作為下次比較的基準。")
		}
	} else if shouldSave {
		fmt.Println("✅ 欄位資料異動，儲存新狀態...")
		if err := SaveCurrentData(d); err != nil {
			log.Printf("❌ 儲存恢復狀態失敗: %v", err)
		}
	} else if shouldAlertError { // (這代表剛剛發生了 Recovery)
		// 如果沒有觸發市場警報，但發生了系統狀態改變 (例如：Fail -> Normal Recovery)
		// 必須儲存 d，以更新 ErrorCount=0 的狀態。
		fmt.Println("✅ 系統恢復，儲存新狀態...")
		if err := SaveCurrentData(d); err != nil {
			log.Printf("❌ 儲存恢復狀態失敗: %v", err)
		}
	}
}
