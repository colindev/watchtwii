package main

import (
	"fmt"
	"log"
	"os"
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

// 環境變數中的 Key
var (
	TelegramToken       = os.Getenv("TELEGRAM_TOKEN")
	TelegramChatIDs     = os.Getenv("TELEGRAM_CHAT_IDS") // 預期格式: "123456,789012"
	ThresholdEnv        = os.Getenv("THRESHOLD")
	ThresholdChangedEnv = os.Getenv("THRESHOLD_CHANGED")
)

func main() {
	fmt.Println("啟動排程檢查...")

	var threshold float64 = 100 // 預設值
	var err error
	// --- 步驟 A: 讀取並驗證閾值 ---
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

	// --- 步驟 B: 判斷盤別 ---
	session, isTrading := GetSessionType()
	fmt.Printf("目前時段: %s, 是否交易中: %v\n", session, isTrading)

	if !isTrading {
		fmt.Println("目前非監控時段，結束程式。")
		return
	}

	// --- 步驟 C: 取得數值 ---
	// 1. 取得加權指數
	rawSpot, err := FetchValueString(SpotURL, SpotXPath)
	if err != nil {
		msg := fmt.Sprintf("❌ 抓取加權指數失敗: %v", err)
		log.Println(msg)
		SendAlert(msg)
		return
	}
	spotVal, err := ParseToFloat(rawSpot)
	if err != nil {
		msg := fmt.Sprintf("❌ 解析加權指數失敗: %v", err)
		log.Println(msg)
		SendAlert(msg)
		return
	}

	// 2. 取得台指期
	rawFuture, err := FetchValueString(FutureURL, FutureXPath)
	if err != nil {
		msg := fmt.Sprintf("❌ 抓取台指期失敗: %v", err)
		log.Println(msg)
		SendAlert(msg)
		return
	}
	futureVal, err := ParseToFloat(rawFuture)
	if err != nil {
		msg := fmt.Sprintf("❌ 解析台指期失敗: %v", err)
		log.Println(msg)
		SendAlert(msg)
		return
	}

	fmt.Printf("📊 加權指數: %.2f | 台指期: %.2f\n", spotVal, futureVal)

	thresholdChanged, err := ParseToFloat(ThresholdChangedEnv)
	if err != nil {
		fmt.Println("沒有設定價差抑制幅度 THRESHOLD_CHANGED, 預設使用10點")
		thresholdChanged = 10
	}

	// 從 Firestore 讀取上次被通知時的價差
	d, err := GetLastNotifiedData()
	if err != nil {
		log.Printf("❌ 無法讀取上次價差，跳過抑制邏輯: %v", err)
	}

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

	// --- 發送 ---
	if shouldNotify {
		fmt.Println("觸發條件，發送 Telegram 通知...")
		SendAlert(alertMsg)
		// 🎯 儲存當前價差，用於下次比較
		d.LastTWIIValue = spotVal
		d.LastDiffValue = spotVal - futureVal
		if err := SaveCurrentData(d); err != nil {
			log.Printf("❌ 儲存當前價差失敗: %v", err)
		} else {
			fmt.Printf("✅ 已儲存當前指數(%.2f)與價差 (%.2f) 作為下次比較的基準。\n", spotVal, spotVal-futureVal)
		}
	}
}
