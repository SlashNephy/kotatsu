package rule_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

const (
	// bsNetworkID と bsServiceID は、faketuner.DefaultOptions の BS のサービス (こたつ BS)。
	bsNetworkID = 4
	bsServiceID = 101
	// episodeName は、同じ回を束ねるテストの番組名。
	episodeName = "灯台の地図 #3「霧笛」"
)

func bsServiceItemID() int64 {
	return faketuner.ServiceItemID(bsNetworkID, bsServiceID)
}

// episodeProgram は、testNow から offset 後に始まる、1 時間の番組を返す。
func episodeProgram(networkID, serviceID, eventID int, offset time.Duration, name string) *faketuner.Program {
	return &faketuner.Program{
		ID: faketuner.ProgramID(networkID, serviceID, eventID), EventID: eventID, ServiceID: serviceID, NetworkID: networkID,
		StartAt: testNow().Add(offset).UnixMilli(), Duration: time.Hour.Milliseconds(), IsFree: true, Name: name, Description: "",
		RelatedItems: []*faketuner.RelatedItem{},
	}
}

// earlyAndLateEpisodes は、同じ回を、地上波で 5 時間後 (eventId 300) と、BS で 29 時間後 (eventId 400) に流す番組。
func earlyAndLateEpisodes() []*faketuner.Program {
	return []*faketuner.Program{
		episodeProgram(grNetworkID, grServiceID, 300, 5*time.Hour, episodeName),
		episodeProgram(bsNetworkID, bsServiceID, 400, 29*time.Hour, "灯台の地図 #3"),
	}
}

// createEpisodeRule は、地上波と BS の「灯台の地図」を予約するルールを作る。
func (f *engineFixture) createEpisodeRule(tb testing.TB, bundle bool) *rule.Rule {
	tb.Helper()

	return f.createRule(tb, `灯台の地図`, func(in *rule.Input) {
		in.Conditions.ServiceIDs = []int64{serviceID(), bsServiceItemID()}
		in.BundleSameEpisodes = bundle
	})
}

// reservedEpisodes は、予約した「灯台の地図」の番組の eventId と、予約が有効かどうかを返す。
func (f *engineFixture) reservedEpisodes(t *testing.T) map[int64]bool {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT event_id, enabled FROM reservations WHERE name LIKE '灯台の地図%'")
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

// TestEngineBundlesSameEpisodes は、同じ回を束ねるルールが、局をまたいだ同じ回のうち、最初の放送だけを予約することを確かめる (#1498)。
func TestEngineBundlesSameEpisodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		bundle bool
		// otherRule は、同じ回を束ねないルールにも、BS の放送を一致させるかどうか。
		otherRule bool
		want      map[int64]bool
	}{
		{name: "最初の放送だけを予約する", bundle: true, otherRule: false, want: map[int64]bool{300: true}},
		{name: "束ねないルールは、どちらも予約する", bundle: false, otherRule: false, want: map[int64]bool{300: true, 400: true}},
		{name: "束ねない別のルールも一致すれば、そのルールが予約する", bundle: true, otherRule: true, want: map[int64]bool{300: true, 400: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newEngineFixture(t, earlyAndLateEpisodes()...)
			f.createEpisodeRule(t, tt.bundle)
			if tt.otherRule {
				f.createRule(t, `灯台の地図`, func(in *rule.Input) { in.Conditions.ServiceIDs = []int64{bsServiceItemID()} })
			}
			f.run(t)

			assert.Equal(t, tt.want, f.reservedEpisodes(t))
		})
	}
}

// TestEngineSkipsRecordedEpisodes は、同じ回を「録った回を見る期間」の内に録っていれば、どの放送も予約しないことを確かめる (#1498)。
// 録画履歴は、自分で録ったものを期間で切り、ほかの録画ソフトから引き継いだものは切らない。録画中の録画も数える。
func TestEngineSkipsRecordedEpisodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// started は、録った回の録画が始まった、testNow からの時間。
		started  time.Duration
		imported bool
		// recording は、録画履歴でなく、録画中の録画として入れるかどうか。
		recording bool
		want      map[int64]bool
	}{
		{name: "期間の内に録った", started: -rule.RecordedEpisodeWindow + time.Minute, want: map[int64]bool{}},
		{name: "期間より前に録った", started: -rule.RecordedEpisodeWindow - time.Minute, want: map[int64]bool{300: true}},
		{name: "引き継いだ履歴は期間で切らない", started: -rule.RecordedEpisodeWindow - time.Minute, imported: true, want: map[int64]bool{}},
		{name: "録画中の録画も数える", started: -10 * time.Minute, recording: true, want: map[int64]bool{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newEngineFixture(t, earlyAndLateEpisodes()...)
			start := testNow().Add(tt.started)
			// 録った回は、別の局で、副題を書かずに流れた同じ回とする。
			const recordedName = "灯台の地図 #3"
			var err error
			if tt.recording {
				_, err = f.pool.Exec(t.Context(),
					`INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
					VALUES ($1, $2, $3, '', $4, $5, 'recording', $4, $4, $4)`,
					id.New(), faketuner.ServiceItemID(grNetworkID, subServiceID), recordedName, start, start.Add(time.Hour))
			} else {
				_, err = f.pool.Exec(t.Context(),
					`INSERT INTO recording_history (id, service_id, title_key, name, imported, start_at, end_at, created_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`,
					id.New(), faketuner.ServiceItemID(grNetworkID, subServiceID), rule.TitleKey(recordedName), recordedName, tt.imported, start, start.Add(time.Hour))
			}
			require.NoError(t, err)
			f.createEpisodeRule(t, true)
			f.run(t)

			assert.Equal(t, tt.want, f.reservedEpisodes(t))
		})
	}
}

// TestEngineKeepsNextEpisodeWhenConflicting は、選んだ放送が割り当てで競合なら、それも予約したまま、次の放送も予約することを確かめる (#1498)。
// 競合の結果は前回の割り当てで決まるので、次の放送が予約されるのは次の照合からである。
func TestEngineKeepsNextEpisodeWhenConflicting(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t, earlyAndLateEpisodes()...)
	f.createEpisodeRule(t, true)
	f.run(t)
	require.Equal(t, map[int64]bool{300: true}, f.reservedEpisodes(t))

	_, err := f.pool.Exec(t.Context(), "UPDATE reservations SET assignment_state = 'conflict' WHERE event_id = 300")
	require.NoError(t, err)
	f.run(t)

	assert.Equal(t, map[int64]bool{300: true, 400: true}, f.reservedEpisodes(t))
}

// TestEngineRemovesBundledReservations は、予約した後に同じ回の早い放送が番組表に入ったときと、同じ回を録り終えたときに、
// 外れた予約を引っ込めることを確かめる (#1498)。別の放送で録るので外れた予約は、番組表の変化で外れたので猶予の内なら残し、
// 録り終えて外れた予約は、猶予の内でもすぐ消す (#1501)。
func TestEngineRemovesBundledReservations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		grace    int
		recorded bool
		want     map[int64]bool
	}{
		{name: "早い放送が入れば、遅い放送の予約を消す", grace: 0, recorded: false, want: map[int64]bool{300: true}},
		{name: "猶予の内なら、遅い放送の予約も残す", grace: 24 * 60, recorded: false, want: map[int64]bool{300: true, 400: true}},
		{name: "録り終えたら、猶予の内でも消す", grace: 24 * 60, recorded: true, want: map[int64]bool{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// 猶予の上限 (1 日) の内に始まるよう、BS の放送を 20 時間後にする。
			f := newEngineFixture(t, episodeProgram(bsNetworkID, bsServiceID, 400, 20*time.Hour, "灯台の地図 #3"))
			f.setGrace(t, tt.grace)
			f.createEpisodeRule(t, true)
			f.run(t)
			require.Equal(t, map[int64]bool{400: true}, f.reservedEpisodes(t))

			if tt.recorded {
				_, err := f.pool.Exec(t.Context(),
					`INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $6)`,
					id.New(), serviceID(), rule.TitleKey(episodeName), episodeName, testNow().Add(-2*time.Hour), testNow().Add(-time.Hour))
				require.NoError(t, err)
			} else {
				f.fake.AddProgram(episodeProgram(grNetworkID, grServiceID, 300, 5*time.Hour, episodeName))
				_, err := f.reconciler.SyncAll(t.Context())
				require.NoError(t, err)
			}
			f.run(t)

			assert.Equal(t, tt.want, f.reservedEpisodes(t))
		})
	}
}

// TestFindSkippedEpisodes は、同じ回の束ねで予約しなかった番組を、理由と一緒に返すことを確かめる (#1498)。
func TestFindSkippedEpisodes(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	t.Run("別の放送で録る", func(t *testing.T) {
		t.Parallel()

		f := newEngineFixture(t, earlyAndLateEpisodes()...)
		r := f.createEpisodeRule(t, true)
		f.run(t)

		skipped, err := rule.FindSkippedDuplicates(t.Context(), f.pool, f.clock, location)
		require.NoError(t, err)
		require.Len(t, skipped, 1)
		assert.Equal(t, &rule.SkippedDuplicate{
			ServiceID: bsServiceItemID(), EventID: 400, Rules: []*rule.SkippedRule{{ID: r.ID, Name: r.Name}},
			Reason: rule.SkipReasonOtherBroadcast, LastRecordedAt: nil, Recorded: nil,
			Instead: &rule.SkippedInstead{ServiceID: serviceID(), EventID: 300},
		}, skipped[0])
	})

	t.Run("同じ回を録画済み", func(t *testing.T) {
		t.Parallel()

		f := newEngineFixture(t, earlyAndLateEpisodes()...)
		end := testNow().Add(-time.Hour).UTC()
		_, err := f.pool.Exec(t.Context(),
			`INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $6)`,
			id.New(), serviceID(), rule.TitleKey(episodeName), episodeName, end.Add(-time.Hour), end)
		require.NoError(t, err)
		f.createEpisodeRule(t, true)
		f.run(t)

		skipped, err := rule.FindSkippedDuplicates(t.Context(), f.pool, f.clock, location)
		require.NoError(t, err)
		require.Len(t, skipped, 2)
		for _, s := range skipped {
			assert.Equal(t, rule.SkipReasonRecordedEpisode, s.Reason)
			assert.Equal(t, &rule.SkippedRecording{ServiceID: serviceID(), Name: episodeName, EndAt: end}, s.Recorded)
			require.NotNil(t, s.LastRecordedAt)
			assert.True(t, end.Equal(*s.LastRecordedAt))
			assert.Nil(t, s.Instead)
		}
	})
}
