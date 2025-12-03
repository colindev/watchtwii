package main

import (
	"strings"
	"testing"
	"time"
)

func TestMessage_Build(t *testing.T) {
	// 基礎設定
	baseThreshold := 50.0
	baseThresholdChanged := 10.0

	// 模擬一個基礎的 Data 狀態
	baseData := &Data{
		LastTWIIValue:  20000.0,
		LastDiffValue:  0.0,
		LastUpdateTime: time.Now(),
		SpotHigh:       20100.0,
		SpotLow:        19900.0,
		FutureHigh:     20100.0,
		FutureLow:      19900.0,
	}

	tests := []struct {
		skip             bool    // 跳過測試
		name             string  // 測試名稱
		session          string  // 盤別
		threshold        float64 // 指數期權價差閾值
		thresholdChanged float64 // 與上次通知的幅度閾值
		d                *Data   // 模擬數據
		spotVal          float64 // 當前現貨
		futureVal        float64 // 當前期貨
		wantNotify       bool    // 預期是否通知
		wantMsgSubstring string  // 預期訊息包含的關鍵字
	}{
		// --- 正常場景 ---
		{
			//skip:             true,
			name:             "正常盤整_無觸發",
			session:          SessionMorning,
			threshold:        baseThreshold,
			thresholdChanged: baseThresholdChanged,
			d:                baseData,
			spotVal:          20000.0,
			futureVal:        20000.0, // 價差 0
			wantNotify:       false,
		},

		// --- 優先級測試 (關鍵驗證點) ---
		{
			//skip:             true,
			name:             "優先級測試_同時破新高與超閾值_應優先顯示新高",
			session:          SessionMorning,
			threshold:        baseThreshold,
			thresholdChanged: baseThresholdChanged,
			d:                baseData, // SpotHigh = 20100
			// 情境：指數噴出到 20500 (破新高)
			// 價差 = -500 (絕對值 500 > 50 閾值)
			// 結果：應該要因為「新高」而通知，而不是「正價差過大」
			spotVal:          20500.0,
			futureVal:        21000.0,
			wantNotify:       true,
			wantMsgSubstring: "加權當日新高", // 關鍵：如果程式邏輯沒改好，這裡會變成 "正價差過大"
		},
		{
			//skip:             true,
			name:             "優先級測試_同時破新低與超閾值_應優先顯示新低",
			session:          SessionNight,
			threshold:        baseThreshold,
			thresholdChanged: baseThresholdChanged,
			d:                baseData, // FutureLow = 19900
			// 情境：期貨崩跌到 19000 (破新低)，現貨 20000
			// 價差 = 1000 (絕對值 1000 > 50 閾值)
			spotVal:          20000.0,
			futureVal:        19000.0,
			wantNotify:       true,
			wantMsgSubstring: "期貨當日新低",
		},

		// --- 漲跌方向測試 ---
		{
			name:             "漲跌方向測試_夜盤下跌幅度縮小_應顯示跌幅縮小",
			session:          SessionNight,
			threshold:        100,
			thresholdChanged: 50,
			d: &Data{
				LastDiffValue: 104.04000000000087,
				LastTWIIValue: 27793.04,
				SpotHigh:      27832.89,
				SpotLow:       27342.53,
				FutureHigh:    27875,
				FutureLow:     27632,
			},
			spotVal:          27793.04,
			futureVal:        27746.00,
			wantNotify:       true,
			wantMsgSubstring: "期貨下跌幅度減少",
		},

		// --- 閾值測試 ---
		{
			//skip:             true,
			name:             "閾值測試_逆價差過大",
			session:          SessionNight,
			threshold:        baseThreshold,
			thresholdChanged: baseThresholdChanged,
			d:                baseData,
			spotVal:          20000.0,
			futureVal:        19940.0, // 價差 60 > 50 (未破新低 19900)
			wantNotify:       true,
			wantMsgSubstring: "夜盤期貨下跌 (低於日盤收盤)\n期貨下跌幅度增加",
		},

		// --- 抑制測試 ---
		{
			//skip:             true,
			name:             "抑制測試_超過閾值但變動微小",
			session:          SessionMorning,
			threshold:        baseThreshold,
			thresholdChanged: baseThresholdChanged,
			d: &Data{
				LastDiffValue: 62.0,  // 上次通知時價差 62
				LastTWIIValue: 19995, // 上次通知的指數
				SpotHigh:      20010,
				SpotLow:       19990,
				FutureHigh:    21000.0,
				FutureLow:     19000.0,
			},
			spotVal:   20000.0,
			futureVal: 19940.0, // 當前價差 60 (>50閾值)
			// 變動 = 60 - 62 = -2 (絕對值 2 < 10 thresholdChanged)
			wantNotify:       false, // 應被抑制
			wantMsgSubstring: "",
		},
		{
			//skip:             true,
			name:             "抑制測試_超過閾值且變動大_應通知",
			session:          SessionMorning,
			threshold:        baseThreshold,
			thresholdChanged: baseThresholdChanged,
			d: &Data{
				LastDiffValue: 80.0,  // 上次通知時價差 80
				LastTWIIValue: 19995, // 上次通知的指數
				SpotHigh:      20010,
				SpotLow:       19990,
				FutureHigh:    21000.0,
				FutureLow:     19000.0,
			},
			spotVal:   20000.0,
			futureVal: 19940.0, // 當前價差 60
			// 變動 = 60 - 80 = -20 (絕對值 20 > 10)
			wantNotify:       true,
			wantMsgSubstring: "價差幅度", // 預期會有變動幅度的描述
		},
		{
			//skip: true,
			// resource.type = "cloud_run_job" resource.labels.job_name = "watchtwii" labels."run.googleapis.com/execution_name" = "watchtwii-vg5b8" resource.labels.location = "asia-east1" timestamp >= "2025-12-03T13:40:05.391706Z" labels."run.googleapis.com/task_index" = "0" severity>=DEFAULT
			// map[ErrorCount:0 FutureHigh:27875 FutureLow:27696 LastDiffValue:49.04000000000087 LastError: LastTWIIValue:27793.04 LastUpdateTime:2025-12-01 17:10:14.583025 +0000 UTC SpotHigh:27832.89 SpotLow:27342.53]
			name:             "抑制測試_期貨跌轉漲幅度超過異動閾值_應通知",
			session:          SessionNight,
			threshold:        100,
			thresholdChanged: 50,
			d: &Data{
				LastDiffValue: 49.04000000000087,
				LastTWIIValue: 27793.04,
				SpotHigh:      27832.89,
				SpotLow:       27342.53,
				FutureHigh:    27875,
				FutureLow:     27696,
			},
			spotVal:          27793.04,
			futureVal:        27820.00,
			wantNotify:       true,
			wantMsgSubstring: "夜盤期貨上漲反轉 (高於日盤收盤)",
		},
		{
			name:             "抑制測試_期貨漲轉跌幅度超過異動閾值_應通知",
			session:          SessionNight,
			threshold:        baseThreshold,
			thresholdChanged: baseThresholdChanged,
			d: &Data{
				LastDiffValue: -20, // 20030
				LastTWIIValue: 20010,
				SpotHigh:      20100,
				SpotLow:       19900,
				FutureHigh:    20100,
				FutureLow:     19900,
			},
			spotVal:          20010,
			futureVal:        20005,
			wantNotify:       true,
			wantMsgSubstring: "夜盤期貨下跌反轉 (低於日盤收盤)",
		},
	}

	for _, tt := range tests {

		if tt.skip {
			// 集中測試特定case用
			continue
		}

		t.Run(tt.name, func(t *testing.T) {
			// 初始化 Message
			msg, err := NewMessage(tt.session)
			if err != nil {
				t.Fatalf("NewMessage failed: %v", err)
			}

			// 執行 Build
			gotMsg, gotNotify := msg.Build(tt.d, tt.spotVal, tt.futureVal, tt.threshold, tt.thresholdChanged)

			// 1. 驗證是否通知
			if gotNotify != tt.wantNotify {
				t.Errorf("Build() notify = %v, want %v", gotNotify, tt.wantNotify)
			}

			// 2. 驗證訊息內容關鍵字 (如果有預期內容)
			if tt.wantNotify && tt.wantMsgSubstring != "" {
				if !strings.Contains(gotMsg, tt.wantMsgSubstring) {
					t.Errorf("Build() msg = %v, want substring %v", gotMsg, tt.wantMsgSubstring)
				}
			}
		})
	}
}
