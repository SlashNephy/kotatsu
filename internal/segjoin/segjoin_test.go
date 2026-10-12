package segjoin_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/jobs"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/segjoin"
	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	poolID    = "main"
	serviceID = 3273601024
	// window は、テストで重なりを探す範囲。testdata/ts/test.ts の全体が入る大きさにする。
	window = 4 << 20
	// kindRecordingJoin などは、投入されるジョブの種類。
	kindRecordingJoin = "recording_join"
	kindFinalize      = "recording_join_finalize"
	kindThumbnail     = "thumbnail_generate"
	kindAnalyze       = "recording_analyze"
	// filesKey は、録画の完了の通知の材料のうち、セグメントの一覧のキー。
	filesKey = "files"
	// joinKey は、録画の完了の通知の材料のうち、セグメントをつなぐかどうかのキー。
	joinKey = "joinSegments"
)

type fixture struct {
	pool     *pgxpool.Pool
	clock    *clocktest.Fake
	dir      string
	store    *kago.Store
	inserter *jobs.Client
	original []byte
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 12, 21, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $2, $2)`, serviceID, clk.Now())
	require.NoError(t, err)
	inserter, err := jobs.NewClient(pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	original, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)
	dir := t.TempDir()

	return &fixture{pool: pool, clock: clk, dir: dir, store: kago.New(map[string]string{poolID: dir}, nil), inserter: inserter, original: original}
}

// segment は、録画のセグメントの中身と、表す時間。
type segment struct {
	data       []byte
	start, end time.Duration
}

// recording は、segments を持つ、完了した録画を作る。各セグメントには、付属ファイルのディレクトリに索引の記録も作る。
func (f *fixture) recording(t *testing.T, segments []segment) (uuid.UUID, []string) {
	t.Helper()

	recordingID := id.New()
	now := f.clock.Now()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, '録画', '', $3, $3, 'completed', false, $3, $3, $3)`, recordingID, serviceID, now)
	require.NoError(t, err)
	paths := make([]string, 0, len(segments))
	for i, seg := range segments {
		path := filepath.Join("録画", []string{"a.m2ts", "b.m2ts", "c.m2ts"}[i])
		require.NoError(t, os.MkdirAll(filepath.Join(f.dir, "録画"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(f.dir, path), seg.data, 0o600))
		sidecar, err := f.store.CreateSidecarDir(poolID, path)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(f.dir, sidecar, "index"), []byte("index"), 0o600))
		for _, file := range []struct {
			kind, path string
			size       int
		}{{"segment", path, len(seg.data)}, {"index", sidecar + "/index", len("index")}} {
			_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $7)`, id.New(), recordingID, file.kind, poolID, file.path, file.size, now.Add(seg.start), now.Add(seg.end))
			require.NoError(t, err)
		}
		paths = append(paths, path)
	}

	return recordingID, paths
}

// files は、録画のファイルの記録を、種類とプール内の相対パスと大きさで返す。
func (f *fixture) files(t *testing.T, recordingID uuid.UUID) []string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT kind || ' ' || path || ' ' || size FROM recording_files WHERE recording_id = $1 ORDER BY started_at, kind", recordingID)
	require.NoError(t, err)
	files, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)

	return files
}

// jobs は、投入されたジョブの種類と引数を返す。
func (f *fixture) jobs(t *testing.T) map[string]json.RawMessage {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT kind, args FROM river_job ORDER BY id")
	require.NoError(t, err)
	got := map[string]json.RawMessage{}
	for rows.Next() {
		var kind string
		var args json.RawMessage
		require.NoError(t, rows.Scan(&kind, &args))
		got[kind] = args
	}
	require.NoError(t, rows.Err())

	return got
}

func (f *fixture) join(t *testing.T, recordingID uuid.UUID) error {
	t.Helper()

	worker := segjoin.NewJoinWorker(f.pool, f.store, f.inserter, slog.New(slog.DiscardHandler), window)

	return worker.Work(t.Context(), &river.Job[segjoin.JoinArgs]{JobRow: new(rivertype.JobRow), Args: segjoin.JoinArgs{RecordingID: recordingID}})
}

func (f *fixture) finalize(t *testing.T, args json.RawMessage) error {
	t.Helper()

	var finalize segjoin.FinalizeArgs
	require.NoError(t, json.Unmarshal(args, &finalize))
	worker := segjoin.NewFinalizeWorker(f.pool, f.store, f.inserter)

	return worker.Work(t.Context(), &river.Job[segjoin.FinalizeArgs]{JobRow: new(rivertype.JobRow), Args: finalize})
}

// shiftPATCC は、PAT のパケットの continuity_counter を delta だけずらした複製を返す。
// 引き継いだ mikan は、チューナーサーバーの別のクライアントなので、PAT の continuity_counter だけが違うストリームを受け取る。
func shiftPATCC(b []byte, delta byte) []byte {
	shifted := bytes.Clone(b)
	for i := 0; i+ts.PacketSize <= len(shifted); i += ts.PacketSize {
		if shifted[i+1]&0x1F == 0 && shifted[i+2] == 0 {
			shifted[i+3] = shifted[i+3]&0xF0 | (shifted[i+3]+delta)&0x0F
		}
	}

	return shifted
}

func TestJoin(t *testing.T) {
	t.Parallel()

	at := func(b []byte, fraction float64) int {
		return int(float64(len(b)/ts.PacketSize)*fraction) * ts.PacketSize
	}

	tests := []struct {
		name string
		// segments は、テストの TS からセグメントを作る。
		segments func(original []byte) []segment
		// want は、つないだファイルの中身。nil なら、つながずに分けたまま残す。
		want func(original []byte) []byte
	}{
		{
			name: "引き継ぎで重なった 2 つを、録り続けたものと同じ 1 つにする",
			segments: func(o []byte) []segment {
				return []segment{
					{data: o[:at(o, 2.0/3)], start: 0, end: 40 * time.Second},
					{data: shiftPATCC(o[at(o, 1.0/3):], 7), start: 20 * time.Second, end: 60 * time.Second},
				}
			},
			want: func(o []byte) []byte { return o },
		},
		{
			name: "再起動で欠けた 2 つは、欠けたまま後ろにつなぐ",
			segments: func(o []byte) []segment {
				return []segment{
					{data: o[:at(o, 1.0/3)], start: 0, end: 20 * time.Second},
					{data: o[at(o, 2.0/3):], start: 40 * time.Second, end: 60 * time.Second},
				}
			},
			want: func(o []byte) []byte { return append(bytes.Clone(o[:at(o, 1.0/3)]), o[at(o, 2.0/3):]...) },
		},
		{
			name: "重なっているはずのパケットが違えば、分けたまま残す",
			segments: func(o []byte) []segment {
				next := bytes.Clone(o[at(o, 1.0/3):])
				next[at(o, 1.0/6)+100] ^= 0xFF

				return []segment{
					{data: o[:at(o, 2.0/3)], start: 0, end: 40 * time.Second},
					{data: next, start: 20 * time.Second, end: 60 * time.Second},
				}
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			segments := tt.segments(f.original)
			recordingID, paths := f.recording(t, segments)
			before := f.files(t, recordingID)

			require.NoError(t, f.join(t, recordingID))

			if tt.want == nil {
				assert.Equal(t, before, f.files(t, recordingID))
				assert.NoFileExists(t, filepath.Join(f.dir, paths[0]+".joining"))
				queued := f.jobs(t)
				assert.Contains(t, queued, kindThumbnail)
				assert.NotContains(t, queued, kindFinalize)

				return
			}
			want := tt.want(f.original)
			// 記録は、最初のセグメントの 1 行だけになる。索引は作り直すので消える。
			assert.Equal(t, []string{"segment " + paths[0] + " " + strconv.Itoa(len(want))}, f.files(t, recordingID))
			queued := f.jobs(t)
			require.Contains(t, queued, kindFinalize)

			require.NoError(t, f.finalize(t, queued[kindFinalize]))
			got, err := os.ReadFile(filepath.Join(f.dir, paths[0]))
			require.NoError(t, err)
			assert.True(t, bytes.Equal(want, got), "the joined file differs from the expected stream")
			assert.NoFileExists(t, filepath.Join(f.dir, paths[0]+".joining"))
			// つないだ元のセグメントは、付属ファイルのディレクトリごと消える。
			assert.NoFileExists(t, filepath.Join(f.dir, paths[1]))
			assert.NoDirExists(t, filepath.Join(f.dir, paths[1]+".kotatsu"))
			assert.Contains(t, f.jobs(t), kindAnalyze)

			// やり直しても、同じ結果になる。
			require.NoError(t, f.finalize(t, queued[kindFinalize]))
			got, err = os.ReadFile(filepath.Join(f.dir, paths[0]))
			require.NoError(t, err)
			assert.True(t, bytes.Equal(want, got))
		})
	}
}

func TestEnqueuer(t *testing.T) {
	t.Parallel()

	twoFiles := []map[string]string{{"poolId": poolID, "path": "a.m2ts"}, {"poolId": poolID, "path": "b.m2ts"}}
	tests := []struct {
		name    string
		kind    notification.Kind
		payload map[string]any
		want    string
	}{
		{
			name: "分かれていて、つなぐ設定なら、つなぐ", kind: notification.KindRecordingCompleted,
			payload: map[string]any{joinKey: true, filesKey: twoFiles}, want: kindRecordingJoin,
		},
		{
			name: "つながない設定なら、画像を作る", kind: notification.KindRecordingCompleted,
			payload: map[string]any{joinKey: false, filesKey: twoFiles}, want: kindThumbnail,
		},
		{
			name: "分かれていなければ、画像を作る", kind: notification.KindRecordingCompleted,
			payload: map[string]any{joinKey: true, filesKey: twoFiles[:1]}, want: kindThumbnail,
		},
		{
			name: "録画の失敗では、何もしない", kind: notification.KindRecordingFailed,
			payload: map[string]any{joinKey: true, filesKey: twoFiles}, want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := id.New()
			tt.payload["recordingId"] = recordingID.String()
			require.NoError(t, notification.Emit(t.Context(), f.pool, f.clock, tt.kind, tt.payload))
			n, err := notification.NewDispatcher(f.pool, f.clock, segjoin.NewEnqueuer(f.inserter)).DispatchOnce(t.Context())
			require.NoError(t, err)
			assert.Equal(t, 1, n)

			queued := f.jobs(t)
			if tt.want == "" {
				assert.Empty(t, queued)

				return
			}
			require.Len(t, queued, 1)
			var args struct {
				RecordingID uuid.UUID `json:"recordingId"`
			}
			require.NoError(t, json.Unmarshal(queued[tt.want], &args))
			assert.Equal(t, recordingID, args.RecordingID)
		})
	}
}
