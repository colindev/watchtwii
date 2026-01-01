package main

import (
	"context"
	"fmt"
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

	// --- 當日高低點紀錄 ---
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
func (d *Data) UpdateDailyHighLow(spotVal, futureVal float64, loc *time.Location) bool {

	// 🎯 儲存當前價差，用於下次比較
	d.LastTWIIValue = spotVal
	d.LastDiffValue = spotVal - futureVal

	session, isTrading := GetSessionType(loc)
	if !isTrading {
		// 休市期間不更新高低點，除非您有特殊的收盤後邏輯
		return false
	}

	shouldSave := false

	// --- 處理早盤 (Spot + Future) ---
	if session == SessionMorning {
		// 現貨 (加權)
		if spotVal > d.SpotHigh {
			d.SpotHigh = spotVal
			shouldSave = true
		}
		// 第一次運行或當日首次運行時初始化
		if d.SpotLow == 0 || spotVal < d.SpotLow {
			d.SpotLow = spotVal
			shouldSave = true
		}
	}

	// --- 處理早盤和夜盤 (Future Only) ---
	// 注意：夜盤時只會執行到這裡，不會更新 SpotHigh/SpotLow

	// 期貨
	if futureVal > d.FutureHigh {
		d.FutureHigh = futureVal
		shouldSave = true
	}
	// 第一次運行或當日首次運行時初始化
	if d.FutureLow == 0 || futureVal < d.FutureLow {
		d.FutureLow = futureVal
		shouldSave = true
	}

	// 額外處理：若為開盤首筆數據，則需初始化所有高低點
	if d.FutureHigh == 0 && d.FutureLow == 0 && d.SpotHigh == 0 && d.SpotLow == 0 {
		d.SpotHigh = spotVal
		d.SpotLow = spotVal
		d.FutureHigh = futureVal
		d.FutureLow = futureVal
		shouldSave = true
	}

	// 確保在夜盤時 Spot 的高低點不會被更新，並且如果是在夜盤時初始化，SpotHigh/Low 會被設為 0
	// 這裡的邏輯需要您進一步確認，如果夜盤運行時 d.SpotHigh/Low 仍應保持日盤收盤價，那需要在 SaveCurrentData 時進行特殊處理。
	// 但就「只更新期貨」的要求而言，目前的寫法是正確的：夜盤時，由於 session != SessionMorning，Spot 高低點的判斷會被跳過。

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
func getFirestoreClient(gcpProject string) (*firestore.Client, error) {
	// 由於 Cloud Run Jobs 無法讀取GCP_PROJECT, 所以部署時餵入
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, gcpProject)
	if err != nil {
		return nil, fmt.Errorf("初始化 Firestore 客戶端失敗: %w", err)
	}
	return client, nil
}

// GetLastNotifiedData 從 Firestore 讀取上次被通知時的價差。
func GetLastNotifiedData(gcpProject string) (*Data, error) {
	client, err := getFirestoreClient(gcpProject)
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
func SaveCurrentData(gcpProject string, d *Data) error {
	client, err := getFirestoreClient(gcpProject)
	if err != nil {
		return err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	d.LastUpdateTime = time.Now()

	_, err = client.Collection(FirestoreCollection).
		Doc(FirestoreDocID).
		Set(ctx, d.Map())

	if err != nil {
		return fmt.Errorf("寫入 Firestore 失敗: %w", err)
	}
	fmt.Printf("✅ 儲存成功, 更新數據%+v\n", d.Map())
	return nil
}
