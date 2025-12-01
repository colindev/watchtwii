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
	build(d *Data, spotVal, futureVal, threshold, thresholdChanged float64) (string, bool)
	info(msg string, spotVal, futureVal float64) string
}

type SessionMorningMessage struct {
	prefix string
}

func (s *SessionMorningMessage) info(msg string, spotVal, futureVal float64) string {
	return fmt.Sprintf("[%s]\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", msg, math.Abs(spotVal-futureVal), spotVal, futureVal)
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

	// 1. **【新高/新低優先判斷】** 期貨突破當日高低點
	if futureVal > d.FutureHigh {
		shouldNotify = true
		alertMsg = fmt.Sprintf("%s (趨勢: %s) 期貨當日新高\n台指期權差距: %.2f 點\n加權: %.2f\n期貨: %.2f",
			s.prefix, "📈", math.Abs(diff), spotVal, futureVal)
	} else if futureVal < d.FutureLow {
		shouldNotify = true
		alertMsg = fmt.Sprintf("%s (趨勢: %s) 期貨當日新低\n台指期權差距: %.2f 點\n加權: %.2f\n期貨: %.2f",
			s.prefix, "📉", math.Abs(diff), spotVal, futureVal)

	} else if diff > threshold {
		// 2. 加權 > 期貨 (逆價差過大, 市場偏空)
		alertMsg = fmt.Sprintf("%s (趨勢: %s) 逆價差過大\n差距: %.2f 點\n加權: %.2f\n期貨: %.2f", "📉", s.prefix, diff, spotVal, futureVal)
		shouldNotify = true
		if math.Abs(changed) < thresholdChanged {
			shouldNotify = false // 跟上次確認差異過小
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n", diff, d.LastDiffValue, thresholdChanged)
		} else if changed > 0 {
			alertMsg = fmt.Sprintf("📉(逆價差幅度擴大:%.2f)\n%s", changed, alertMsg)
		} else if changed < 0 {
			alertMsg = fmt.Sprintf("📈(逆價差幅度縮小:%.2f)\n%s", changed, alertMsg)
		}

	} else if diff < -threshold {
		// 3. 加權 < 期貨 (正價差過大, 市場偏多)
		alertMsg = fmt.Sprintf("%s (趨勢: %s) 正價差過大\n差距: %.2f 點\n加權: %.2f\n期貨: %.2f", "📈", s.prefix, math.Abs(diff), spotVal, futureVal)
		shouldNotify = true
		if math.Abs(changed) < thresholdChanged {
			shouldNotify = false // 跟上次確認差異過小
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n", diff, d.LastDiffValue, thresholdChanged)
		} else if changed < 0 {
			alertMsg = fmt.Sprintf("📈(正價差幅度擴大:%.2f)\n%s", changed, alertMsg)
		} else if changed > 0 {
			alertMsg = fmt.Sprintf("📉(正價差幅度縮小:%.2f)\n%s", changed, alertMsg)
		}

	} else {
		// 4. 未達通知閾值
		fmt.Printf("%s 台指期權差距: %.2f(閾值: %.2f), 價差變動幅度: %.2f(閾值: %.2f), 均未達通知閾值\n", s.prefix, diff, threshold, changed, thresholdChanged)
	}
	return alertMsg, shouldNotify
}

type SessionNightMessage struct {
	prefix string
}

func (s *SessionNightMessage) info(msg string, spotVal, futureVal float64) string {
	return fmt.Sprintf("[%s]\n差距: %.2f 點\n加權: %.2f\n台指: %.2f", msg, math.Abs(spotVal-futureVal), spotVal, futureVal)
}

func (s *SessionNightMessage) build(d *Data, spotVal, futureVal, threshold, thresholdChanged float64) (string, bool) {
	var alertMsg string
	var shouldNotify bool
	// 計算價差 (夜盤期貨 - 日盤收盤加權)
	// 正數 = 期貨高於收盤
	// 負數 = 期貨低於收盤
	diff := futureVal - d.LastTWIIValue
	changed := futureVal - d.LastTWIIValue - d.LastDiffValue // 實質上是 diff - d.LastDiffValue
	// --- 夜盤邏輯 ---

	// 1. **【新高/新低優先判斷】** 期貨突破當日高低點
	if futureVal > d.FutureHigh {
		shouldNotify = true
		alertMsg = fmt.Sprintf("%s (趨勢: %s) 期貨當日新高\n期貨與日盤收盤差距: %.2f 點\n日盤收盤加權: %.2f\n夜盤期貨: %.2f",
			s.prefix, "📈", math.Abs(diff), d.LastTWIIValue, futureVal)

	} else if futureVal < d.FutureLow {
		shouldNotify = true
		alertMsg = fmt.Sprintf("%s (趨勢: %s) 期貨當日新低\n期貨與日盤收盤差距: %.2f 點\n日盤收盤加權: %.2f\n夜盤期貨: %.2f",
			s.prefix, "📉", math.Abs(diff), d.LastTWIIValue, futureVal)

	} else if diff > threshold {
		// 2. 期貨 > 收盤 (大漲)
		alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨大漲 (高於日盤收盤)\n差距: %.2f 點\n日盤收盤加權: %.2f\n夜盤期貨: %.2f", s.prefix, "📈", diff, d.LastTWIIValue, futureVal)
		shouldNotify = true
		if math.Abs(changed) < thresholdChanged {
			shouldNotify = false // 跟上次確認差異過小
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n", diff, d.LastDiffValue, thresholdChanged)
		} else if changed > 0 {
			alertMsg = fmt.Sprintf("📈(期貨上漲幅度擴大:%.2f)\n%s", changed, alertMsg)
		} else if changed < 0 {
			alertMsg = fmt.Sprintf("📉(期貨上漲幅度縮小:%.2f)\n%s", changed, alertMsg)
		}

	} else if diff < -threshold {
		// 3. 期貨 < 收盤 (大跌)
		alertMsg = fmt.Sprintf("%s (趨勢: %s)\n夜盤期貨大跌 (低於日盤收盤)\n差距: %.2f 點\n日盤收盤加權: %.2f\n夜盤期貨: %.2f", s.prefix, "📉", math.Abs(diff), d.LastTWIIValue, futureVal)
		shouldNotify = true
		if math.Abs(changed) < thresholdChanged {
			shouldNotify = false // 跟上次確認差異過小
			fmt.Printf("✅ 已超過閾值 (%.2f)，但與上次通知值 (%.2f) 變動幅度不超過 %.2f，抑制通知。\n", diff, d.LastDiffValue, thresholdChanged)
		} else if changed < 0 {
			alertMsg = fmt.Sprintf("📉(期貨下跌幅度擴大:%.2f)\n%s", changed, alertMsg)
		} else if changed > 0 {
			alertMsg = fmt.Sprintf("📈(期貨下跌幅度縮小:%.2f)\n%s", changed, alertMsg)
		}

	} else {
		// 4. 未達通知閾值
		fmt.Printf("%s 期貨與日盤收盤差距: %.2f(閾值: %.2f), 期貨漲跌幅度: %.2f(閾值: %.2f), 均未達通知閾值\n", s.prefix, diff, threshold, changed, thresholdChanged)
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
