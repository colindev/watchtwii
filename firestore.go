package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// Firestore 設定
const (
	FirestoreCollection = "TraderAlerts"
	FirestoreDocID      = "WatchTwiiDiff"
)

type Data struct {
	LastTWIIValue  float64
	LastDiffValue  float64
	LastUpdateTime time.Time

	// --- 新增：當日高低點紀錄 ---
	SpotHigh   float64 // 現貨當日最高
	SpotLow    float64 // 現貨當日最低
	FutureHigh float64 // 期貨當日最高
	FutureLow  float64 // 期貨當日最低

	// 錯誤處理
	ErrorCount int    // 連續失敗計數
	LastError  string // 記錄最後一次錯誤訊息
}

func (d *Data) Map() map[string]interface{} {
	return map[string]interface{}{
		"LastTWIIValue":  d.LastTWIIValue,
		"LastDiffValue":  d.LastDiffValue,
		"LastUpdateTime": d.LastUpdateTime, // 直接存 Time 物件

		// --- 新增映射 ---
		"SpotHigh":   d.SpotHigh,
		"SpotLow":    d.SpotLow,
		"FutureHigh": d.FutureHigh,
		"FutureLow":  d.FutureLow,

		"ErrorCount": d.ErrorCount,
		"LastError":  d.LastError,
	}
}

func (d *Data) Clone(m map[string]interface{}) *Data {
	// 輔助函式：安全讀取 float64
	getFloat := func(key string) float64 {
		if val, ok := m[key]; ok {
			if v, isFloat := val.(float64); isFloat {
				return v
			}
		}
		return 0.0
	}

	d.LastTWIIValue = getFloat("LastTWIIValue")
	d.LastDiffValue = getFloat("LastDiffValue")

	// --- 新增讀取 ---
	d.SpotHigh = getFloat("SpotHigh")
	d.SpotLow = getFloat("SpotLow")
	d.FutureHigh = getFloat("FutureHigh")
	d.FutureLow = getFloat("FutureLow")

	if val, ok := m["LastUpdateTime"]; ok {
		// Firestore 儲存時間通常是 time.Time，但也可能被讀為 int64 (如果是舊資料)
		if v, isTime := val.(time.Time); isTime {
			d.LastUpdateTime = v
		} else if v, isInt64 := val.(int64); isInt64 {
			d.LastUpdateTime = time.Unix(v, 0)
		}
	}

	if val, ok := m["ErrorCount"]; ok {
		if v, isInt := val.(int64); isInt {
			d.ErrorCount = int(v)
		} else if v, isInt := val.(int); isInt {
			d.ErrorCount = v
		}
	}
	if val, ok := m["LastError"]; ok {
		if v, isStr := val.(string); isStr {
			d.LastError = v
		}
	}

	return d
}

// UpdateDailyHighLow 更新當日最高最低價
// 邏輯：每天 08:45 (早盤開盤) 重置數據，其餘時間比較並更新極值
func (d *Data) UpdateDailyHighLow(spotVal, futureVal float64) bool {
	loc, _ := time.LoadLocation("Asia/Taipei")
	now := time.Now().In(loc)

	// 取得當前時間 HHMM
	currentTime := now.Hour()*100 + now.Minute()

	// 取得上次更新時間的日期 (YYYYMMDD)
	lastDate := d.LastUpdateTime.In(loc).Format("20060102")
	currentDate := now.Format("20060102")

	// 特殊情況：如果上次更新是昨天，但現在是今天的 00:00~05:00 (夜盤尾段)，這屬於「昨天的交易日延續」
	// 所以我們只在「日期變更 且 時間 >= 8:45」時才視為全新的一天重置。
	// 修正邏輯：只要遇到 08:45 ~ 08:50 這個區間，就強制視為新的一天開始並重置。
	// 為了避免重複重置，我們比較日期。

	// 簡化策略：只要現在是 08:45 ~ 08:50 之間，且 LastUpdateTime 不在今天的這個區間，就重置。
	// 或者，簡單地判斷：如果 LastUpdateTime 是昨天以前，且現在 >= 845，就重置。

	shouldReset := false
	if currentDate != lastDate {
		// 日期不同了
		if currentTime >= 845 {
			// 已經是早盤時間，重置
			shouldReset = true
		} else {
			// 現在是凌晨 (00:00~05:00)，屬於夜盤延續，不重置
			shouldReset = false
		}
	} else {
		// 同一天
		// 如果程式中間掛了很久，上次更新是 08:00，現在是 08:45，也該重置
		lastTime := d.LastUpdateTime.In(loc).Hour()*100 + d.LastUpdateTime.In(loc).Minute()
		if lastTime < 845 && currentTime >= 845 {
			shouldReset = true
		}
	}

	shouldSave := false
	if shouldReset {
		fmt.Println("🔄 [新交易日] 08:45 開盤重置高低點紀錄")
		d.SpotHigh = spotVal
		d.SpotLow = spotVal
		d.FutureHigh = futureVal
		d.FutureLow = futureVal
		shouldSave = true
	} else {
		// 正常更新邏輯

		// 防止初始值為 0 的情況 (如果是第一次運行)
		if d.SpotHigh == 0 {
			d.SpotHigh = spotVal
		}
		if d.SpotLow == 0 || d.SpotLow > spotVal {
			d.SpotLow = spotVal
		} // 防止 0 變成最低價
		if d.FutureHigh == 0 {
			d.FutureHigh = futureVal
		}
		if d.FutureLow == 0 || d.FutureLow > futureVal {
			d.FutureLow = futureVal
		}

		// 比較最大值
		d.SpotHigh = math.Max(d.SpotHigh, spotVal)
		d.FutureHigh = math.Max(d.FutureHigh, futureVal)

		// 比較最小值 (過濾掉 0 值異常)
		if spotVal > 0 {
			if d.SpotLow == 0 {
				d.SpotLow = spotVal
			} else {
				d.SpotLow = math.Min(d.SpotLow, spotVal)
			}
		}
		if futureVal > 0 {
			if d.FutureLow == 0 {
				d.FutureLow = futureVal
			} else {
				d.FutureLow = math.Min(d.FutureLow, futureVal)
			}
		}

		if d.SpotHigh == spotVal ||
			d.SpotLow == spotVal ||
			d.FutureHigh == futureVal ||
			d.FutureLow == futureVal {
			shouldSave = true
		}
	}

	// 更新最後時間
	if shouldSave {
		d.LastUpdateTime = now
	}

	return shouldSave
}

// CheckErrorState 檢查錯誤狀態變化
// 回傳: (是否需要通知, 通知訊息)
func (d *Data) CheckErrorState(currentErr error) (bool, string) {
	if currentErr != nil {
		// 情況 A: 發生錯誤
		d.LastError = currentErr.Error()
		d.ErrorCount++

		if d.ErrorCount == 1 {
			// 1. 正常 -> 失敗 (初次發生)
			return true, fmt.Sprintf("❌ [系統異常] 資料抓取失敗\n錯誤: %v", currentErr)
		} else {
			// 3. 失敗 -> 失敗 (持續失敗中) -> 靜默 (Log only)
			// 可選擇每累積 N 次 (例如 12 次 = 1小時) 才提醒一次
			if d.ErrorCount%12 == 0 {
				return true, fmt.Sprintf("⚠️ [系統持續異常] 已連續失敗 %d 次\n錯誤: %v", d.ErrorCount, currentErr)
			}
			return false, "" // 不發送通知
		}
	} else {
		// 情況 B: 正常成功
		if d.ErrorCount > 0 {
			// 2. 失敗 -> 正常 (恢復)
			failCount := d.ErrorCount
			d.ErrorCount = 0
			d.LastError = ""
			return true, fmt.Sprintf("✅ [系統恢復] 服務已恢復正常\n(先前連續失敗 %d 次)", failCount)
		}
		// 4. 正常 -> 正常 -> 靜默
		return false, ""
	}
}

// 輔助函式：取得 Firestore 客戶端
func getFirestoreClient() (*firestore.Client, error) {
	// 由於 Cloud Run Jobs 無法讀取GCP_PROJECT, 所以部署時餵入
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, os.Getenv("GCP_PROJECT")) // 必須從 ENV 讀取 GCP_PROJECT ID
	if err != nil {
		return nil, fmt.Errorf("初始化 Firestore 客戶端失敗: %w", err)
	}
	return client, nil
}

// GetLastNotifiedData 從 Firestore 讀取上次被通知時的價差。
func GetLastNotifiedData() (*Data, error) {
	client, err := getFirestoreClient()
	if err != nil {
		return nil, err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var d = &Data{}
	doc, err := client.Collection(FirestoreCollection).
		Doc(FirestoreDocID).
		Get(ctx)

	if err != nil {
		if iterator.Done == err {
			// 第一次運行，文件不存在，返回 0.0
			return d, nil
		}
		return nil, fmt.Errorf("讀取 Firestore 文件失敗: %w", err)
	}

	return d.Clone(doc.Data()), nil
}

// SaveCurrentData 將當前的價差儲存到 Firestore。
func SaveCurrentData(d *Data) error {
	client, err := getFirestoreClient()
	if err != nil {
		return err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.Collection(FirestoreCollection).
		Doc(FirestoreDocID).
		Set(ctx, d.Map())

	if err != nil {
		return fmt.Errorf("寫入 Firestore 失敗: %w", err)
	}
	return nil
}
