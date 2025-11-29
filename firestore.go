package main

import (
	"context"
	"fmt"
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
	ErrorCount     int    // 連續失敗計數
	LastError      string // 記錄最後一次錯誤訊息
}

func (d *Data) Map() map[string]interface{} {
	return map[string]interface{}{
		"LastTWIIValue":  d.LastTWIIValue,
		"LastDiffValue":  d.LastDiffValue,
		"LastUpdateTime": time.Now(),
		"ErrorCount":     d.ErrorCount,
		"LastError":      d.LastError,
	}
}

func (d *Data) Clone(m map[string]interface{}) *Data {

	if val, ok := m["LastTWIIValue"]; ok {
		if v, isFloat := val.(float64); isFloat {
			d.LastTWIIValue = v
		}
	}
	if val, ok := m["LastDiffValue"]; ok {
		if v, isFloat := val.(float64); isFloat {
			d.LastDiffValue = v
		}
	}
	if val, ok := m["LastUpdateTime"]; ok {
		if v, isInt64 := val.(int64); isInt64 {
			d.LastUpdateTime = time.Unix(v, 0)
		}
	}
	if val, ok := m["ErrorCount"]; ok {
		// Firestore 數字有時會轉為 int64，需做類型斷言檢查
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
