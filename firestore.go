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
	FirestoreFieldKey   = "LastDiffValue"
	FirestoreFieldTime  = "LastUpdateTime"
)

// 輔助函式：取得 Firestore 客戶端
func getFirestoreClient() (*firestore.Client, error) {
	// 由於 Cloud Run Jobs 運行在 GCP 專案內，環境會自動提供驗證
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, os.Getenv("GCP_PROJECT")) // 必須從 ENV 讀取 GCP_PROJECT ID
	if err != nil {
		return nil, fmt.Errorf("初始化 Firestore 客戶端失敗: %w", err)
	}
	return client, nil
}

// GetLastNotifiedDiff 從 Firestore 讀取上次被通知時的價差。
func GetLastNotifiedDiff() (float64, error) {
	client, err := getFirestoreClient()
	if err != nil {
		return 0, err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	doc, err := client.Collection(FirestoreCollection).Doc(FirestoreDocID).Get(ctx)
	if err != nil {
		if iterator.Done == err {
			// 第一次運行，文件不存在，返回 0.0
			return 0.0, nil
		}
		return 0, fmt.Errorf("讀取 Firestore 文件失敗: %w", err)
	}

	data := doc.Data()
	if val, ok := data[FirestoreFieldKey]; ok {
		if diff, isFloat := val.(float64); isFloat {
			return diff, nil
		}
	}

	return 0.0, nil // 文件存在但內容格式錯誤，返回 0.0
}

// SaveCurrentDiff 將當前的價差儲存到 Firestore。
func SaveCurrentDiff(currentDiff float64) error {
	client, err := getFirestoreClient()
	if err != nil {
		return err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.Collection(FirestoreCollection).Doc(FirestoreDocID).Set(ctx, map[string]interface{}{
		FirestoreFieldKey:  currentDiff,
		FirestoreFieldTime: time.Now(),
	})

	if err != nil {
		return fmt.Errorf("寫入 Firestore 失敗: %w", err)
	}
	return nil
}
