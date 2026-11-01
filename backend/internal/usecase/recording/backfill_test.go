package recording_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// timeshiftStore は、Recorder の Store と、そのタイムシフトのディレクトリ。
type timeshiftStore struct {
	store *kago.Store
	dir   string
}

// newRecorderWithTimeshift は、プール main と、タイムシフトのディレクトリを一時ディレクトリに置いた Recorder と、その Store を返す。
func (f *fixture) newRecorderWithTimeshift(t *testing.T, tuner recording.StreamOpener) (*recording.Recorder, *timeshiftStore) {
	t.Helper()

	dir := t.TempDir()
	timeshiftDir := t.TempDir()
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	store := kago.New(map[string]string{poolID: dir}, func(string) (uint64, error) { return 1 << 40, nil }).
		WithDataDir(t.TempDir()).WithTimeshiftDir(timeshiftDir)
	recorder := recording.NewRecorder(f.pool, f.clock, tuner, store, nil, tokyo, slog.New(slog.DiscardHandler), &recording.RecorderOptions{
		TuningTime: 0, Tick: 10 * time.Millisecond, RetryInterval: 20 * time.Millisecond, ReleaseAfter: 0, Peers: nil, DatabaseTimeout: 0,
	})

	return recorder, &timeshiftStore{store: store, dir: timeshiftDir}
}

// chunk は、タイムシフトのバッファのチャンク。end が nil なら、書いている途中。
type chunk struct {
	start time.Time
	end   *time.Time
}

// addChunk は、サービス serviceID のバッファに、data を書いたチャンクを、索引と一緒に置く。
func (f *fixture) addChunk(t *testing.T, store *timeshiftStore, serviceID int64, c chunk, data []byte) {
	t.Helper()

	chunkID := id.New()
	rel := filepath.Join(strconv.FormatInt(serviceID, 10), c.start.UTC().Format("20060102T150405Z")+".m2ts")
	root, err := os.OpenRoot(store.dir)
	require.NoError(t, err)
	defer root.Close()
	require.NoError(t, root.MkdirAll(filepath.Dir(rel), 0o755))
	require.NoError(t, root.WriteFile(rel, data, 0o600))
	analyzer, index, err := recording.CreateIndex(store.store, chunkID)
	require.NoError(t, err)
	_, err = analyzer.Write(data)
	require.NoError(t, err)
	require.NoError(t, index.Close())
	_, err = f.pool.Exec(t.Context(), `INSERT INTO timeshift_chunks (id, service_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $5)`, chunkID, serviceID, filepath.ToSlash(rel), len(data), c.start, c.end)
	require.NoError(t, err)
}

// segmentSpans は、録画のセグメントのうち、書いている途中か、書けたものの始まりと終わりを、始まりの順に返す。
func (f *fixture) segmentSpans(t *testing.T, reservationID uuid.UUID) []chunk {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), `SELECT started_at, ended_at FROM recording_files
		WHERE recording_id = (SELECT id FROM recordings WHERE reservation_id = $1) AND kind = 'segment' AND (size > 0 OR ended_at IS NULL) ORDER BY started_at`, reservationID)
	require.NoError(t, err)
	defer rows.Close()
	spans := []chunk{}
	for rows.Next() {
		var c chunk
		require.NoError(t, rows.Scan(&c.start, &c.end))
		spans = append(spans, c)
	}
	require.NoError(t, rows.Err())

	return spans
}

// TestRecorderBackfillsFromTimeshift は、録画を始めるのが予定の開始より遅れたとき、サービスをタイムシフトで録っていれば、
// 予定の開始からの分をバッファから写し、写せなかった分だけを途切れた区間にすることを確かめる (#826、設計書 5.7)。
func TestRecorderBackfillsFromTimeshift(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	planned := plannedStart()
	late := 2 * time.Minute
	tests := []struct {
		name string
		// timeshift は、サービスをタイムシフトで録っているかどうか。
		timeshift bool
		chunks    []chunk
		// wantStarts は、録画のセグメントの始まり。最後はライブのストリームで書いたセグメント (予定の開始から late の後)。
		wantStarts []time.Time
		wantState  recording.State
		wantGaps   int
	}{
		{
			name: "予定の開始より前から録れていれば、予定の開始から写し、冒頭を途切れた区間にしない", timeshift: true,
			chunks: []chunk{
				{start: planned.Add(-5 * time.Minute), end: new(planned.Add(time.Minute))},
				{start: planned.Add(time.Minute), end: nil},
			},
			wantStarts: []time.Time{planned, planned.Add(late)}, wantState: recording.StateCompleted, wantGaps: 0,
		},
		{
			name: "バッファが予定の開始より後から始まるなら、その間を途切れた区間にする", timeshift: true,
			chunks:     []chunk{{start: planned.Add(time.Minute), end: nil}},
			wantStarts: []time.Time{planned.Add(time.Minute), planned.Add(late)}, wantState: recording.StatePartial, wantGaps: 1,
		},
		{
			name: "バッファに穴があれば、続いたチャンクごとにセグメントに写し、穴を途切れた区間にする", timeshift: true,
			chunks: []chunk{
				{start: planned.Add(-time.Minute), end: new(planned.Add(30 * time.Second))},
				{start: planned.Add(time.Minute), end: nil},
			},
			wantStarts: []time.Time{planned, planned.Add(time.Minute), planned.Add(late)}, wantState: recording.StatePartial, wantGaps: 1,
		},
		{
			name: "サービスをタイムシフトで録っていなければ、写さない", timeshift: false,
			chunks:     []chunk{{start: planned.Add(-time.Minute), end: nil}},
			wantStarts: []time.Time{planned.Add(late)}, wantState: recording.StatePartial, wantGaps: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			recorder, store := f.newRecorderWithTimeshift(t, onceStream{data: data})
			if tt.timeshift {
				_, err := f.pool.Exec(t.Context(), `INSERT INTO timeshift_channels (service_id, retention_seconds, created_at, updated_at)
					VALUES ($1, 3600, $2, $2)`, grService(0), f.clock.Now())
				require.NoError(t, err)
			}
			for _, c := range tt.chunks {
				f.addChunk(t, store, grService(0), c, data)
			}
			f.clock.Advance(planned.Add(late).Sub(f.clock.Now()))
			_, done := f.session(t, recorder, reservationID)
			require.Eventually(t, func() bool {
				return f.segments(t, reservationID) >= 1
			}, waitTimeout, 10*time.Millisecond)

			// 書いている途中のチャンクが、ライブのストリームで書き始めた位置を越えるまで待ってから写す。
			f.clock.Advance(10 * time.Second)
			if len(tt.wantStarts) > 1 {
				require.Eventually(t, func() bool {
					return len(f.segmentSpans(t, reservationID)) == len(tt.wantStarts)
				}, waitTimeout, 10*time.Millisecond)
			}
			f.clock.Advance(programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
			require.NoError(t, wait(t, done))

			spans := f.segmentSpans(t, reservationID)
			starts := make([]time.Time, 0, len(spans))
			for _, span := range spans {
				assert.NotNil(t, span.end)
				starts = append(starts, span.start.UTC())
			}
			assert.Equal(t, tt.wantStarts, starts)
			got, _ := f.recording(t, reservationID)
			assert.Equal(t, string(tt.wantState), got.state)
			assert.Equal(t, gapsAndLost{gaps: tt.wantGaps, programLost: false}, f.gapsAndLost(t, reservationID))
			var recordedFrom time.Time
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT recorded_from FROM recordings WHERE reservation_id = $1", reservationID).
				Scan(&recordedFrom))
			assert.Equal(t, tt.wantStarts[0], recordedFrom.UTC())
		})
	}
}
