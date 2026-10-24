package notification

import (
	"fmt"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/halfwidth"
)

// Message は、イベントを、人が読む 1 行の文にする。Discord や Slack の Webhook のように、文で届ける通知先に使う。
// 日時は、表示のタイムゾーン location で書く。nil なら UTC で書く。payload に材料がなければ、種類だけの文にする。
// 番組名とサービス名は、画面の表示と同じく、全角の英数字を半角にして書く。
func Message(kind domain.NotificationKind, payload map[string]any, location *time.Location) string {
	if location == nil {
		location = time.UTC
	}
	name := payloadText(payload, "name")
	// withName は、番組名があれば「番組名」の録画を…の形にし、なければ録画を…の形にする。
	withName := func(text string) string {
		if name == "" {
			return text
		}

		return fmt.Sprintf("「%s」の%s", name, text)
	}

	switch kind {
	case domain.NotificationKindRecordingStarted:
		return withName("録画を始めました")
	case domain.NotificationKindRecordingCompleted:
		if notBroadcast, _ := payload["notBroadcast"].(bool); notBroadcast {
			text := "予約した番組は、予定の時間に放送されませんでした"
			if name != "" {
				text = fmt.Sprintf("「%s」は、予定の時間に放送されませんでした", name)
			}
			if aired := payloadText(payload, "airedName"); aired != "" {
				text += fmt.Sprintf("。放送されていたのは「%s」です", aired)
			}

			return text
		}

		return withName("録画が終わりました")
	case domain.NotificationKindRecordingFailed:
		return withName("録画に失敗しました")
	case domain.NotificationKindReservationConflict:
		text := "チューナーが足りず、録画できない見込みの予約があります"
		if detail := conflictDetail(payload, location); detail != "" {
			return fmt.Sprintf("%s。%s", text, detail)
		}

		return text
	case domain.NotificationKindStorageLow:
		return "保存先の空き容量が足りません"
	case domain.NotificationKindTunerServerUnreachable:
		return "チューナーサーバーに接続できません"
	case domain.NotificationKindWatchReminder:
		return watchReminderMessage(name, payload, location)
	default:
		return string(kind)
	}
}

// payloadText は、payload の key の文字列を、全角の英数字を半角にして返す。なければ空にする。
func payloadText(payload map[string]any, key string) string {
	value, _ := payload[key].(string)

	return halfwidth.Convert(value)
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
	if service := payloadText(payload, "serviceName"); service != "" {
		if detail != "" {
			detail += " "
		}
		detail += service
	}
	if name := payloadText(payload, "name"); name != "" {
		detail += "「" + name + "」"
	}

	return detail
}

// weekdayName は、曜日の日本語の略称を返す。
func weekdayName(weekday time.Weekday) string {
	return [...]string{"日", "月", "火", "水", "木", "金", "土"}[weekday]
}

// watchReminderMessage は、視聴予約した番組を「21:00 から NHK総合で「番組名」が始まります」の形にする。
// 放送の数分前に送るので、日付は書かない。
func watchReminderMessage(name string, payload map[string]any, location *time.Location) string {
	if name == "" {
		return "視聴予約した番組が始まります"
	}
	text := fmt.Sprintf("「%s」が始まります", name)
	if service := payloadText(payload, "serviceName"); service != "" {
		text = fmt.Sprintf("%sで%s", service, text)
	}
	if raw, _ := payload["startAt"].(string); raw != "" {
		if startAt, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			text = fmt.Sprintf("%s から %s", startAt.In(location).Format("15:04"), text)
		}
	}

	return text
}
