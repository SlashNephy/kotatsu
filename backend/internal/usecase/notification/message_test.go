package notification_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
)

const (
	keyName         = "name"
	keyServiceName  = "serviceName"
	keyNotBroadcast = "notBroadcast"
	keyStartAt      = "startAt"
	keyAiredName    = "airedName"
	serviceName     = "NHK総合"
	programName     = "こたつ特集"
	evening         = "2026-10-10T12:00:00Z"
)

func TestMessage(t *testing.T) {
	t.Parallel()

	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	conflict := map[string]any{keyName: programName, keyServiceName: serviceName, keyStartAt: "2026-10-10T15:00:00Z"}
	tests := []struct {
		name     string
		kind     domain.NotificationKind
		payload  map[string]any
		location *time.Location
		want     string
	}{
		{name: "番組名のある録画の開始", kind: domain.NotificationKindRecordingStarted, payload: map[string]any{keyName: programName}, location: tokyo, want: "「こたつ特集」の録画を始めました"},
		{name: "番組名のない録画の完了", kind: domain.NotificationKindRecordingCompleted, payload: map[string]any{}, location: tokyo, want: "録画が終わりました"},
		{
			name: "放送されなかった録画の完了", kind: domain.NotificationKindRecordingCompleted, location: tokyo,
			payload: map[string]any{keyName: programName, keyNotBroadcast: true, keyAiredName: "ニュース"},
			want:    "「こたつ特集」は、予定の時間に放送されませんでした。放送されていたのは「ニュース」です",
		},
		{
			name: "放送されていた番組が分からない", kind: domain.NotificationKindRecordingCompleted, location: tokyo,
			payload: map[string]any{keyNotBroadcast: true}, want: "予約した番組は、予定の時間に放送されませんでした",
		},
		{name: "録画の失敗", kind: domain.NotificationKindRecordingFailed, payload: map[string]any{keyName: programName}, location: tokyo, want: "「こたつ特集」の録画に失敗しました"},
		{
			name: "競合は、表示のタイムゾーンの日時で書く。日付をまたぐ", kind: domain.NotificationKindReservationConflict, payload: conflict, location: tokyo,
			want: "チューナーが足りず、録画できない見込みの予約があります。10/11（日）00:00 NHK総合「こたつ特集」",
		},
		{
			name: "材料のない競合", kind: domain.NotificationKindReservationConflict, payload: map[string]any{}, location: tokyo,
			want: "チューナーが足りず、録画できない見込みの予約があります",
		},
		{
			name: "競合は、タイムゾーンがなければ UTC で書く", kind: domain.NotificationKindReservationConflict, payload: conflict, location: nil,
			want: "チューナーが足りず、録画できない見込みの予約があります。10/10（土）15:00 NHK総合「こたつ特集」",
		},
		{name: "空き容量の不足", kind: domain.NotificationKindStorageLow, payload: map[string]any{}, location: tokyo, want: "保存先の空き容量が足りません"},
		{name: "チューナーサーバーへの接続不可", kind: domain.NotificationKindTunerServerUnreachable, payload: map[string]any{}, location: tokyo, want: "チューナーサーバーに接続できません"},
		{
			name: "視聴予約は、表示のタイムゾーンの時刻で書く", kind: domain.NotificationKindWatchReminder,
			payload: map[string]any{keyName: programName, keyServiceName: serviceName, keyStartAt: evening}, location: tokyo,
			want: "21:00 から NHK総合で「こたつ特集」が始まります",
		},
		{
			name: "視聴予約の番組名とサービス名は、全角の英数字を半角にする", kind: domain.NotificationKindWatchReminder,
			payload: map[string]any{keyName: "アニメ　＃１２「再会」", keyServiceName: "ＴＯＫＹＯ　ＭＸ１", keyStartAt: evening}, location: tokyo,
			want: "21:00 から TOKYO MX1で「アニメ #12「再会」」が始まります",
		},
		{
			name: "競合の番組名とサービス名も半角にする", kind: domain.NotificationKindReservationConflict, location: tokyo,
			payload: map[string]any{keyName: "ニュース１０", keyServiceName: "ＢＳ１１", keyStartAt: evening},
			want:    "チューナーが足りず、録画できない見込みの予約があります。10/10（土）21:00 BS11「ニュース10」",
		},
		{
			name: "放送されていた番組名も半角にする", kind: domain.NotificationKindRecordingCompleted, location: tokyo,
			payload: map[string]any{keyName: "ドラマ２４", keyNotBroadcast: true, keyAiredName: "ニュース　ＷＢＳ"},
			want:    "「ドラマ24」は、予定の時間に放送されませんでした。放送されていたのは「ニュース WBS」です",
		},
		{name: "材料のない視聴予約", kind: domain.NotificationKindWatchReminder, payload: map[string]any{}, location: tokyo, want: "視聴予約した番組が始まります"},
		{
			name: "見たアニメの記録は、番組名と話数と記録先を書く", kind: domain.NotificationKindAnimeRecorded, location: tokyo,
			payload: map[string]any{keyName: "アニメ　＃３", "destination": "annict", "episodes": "#3、#4"},
			want:    "「アニメ #3」の#3、#4をAnnictに記録しました",
		},
		{
			name: "記録できなかったときは、理由を添える", kind: domain.NotificationKindAnimeRecordFailed, location: tokyo,
			payload: map[string]any{keyName: programName, "destination": "anilist", "episodes": "#3", "reason": "連携していません"},
			want:    "「こたつ特集」の#3をAniListに記録できませんでした（連携していません）",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, notification.Message(tt.kind, tt.payload, tt.location))
		})
	}
}

// TestMessageEscaped は、番組名、放送されていた番組名、サービス名を、escape で書き換えることを確かめる (#1010)。
func TestMessageEscaped(t *testing.T) {
	t.Parallel()

	mark := func(text string) string { return "<" + text + ">" }
	tests := []struct {
		name    string
		kind    domain.NotificationKind
		payload map[string]any
		want    string
	}{
		{name: "録画の開始の番組名", kind: domain.NotificationKindRecordingStarted, payload: map[string]any{keyName: programName}, want: "「<こたつ特集>」の録画を始めました"},
		{
			name: "放送されなかった録画の番組名", kind: domain.NotificationKindRecordingCompleted,
			payload: map[string]any{keyName: programName, keyNotBroadcast: true, keyAiredName: "ニュース"},
			want:    "「<こたつ特集>」は、予定の時間に放送されませんでした。放送されていたのは「<ニュース>」です",
		},
		{
			name: "競合した予約のサービス名と番組名", kind: domain.NotificationKindReservationConflict,
			payload: map[string]any{keyName: programName, keyServiceName: serviceName},
			want:    "チューナーが足りず、録画できない見込みの予約があります。<NHK総合>「<こたつ特集>」",
		},
		{
			name: "視聴予約のサービス名と番組名", kind: domain.NotificationKindWatchReminder,
			payload: map[string]any{keyName: programName, keyServiceName: serviceName},
			want:    "<NHK総合>で「<こたつ特集>」が始まります",
		},
		{name: "名前がなければ書き換えない", kind: domain.NotificationKindRecordingFailed, payload: map[string]any{}, want: "録画に失敗しました"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, notification.MessageEscaped(tt.kind, tt.payload, time.UTC, mark))
		})
	}
}
