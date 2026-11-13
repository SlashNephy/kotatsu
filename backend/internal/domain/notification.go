package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
	"uuid"
)

//go:generate go tool go-enum --values --nocomments --sql

// NotificationKind は、通知するイベントの種類 (設計書 6.2)。値を並べた順が、画面に並べる順になる。
//
// ENUM(
// recording_started
// recording_completed
// recording_failed
// reservation_conflict
// storage_low
// tuner_server_unreachable
// watch_reminder // 視聴予約した番組の放送が近いことを知らせる (設計書 4.8)。予約したユーザーだけに届け、Webhook では送らない。
// anime_recorded // 見たアニメを Annict や AniList に記録したことを知らせる (#927)。見たユーザーだけに届け、Webhook では送らない。
// anime_record_failed // 見たアニメを記録できなかったことを知らせる (#927)。見たユーザーだけに届け、Webhook では送らない。
// scrambled_recordings // スクランブルされたまま録れた録画が続いたことを知らせる (#1517)。B-CAS カードを確かめられるのは管理者なので、管理者にだけ届ける。管理者が設定する Webhook でも送る。
// )
type NotificationKind string

// WebhookNotificationKinds は、Webhook で送れるイベントの種類を、画面に並べる順に返す。
// 視聴予約とアニメの記録の通知は個人に届けるものなので、管理者が設定する全体向けの Webhook では送らない。
func WebhookNotificationKinds() []NotificationKind {
	return slices.DeleteFunc(NotificationKindValues(), func(kind NotificationKind) bool {
		return kind == NotificationKindWatchReminder || kind == NotificationKindAnimeRecorded || kind == NotificationKindAnimeRecordFailed
	})
}

// NotificationExternalURL は、イベントが示す外部のページ (記録した Annict のページなど) の URL を返す。
// payload に externalUrl がないか、https の URL でなければ空文字列を返す。
func NotificationExternalURL(payload map[string]any) string {
	url, _ := payload["externalUrl"].(string)
	if !strings.HasPrefix(url, "https://") {
		return ""
	}

	return url
}

// NotificationURL は、イベントを押したときに開く画面のパスを返す。payload に url がなければ空文字列を返す。
func NotificationURL(payload map[string]any) string {
	url, _ := payload["url"].(string)

	return url
}

// Notification は、ユーザーに届いた画面内の通知。
type Notification struct {
	ID         uuid.UUID
	Kind       NotificationKind
	OccurredAt time.Time
	Payload    map[string]any
	CreatedAt  time.Time
	// ReadAt は、既読にした日時。未読なら nil。
	ReadAt *time.Time
}

// NotificationPosition は、通知の一覧での位置。通知を作った日時と ID で並べる。
type NotificationPosition struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// ErrInvalidNotificationLimit は、一覧の件数の上限が 1 未満であることを表す。
var ErrInvalidNotificationLimit = errors.New("the limit must be positive")

// NotificationPreference は、イベントの種類ごとの通知設定。
type NotificationPreference struct {
	Kind    NotificationKind
	InApp   bool
	WebPush bool
	// RuleReservations は、ルール予約の通知も受け取るかどうか。録画の通知と競合の通知でだけ使う。
	RuleReservations bool
}
