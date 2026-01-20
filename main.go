package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/colindev/osenv"
	"github.com/joho/godotenv"
)

// --- 設定區 (建議透過環境變數注入) ---
const (

	// 這裡填入您實際要抓取的網站與 XPath
	// 範例：Yahoo 股市 (僅供參考，XPath 需隨網頁結構更新)
	SpotURL   = "https://tw.stock.yahoo.com/quote/%5ETWII" // 加權指數
	SpotXPath = "//*[@id='main-0-QuoteHeader-Proxy']/div/div[2]/div[1]/div/span[1]"

	FutureURL   = "https://tw.stock.yahoo.com/future/futures.html?fumr=futurefull" // 台指近一 (需確認網址是否為連續月)
	FutureXPath = "/html/body/div[1]/div/div/div/div/div[3]/div[1]/div/div/div[2]/div[3]/div[2]/div/div/ul/li[2]/div/div[4]/span"
)

func ScrapeData() (spotVal float64, futureVal float64, errs error) {

	// 取得台指期
	rawFuture, err := FetchValueString(FutureURL, FutureXPath)
	if err != nil {
		err = fmt.Errorf("抓取台指期失敗: %w", err)
	} else if futureVal, err = ParseToFloat(rawFuture); err != nil {
		err = fmt.Errorf("解析台指期失敗: %w", err)
	}

	// 取得加權指數
	rawSpot, errSpot := FetchValueString(SpotURL, SpotXPath)
	if errSpot != nil {
		errs = errors.Join(errs, fmt.Errorf("抓取加權指數失敗: %w", errSpot))
	} else if spotVal, errSpot = ParseToFloat(rawSpot); errSpot != nil {
		errs = errors.Join(errs, fmt.Errorf("解析加權指數失敗: %w", errSpot))
	}

	return
}

// 環境變數中的 Key
var (
	DebugEnv = os.Getenv("DEBUG")
)

// Config 定義了程式所需的所有外部設定
type Config struct {
	Version string `env:"VERSION"`

	// GCP 相關
	GCPProject string `env:"RUN_PROJECT"` // 如果是在 Cloud Run 執行，通常需要手動傳入

	// Telegram 相關
	TelegramToken   string `env:"TELEGRAM_TOKEN"`
	TelegramChatIDs string `env:"TELEGRAM_CHAT_IDS"`

	// 監控閾值
	Threshold        float64 `env:"THRESHOLD"`
	ThresholdChanged float64 `env:"THRESHOLD_CHANGED"`

	// 特殊休市日 (格式: 2026-01-01,2026-01-02)
	SpecialDates string `env:"SPECIAL_DATES"`
}

// LoadConfig 負責載入並驗證設定，若缺少必要欄位則直接回傳 error (Fail-Fast)
func LoadConfig() (*Config, error) {

	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("載入環境變數檔案失敗: %w", err)
	}

	cfg := &Config{}

	// 使用您的 osenv 庫載入設定
	if err := osenv.LoadTo(cfg); err != nil {
		return nil, fmt.Errorf("載入環境變數失敗: %w", err)
	}

	// 這裡進行 Fail-Fast 的強驗證
	if cfg.TelegramToken == "" {
		return nil, fmt.Errorf("缺少必填環境變數: TELEGRAM_TOKEN")
	}
	if cfg.TelegramChatIDs == "" {
		return nil, fmt.Errorf("缺少必填環境變數: TELEGRAM_CHAT_IDS")
	}
	if cfg.Threshold == 0 {
		log.Println("⚠️ 警告: THRESHOLD 設定為 0，將會頻繁觸發通知")
	}

	return cfg, nil
}

var loc *time.Location

func init() {
	var err error
	loc, err = time.LoadLocation("Asia/Taipei")
	if err != nil {
		log.Fatal("無法載入台北時區:", err)
	}
}

func main() {

	// 設定提取與驗證 (Fail-Fast)
	cfg, err := LoadConfig()
	if err != nil {
		// 在這裡直接中斷，避免程式在無效設定下運行
		log.Fatalf("❌ 程式初始化失敗: %v", err)
	}

	fmt.Println("啟動排程檢查...")

	// 休市判斷
	if IsTodayInDateList(cfg.SpecialDates, loc) {
		fmt.Println("☕ 今天是預設休市日，程式結束。")
		return // 直接中斷
	}

	// --- 判斷盤別 ---
	session, isTrading := GetSessionType(loc)
	fmt.Printf("目前時段: %s, 是否交易中: %v\n", session, isTrading)

	if !isTrading {
		fmt.Println("目前非監控時段，結束程式。")
		return
	}

	// 從 Firestore 讀取上次被通知時的價差
	d, err := GetLastNotifiedData(cfg.GCPProject)
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
	maxRetries := 3
	if scrapeErr != nil && spotVal == 0 && (IsTaipexPreOpen(loc) || session == SessionNight) {
		if futureVal == 0 { // 有機會爬到0
			for i := 1; i <= maxRetries; i++ {
				fmt.Printf("⚠️ 盤前/夜盤期貨數值異常 (0), 等待 10秒後重試 (%d/%d)...\n", i, maxRetries)
				time.Sleep(time.Second * 10) // 等一下再重試
				_, futureVal, scrapeErr = ScrapeData()
				if futureVal > 0 {
					fmt.Printf("✅ 重試成功！取得期貨數值: %.2f\n", futureVal)
					break // 成功抓到，跳出迴圈
				}
			}
		}
		// 重試結束後的最終判斷
		if futureVal > 0 {
			// 情況 A: 成功取得期貨 (或是原本就有，或是重試後拿到)
			// 此時我們使用 "上次的加權指數" 來填補 spotVal (因為盤前/夜盤 spot 本來就是 0)
			spotVal = d.LastTWIIValue

			// 重要：既然我們已經用 fallback 數據修復了，就應該清除錯誤
			scrapeErr = nil
		} else {
			// 情況 B: 重試後期貨依然是 0
			// 我們不 return，而是確保 scrapeErr 有值，讓後面的 CheckErrorState 處理
			if scrapeErr == nil {
				scrapeErr = fmt.Errorf("盤前/夜盤無法取得期貨報價 (數值為 0)")
			}
			// 程式繼續往下執行... -> 進到 CheckErrorState -> 記錄錯誤 -> 發送 System Alert -> Save Error -> Exit
		}
	}

	// 🎯 核心：使用 CheckErrorState 處理狀態變化 (正常<->失敗)
	shouldAlertError, errorMsg := d.CheckErrorState(scrapeErr)

	if shouldAlertError {
		fmt.Println("狀態改變，發送系統通知...")
		SendAlert(cfg.TelegramToken, cfg.TelegramChatIDs, errorMsg)
	}

	// 發生錯誤後的處理：儲存錯誤狀態並退出
	if scrapeErr != nil {
		log.Printf("執行失敗: %v (Count: %d)", scrapeErr, d.ErrorCount)
		// ⚠️ 重要：即使失敗也要儲存，這樣下次才知道 ErrorCount > 0
		if err := SaveCurrentData(cfg.GCPProject, d); err != nil {
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

	alertMsg, shouldNotify := msg.Build(d, spotVal, futureVal, cfg.Threshold, cfg.ThresholdChanged)

	// 判斷是否為關鍵時間
	specificAlterMsg, isSpecificTime := CheckSpecificTimeAlert(loc)
	if isSpecificTime {
		shouldNotify = true
		// 如果沒有符合觸發條件, 但是特定時間點依然發送, 要補上訊息
		if alertMsg == "" {
			alertMsg = msg.Info(specificAlterMsg, spotVal, futureVal)
		}
	}

	shouldSave := d.UpdateDailyHighLow(spotVal, futureVal, loc)

	// --- 發送 ---
	if shouldNotify {
		fmt.Println("觸發條件，發送 Telegram 通知...")
		SendAlert(cfg.TelegramToken, cfg.TelegramChatIDs, alertMsg)
		if err := SaveCurrentData(cfg.GCPProject, d); err != nil {
			log.Printf("❌ 儲存當前價差失敗: %v\n", err)
		} else {
			fmt.Println("✅ 已儲存當前數據作為下次比較的基準。")
		}
	} else if shouldSave {
		fmt.Println("✅ 欄位資料異動，儲存新狀態...")
		if err := SaveCurrentData(cfg.GCPProject, d); err != nil {
			log.Printf("❌ 儲存恢復狀態失敗: %v", err)
		}
	} else if shouldAlertError { // (這代表剛剛發生了 Recovery)
		// 如果沒有觸發市場警報，但發生了系統狀態改變 (例如：Fail -> Normal Recovery)
		// 必須儲存 d，以更新 ErrorCount=0 的狀態。
		fmt.Println("✅ 系統恢復，儲存新狀態...")
		if err := SaveCurrentData(cfg.GCPProject, d); err != nil {
			log.Printf("❌ 儲存恢復狀態失敗: %v", err)
		}
	}
}
