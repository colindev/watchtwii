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

	// 1. 從 Firestore 讀取上次被通知時的價差
	lastDiff, err := GetLastNotifiedDiff()
	if err != nil {
		log.Printf("❌ 無法讀取上次價差，跳過抑制邏輯: %v", err)
	}

	if session == SessionMorning {
		// --- 早盤邏輯 ---
		if diff > threshold {
			// 加權 > 台指 (逆價差過大)
			alertMsg = fmt.Sprintf("☀️ [早盤警示] (趨勢: %s)\n現貨強於期貨 (逆價差)\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", "UP", math.Abs(diff), spotVal, futureVal)
			shouldNotify = true
		} else if diff < -threshold {
			// 加權 < 台指 (正價差過大)
			alertMsg = fmt.Sprintf("☀️ [早盤警示] (趨勢: %s)\n期貨強於現貨 (正價差)\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", "DOWN", math.Abs(diff), spotVal, futureVal)
			shouldNotify = true
		}
	} else if session == SessionNight {
		// --- 夜盤邏輯 ---
		// 注意：夜盤的加權是指數收盤價，通常用來參考國際盤對台指的拉動
		if diff > threshold {
			alertMsg = fmt.Sprintf("🌙 [夜盤警示] (趨勢: %s)\n夜盤期貨大跌 (低於日盤收盤)\n差距: %.2f 點\n收盤加權: %.2f\n夜盤台指: %.2f", "DOWN", math.Abs(diff), spotVal, futureVal)
			shouldNotify = true
		} else if diff < -threshold {
			alertMsg = fmt.Sprintf("🌙 [夜盤警示] (趨勢: %s)\n夜盤期貨大漲 (高於日盤收盤)\n差距: %.2f 點\n收盤加權: %.2f\n夜盤台指: %.2f", "UP", math.Abs(diff), spotVal, futureVal)
			shouldNotify = true
		}
	}

	thresholdChanged, err := ParseToFloat(ThresholdChangedEnv)
	if err != nil {
		fmt.Println("沒有設定價差抑制幅度 THRESHOLD_CHANGED, 預設使用 0.1 (10%)")
		thresholdChanged = 0.1
	}

	// 判斷是否滿足主要警報條件 (|diff| > threshold)
	isSignificantChange := false
	if math.Abs(diff) > threshold {
		// 檢查抑制條件：當前價差是否比上次通知的價差變動超過 10%
		// 如果上次價差為 0 (首次運行)，或變動大於 10%，則繼續通知。
		// 公式: |diffAbs - math.Abs(lastDiff)| / math.Abs(lastDiff) > 0.1

		// 為了避免 lastDiff 為 0 導致除以 0 錯誤，我們使用一個閾值檢查：
		if math.Abs(lastDiff) < 1.0 {
			// 如果上次價差接近 0，則視為重大變動 (首次或趨勢剛形成)
			isSignificantChange = true
		} else {
			// 變動百分比
			changePct := math.Abs(math.Abs(diff)-math.Abs(lastDiff)) / math.Abs(lastDiff)
			if changePct > thresholdChanged {
				isSignificantChange = true
			}
		}

		// 只有在超過閾值 AND 變動顯著時才設置 shouldNotify = true
		if isSignificantChange {
			shouldNotify = true
			// 如果決定通知，則在 if shouldNotify 區塊內寫入新值
		} else {
			shouldNotify = false
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動不超過 %.2f%%，抑制通知。\n", math.Abs(diff), math.Abs(lastDiff), thresholdChanged*100)
		}

	} else {
		fmt.Printf("價差 %.2f 未超過閾值 %.2f，不發送通知。\n", diff, threshold)
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
		if err := SaveCurrentDiff(diff); err != nil {
			log.Printf("❌ 儲存當前價差失敗: %v", err)
		} else {
			fmt.Printf("✅ 已儲存當前價差 (%.2f) 作為下次比較的基準。\n", diff)
		}
	}
}
