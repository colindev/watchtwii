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
}

func (d *Data) Map() map[string]interface{} {
	return map[string]interface{}{
		"LastTWIIValue":  d.LastTWIIValue,
		"LastDiffValue":  d.LastDiffValue,
		"LastUpdateTime": time.Now(),
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

	return d
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
	doc, err := client.Collection(FirestoreCollection).Doc(FirestoreDocID).Get(ctx)
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

	_, err = client.Collection(FirestoreCollection).Doc(FirestoreDocID).Set(ctx, d.Map())

	if err != nil {
		return fmt.Errorf("寫入 Firestore 失敗: %w", err)
	}
	return nil
}
