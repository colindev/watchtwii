package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	tele "gopkg.in/telebot.v3"
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
	TelegramToken   = os.Getenv("TELEGRAM_TOKEN")
	TelegramChatIDs = os.Getenv("TELEGRAM_CHAT_IDS") // 預期格式: "123456,789012"
	ThresholdEnv    = os.Getenv("THRESHOLD")
)

// 定義盤別常數
const (
	SessionMorning = "Morning" // 早盤 (08:45 ~ 13:45)
	SessionNight   = "Night"   // 夜盤 (15:00 ~ 05:00)
	SessionClosed  = "Closed"  // 休市
)

// 1. 判斷台股早盤或夜盤
// 回傳: sessionType (SessionMorning, SessionNight, SessionClosed), isTrading (bool)
func GetSessionType() (string, bool) {
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		log.Fatal("無法載入台北時區:", err)
	}
	now := time.Now().In(loc)

	hour := now.Hour()
	minute := now.Minute()
	currentTime := hour*100 + minute

	// 早盤判斷: 08:45 ~ 13:45
	if currentTime >= 845 && currentTime <= 1345 {
		return SessionMorning, true
	}

	// 夜盤判斷: 15:00 ~ 23:59 OR 00:00 ~ 05:00
	if currentTime >= 1500 || currentTime <= 500 {
		return SessionNight, true
	}

	return SessionClosed, false
}

// 2. 透過 URL 跟 XPath 取得原始字串
func FetchValueString(urlLink string, xpathStr string) (string, error) {
	doc, err := htmlquery.LoadURL(urlLink)
	if err != nil {
		return "", fmt.Errorf("載入 URL 失敗: %v", err)
	}

	node := htmlquery.FindOne(doc, xpathStr)
	if node == nil {
		return "", fmt.Errorf("找不到 XPath 節點: %s", xpathStr)
	}

	return htmlquery.InnerText(node), nil
}

// 3. 解析字串為 float64 (處理逗號與空白)
func ParseToFloat(raw string) (float64, error) {
	// 移除逗號 (例如: "23,000.50" -> "23000.50")
	clean := strings.ReplaceAll(raw, ",", "")
	clean = strings.TrimSpace(clean)

	val, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, fmt.Errorf("無法轉換為浮點數 '%s': %v", raw, err)
	}
	return val, nil
}

// 4. 發送 Telegram 通知
func SendAlert(msg string) {
	if TelegramToken == "" || TelegramChatIDs == "" {
		log.Println("⚠️ 未設定 Telegram Token 或 Chat IDs，跳過通知")
		log.Println("內容:", msg)
		return
	}

	pref := tele.Settings{
		Token:  TelegramToken,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		log.Println("Telegram Bot 初始化失敗:", err)
		return
	}

	// 1. 使用逗號切割 ID 字串
	ids := strings.Split(TelegramChatIDs, ",")

	for _, idStr := range ids {
		// 2. 去除前後空白 (避免設定變數時多打空白導致錯誤)
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}

		// 3. 轉換 ID 為 int64
		chatID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			log.Printf("❌ 無法解析 Chat ID '%s': %v\n", idStr, err)
			continue // 跳過這個錯誤的 ID，繼續發送給下一個
		}

		// 4. 發送訊息
		user := &tele.User{ID: chatID}
		_, err = b.Send(user, msg)
		if err != nil {
			log.Printf("❌ 發送給 ID [%d] 失敗: %v\n", chatID, err)
		} else {
			log.Printf("✅ 通知已發送給 ID [%d]\n", chatID)
		}
	}
}

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

	// --- 步驟 B: 取得數值 ---
	// 1. 取得加權指數
	rawSpot, err := FetchValueString(SpotURL, SpotXPath)
	if err != nil {
		log.Printf("❌ 抓取加權指數失敗: %v", err)
		return
	}
	spotVal, err := ParseToFloat(rawSpot)
	if err != nil {
		log.Printf("❌ 解析加權指數失敗: %v", err)
		return
	}

	// 2. 取得台指期
	rawFuture, err := FetchValueString(FutureURL, FutureXPath)
	if err != nil {
		log.Printf("❌ 抓取台指期失敗: %v", err)
		return
	}
	futureVal, err := ParseToFloat(rawFuture)
	if err != nil {
		log.Printf("❌ 解析台指期失敗: %v", err)
		return
	}

	fmt.Printf("📊 加權指數: %.2f | 台指期: %.2f\n", spotVal, futureVal)

	// --- 步驟 C: 比較邏輯與通知 ---
	// 計算價差 (加權 - 期貨)
	// 正數 = 逆價差 (期貨 < 加權, 市場偏空)
	// 負數 = 正價差 (期貨 > 加權, 市場偏多)
	diff := spotVal - futureVal
	absDiff := diff
	if absDiff < 0 {
		absDiff = -absDiff
	}

	// 通知訊息內容建構
	var alertMsg string
	shouldNotify := false

	if session == SessionMorning {
		// --- 早盤邏輯 ---
		if diff > threshold {
			// 加權 > 台指 (逆價差過大)
			alertMsg = fmt.Sprintf("☀️ [早盤警示]\n現貨強於期貨 (逆價差)\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", diff, spotVal, futureVal)
			shouldNotify = true
		} else if diff < -threshold {
			// 加權 < 台指 (正價差過大)
			alertMsg = fmt.Sprintf("☀️ [早盤警示]\n期貨強於現貨 (正價差)\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", -diff, spotVal, futureVal)
			shouldNotify = true
		}
	} else if session == SessionNight {
		// --- 夜盤邏輯 ---
		// 注意：夜盤的加權是指數收盤價，通常用來參考國際盤對台指的拉動
		if diff > threshold {
			alertMsg = fmt.Sprintf("🌙 [夜盤警示]\n夜盤期貨大跌 (低於日盤收盤)\n差距: %.2f 點\n收盤加權: %.2f\n夜盤台指: %.2f", diff, spotVal, futureVal)
			shouldNotify = true
		} else if diff < -threshold {
			alertMsg = fmt.Sprintf("🌙 [夜盤警示]\n夜盤期貨大漲 (高於日盤收盤)\n差距: %.2f 點\n收盤加權: %.2f\n夜盤台指: %.2f", -diff, spotVal, futureVal)
			shouldNotify = true
		}
	}

	// --- 步驟 D: 發送 ---
	if shouldNotify {
		fmt.Println("觸發條件，發送 Telegram 通知...")
		SendAlert(alertMsg)
	} else {
		fmt.Printf("價差 %.2f 未超過閾值 %.2f，不發送通知。\n", diff, threshold)
	}
}
