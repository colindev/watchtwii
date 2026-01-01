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
func GetSessionType(loc *time.Location) (string, bool) {
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

// IsUSMarketInWinterTime 判斷美股市場當前是否處於標準時間 (冬令時間)。
// 美股 DST (夏令時間) 通常從三月第二個週日到十一月第一個週日。
func IsUSMarketInWinterTime() (bool, error) {
	// 載入美國/紐約時區 (美股交易所時區)
	nyLoc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return false, fmt.Errorf("無法載入 'America/New_York' 時區: %w", err)
	}

	// 取得當前時間，並將其轉換到紐約時區
	now := time.Now().In(nyLoc)

	// time.Time.Zone() 會返回時區名稱和 UTC 偏移量（秒）。
	// 如果時區名稱包含 "EDT" (夏令時間)，則不是冬令時間。
	zoneName, _ := now.Zone()

	// 假設只要時區名稱不是 "EST" 或其等效的標準時間縮寫，就認定為夏令時間。
	// 更簡單且穩定的方法是檢查偏移量或直接依賴 time 庫的內建判斷。

	// ***穩健判斷法 (檢查時區名稱是否為標準時間的縮寫)***
	if zoneName == "EST" || zoneName == "ET" {
		// 紐約標準時間 (冬令時間)
		return true, nil
	}

	// 如果時區名稱是 EDT 或其他非標準名稱，則為夏令時間
	return false, nil
}

// IsTodayInDateList 檢查今天是否在指定的日期清單中
// input: "2025-11-11_2025-12-25"
func IsTodayInDateList(dateListStr string, loc *time.Location) bool {
	if dateListStr == "" {
		return false
	}

	// 1. 取得指定時區的當天日期字串 (格式: YYYY-MM-DD)
	todayStr := time.Now().In(loc).Format("2006-01-02")

	// 2. 將輸入字串拆分成切片
	dates := strings.Split(dateListStr, ",")

	// 3. 比對是否符合
	for _, d := range dates {
		// 使用 TrimSpace 避免輸入字串包含空格 (例如 "2025-01-01, 2025-01-02")
		if strings.TrimSpace(d) == todayStr {
			return true
		}
	}

	return false
}

func GetCurrentTime(loc *time.Location) (currentTime int) {

	now := time.Now().In(loc)

	// 使用 hour*100 + minute 格式來做快速比較
	currentTime = now.Hour()*100 + now.Minute()
	return
}

func IsTaipexPreOpen(loc *time.Location) bool {
	currentTime := GetCurrentTime(loc)
	return currentTime >= 845 && currentTime <= 900
}

// 透過 URL 跟 XPath 取得原始字串
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
func SendAlert(tgToken, tgChatIDs, msg string) {

	pref := tele.Settings{
		Token:  tgToken,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		log.Println("Telegram Bot 初始化失敗:", err)
		return
	}

	// 1. 使用逗號切割 ID 字串
	ids := strings.Split(tgChatIDs, ",")

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
