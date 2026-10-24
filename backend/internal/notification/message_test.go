package notification_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/notification"
)

const (
	keyName         = "name"
	keyServiceName  = "serviceName"
	keyNotBroadcast = "notBroadcast"
	keyStartAt      = "startAt"
	programName     = "こたつ特集"
	evening         = "2026-10-10T12:00:00Z"
)

func TestMessage(t *testing.T) {
	t.Parallel()

	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	conflict := map[string]any{keyName: programName, keyServiceName: "NHK総合", keyStartAt: "2026-10-10T15:00:00Z"}
	tests := []struct {
		name     string
		kind     notification.Kind
		payload  map[string]any
		location *time.Location
		want     string
	}{
		{name: "番組名のある録画の開始", kind: notification.KindRecordingStarted, payload: map[string]any{keyName: programName}, location: tokyo, want: "「こたつ特集」の録画を始めました"},
		{name: "番組名のない録画の完了", kind: notification.KindRecordingCompleted, payload: map[string]any{}, location: tokyo, want: "録画が終わりました"},
		{
			name: "放送されなかった録画の完了", kind: notification.KindRecordingCompleted, location: tokyo,
			payload: map[string]any{keyName: programName, keyNotBroadcast: true, "airedName": "ニュース"},
			want:    "「こたつ特集」は、予定の時間に放送されませんでした。放送されていたのは「ニュース」です",
		},
		{
			name: "放送されていた番組が分からない", kind: notification.KindRecordingCompleted, location: tokyo,
			payload: map[string]any{keyNotBroadcast: true}, want: "予約した番組は、予定の時間に放送されませんでした",
		},
		{name: "録画の失敗", kind: notification.KindRecordingFailed, payload: map[string]any{keyName: programName}, location: tokyo, want: "「こたつ特集」の録画に失敗しました"},
		{
			name: "競合は、表示のタイムゾーンの日時で書く。日付をまたぐ", kind: notification.KindReservationConflict, payload: conflict, location: tokyo,
			want: "チューナーが足りず、録画できない見込みの予約があります。10/11（日）00:00 NHK総合「こたつ特集」",
		},
		{
			name: "材料のない競合", kind: notification.KindReservationConflict, payload: map[string]any{}, location: tokyo,
			want: "チューナーが足りず、録画できない見込みの予約があります",
		},
		{
			name: "競合は、タイムゾーンがなければ UTC で書く", kind: notification.KindReservationConflict, payload: conflict, location: nil,
			want: "チューナーが足りず、録画できない見込みの予約があります。10/10（土）15:00 NHK総合「こたつ特集」",
		},
		{name: "空き容量の不足", kind: notification.KindStorageLow, payload: map[string]any{}, location: tokyo, want: "保存先の空き容量が足りません"},
		{name: "チューナーサーバーへの接続不可", kind: notification.KindTunerServerUnreachable, payload: map[string]any{}, location: tokyo, want: "チューナーサーバーに接続できません"},
		{
			name: "視聴予約は、表示のタイムゾーンの時刻で書く", kind: notification.KindWatchReminder,
			payload: map[string]any{keyName: programName, keyServiceName: "NHK総合", keyStartAt: evening}, location: tokyo,
			want: "21:00 から NHK総合で「こたつ特集」が始まります",
		},
		{
			name: "視聴予約の番組名とサービス名は、全角の英数字を半角にする", kind: notification.KindWatchReminder,
			payload: map[string]any{keyName: "アニメ　＃１２「再会」", keyServiceName: "ＴＯＫＹＯ　ＭＸ１", keyStartAt: evening}, location: tokyo,
			want: "21:00 から TOKYO MX1で「アニメ #12「再会」」が始まります",
		},
		{
			name: "競合の番組名とサービス名も半角にする", kind: notification.KindReservationConflict, location: tokyo,
			payload: map[string]any{keyName: "ニュース１０", keyServiceName: "ＢＳ１１", keyStartAt: evening},
			want:    "チューナーが足りず、録画できない見込みの予約があります。10/10（土）21:00 BS11「ニュース10」",
		},
		{
			name: "放送されていた番組名も半角にする", kind: notification.KindRecordingCompleted, location: tokyo,
			payload: map[string]any{keyName: "ドラマ２４", keyNotBroadcast: true, "airedName": "ニュース　ＷＢＳ"},
			want:    "「ドラマ24」は、予定の時間に放送されませんでした。放送されていたのは「ニュース WBS」です",
		},
		{name: "材料のない視聴予約", kind: notification.KindWatchReminder, payload: map[string]any{}, location: tokyo, want: "視聴予約した番組が始まります"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, notification.Message(tt.kind, tt.payload, tt.location))
		})
	}
}
