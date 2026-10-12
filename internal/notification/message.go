package notification

import (
	"fmt"
	"time"
)

// Message は、イベントを、人が読む 1 行の文にする。Discord や Slack の Webhook のように、文で届ける通知先に使う。
// 日時は、表示のタイムゾーン location で書く。nil なら UTC で書く。payload に材料がなければ、種類だけの文にする。
func Message(kind Kind, payload map[string]any, location *time.Location) string {
	if location == nil {
		location = time.UTC
	}
	name, _ := payload["name"].(string)
	// withName は、番組名があれば「番組名」の録画を…の形にし、なければ録画を…の形にする。
	withName := func(text string) string {
		if name == "" {
			return text
		}

		return fmt.Sprintf("「%s」の%s", name, text)
	}

	switch kind {
	case KindRecordingStarted:
		return withName("録画を始めました")
	case KindRecordingCompleted:
		return withName("録画が終わりました")
	case KindRecordingFailed:
		return withName("録画に失敗しました")
	case KindReservationConflict:
		text := "チューナーが足りず、録画できない見込みの予約があります"
		if detail := conflictDetail(payload, location); detail != "" {
			return fmt.Sprintf("%s。%s", text, detail)
		}

		return text
	case KindStorageLow:
		return "保存先の空き容量が足りません"
	case KindTunerServerUnreachable:
		return "チューナーサーバーに接続できません"
	default:
		return string(kind)
	}
}

// conflictDetail は、競合した予約を「1/2（金）21:00 NHK総合「番組名」」の形にする。
func conflictDetail(payload map[string]any, location *time.Location) string {
	detail := ""
	if raw, ok := payload["startAt"].(string); ok {
		if startAt, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			local := startAt.In(location)
			detail = fmt.Sprintf("%d/%d（%s）%s", local.Month(), local.Day(), weekdayName(local.Weekday()), local.Format("15:04"))
		}
	}
	if service, ok := payload["serviceName"].(string); ok && service != "" {
		if detail != "" {
			detail += " "
		}
		detail += service
	}
	if name, ok := payload["name"].(string); ok && name != "" {
		detail += "「" + name + "」"
	}

	return detail
}

// weekdayName は、曜日の日本語の略称を返す。
func weekdayName(weekday time.Weekday) string {
	return [...]string{"日", "月", "火", "水", "木", "金", "土"}[weekday]
}
