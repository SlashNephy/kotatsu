package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// NotificationKind は、通知するイベントの種類 (設計書 6.2)。
type NotificationKind string

const (
	NotificationKindRecordingStarted       NotificationKind = "recording_started"
	NotificationKindRecordingCompleted     NotificationKind = "recording_completed"
	NotificationKindRecordingFailed        NotificationKind = "recording_failed"
	NotificationKindReservationConflict    NotificationKind = "reservation_conflict"
	NotificationKindStorageLow             NotificationKind = "storage_low"
	NotificationKindTunerServerUnreachable NotificationKind = "tuner_server_unreachable"
	// NotificationKindWatchReminder は、視聴予約した番組の放送が近いことを知らせる (設計書 4.8)。予約したユーザーだけに届け、Webhook では送らない。
	NotificationKindWatchReminder NotificationKind = "watch_reminder"
	// NotificationKindAnimeRecorded は、見たアニメを Annict や AniList に記録したことを知らせる (#927)。見たユーザーだけに届け、Webhook では送らない。
	NotificationKindAnimeRecorded NotificationKind = "anime_recorded"
	// NotificationKindAnimeRecordFailed は、見たアニメを記録できなかったことを知らせる (#927)。見たユーザーだけに届け、Webhook では送らない。
	NotificationKindAnimeRecordFailed NotificationKind = "anime_record_failed"
)

// ErrUnknownNotificationKind は、知らない種類のイベントであることを表す。
var ErrUnknownNotificationKind = errors.New("unknown notification kind")

// NotificationKinds は、通知するイベントの種類を、画面に並べる順に返す。
func NotificationKinds() []NotificationKind {
	return []NotificationKind{
		NotificationKindRecordingStarted, NotificationKindRecordingCompleted, NotificationKindRecordingFailed,
		NotificationKindReservationConflict, NotificationKindStorageLow, NotificationKindTunerServerUnreachable, NotificationKindWatchReminder,
		NotificationKindAnimeRecorded, NotificationKindAnimeRecordFailed,
	}
}

// WebhookNotificationKinds は、Webhook で送れるイベントの種類を、画面に並べる順に返す。
// 視聴予約とアニメの記録の通知は個人に届けるものなので、管理者が設定する全体向けの Webhook では送らない。
func WebhookNotificationKinds() []NotificationKind {
	return slices.DeleteFunc(NotificationKinds(), func(kind NotificationKind) bool {
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

// ParseNotificationKind は、文字列をイベントの種類にする。
func ParseNotificationKind(s string) (NotificationKind, error) {
	kind := NotificationKind(s)
	if !slices.Contains(NotificationKinds(), kind) {
		return "", fmt.Errorf("%w: %q", ErrUnknownNotificationKind, s)
	}

	return kind, nil
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
