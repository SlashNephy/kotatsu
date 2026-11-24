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

// keyAdded と keyRemoved は、局の顔ぶれの変化の数の項目 (#1519)。
const (
	keyLineupKind = "kind"
	keyNetworkID  = "networkId"
	keyAdded      = "added"
	keyRemoved    = "removed"
	// keyAddedServices と keyRemovedServices は、新しい局と終わった局の並びの項目 (#2034)。
	keyAddedServices   = "addedServices"
	keyRemovedServices = "removedServices"
)

// keyDestination と keyEpisodes は、見たアニメの記録の通知の項目 (#927)。
const (
	keyDestination = "destination"
	keyEpisodes    = "episodes"
	annict         = "annict"
	anilist        = "anilist"
	simkl          = "simkl"
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
		{
			name: "スクランブルされたまま録れた録画の完了", kind: domain.NotificationKindRecordingCompleted, location: tokyo,
			payload: map[string]any{keyName: programName, "scrambleWarning": true, "scrambledRatio": 0.99},
			want:    "「こたつ特集」の録画が終わりましたが、映像と音声がスクランブルされたままです",
		},
		{
			name: "スクランブルされたまま録れた録画が続いた", kind: domain.NotificationKindScrambledRecordings, location: tokyo,
			payload: map[string]any{keyName: programName, "consecutive": 2},
			want:    "スクランブルされたまま録れた録画が続いています。チューナーサーバーの B-CAS カードが読めていない可能性があります",
		},
		{
			name: "局の顔ぶれの変化は、数が 0 の項目を書かない", kind: domain.NotificationKindLineupChanged, location: tokyo,
			payload: map[string]any{keyLineupKind: "BS", keyNetworkID: 4, keyAdded: 3, keyRemoved: 0},
			want:    "BS のチャンネルが放送で変わりました（新しい局 3）。チューナーサーバーのチャンネルをスキャンし直してください",
		},
		{
			name: "局の並びがあれば、衛星の新しい局を中継器とサービスで書く", kind: domain.NotificationKindLineupChanged, location: tokyo,
			payload: map[string]any{
				keyLineupKind: "BS", keyNetworkID: 4, keyAdded: 1, keyRemoved: 0,
				keyAddedServices: []any{"BS15 のサービス 103"}, keyRemovedServices: []any{},
			},
			want: "BS のチャンネルが放送で変わりました（新しい局: BS15 のサービス 103）。チューナーサーバーのチャンネルをスキャンし直してください",
		},
		{
			name: "新しい局と終わった局を並べるときは、項目を／でつなぎ、名前を半角にする", kind: domain.NotificationKindLineupChanged, location: tokyo,
			payload: map[string]any{
				keyLineupKind: "CS", keyNetworkID: 6, keyAdded: 2, keyRemoved: 1,
				keyAddedServices:   []string{"CS2 のサービス 55", "CS4 のサービス 56"},
				keyRemovedServices: []string{"ＣＳこたつ（サービス 57）"},
			},
			want: "CS のチャンネルが放送で変わりました（新しい局: CS2 のサービス 55、CS4 のサービス 56／終わった局: CSこたつ（サービス 57））。" +
				"チューナーサーバーのチャンネルをスキャンし直してください",
		},
		{
			name: "並べる局が多ければ、残りの数をまとめる", kind: domain.NotificationKindLineupChanged, location: tokyo,
			payload: map[string]any{
				keyLineupKind: "BS", keyNetworkID: 4, keyAdded: 7, keyRemoved: 0,
				keyAddedServices: []string{"サービス 1", "サービス 2", "サービス 3", "サービス 4", "サービス 5", "サービス 6", "サービス 7"},
			},
			want: "BS のチャンネルが放送で変わりました（新しい局: サービス 1、サービス 2、サービス 3、サービス 4、サービス 5、ほか 2 局）。" +
				"チューナーサーバーのチャンネルをスキャンし直してください",
		},
		{
			name: "地上波は、新しい局を SDT の名前で書く", kind: domain.NotificationKindLineupChanged, location: tokyo,
			payload: map[string]any{
				keyLineupKind: "GR", "channel": "27", keyNetworkID: 32736, keyAdded: 1, keyRemoved: 0,
				keyAddedServices: []any{"こたつテレビ（サービス 1032）"},
			},
			want: "地上波の 27ch で受かる局が、チューナーサーバーの登録と食い違っています（新しい局: こたつテレビ（サービス 1032））。" +
				"再スキャンすれば、新しい局を選べるようになります",
		},
		{
			name: "地上波は、チャンネルで受かる局の食い違いを書く", kind: domain.NotificationKindLineupChanged, location: tokyo,
			payload: map[string]any{keyLineupKind: "GR", "channel": "27", keyNetworkID: 32736, keyAdded: 1, keyRemoved: 0},
			want:    "地上波の 27ch で受かる局が、チューナーサーバーの登録と食い違っています（新しい局 1）。再スキャンすれば、新しい局を選べるようになります",
		},
		{
			name: "地上波の続けて選局できないチャンネルは、チャンネルを並べる", kind: domain.NotificationKindLineupChanged, location: tokyo,
			payload: map[string]any{keyLineupKind: "GR", "unreachable": []string{"22", "25"}},
			want:    "地上波の 22ch・25ch の選局が続けて失敗しています。周波数が変わったのなら、チューナーサーバーで地上波を再スキャンすれば選局できるようになります",
		},
		{
			name: "DB から読み直した衛星の続けて選局できないチャンネルは、配列が []any で届く", kind: domain.NotificationKindLineupChanged, location: tokyo,
			payload: map[string]any{keyLineupKind: "BS", "unreachable": []any{"BS15_0"}},
			want:    "BS の BS15_0 の選局が続けて失敗しています。放送で変わったのなら、チューナーサーバーのチャンネルをスキャンし直せば選局できるようになります",
		},
		{
			name: "DB から読み直した局の顔ぶれの変化は、数が float64 で届く", kind: domain.NotificationKindLineupChanged, location: tokyo,
			payload: map[string]any{keyLineupKind: "CS", keyNetworkID: float64(6), keyAdded: float64(1), keyRemoved: float64(2)},
			want:    "CS のチャンネルが放送で変わりました（新しい局 1、終わった局 2）。チューナーサーバーのチャンネルをスキャンし直してください",
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
			name: "見たアニメの記録は、作品名と話数と記録先を書く", kind: domain.NotificationKindAnimeRecorded, location: tokyo,
			payload: map[string]any{keyName: "作品名　＃６", keyDestination: anilist, keyEpisodes: "#6"},
			want:    "「作品名」の#6をAniListに記録しました",
		},
		// 番組名の話数と、記録した話数を重ねない (#1925)。
		{
			name: "番組名の話数は、作品名にして除く", kind: domain.NotificationKindAnimeRecorded, location: tokyo,
			payload: map[string]any{
				keyName: "凶乱令嬢ニア・リストン 病弱令嬢に転生した神殺しの武人の華麗なる無双録 #6", keyDestination: anilist, keyEpisodes: "#6",
			},
			want: "「凶乱令嬢ニア・リストン 病弱令嬢に転生した神殺しの武人の華麗なる無双録」の#6をAniListに記録しました",
		},
		{
			name: "かぎ括弧の作品名と、サブタイトルと印を除く", kind: domain.NotificationKindAnimeRecorded, location: tokyo,
			payload: map[string]any{keyName: "🈑TVアニメ「作品名」第3話「サブタイトル」", keyDestination: annict, keyEpisodes: "#3、#4"},
			want:    "「作品名」の#3、#4をAnnictに記録しました",
		},
		// 空白で区切った数の話数とサブタイトルも、作品名にして除く (#1929)。
		{
			name: "空白で区切った数の話数とサブタイトルを除く", kind: domain.NotificationKindAnimeRecorded, location: tokyo,
			payload: map[string]any{keyName: "アニメ　天は赤い河のほとり　２０　亀裂", keyDestination: annict, keyEpisodes: "#20"},
			want:    "「天は赤い河のほとり」の#20をAnnictに記録しました",
		},
		{
			name: "話数のない番組名には、話数を足す", kind: domain.NotificationKindAnimeRecorded, location: tokyo,
			payload: map[string]any{keyName: "作品名", keyDestination: annict, keyEpisodes: "#12"},
			want:    "「作品名」の#12をAnnictに記録しました",
		},
		{
			name: "作品名に話数が残ったら、話数を重ねない", kind: domain.NotificationKindAnimeRecorded, location: tokyo,
			payload: map[string]any{keyName: "アニメ　＃０３", keyDestination: annict, keyEpisodes: "#3"},
			want:    "「アニメ #03」をAnnictに記録しました",
		},
		{
			name: "作品名に残った話数が、記録した話の一部なら、話数を書く", kind: domain.NotificationKindAnimeRecorded, location: tokyo,
			payload: map[string]any{keyName: "アニメ　＃３", keyDestination: annict, keyEpisodes: "#3、#4"},
			want:    "「アニメ #3」の#3、#4をAnnictに記録しました",
		},
		{
			name: "作品名に残った数が、違う話数なら、話数を書く", kind: domain.NotificationKindAnimeRecorded, location: tokyo,
			payload: map[string]any{keyName: "アニメ　＃１２", keyDestination: annict, keyEpisodes: "#1"},
			want:    "「アニメ #12」の#1をAnnictに記録しました",
		},
		{
			name: "作品名が読めない番組名は、番組名のまま書く", kind: domain.NotificationKindAnimeRecorded, location: tokyo,
			payload: map[string]any{keyName: "🈑", keyDestination: annict, keyEpisodes: "#3"},
			want:    "「🈑」の#3をAnnictに記録しました",
		},
		{
			name: "番組名がなければ、見たアニメとする", kind: domain.NotificationKindAnimeRecorded, location: tokyo,
			payload: map[string]any{keyDestination: annict, keyEpisodes: "#3"},
			want:    "見たアニメの#3をAnnictに記録しました",
		},
		{
			name: "記録できなかったときは、理由を添える", kind: domain.NotificationKindAnimeRecordFailed, location: tokyo,
			payload: map[string]any{keyName: programName, keyDestination: anilist, keyEpisodes: "#3", "reason": "連携していません"},
			want:    "「こたつ特集」の#3をAniListに記録できませんでした（連携していません）",
		},
		{
			name: "見たドラマの記録は、番組名から読んだ作品名とシーズンと話を書く", kind: domain.NotificationKindDramaRecorded, location: tokyo,
			payload: map[string]any{keyName: "土曜ドラマ「こたつの家」　＃３「冬支度」", keyDestination: simkl, keyEpisodes: "シーズン1 第3話"},
			want:    "「こたつの家」のシーズン1 第3話をSimklに記録しました",
		},
		{
			name: "見たドラマを記録できなかったときは、理由を添える", kind: domain.NotificationKindDramaRecordFailed, location: tokyo,
			payload: map[string]any{keyName: "こたつの家 #3", keyDestination: simkl, keyEpisodes: "特別編 第1話", "reason": "Simkl に話が見つかりません"},
			want:    "「こたつの家」の特別編 第1話をSimklに記録できませんでした（Simkl に話が見つかりません）",
		},
		{
			name: "材料のないドラマの記録", kind: domain.NotificationKindDramaRecorded, location: tokyo,
			payload: map[string]any{}, want: "見たドラマを記録先に記録しました",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, notification.Message(tt.kind, tt.payload, tt.location))
		})
	}
}

// TestMessageEscaped は、番組名、放送されていた番組名、サービス名を、escape で書き換えることを確認する (#1010)。
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
		{
			name: "見たアニメの記録は、作品名にしてから書き換える", kind: domain.NotificationKindAnimeRecorded,
			payload: map[string]any{keyName: "作品名 #6", keyDestination: annict, keyEpisodes: "#6"},
			want:    "「<作品名>」の#6をAnnictに記録しました",
		},
		{
			name: "局の顔ぶれの変化の局の名前", kind: domain.NotificationKindLineupChanged,
			payload: map[string]any{
				keyLineupKind: "BS", keyNetworkID: 4, keyAdded: 1, keyRemoved: 1,
				keyAddedServices: []any{"BS15 のサービス 103"}, keyRemovedServices: []any{"*こたつ*（サービス 191）"},
			},
			want: "<BS> のチャンネルが放送で変わりました（新しい局: <BS15 のサービス 103>／終わった局: <*こたつ*（サービス 191）>）。" +
				"チューナーサーバーのチャンネルをスキャンし直してください",
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
