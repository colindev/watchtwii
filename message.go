package main

import (
	"fmt"
	"log"
	"math"
)

// 輔助函式：檢查特定時間點是否觸發提醒 (誤差在 1 分鐘內)
func CheckSpecificTimeAlert() (string, bool) {

	currentTime := GetCurrentTime()

	// 儲存提醒訊息
	var alertMsg string

	// 判斷邏輯: 檢查當前時間是否在目標時間的 [目標時間-1分鐘, 目標時間+1分鐘] 區間內

	// 為了確保精確性，這裡加入夏令/非夏令時間判斷
	isEST, err := IsUSMarketInWinterTime()
	if err != nil {
		log.Printf("❌ 無法判斷美東冬令時間: %v\n", err)
	}

	if currentTime >= 844 && currentTime <= 846 {
		// 1. 早上 8:45 -> 台指期早盤開盤
		alertMsg = "🔔 台指期早盤開盤倒數中 (08:45)"
	} else if currentTime >= 859 && currentTime <= 901 {
		// 2. 早上 9:00 -> 台股開盤
		alertMsg = "🔔 台股現貨市場開盤 (09:00)"
	} else if currentTime >= 1459 && currentTime <= 1501 {
		// 3. 下午 15:00 -> 台指期夜盤開盤
		alertMsg = "🔔 台指期夜盤開盤 (15:00)"
	} else if isEST && currentTime >= 1659 && currentTime <= 1701 {
		// 4. 下午 17:00 -> 美股盤前 (通常指 CME 交易開始或歐盤收盤前後)
		alertMsg = "🔔 美股盤前交易時段 (17:00 - 冬令時間)"
	} else if isEST && currentTime >= 2229 && currentTime <= 2231 {
		// 5. 下午 22:30 -> 美股開盤 (注意：非夏令時間是 22:30，夏令時間是 21:30)
		alertMsg = "🔔 美股市場開盤 (22:30 - 冬令時間)"
	} else if !isEST && currentTime >= 1559 && currentTime <= 1601 {
		alertMsg = "🔔 美股盤前交易時段 (16:00 - 夏令時間)"
	} else if !isEST && currentTime >= 2129 && currentTime <= 2131 {
		alertMsg = "🔔 美股市場開盤 (21:30 - 夏令時間)"
	}

	isSpecificTime := alertMsg != ""
	return alertMsg, isSpecificTime
}

type SessionMessage interface {
	// lastTWIIVal 前次指數
	// lastDiff 前次差異
	// spotVal 指數
	// futureVal 期權
	// threshold 閾值
	// thresholdChanged 變化幅度
	// return message, shouldNotify
	build(d *Data, spotVal, futureVal, threshold, thresholdChanged float64) (string, bool)
	info(msg string, spotVal, futureVal float64) string
}

type SessionMorningMessage struct {
	prefix string
}

func (s *SessionMorningMessage) info(msg string, spotVal, futureVal float64) string {
	return fmt.Sprintf("[%s]\n台指期權差距: %.2f 點\n加權: %.2f\n期貨: %.2f", msg, math.Abs(spotVal-futureVal), spotVal, futureVal)
}

func (s *SessionMorningMessage) build(d *Data, spotVal, futureVal, threshold, thresholdChanged float64) (string, bool) {
	var alertMsg string
	var shouldNotify bool
	// 計算價差 (加權 - 期貨)
	// 正數 = 逆價差 (期貨 < 加權, 市場偏空)
	// 負數 = 正價差 (期貨 > 加權, 市場偏多)
	diff := spotVal - futureVal
	changed := diff - d.LastDiffValue
	// --- 早盤邏輯 ---

	// 1. **【新高/新低優先判斷】** 期貨突破當早高低點
	if spotVal > d.SpotHigh {
		shouldNotify = true
		alertMsg = fmt.Sprintf("%s (趨勢: %s)\n加權當早新高(前高: %.2f)\n台指期權差距: %.2f 點\n加權: %.2f\n期貨: %.2f",
			s.prefix, "📈", d.SpotHigh, math.Abs(diff), spotVal, futureVal)

	} else if spotVal < d.SpotLow {
		shouldNotify = true
		alertMsg = fmt.Sprintf("%s (趨勢: %s)\n加權當早新低(前低: %.2f)\n台指期權差距: %.2f 點\n加權: %.2f\n期貨: %.2f",
			s.prefix, "📉", d.SpotLow, math.Abs(diff), spotVal, futureVal)

	} else if (spotVal - d.LastTWIIValue) > thresholdChanged {
		shouldNotify = true
		alertMsg = fmt.Sprintf("%s (趨勢: %s)\n加權上漲幅度: %.2f (前值: %.2f)\n台指期權差距: %.2f 點\n加權: %.2f\n期貨: %.2f",
			s.prefix, "📈", math.Abs(spotVal-d.LastTWIIValue), d.LastTWIIValue, math.Abs(diff), spotVal, futureVal)

	} else if (spotVal - d.LastTWIIValue) < -thresholdChanged {
		shouldNotify = true
		alertMsg = fmt.Sprintf("%s (趨勢: %s)\n加權下跌幅度: %.2f (前值: %.2f)\n台指期權差距: %.2f 點\n加權: %.2f\n期貨: %.2f",
			s.prefix, "📉", math.Abs(spotVal-d.LastTWIIValue), d.LastTWIIValue, math.Abs(diff), spotVal, futureVal)

	} else if diff > threshold {
		shouldNotify = true
		// 加權 > 期貨 (逆價差過大, 市場偏空)
		alertMsg = fmt.Sprintf("%s (趨勢: %s) 逆價差過大\n台指期權差距: %.2f 點\n加權: %.2f\n期貨: %.2f",
			s.prefix, "📉", math.Abs(diff), spotVal, futureVal)

		if math.Abs(changed) < thresholdChanged {
			shouldNotify = false // 跟上次確認差異過小
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n",
				math.Abs(diff), math.Abs(d.LastDiffValue), thresholdChanged)

		} else if changed > 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s) 逆價差過大\n逆價差幅度增加: %.2f (前值: %.2f 當前: %.2f)\n加權: %.2f\n期貨: %.2f",
				s.prefix, "📉", math.Abs(changed), d.LastDiffValue, diff, spotVal, futureVal)
		} else if changed < 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s) 逆價差過大\n逆價差幅度減少: %.2f (前值: %.2f 當前: %.2f)\n加權: %.2f\n期貨: %.2f",
				s.prefix, "📉", math.Abs(changed), d.LastDiffValue, diff, spotVal, futureVal)
		}

	} else if diff < -threshold {
		shouldNotify = true
		// 加權 < 期貨 (正價差過大, 市場偏多)
		alertMsg = fmt.Sprintf("%s (趨勢: %s) 正價差過大\n台指期權差距: %.2f 點\n加權: %.2f\n期貨: %.2f",
			s.prefix, "📈", math.Abs(diff), spotVal, futureVal)

		if math.Abs(changed) < thresholdChanged {
			shouldNotify = false // 跟上次確認差異過小
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n",
				math.Abs(diff), math.Abs(d.LastDiffValue), thresholdChanged)

		} else if changed < 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s) 正價差過大\n正價差幅度增加: %.2f (前值: %.2f 當前: %.2f)\n加權: %.2f\n期貨: %.2f",
				s.prefix, "📈", math.Abs(changed), d.LastDiffValue, diff, spotVal, futureVal)
		} else if changed > 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s) 正價差過大\n正價差幅度減少: %.2f (前值: %.2f 當前: %.2f)\n加權: %.2f\n期貨: %.2f",
				s.prefix, "📈", math.Abs(changed), d.LastDiffValue, diff, spotVal, futureVal)
		}

	} else {
		// 未達通知閾值, 早盤不單獨判斷增減幅度超過閾值
		fmt.Printf("%s 台指期權差距: %.2f(閾值: %.2f), 未達通知閾值\n",
			s.prefix, math.Abs(diff), threshold)
	}

	return alertMsg, shouldNotify
}

type SessionNightMessage struct {
	prefix string
}

func (s *SessionNightMessage) info(msg string, spotVal, futureVal float64) string {
	return fmt.Sprintf("[%s]\n期貨與早盤收盤差距: %.2f 點\n早盤收盤加權: %.2f\n夜盤期貨: %.2f", msg, math.Abs(spotVal-futureVal), spotVal, futureVal)
}

func (s *SessionNightMessage) build(d *Data, spotVal, futureVal, threshold, thresholdChanged float64) (string, bool) {
	var alertMsg string
	var shouldNotify bool
	// 計算價差 (早盤收盤加權 - 夜盤期貨)
	// 正數 = 收盤高於期貨
	// 負數 = 收盤低於期貨
	diff := d.LastTWIIValue - futureVal
	changed := diff - d.LastDiffValue
	// --- 夜盤邏輯 ---

	// **【新高/新低優先判斷】** 期貨突破當早高低點
	if futureVal > d.FutureHigh {
		shouldNotify = true
		alertMsg = fmt.Sprintf("%s (趨勢: %s)\n期貨當早新高(前高: %.2f)\n期貨與早盤收盤差距: %.2f 點\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
			s.prefix, "📈", d.FutureHigh, math.Abs(diff), d.LastTWIIValue, futureVal)

	} else if futureVal < d.FutureLow {
		shouldNotify = true
		alertMsg = fmt.Sprintf("%s (趨勢: %s)\n期貨當早新低(前低: %.2f)\n期貨與早盤收盤差距: %.2f 點\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
			s.prefix, "📉", d.FutureLow, math.Abs(diff), d.LastTWIIValue, futureVal)

		// ** 價差變動超過閾值
	} else if diff > threshold {
		shouldNotify = true
		// 收盤 > 期貨 (期貨大跌)
		alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨大跌 (低於早盤收盤)\n期貨與早盤收盤差距: %.2f 點\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
			s.prefix, "📉", math.Abs(diff), d.LastTWIIValue, futureVal)

		if math.Abs(changed) < thresholdChanged {
			shouldNotify = false // 跟上次確認差異過小
			alertMsg = ""
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n",
				math.Abs(diff), math.Abs(d.LastDiffValue), thresholdChanged)

		} else if changed < 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨下跌 (低於早盤收盤)\n期貨下跌幅度減少: %.2f (前值: %.2f, 當前: %.2f)\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
				s.prefix, "📉", math.Abs(changed), d.LastDiffValue, diff, d.LastTWIIValue, futureVal)
		} else if changed > 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨下跌 (低於早盤收盤)\n期貨下跌幅度增加: %.2f (前值: %.2f, 當前: %.2f)\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
				s.prefix, "📉", math.Abs(changed), d.LastDiffValue, diff, d.LastTWIIValue, futureVal)
		}

	} else if diff < -threshold {
		shouldNotify = true
		// 收盤 < 期貨 (期貨大漲)
		alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨大漲 (高於早盤收盤)\n期貨與早盤收盤差距: %.2f 點\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
			s.prefix, "📈", math.Abs(diff), d.LastTWIIValue, futureVal)

		if math.Abs(changed) < thresholdChanged {
			shouldNotify = false // 跟上次確認差異過小
			alertMsg = ""
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n",
				math.Abs(diff), math.Abs(d.LastDiffValue), thresholdChanged)

		} else if changed > 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨上漲 (高於早盤收盤)\n期貨上漲幅度減少: %.2f (前值: %.2f, 當前: %.2f)\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
				s.prefix, "📈", math.Abs(changed), d.LastDiffValue, diff, d.LastTWIIValue, futureVal)
		} else if changed < 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨上漲 (高於早盤收盤)\n期貨上漲幅度增加: %.2f (前值: %.2f, 當前: %.2f)\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
				s.prefix, "📈", math.Abs(changed), d.LastDiffValue, diff, d.LastTWIIValue, futureVal)
		}

		// ** 價差變動幅度超過閾值
	} else if changed > thresholdChanged {
		shouldNotify = true
		if diff < 0 && d.LastDiffValue < 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨上漲 (高於早盤收盤)\n期貨上漲幅度減少: %.2f (前值: %.2f, 當前: %.2f)\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
				s.prefix, "📈", math.Abs(changed), d.LastDiffValue, diff, d.LastTWIIValue, futureVal)
		} else if diff < 0 && d.LastDiffValue > 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨上漲反轉 (高於早盤收盤)\n期貨反轉幅度: %.2f (前值: %.2f, 當前: %.2f)\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
				s.prefix, "📈", math.Abs(changed), d.LastDiffValue, diff, d.LastTWIIValue, futureVal)
		} else if diff >= 0 && d.LastDiffValue < 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨下跌反轉 (低於早盤收盤)\n期貨反轉幅度: %.2f (前值: %.2f, 當前: %.2f)\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
				s.prefix, "📉", math.Abs(changed), d.LastDiffValue, diff, d.LastTWIIValue, futureVal)
		} else if diff >= 0 && d.LastDiffValue > 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨下跌 (低於早盤收盤)\n期貨下跌幅度增加: %.2f (前值: %.2f, 當前: %.2f)\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
				s.prefix, "📉", math.Abs(changed), d.LastDiffValue, diff, d.LastTWIIValue, futureVal)
		}

	} else if changed < -thresholdChanged {
		shouldNotify = true
		if diff < 0 && d.LastDiffValue < 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨上漲 (高於早盤收盤)\n期貨上漲幅度增加: %.2f (前值: %.2f, 當前: %.2f)\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
				s.prefix, "📈", math.Abs(changed), d.LastDiffValue, diff, d.LastTWIIValue, futureVal)
		} else if diff < 0 && d.LastDiffValue > 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨上漲反轉 (高於早盤收盤)\n期貨反轉幅度: %.2f (前值: %.2f, 當前: %.2f)\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
				s.prefix, "📈", math.Abs(changed), d.LastDiffValue, diff, d.LastTWIIValue, futureVal)
		} else if diff >= 0 && d.LastDiffValue < 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨下跌反轉 (低於早盤收盤)\n期貨反轉幅度: %.2f (前值: %.2f, 當前: %.2f)\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
				s.prefix, "📉", math.Abs(changed), d.LastDiffValue, diff, d.LastTWIIValue, futureVal)
		} else if diff >= 0 && d.LastDiffValue > 0 {
			alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨下跌 (低於早盤收盤)\n期貨下跌幅度減少: %.2f (前值: %.2f, 當前: %.2f)\n早盤收盤加權: %.2f\n夜盤期貨: %.2f",
				s.prefix, "📉", math.Abs(changed), d.LastDiffValue, diff, d.LastTWIIValue, futureVal)
		}

	} else {
		// 未達通知閾值
		fmt.Printf("%s 期貨與早盤收盤差距: %.2f(閾值: %.2f), 期貨漲跌幅度: %.2f(閾值: %.2f), 均未達通知閾值\n",
			s.prefix, math.Abs(diff), threshold, math.Abs(changed), thresholdChanged)
	}

	return alertMsg, shouldNotify
}

func newSessionMessage(s string) (SessionMessage, error) {
	var o SessionMessage = nil
	var err error
	switch s {
	case SessionMorning:
		o = &SessionMorningMessage{"☀️ [早盤警示]"}
	case SessionNight:
		o = &SessionNightMessage{"🌙 [夜盤警示]"}
	default:
		err = fmt.Errorf("未知市場%s", s)
	}

	return o, err
}

type Message struct {
	s SessionMessage
	d *Data
}

func NewMessage(s string) (*Message, error) {
	sm, err := newSessionMessage(s)
	if err != nil {
		return nil, err
	}

	return &Message{
		s: sm,
	}, nil
}

func (m *Message) Info(msg string, spotVal, futureVal float64) string {
	return m.s.info(msg, spotVal, futureVal)
}

func (m *Message) Build(d *Data, spotVal, futureVal, threshold, thresholdChanged float64) (string, bool) {
	return m.s.build(d, spotVal, futureVal, threshold, thresholdChanged)
}
