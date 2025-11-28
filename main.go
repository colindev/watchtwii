package main

import (
	"fmt"
	"log"
	"math"
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

	// --- 步驟 D: 比較邏輯與通知 ---
	// 計算價差 (加權 - 期貨)
	// 正數 = 逆價差 (期貨 < 加權, 市場偏空)
	// 負數 = 正價差 (期貨 > 加權, 市場偏多)
	diff := spotVal - futureVal

	// 通知訊息內容建構
	var alertMsg string
	shouldNotify := false

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

	var lastDiff = d.LastDiffValue
	var changed = diff - lastDiff
	var needCheckChanged bool

	if session == SessionMorning {
		// --- 早盤邏輯 ---
		if diff > threshold {
			// 加權 > 台指 (逆價差過大)
			alertMsg = fmt.Sprintf("☀️ [早盤警示] (趨勢: %s)\n現貨強於期貨 (逆價差)\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", "📈", math.Abs(diff), spotVal, futureVal)
			shouldNotify = true
			needCheckChanged = true
		} else if diff < -threshold {
			// 加權 < 台指 (正價差過大)
			alertMsg = fmt.Sprintf("☀️ [早盤警示] (趨勢: %s)\n期貨強於現貨 (正價差)\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", "📉", math.Abs(diff), spotVal, futureVal)
			shouldNotify = true
			needCheckChanged = true
		} else if (spotVal - d.LastTWIIValue) > thresholdChanged {
			// 指數上漲 - 早盤關注加權變動
			alertMsg = fmt.Sprintf("☀️ [早盤警示] (趨勢: %s)\n指數上漲(%.2f)\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", "📈", (spotVal - d.LastTWIIValue), math.Abs(diff), spotVal, futureVal)
			shouldNotify = true
		} else if (spotVal - d.LastTWIIValue) < -thresholdChanged {
			// 指數下跌 - 早盤關注加權變動
			alertMsg = fmt.Sprintf("☀️ [早盤警示] (趨勢: %s)\n指數下跌(%.2f)\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", "📉", (spotVal - d.LastTWIIValue), math.Abs(diff), spotVal, futureVal)
			shouldNotify = true
		}

	} else if session == SessionNight {
		// --- 夜盤邏輯 ---
		// 注意：夜盤的加權是指數收盤價，通常用來參考國際盤對台指的拉動
		if diff > threshold {
			alertMsg = fmt.Sprintf("🌙 [夜盤警示] (趨勢: %s)\n夜盤期貨大跌 (低於日盤收盤)\n差距: %.2f 點\n收盤加權: %.2f\n夜盤台指: %.2f", "📉", math.Abs(diff), spotVal, futureVal)
			shouldNotify = true
			needCheckChanged = true
		} else if diff < -threshold {
			alertMsg = fmt.Sprintf("🌙 [夜盤警示] (趨勢: %s)\n夜盤期貨大漲 (高於日盤收盤)\n差距: %.2f 點\n收盤加權: %.2f\n夜盤台指: %.2f", "📈", math.Abs(diff), spotVal, futureVal)
			shouldNotify = true
			needCheckChanged = true
		}
	}

	// 只有特定前置條件才需要進入異動幅度判斷
	if needCheckChanged {

		if math.Abs(changed) >= thresholdChanged {
			if changed > 0 {
				alertMsg = fmt.Sprintf("📈(幅度增加:%.2f)\n%s", changed, alertMsg)
			} else if changed < 0 {
				alertMsg = fmt.Sprintf("📉(幅度減少:%.2f)\n%s", changed, alertMsg)
			}

		} else {
			shouldNotify = false
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n", diff, lastDiff, thresholdChanged)
		}

	}

	// 判斷是否為關鍵時間
	specificAlterMsg, isSpecificTime := CheckSpecificTimeAlert()
	if isSpecificTime {
		shouldNotify = true
		// 如果沒有符合觸發條件, 但是特定時間點依然發送, 要補上訊息
		if alertMsg == "" {
			alertMsg = fmt.Sprintf("[%s]\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", specificAlterMsg, math.Abs(diff), spotVal, futureVal)
		}
	}

	// --- 發送 ---
	if shouldNotify {
		fmt.Println("觸發條件，發送 Telegram 通知...")
		SendAlert(alertMsg)
		// 🎯 儲存當前價差，用於下次比較
		d.LastTWIIValue = spotVal
		d.LastDiffValue = diff
		if err := SaveCurrentData(d); err != nil {
			log.Printf("❌ 儲存當前價差失敗: %v", err)
		} else {
			fmt.Printf("✅ 已儲存當前指數(%.2f)與價差 (%.2f) 作為下次比較的基準。\n", spotVal, diff)
		}
	}
}
