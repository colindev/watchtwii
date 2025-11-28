package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	tele "gopkg.in/telebot.v3"
)

// 定義盤別常數
const (
	SessionMorning = "Morning" // 早盤 (08:45 ~ 13:45)
	SessionNight   = "Night"   // 夜盤 (15:00 ~ 05:00)
	SessionClosed  = "Closed"  // 休市
)

// 判斷台股早盤或夜盤
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

// 解析字串為 float64 (處理逗號與空白)
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

// 發送 Telegram 通知
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
