package main

import (
	"fmt"
	"math"
)

type SessionMessage interface {
	// lastTWIIVal 前次指數
	// lastDiff 前次差異
	// spotVal 指數
	// futureVal 期權
	// threshold 閾值
	// thresholdChanged 變化幅度
	// return message, shouldNotify
	build(lastTWIIVal, lastDiff, spotVal, futureVal, threshold, thresholdChanged float64) (string, bool)
	info(msg string, spotVal, futureVal float64) string
}

type SessionMorningMessage struct {
	prefix string
}

func (s *SessionMorningMessage) info(msg string, spotVal, futureVal float64) string {
	return fmt.Sprintf("[%s]\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", msg, math.Abs(spotVal-futureVal), spotVal, futureVal)
}

func (s *SessionMorningMessage) build(lastTWIIVal, lastDiff, spotVal, futureVal, threshold, thresholdChanged float64) (string, bool) {
	var alertMsg string
	var shouldNotify bool
	// 計算價差 (加權 - 期貨)
	// 正數 = 逆價差 (期貨 < 加權, 市場偏空)
	// 負數 = 正價差 (期貨 > 加權, 市場偏多)
	diff := spotVal - futureVal
	changed := diff - lastDiff
	// --- 早盤邏輯 ---
	if diff > threshold {
		// 加權 > 台指 (逆價差過大)
		alertMsg = fmt.Sprintf("☀️ [早盤警示] (趨勢: %s)\n現貨強於期貨 (逆價差)\n台指期權差距: %.2f 點\n台指: %.2f\n期權: %.2f", "📈", math.Abs(diff), spotVal, futureVal)
		shouldNotify = true
		if math.Abs(changed) < thresholdChanged {
			shouldNotify = false // 跟上次確認差異過小
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n", diff, lastDiff, thresholdChanged)
		} else if changed < 0 {
			alertMsg = fmt.Sprintf("📉(價差幅度縮小:%.2f)\n%s", changed, alertMsg)
		} else if changed > 0 {
			alertMsg = fmt.Sprintf("📈(價差幅度擴大:%.2f)\n%s", changed, alertMsg)
		}
	} else if diff < -threshold {
		// 加權 < 台指 (正價差過大)
		alertMsg = fmt.Sprintf("☀️ [早盤警示] (趨勢: %s)\n期貨強於現貨 (正價差)\n台指期權差距: %.2f 點\n台指: %.2f\n期權: %.2f", "📉", math.Abs(diff), spotVal, futureVal)
		shouldNotify = true
		if math.Abs(changed) < thresholdChanged {
			shouldNotify = false // 跟上次確認差異過小
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n", diff, lastDiff, thresholdChanged)
		} else if changed < 0 {
			alertMsg = fmt.Sprintf("📉(價差幅度縮小:%.2f)\n%s", changed, alertMsg)
		} else if changed > 0 {
			alertMsg = fmt.Sprintf("📈(價差幅度擴大:%.2f)\n%s", changed, alertMsg)
		}
	} else if (spotVal - lastTWIIVal) > thresholdChanged {
		// 指數上漲 - 早盤關注加權變動
		alertMsg = fmt.Sprintf("☀️ [早盤警示] (趨勢: %s)\n指數上漲(%.2f last: %.2f)\n台指期權差距: %.2f 點\n台指: %.2f\n期權: %.2f", "📈", (spotVal - lastTWIIVal), lastTWIIVal, math.Abs(diff), spotVal, futureVal)
		shouldNotify = true
	} else if (spotVal - lastTWIIVal) < -thresholdChanged {
		// 指數下跌 - 早盤關注加權變動
		alertMsg = fmt.Sprintf("☀️ [早盤警示] (趨勢: %s)\n指數下跌(%.2f last: %.2f)\n台指期權差距: %.2f 點\n台指: %.2f\n期權: %.2f", "📉", (spotVal - lastTWIIVal), lastTWIIVal, math.Abs(diff), spotVal, futureVal)
		shouldNotify = true
	} else {
		fmt.Printf("☀️ [早盤警示] 台指期權差距: %.2f(閾值: %.2f), 台指變動幅度: %.2f(閾值: %.2f), 均未達通知閾值\n", diff, threshold, changed, thresholdChanged)
	}

	return alertMsg, shouldNotify
}

type SessionNightMessage struct {
	prefix string
}

func (s *SessionNightMessage) info(msg string, spotVal, futureVal float64) string {
	return fmt.Sprintf("[%s]\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", msg, math.Abs(spotVal-futureVal), spotVal, futureVal)
}

func (s *SessionNightMessage) build(lastTWIIVal, lastDiff, spotVal, futureVal, threshold, thresholdChanged float64) (string, bool) {
	var alertMsg string
	var shouldNotify bool
	// 計算價差 (加權 - 期貨)
	// 正數 = 逆價差 (期貨 < 加權, 市場偏空)
	// 負數 = 正價差 (期貨 > 加權, 市場偏多)
	diff := spotVal - futureVal
	changed := diff - lastDiff
	// --- 夜盤邏輯 ---
	// 注意：夜盤的加權是指數收盤價，通常用來參考國際盤對台指的拉動
	if diff > threshold {
		alertMsg = fmt.Sprintf("🌙 [夜盤警示] (趨勢: %s)\n夜盤期貨大跌 (低於日盤收盤)\n台指期權差距: %.2f 點\n收盤台指: %.2f\n夜盤期權: %.2f", "📉", math.Abs(diff), spotVal, futureVal)
		shouldNotify = true
		if math.Abs(changed) < thresholdChanged {
			shouldNotify = false // 跟上次確認差異過小,抑制通知
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n", diff, lastDiff, thresholdChanged)
		} else if changed < 0 {
			alertMsg = fmt.Sprintf("📈(期貨下跌幅度縮小:%.2f)\n%s", changed, alertMsg)
		} else if changed > 0 {
			alertMsg = fmt.Sprintf("📉(期貨下跌幅度擴大:%.2f)\n%s", changed, alertMsg)
		}
	} else if diff < -threshold {
		alertMsg = fmt.Sprintf("🌙 [夜盤警示] (趨勢: %s)\n夜盤期貨大漲 (高於日盤收盤)\n台指期權差距: %.2f 點\n收盤加權: %.2f\n夜盤台指: %.2f", "📈", math.Abs(diff), spotVal, futureVal)
		shouldNotify = true
		if math.Abs(changed) < thresholdChanged {
			shouldNotify = false // 跟上次確認差異過小
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n", diff, lastDiff, thresholdChanged)
		} else if changed < 0 {
			alertMsg = fmt.Sprintf("📈(期貨上漲幅度擴大:%.2f)\n%s", changed, alertMsg)
		} else if changed > 0 {
			alertMsg = fmt.Sprintf("📉(期貨上漲幅度縮小:%.2f)\n%s", changed, alertMsg)
		}
	} else if math.Abs(changed) >= thresholdChanged {
		alertMsg = fmt.Sprintf("🌙 [夜盤警示] 台指期權差距: %.2f(閾值: %.2f), 未達通知閾值\n", diff, threshold)
		if diff > 0 {
			alertMsg = fmt.Sprintf("📉(期貨下跌幅度擴大:%.2f)\n%s", changed, alertMsg)
		} else if diff < 0 {
			alertMsg = fmt.Sprintf("📈(期貨上漲幅度擴大:%.2f)\n%s", changed, alertMsg)
		}
	} else {
		fmt.Printf("🌙 [夜盤警示] 台指期權差距: %.2f(閾值: %.2f), 期權漲跌幅度: %.2f(閾值: %.2f), 均未達通知閾值\n", diff, threshold, changed, thresholdChanged)
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
	return m.s.build(d.LastTWIIValue, d.LastDiffValue, spotVal, futureVal, threshold, thresholdChanged)
}
