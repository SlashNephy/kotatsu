package nextepisode_test

import (
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/nextepisode"
)

// addUser は、ユーザーを足し、その ID を返す。
func (f *findFixture) addUser(t *testing.T, name string) uuid.UUID {
	t.Helper()

	userID := id.New()
	f.exec(t, `INSERT INTO users (id, username, display_name, role, created_at, updated_at) VALUES ($1, $2, $2, 'member', $3, $3)`,
		userID, name, findNow())

	return userID
}

// addWatched は、3 日前に地上波で録った 30 分の録画 name を足し、ユーザー userID が、録画の長さの ratio まで、
// findNow の ago 前に再生したことにする。録画の ID を返す。
func (f *findFixture) addWatched(t *testing.T, userID uuid.UUID, name string, ratio float64, ago time.Duration) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	start := findNow().Add(-72 * time.Hour)
	f.exec(t, `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, recorded_until,
		created_at, updated_at)
		VALUES ($1, $2, $3, '', $4, $5, 'completed', $4, $5, $4, $4)`,
		recordingID, faketuner.ServiceItemID(fakeGRNetwork, fakeGRService), name, start, start.Add(30*time.Minute))
	f.exec(t, `INSERT INTO watch_progress (user_id, recording_id, position_ms, updated_at) VALUES ($1, $2, $3, $4)`,
		userID, recordingID, int64(ratio*float64((30*time.Minute).Milliseconds())), findNow().Add(-ago))

	return recordingID
}

// suggestion は、提案の見た録画と、続きの放送の eventId。
type suggestion struct {
	recording uuid.UUID
	eventID   int64
}

func suggestionsOf(got []*nextepisode.Suggestion) []suggestion {
	out := make([]suggestion, 0, len(got))
	for _, s := range got {
		out = append(out, suggestion{recording: s.RecordingID, eventID: s.Broadcast.EventID})
	}

	return out
}

// TestFindForUser は、ユーザーが見たシリーズごとに、見た回の最大の話数より後の最も早い放送を 1 件ずつ、放送の早い順に返し、
// 見た割合の足りない回、期間より前に見た回、話数を読めない回、録画中の回、ほかのユーザーが見た回を、見た回に数えないことを確認する (#1564)。
func TestFindForUser(t *testing.T) {
	t.Parallel()

	f := newFindFixture(t, []*faketuner.Program{
		fakeProgram(fakeGRNetwork, fakeGRService, 500, 2*time.Hour, "霧笛の港 #4"),
		fakeProgram(fakeBSNetwork, fakeBSService, 501, 3*time.Hour, "霧笛の港 #5"),
		// 見た回 (#3) より前の回は出さない。
		fakeProgram(fakeGRNetwork, fakeGRService, 502, 30*time.Minute, "霧笛の港 #3"),
		fakeProgram(fakeGRNetwork, fakeGRService, 510, time.Hour, "灯台の地図 #3"),
		fakeProgram(fakeGRNetwork, fakeGRService, 520, 4*time.Hour, "銀の鈴 #2"),
		fakeProgram(fakeGRNetwork, fakeGRService, 530, 5*time.Hour, "星の駅 #8"),
		// 予約のある #2 と同じ話数の、ほかの局の放送は出さず、次の話数を出す。
		fakeProgram(fakeGRNetwork, fakeGRService, 541, 90*time.Minute, "風の丘 #2"),
		fakeProgram(fakeBSNetwork, fakeBSService, 540, 6*time.Hour, "風の丘 #2"),
		fakeProgram(fakeGRNetwork, fakeGRService, 542, 7*time.Hour, "風の丘 #3"),
		fakeProgram(fakeGRNetwork, fakeGRService, 550, 8*time.Hour, "ニュース7"),
		fakeProgram(fakeGRNetwork, fakeGRService, 560, 9*time.Hour, "雪の町 #2"),
	})
	f.reserve(t, fakeGRNetwork, fakeGRService, 541, true)

	user := f.addUser(t, "user")
	f.addWatched(t, user, "霧笛の港 #2", 1, time.Hour)
	harbor3 := f.addWatched(t, user, "霧笛の港 #3", 0.6, 2*time.Hour)
	map2 := f.addWatched(t, user, "灯台の地図 #2", nextepisode.WatchedRatio, time.Hour)
	f.addWatched(t, user, "銀の鈴 #1", 0.3, time.Hour)
	f.addWatched(t, user, "星の駅 #7", 1, nextepisode.WatchedWindow+time.Hour)
	hill1 := f.addWatched(t, user, "風の丘 #1", 1, 3*time.Hour)
	f.addWatched(t, user, "ニュース7", 1, time.Hour)
	// 録画中の録画は、長さが決まらず、見た割合を出せないので数えない。
	recording := f.addWatched(t, user, "雪の町 #1", 1, time.Hour)
	f.exec(t, `UPDATE recordings SET state = 'recording', recorded_until = NULL WHERE id = $1`, recording)
	other := f.addUser(t, "other")
	bell1 := f.addWatched(t, other, "銀の鈴 #1", 1, time.Hour)
	nobody := f.addUser(t, "nobody")

	tests := []struct {
		name string
		user uuid.UUID
		want []suggestion
	}{
		{
			name: "見たシリーズごとに、続きの最も早い放送を、放送の早い順に返す",
			user: user,
			want: []suggestion{{recording: map2, eventID: 510}, {recording: harbor3, eventID: 500}, {recording: hill1, eventID: 542}},
		},
		{name: "ほかのユーザーの見た回は数えない", user: other, want: []suggestion{{recording: bell1, eventID: 520}}},
		{name: "見た回がなければ空", user: nobody, want: []suggestion{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := nextepisode.FindForUser(t.Context(), f.pool, tt.user, findNow())
			require.NoError(t, err)
			assert.Equal(t, tt.want, suggestionsOf(got))
		})
	}
}

// TestFindForUserByAnime は、しょぼいカレンダーの話に結びついた見た回では TID と話数で探し、同じ番組がシリーズ名で探した
// 別のシリーズにも当たったら、1 行にすることを確認する (#1564)。
func TestFindForUserByAnime(t *testing.T) {
	t.Parallel()

	f := newFindFixture(t, []*faketuner.Program{
		fakeProgram(fakeGRNetwork, fakeGRService, 600, 2*time.Hour, "灯台の地図 #2"),
		// 録った話数は外す。
		fakeProgram(fakeGRNetwork, fakeGRService, 601, time.Hour, "灯台の地図 #3"),
		// 同じ作品の後の回は、シリーズ名で探したシリーズからも出さない。
		fakeProgram(fakeGRNetwork, fakeGRService, 602, 3*time.Hour, "灯台の地図 #4"),
	})
	serviceID := faketuner.ServiceItemID(fakeGRNetwork, fakeGRService)
	for eventID, count := range map[int]float64{600: 2, 601: 3, 602: 4} {
		f.exec(t, `INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at)
			SELECT service_id, start_at, event_id, $3 FROM programs WHERE service_id = $1 AND event_id = $2`, serviceID, eventID, findNow())
		f.exec(t, `INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end, is_first, is_last)
			SELECT service_id, start_at, 0, 'syobocal', $3, $4, $4, false, false FROM programs WHERE service_id = $1 AND event_id = $2`,
			serviceID, eventID, animeTID, count)
	}
	f.addHistory(t, "灯台の地図 #3", "灯台の地図", new(int32(animeTID)), new(float64(3)))

	user := f.addUser(t, "user")
	anime := f.addWatched(t, user, "灯台の地図 #1", 1, 2*time.Hour)
	f.exec(t, `INSERT INTO recording_anime_matches (recording_id, matched_at) VALUES ($1, $2)`, anime, findNow())
	f.exec(t, `INSERT INTO recording_anime_episodes (recording_id, position, source, syobocal_tid, count_start, count_end, is_first, is_last)
		VALUES ($1, 0, 'syobocal', $2, 1, 1, false, false)`, anime, animeTID)
	// 結びつきのない録画は、シリーズ名で探す。#3 は録画履歴のシリーズ名でも録った回なので、#2 に当たる。
	f.addWatched(t, user, "灯台の地図 #1", 1, time.Hour)

	got, err := nextepisode.FindForUser(t.Context(), f.pool, user, findNow())
	require.NoError(t, err)

	assert.Equal(t, []suggestion{{recording: anime, eventID: 600}}, suggestionsOf(got))
}

// TestFindForUserByDrama は、番組名から話数を読めないドラマの見た回では、TMDB の作品とシーズンと話の番号で探し、
// 見た話の最大の番号より後の、録っていない話の放送を出すことと、同じ番組がシリーズ名で探したシリーズにも当たったら、
// シリーズ名で探した方の 1 行にすることを確認する (#1564)。
func TestFindForUserByDrama(t *testing.T) {
	t.Parallel()

	f := newFindFixture(t, []*faketuner.Program{
		fakeProgram(fakeGRNetwork, fakeGRService, 800, time.Hour, "夜明けの診療所"),
		// 録った話 (第 5 話) は外す。
		fakeProgram(fakeGRNetwork, fakeGRService, 801, 2*time.Hour, "夜明けの診療所"),
		fakeProgram(fakeGRNetwork, fakeGRService, 802, 4*time.Hour, "夜明けの診療所"),
		fakeProgram(fakeGRNetwork, fakeGRService, 810, 3*time.Hour, "灯台の町 #5"),
	})
	f.matchDrama(t, 800, dramaSeries, 1, 4)
	f.matchDrama(t, 801, dramaSeries, 1, 5)
	f.matchDrama(t, 802, dramaSeries, 1, 6)
	f.matchDrama(t, 810, otherDramaSeries, 1, 5)
	f.exec(t, `INSERT INTO recording_history (id, service_id, title_key, name, tmdb_series_id, tmdb_season_number, tmdb_episode_number,
		start_at, end_at, created_at) VALUES ($1, $2, '夜明けの診療所', '夜明けの診療所', $3, 1, 5, $4, $4, $4)`,
		id.New(), faketuner.ServiceItemID(fakeGRNetwork, fakeGRService), dramaSeries, findNow().Add(-24*time.Hour))

	user := f.addUser(t, "user")
	f.linkDrama(t, f.addWatched(t, user, "夜明けの診療所", 1, 2*time.Hour), dramaSeries, 3)
	clinic4 := f.addWatched(t, user, "夜明けの診療所", 1, time.Hour)
	f.linkDrama(t, clinic4, dramaSeries, 4)
	// 同じ作品を、番組名の話数を読める録画 (シリーズ名で探す) と読めない録画 (TMDB の作品で探す) で見た。
	// TMDB の作品で探す録画を後に見ても、シリーズ名で探した方を残す。
	town4 := f.addWatched(t, user, "灯台の町 #4", 1, 2*time.Hour)
	f.linkDrama(t, f.addWatched(t, user, "灯台の町", 1, time.Hour), otherDramaSeries, 4)

	got, err := nextepisode.FindForUser(t.Context(), f.pool, user, findNow())
	require.NoError(t, err)

	assert.Equal(t, []suggestion{{recording: town4, eventID: 810}, {recording: clinic4, eventID: 802}}, suggestionsOf(got))
}

// TestFindForUserByDramaNextSeason は、ホームの続きの放送でも、TMDB の作品で探すシリーズの今のシーズンの後の話が
// これから放送されないときだけ、次のシーズンの第 1 話を出すことを確認する (#1564)。
// 見た回は、作品 dramaSeries のシーズン 1 の第 10 話と、作品 otherDramaSeries のシーズン 1 の第 4 話。
func TestFindForUserByDramaNextSeason(t *testing.T) {
	t.Parallel()

	type broadcast struct {
		eventID  int
		offset   time.Duration
		seriesID int32
		season   int32
		episode  int32
	}
	tests := []struct {
		name          string
		broadcasts    []broadcast
		recordedFirst bool
		// want は、作品 dramaSeries と otherDramaSeries の見た回の順の、続きの放送の eventId。0 なら、その作品の行を出さない。
		want [2]int64
	}{
		{
			name: "今のシーズンの後の話がない作品だけ、次のシーズンの第 1 話を出す",
			broadcasts: []broadcast{
				{800, 2 * time.Hour, dramaSeries, 2, 2},
				{801, 3 * time.Hour, dramaSeries, 2, 1},
				{810, time.Hour, otherDramaSeries, 2, 1},
				{811, 4 * time.Hour, otherDramaSeries, 1, 5},
			},
			recordedFirst: false,
			want:          [2]int64{801, 811},
		},
		{
			name:          "次のシーズンの第 1 話を録っていれば出さない",
			broadcasts:    []broadcast{{801, 3 * time.Hour, dramaSeries, 2, 1}, {811, 4 * time.Hour, otherDramaSeries, 1, 5}},
			recordedFirst: true,
			want:          [2]int64{0, 811},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			programs := make([]*faketuner.Program, 0, len(tt.broadcasts))
			for _, b := range tt.broadcasts {
				programs = append(programs, fakeProgram(fakeGRNetwork, fakeGRService, b.eventID, b.offset, "夜明けの診療所"))
			}
			f := newFindFixture(t, programs)
			for _, b := range tt.broadcasts {
				f.matchDrama(t, b.eventID, b.seriesID, b.season, b.episode)
			}
			if tt.recordedFirst {
				f.addDramaHistory(t, 2, 1)
			}
			user := f.addUser(t, "user")
			watched := [2]uuid.UUID{
				f.addWatched(t, user, "夜明けの診療所", 1, time.Hour), f.addWatched(t, user, "灯台の町", 1, time.Hour),
			}
			f.linkDrama(t, watched[0], dramaSeries, 10)
			f.linkDrama(t, watched[1], otherDramaSeries, 4)

			got, err := nextepisode.FindForUser(t.Context(), f.pool, user, findNow())
			require.NoError(t, err)

			want := []suggestion{}
			for i, eventID := range tt.want {
				if eventID != 0 {
					want = append(want, suggestion{recording: watched[i], eventID: eventID})
				}
			}
			assert.Equal(t, want, suggestionsOf(got))
		})
	}
}

// TestFindForUserMergesSubtitledSeries は、番組名にサブタイトルが入ってシリーズ名が回ごとに変わる作品でも、サブタイトルのない回があれば
// 1 つのシリーズにまとめ、見た回の再放送を出さず、次の回を出すことを確認する (#2023)。
func TestFindForUserMergesSubtitledSeries(t *testing.T) {
	t.Parallel()

	programs := []*faketuner.Program{
		// 見た第 5 話の再放送は、サブタイトルのない番組名で、第 6 話より先に放送される。
		fakeProgram(fakeGRNetwork, fakeGRService, 600, time.Hour, "港町の灯　＃０５🈞"),
		fakeProgram(fakeGRNetwork, fakeGRService, 601, 5*time.Hour, "港町の灯　六つ目の嵐　＃０６🈑🈓"),
		// シリーズ名が作品名で始まるだけの別の番組は、作品名を読み直して外す。
		fakeProgram(fakeGRNetwork, fakeGRService, 602, 90*time.Minute, "港町の灯台　＃０９"),
		// 枠の名前の後ろに作品名を書く番組は、サブタイトルのない回がなければまとめない。見た「霧の駅」の続きに、別の作品を出さない。
		fakeProgram(fakeGRNetwork, fakeGRService, 611, 3*time.Hour, "連続劇場　雪の峠　＃５"),
		// 作品名の後ろの語が期 (数字) なら、サブタイトルのない回があってもまとめない。
		fakeProgram(fakeGRNetwork, fakeGRService, 620, 4*time.Hour, "灯台守　＃３"),
		fakeProgram(fakeGRNetwork, fakeGRService, 621, 6*time.Hour, "灯台守　４　＃１６"),
	}
	watch := func(t *testing.T, f *findFixture, user uuid.UUID) uuid.UUID {
		t.Helper()

		f.addWatched(t, user, "港町の灯　＃０１🈑🈓", 1, 5*time.Hour)
		f.addWatched(t, user, "港町の灯　二つの影　＃０２🈑🈓", 1, 4*time.Hour)
		f.addWatched(t, user, "港町の灯　三つの鐘　＃０３🈑🈓", 1, 3*time.Hour)
		f.addWatched(t, user, "港町の灯　四つの窓　＃０４🈑🈓", 1, 2*time.Hour)
		ep5 := f.addWatched(t, user, "港町の灯　五つ目の約束　＃０５🈑🈓", 1, time.Hour)
		f.addWatched(t, user, "連続劇場　霧の駅　＃３", 1, time.Hour)
		f.addWatched(t, user, "灯台守　＃１４", 1, time.Hour)

		return ep5
	}

	t.Run("見た回の再放送を出さず、副題のある次の回を出す", func(t *testing.T) {
		t.Parallel()

		f := newFindFixture(t, programs)
		user := f.addUser(t, "user")
		ep5 := watch(t, f, user)

		got, err := nextepisode.FindForUser(t.Context(), f.pool, user, findNow())
		require.NoError(t, err)
		assert.Equal(t, []suggestion{{recording: ep5, eventID: 601}}, suggestionsOf(got))
	})
	t.Run("次の回に予約があれば出さない", func(t *testing.T) {
		t.Parallel()

		f := newFindFixture(t, programs)
		f.reserve(t, fakeGRNetwork, fakeGRService, 601, true)
		user := f.addUser(t, "user")
		watch(t, f, user)

		got, err := nextepisode.FindForUser(t.Context(), f.pool, user, findNow())
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("副題のある回だけを見ても、番組表に副題のない回があればまとめる", func(t *testing.T) {
		t.Parallel()

		f := newFindFixture(t, programs)
		user := f.addUser(t, "user")
		f.addWatched(t, user, "港町の灯　三つの鐘　＃０３🈑🈓", 1, 2*time.Hour)
		ep5 := f.addWatched(t, user, "港町の灯　五つ目の約束　＃０５🈑🈓", 1, time.Hour)

		got, err := nextepisode.FindForUser(t.Context(), f.pool, user, findNow())
		require.NoError(t, err)
		assert.Equal(t, []suggestion{{recording: ep5, eventID: 601}}, suggestionsOf(got))
	})
	t.Run("副題に空白が入っても、副題のない回のシリーズまでたどってまとめる", func(t *testing.T) {
		t.Parallel()

		f := newFindFixture(t, programs)
		user := f.addUser(t, "user")
		f.addWatched(t, user, "港町の灯　＃０１🈑🈓", 1, 3*time.Hour)
		f.addWatched(t, user, "港町の灯　嵐の夜　＃０４🈑🈓", 1, 2*time.Hour)
		ep5 := f.addWatched(t, user, "港町の灯　嵐の夜　その二　＃０５🈑🈓", 1, time.Hour)

		got, err := nextepisode.FindForUser(t.Context(), f.pool, user, findNow())
		require.NoError(t, err)
		assert.Equal(t, []suggestion{{recording: ep5, eventID: 601}}, suggestionsOf(got))
	})
	t.Run("副題のある回を録画済みなら出さない", func(t *testing.T) {
		t.Parallel()

		f := newFindFixture(t, programs)
		f.addHistory(t, "港町の灯　六つ目の嵐　＃０６🈑🈓", "港町の灯六つ目の嵐", nil, nil)
		user := f.addUser(t, "user")
		watch(t, f, user)

		got, err := nextepisode.FindForUser(t.Context(), f.pool, user, findNow())
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}
