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
// 見た割合の足りない回、期間より前に見た回、話数を読めない回、録画中の回、ほかのユーザーが見た回を、見た回に数えないことを確かめる (#1564)。
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
// 別のシリーズにも当たったら、1 行にすることを確かめる (#1564)。
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
