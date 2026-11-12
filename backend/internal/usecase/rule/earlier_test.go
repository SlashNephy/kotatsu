package rule_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

// seriesName は、前の回を録らないテストのシリーズ名。
const seriesName = "霧笛の港"

// grEpisode と bsEpisode は、地上波 (こたつテスト1) と BS (こたつ BS) で、testNow から offset 後に始まる 1 時間の番組を返す。
func grEpisode(eventID int, offset time.Duration, name string) *faketuner.Program {
	return episodeProgram(grNetworkID, grServiceID, eventID, offset, name)
}

func bsEpisode(eventID int, offset time.Duration, name string) *faketuner.Program {
	return episodeProgram(bsNetworkID, bsServiceID, eventID, offset, name)
}

// createSeriesRule は、地上波と BS の「霧笛の港」を予約するルールを作る。
func (f *engineFixture) createSeriesRule(tb testing.TB, skip, bundle bool) *rule.Rule {
	tb.Helper()

	return f.createRule(tb, seriesName, func(in *rule.Input) {
		in.Conditions.ServiceIDs = []int64{serviceID(), bsServiceItemID()}
		in.SkipEarlierEpisodes = skip
		in.BundleSameEpisodes = bundle
	})
}

// addHistory は、名前 name の録画を、testNow から started 後に始まった録画として、録画履歴に入れる。
func (f *engineFixture) addHistory(tb testing.TB, name string, started time.Duration) {
	tb.Helper()

	start := testNow().Add(started)
	_, err := f.pool.Exec(tb.Context(),
		`INSERT INTO recording_history (id, service_id, title_key, name, series, start_at, end_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`,
		id.New(), faketuner.ServiceItemID(grNetworkID, subServiceID), rule.TitleKey(name), name, episode.SeriesOf(name), start, start.Add(time.Hour))
	require.NoError(tb, err)
}

// reservedSeries は、予約した「霧笛の港」の番組の eventId と、予約が有効かどうかを返す。
func (f *engineFixture) reservedSeries(t *testing.T) map[int64]bool {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT event_id, enabled FROM reservations WHERE name LIKE $1", seriesName+"%")
	require.NoError(t, err)
	defer rows.Close()
	reserved := map[int64]bool{}
	for rows.Next() {
		var (
			eventID int64
			enabled bool
		)
		require.NoError(t, rows.Scan(&eventID, &enabled))
		reserved[eventID] = enabled
	}
	require.NoError(t, rows.Err())

	return reserved
}

// TestEngineSkipsEarlierEpisodes は、前の回を録らないルールが、録った回、予約してある回、先に流れる放送より話数の小さい回を、
// その放送より前に始まる相手とだけ比べて外すことを確かめる (#1499)。
func TestEngineSkipsEarlierEpisodes(t *testing.T) {
	t.Parallel()

	const week = 7 * 24 * time.Hour
	tests := []struct {
		name     string
		programs []*faketuner.Program
		// history は、録画履歴に入れる録画の名前と、始まった testNow からの時間。
		history map[string]time.Duration
		skip    bool
		// otherRule は、前の回を録らない設定を入れていないルールにも、一致させるかどうか。
		otherRule bool
		want      map[int64]bool
	}{
		{
			name: "一挙放送の 1〜3 話は、録画済みの 4 話の前の回なので外す",
			programs: []*faketuner.Program{
				grEpisode(501, 2*time.Hour, seriesName+" #1"), grEpisode(502, 3*time.Hour, seriesName+" #2"),
				grEpisode(503, 4*time.Hour, seriesName+" #3"),
			},
			// 期間で切らないので、1 年より前の録画も数える。
			history: map[string]time.Duration{seriesName + " #4": -400 * 24 * time.Hour},
			skip:    true,
			want:    map[int64]bool{},
		},
		{
			name: "設定を入れていないルールは、前の回も予約する",
			programs: []*faketuner.Program{
				grEpisode(501, 2*time.Hour, seriesName+" #1"), grEpisode(502, 3*time.Hour, seriesName+" #2"),
			},
			history: map[string]time.Duration{seriesName + " #4": -week},
			skip:    false,
			want:    map[int64]bool{501: true, 502: true},
		},
		{
			name: "設定を入れていない別のルールも一致すれば、そのルールが予約する",
			programs: []*faketuner.Program{
				grEpisode(501, 2*time.Hour, seriesName+" #1"),
			},
			history:   map[string]time.Duration{seriesName + " #4": -week},
			skip:      true,
			otherRule: true,
			want:      map[int64]bool{501: true},
		},
		{
			name: "これからの 4 話より前に流れる一挙放送の 1〜3 話は外さない",
			programs: []*faketuner.Program{
				grEpisode(501, 2*time.Hour, seriesName+" #1"), grEpisode(502, 3*time.Hour, seriesName+" #2"),
				grEpisode(503, 4*time.Hour, seriesName+" #3"), grEpisode(504, 26*time.Hour, seriesName+" #4"),
			},
			skip: true,
			want: map[int64]bool{501: true, 502: true, 503: true, 504: true},
		},
		{
			name: "来週の 6 話があっても、今週の 5 話は外さない",
			programs: []*faketuner.Program{
				grEpisode(505, 5*time.Hour, seriesName+" #5"), grEpisode(506, 5*time.Hour+week, seriesName+" #6"),
			},
			skip: true,
			want: map[int64]bool{505: true, 506: true},
		},
		{
			name: "6 話の後に別の局で流れる 5 話は外す",
			programs: []*faketuner.Program{
				grEpisode(506, 5*time.Hour, seriesName+" #6"), bsEpisode(605, 29*time.Hour, seriesName+" #5"),
			},
			skip: true,
			want: map[int64]bool{506: true},
		},
		{
			name: "期の違うシリーズ名は外さない",
			programs: []*faketuner.Program{
				grEpisode(503, 2*time.Hour, seriesName+" 第2期 #3"),
			},
			history: map[string]time.Duration{seriesName + " #12": -90 * 24 * time.Hour},
			skip:    true,
			want:    map[int64]bool{503: true},
		},
		{
			name: "話数の読めない放送は外さない",
			programs: []*faketuner.Program{
				grEpisode(500, 2*time.Hour, seriesName+" 特別編"),
			},
			history: map[string]time.Duration{seriesName + " #4": -week},
			skip:    true,
			want:    map[int64]bool{500: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newEngineFixture(t, tt.programs...)
			for name, started := range tt.history {
				f.addHistory(t, name, started)
			}
			f.createSeriesRule(t, tt.skip, false)
			if tt.otherRule {
				f.createSeriesRule(t, false, false)
			}
			f.run(t)

			assert.Equal(t, tt.want, f.reservedSeries(t))
		})
	}
}

// TestEngineIgnoresRejectedLaterEpisodes は、競合の 6 話と、録画しない (無効にした) 6 話を、別の局の 5 話を外す相手にしないことを
// 確かめる (#1499)。録れない見込みの 6 話を理由に、5 話を落とさないためである。
func TestEngineIgnoresRejectedLaterEpisodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reject func(t *testing.T, f *engineFixture)
		want   map[int64]bool
	}{
		{
			name: "競合の 6 話",
			reject: func(t *testing.T, f *engineFixture) {
				t.Helper()

				_, err := f.pool.Exec(t.Context(), "UPDATE reservations SET assignment_state = 'conflict' WHERE event_id = 506")
				require.NoError(t, err)
			},
			want: map[int64]bool{506: true, 605: true},
		},
		{
			name: "録画しない 6 話",
			reject: func(t *testing.T, f *engineFixture) {
				t.Helper()

				require.NoError(t, f.reservations.Delete(t.Context(), f.pool, f.user, f.reservationOf(t, 506).ID))
			},
			want: map[int64]bool{506: false, 605: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newEngineFixture(t, grEpisode(506, 5*time.Hour, seriesName+" #6"), bsEpisode(605, 29*time.Hour, seriesName+" #5"))
			f.createSeriesRule(t, true, false)
			f.run(t)
			require.Equal(t, map[int64]bool{506: true}, f.reservedSeries(t))

			tt.reject(t, f)
			f.run(t)

			assert.Equal(t, tt.want, f.reservedSeries(t))
		})
	}
}

// TestEngineRemovesEarlierEpisodeReservations は、予約した後に、より大きい話数が先に流れると分かったときと、より大きい話数を
// 録り終えたときに、前の回の予約を引っ込めることを確かめる (#1499)。番組表の放送だけが理由なら、猶予の内は残し、
// 録画が理由なら、猶予の内でもすぐ消す (#1501)。
func TestEngineRemovesEarlierEpisodeReservations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		grace    int
		recorded bool
		want     map[int64]bool
	}{
		{name: "先に流れる 6 話が入れば、5 話の予約を消す", grace: 0, recorded: false, want: map[int64]bool{506: true}},
		{name: "猶予の内なら、5 話の予約も残す", grace: 24 * 60, recorded: false, want: map[int64]bool{506: true, 605: true}},
		{name: "6 話を録り終えたら、猶予の内でも消す", grace: 24 * 60, recorded: true, want: map[int64]bool{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// 猶予の上限 (1 日) の内に始まるよう、5 話を 20 時間後にする。
			f := newEngineFixture(t, bsEpisode(605, 20*time.Hour, seriesName+" #5"))
			f.setGrace(t, tt.grace)
			f.createSeriesRule(t, true, false)
			f.run(t)
			require.Equal(t, map[int64]bool{605: true}, f.reservedSeries(t))

			if tt.recorded {
				f.addHistory(t, seriesName+" #6", -2*time.Hour)
			} else {
				f.fake.AddProgram(grEpisode(506, 5*time.Hour, seriesName+" #6"))
				_, err := f.reconciler.SyncAll(t.Context())
				require.NoError(t, err)
			}
			f.run(t)

			assert.Equal(t, tt.want, f.reservedSeries(t))
		})
	}
}

// TestFindSkippedEarlierEpisodes は、前の回を録らない設定で予約しなかった番組を、一番大きい話数と一緒に返し、
// 同じ回の束ねにも当たる番組は、束ねた理由を優先して出すことを確かめる (#1499)。
func TestFindSkippedEarlierEpisodes(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	t.Run("前の回", func(t *testing.T) {
		t.Parallel()

		f := newEngineFixture(t, grEpisode(502, 2*time.Hour, seriesName+" #2"))
		f.addHistory(t, seriesName+" #4", -2*24*time.Hour)
		f.addHistory(t, seriesName+" #3", -9*24*time.Hour)
		r := f.createSeriesRule(t, true, false)
		f.run(t)

		skipped, err := rule.FindSkippedDuplicates(t.Context(), f.pool, f.clock, location)
		require.NoError(t, err)
		assert.Equal(t, []*rule.SkippedDuplicate{{
			ServiceID: serviceID(), EventID: 502, Rules: []*rule.SkippedRule{{ID: r.ID, Name: r.Name}},
			Reason: rule.SkipReasonEarlierEpisode, LastRecordedAt: nil, Recorded: nil, Instead: nil, LaterEpisode: new(4),
		}}, skipped)
	})

	t.Run("同じ回の束ねにも当たれば、束ねた理由を出す", func(t *testing.T) {
		t.Parallel()

		// 3 話は、地上波で先に、BS で後に流れる。4 話は録画済み。地上波の 3 話は、前の回を録らない設定のない束ねるルールが予約する。
		f := newEngineFixture(t, grEpisode(503, 5*time.Hour, seriesName+" #3"), bsEpisode(603, 29*time.Hour, seriesName+" #3"))
		f.addHistory(t, seriesName+" #4", -2*24*time.Hour)
		bundleRule := f.createSeriesRule(t, false, true)
		skipRule := f.createSeriesRule(t, true, false)
		f.run(t)
		require.Equal(t, map[int64]bool{503: true}, f.reservedSeries(t))

		skipped, err := rule.FindSkippedDuplicates(t.Context(), f.pool, f.clock, location)
		require.NoError(t, err)
		assert.Equal(t, []*rule.SkippedDuplicate{{
			ServiceID: bsServiceItemID(), EventID: 603,
			Rules:  []*rule.SkippedRule{{ID: bundleRule.ID, Name: bundleRule.Name}, {ID: skipRule.ID, Name: skipRule.Name}},
			Reason: rule.SkipReasonOtherBroadcast, LastRecordedAt: nil, Recorded: nil,
			Instead: &rule.SkippedInstead{ServiceID: serviceID(), EventID: 503}, LaterEpisode: nil,
		}}, skipped)
	})
}
