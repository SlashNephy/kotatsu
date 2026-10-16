package analysis_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/analysis"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/jobs"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	poolID    = "main"
	serviceID = 3273601024
	// segmentName は、テストの録画のセグメントの、プール内の相対パス。
	segmentName = "anime/録画.m2ts"
	// videoPID は、testdata/ts/test.ts の映像の PID。scripts/generate-test-ts.sh で作るときに指定している。
	videoPID = 0x0111
	// completed と completedWithErrors は、録画を終えた録画の状態。
	completed           = "completed"
	completedWithErrors = "completed_with_errors"
)

// wantIFramePTS は、testdata/ts/test.ts の I フレームの PTS。internal/ts の索引のテストと同じ値。
func wantIFramePTS() []int64 {
	return []int64{129003, 174048, 219093, 264138, 309183, 354228, 399273, 444318, 489363, 534408, 579453, 624498}
}

type fixture struct {
	pool     *pgxpool.Pool
	clock    *clocktest.Fake
	dir      string
	store    *kago.Store
	inserter *jobs.Client
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	inserter, err := jobs.NewClient(pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $2, $2)`, serviceID, clk.Now())
	require.NoError(t, err)
	dir := t.TempDir()

	return &fixture{pool: pool, clock: clk, dir: dir, store: kago.New(map[string]string{poolID: dir}, nil), inserter: inserter}
}

// segmentNames は、テストの録画のセグメントの、プール内の相対パス。n 番目のセグメントは segmentNames()[n] に置く。
func segmentNames() []string {
	return []string{segmentName, "anime/録画-2.m2ts"}
}

// recording は、EPGStation から取り込んだときと同じく、索引のないセグメントの録画を作る。セグメントの中身は segments の順に置く。
// 録画の集計は、EPGStation から取り込んだ値 (ドロップ 2、エラー 1、スクランブル 3) にする。
// EPGStation から取り込んだ録画では、どのセグメントも番組の始まりを始まりとして記録する。
func (f *fixture) recording(t *testing.T, state string, segments ...[]byte) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	startAt := f.clock.Now().Add(-time.Hour)
	endAt := startAt.Add(30 * time.Minute)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, drops, errors, scrambled, recorded_from, recorded_until, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, '録画', '', $3, $4, $5, false, 2, 1, 3, $3, $4, $3, $3)`, recordingID, serviceID, startAt, endAt, state)
	require.NoError(t, err)
	for i, data := range segments {
		name := segmentNames()[i]
		require.NoError(t, os.MkdirAll(filepath.Join(f.dir, filepath.Dir(name)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(f.dir, name), data, 0o600))
		_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
			VALUES ($1, $2, 'segment', $3, $4, $5, $6, $7, $6)`, id.New(), recordingID, poolID, name, len(data), startAt, endAt)
		require.NoError(t, err)
	}

	return recordingID
}

func (f *fixture) analyze(t *testing.T, recordingID uuid.UUID) error {
	t.Helper()

	worker := analysis.NewAnalyzeWorker(f.pool, f.store, f.clock, f.inserter)
	job := &river.Job[analysis.AnalyzeArgs]{JobRow: new(rivertype.JobRow), Args: analysis.AnalyzeArgs{RecordingID: recordingID}}

	return worker.Work(t.Context(), job)
}

func loadTestTS(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)

	return data
}

// singleTestTS は、テスト用の TS だけをセグメントに持つ録画の中身を返す。
func singleTestTS(t *testing.T) [][]byte {
	t.Helper()

	return [][]byte{loadTestTS(t)}
}

// withoutVideoPackets は、data から、映像の n 番目 (0 から数える) のパケットを抜いた TS を返す。
func withoutVideoPackets(t *testing.T, data []byte, ns ...int) []byte {
	t.Helper()

	drop := map[int]bool{}
	for _, n := range ns {
		drop[n] = true
	}
	out := make([]byte, 0, len(data))
	video := 0
	for offset := 0; offset+ts.PacketSize <= len(data); offset += ts.PacketSize {
		packet := data[offset : offset+ts.PacketSize]
		if pid := uint16(packet[1]&0x1F)<<8 | uint16(packet[2]); pid == videoPID {
			video++
			if drop[video-1] {
				continue
			}
		}
		out = append(out, packet...)
	}
	require.Less(t, len(out), len(data), "some packets must be dropped")

	return out
}

// withFlaggedVideoPackets は、data の映像の n 番目 (0 から数える) のパケットに、flag で印を付けた TS を返す。
func withFlaggedVideoPackets(t *testing.T, data []byte, flag func(packet []byte), ns ...int) []byte {
	t.Helper()

	out := bytes.Clone(data)
	targets := map[int]bool{}
	for _, n := range ns {
		targets[n] = true
	}
	video, flagged := 0, 0
	for offset := 0; offset+ts.PacketSize <= len(out); offset += ts.PacketSize {
		packet := out[offset : offset+ts.PacketSize]
		if pid := uint16(packet[1]&0x1F)<<8 | uint16(packet[2]); pid == videoPID {
			if targets[video] {
				flag(packet)
				flagged++
			}
			video++
		}
	}
	require.Len(t, ns, flagged, "every packet must be flagged")

	return out
}

// transportError は、パケットの transport_error_indicator を立てる。
func transportError(packet []byte) { packet[1] |= 0x80 }

// scrambled は、パケットの transport_scrambling_control を 0 以外にする。
func scrambled(packet []byte) { packet[3] |= 0x80 }

// TestAnalyzeWorker は、取り込んだ録画のセグメントを読み直し、付属ファイルのディレクトリに索引を作り、
// ドロップ、エラー、スクランブルを数え直して状態を決め直すことを確かめる (設計書 6.1)。
// 何度実行しても、索引の行は増えない。
func TestAnalyzeWorker(t *testing.T) {
	t.Parallel()

	type stats struct {
		State     string
		Drops     int64
		Errors    int64
		Scrambled int64
	}
	tests := []struct {
		name  string
		state string
		data  func(t *testing.T) [][]byte
		want  stats
	}{
		{
			name: "ドロップがなければ、EPGStation の集計を 0 にし、完了にする", state: completedWithErrors,
			data: singleTestTS,
			want: stats{State: completed, Drops: 0, Errors: 0, Scrambled: 0},
		},
		{
			name: "映像のパケットが抜けていれば、ドロップを数え、エラーありの完了にする", state: completed,
			data: func(t *testing.T) [][]byte {
				t.Helper()

				return [][]byte{withoutVideoPackets(t, loadTestTS(t), 100, 300)}
			},
			want: stats{State: completedWithErrors, Drops: 2, Errors: 0, Scrambled: 0},
		},
		{
			name: "エラーとスクランブルを数えるが、ドロップがなければ完了にする", state: completedWithErrors,
			data: func(t *testing.T) [][]byte {
				t.Helper()

				data := withFlaggedVideoPackets(t, loadTestTS(t), transportError, 50)

				return [][]byte{withFlaggedVideoPackets(t, data, scrambled, 200, 400)}
			},
			want: stats{State: completed, Drops: 0, Errors: 1, Scrambled: 2},
		},
		{
			name: "セグメントが 2 つあれば、それぞれに索引を作り、集計を合わせる", state: completed,
			data: func(t *testing.T) [][]byte {
				t.Helper()

				return [][]byte{withoutVideoPackets(t, loadTestTS(t), 100), withoutVideoPackets(t, loadTestTS(t), 200, 400)}
			},
			want: stats{State: completedWithErrors, Drops: 3, Errors: 0, Scrambled: 0},
		},
		{
			name: "途中で失敗した録画は、状態を変えない", state: "partial",
			data: singleTestTS,
			want: stats{State: "partial", Drops: 0, Errors: 0, Scrambled: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			segments := tt.data(t)
			recordingID := f.recording(t, tt.state, segments...)
			for range 2 {
				require.NoError(t, f.analyze(t, recordingID))
			}

			var got stats
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT state, drops, errors, scrambled FROM recordings WHERE id = $1", recordingID).
				Scan(&got.State, &got.Drops, &got.Errors, &got.Scrambled))
			assert.Equal(t, tt.want, got)

			// 索引は、録画中に作るものと同じく、I フレームの位置と PTS を並べる。セグメントごとに、同じ時間の範囲を表す行として 1 つだけ記録する。
			type file struct {
				Kind      string
				Path      string
				Size      int64
				StartedAt time.Time
				EndedAt   *time.Time
			}
			startAt := f.clock.Now().Add(-time.Hour)
			endAt := startAt.Add(30 * time.Minute)
			wantIndexes := make([]file, 0, len(segments))
			for i := range segments {
				name := segmentNames()[i]
				index, err := os.ReadFile(filepath.Join(f.dir, name+".kotatsu", "index"))
				require.NoError(t, err)
				entries, err := ts.ReadIndex(bytes.NewReader(index))
				require.NoError(t, err)
				pts := make([]int64, 0, len(entries))
				for _, entry := range entries {
					assert.Zero(t, entry.Offset%ts.PacketSize, "the offset must point at a packet")
					pts = append(pts, entry.PTS)
				}
				assert.Equal(t, wantIFramePTS(), pts, name)
				wantIndexes = append(wantIndexes, file{
					Kind: "index", Path: name + ".kotatsu/index", Size: int64(len(index)), StartedAt: startAt, EndedAt: &endAt,
				})
			}
			rows, err := f.pool.Query(t.Context(),
				"SELECT kind, path, size, started_at, ended_at FROM recording_files WHERE recording_id = $1 AND kind = 'index'", recordingID)
			require.NoError(t, err)
			indexes, err := pgx.CollectRows(rows, pgx.RowToStructByPos[file])
			require.NoError(t, err)
			assert.ElementsMatch(t, wantIndexes, indexes)

			// 代表画像とプレビュー画像を作るジョブを、重いジョブのキューに 1 つだけ投入する。
			// 取り込んだ録画の画像なので、優先度を下げる (#551)。
			rows, err = f.pool.Query(t.Context(),
				"SELECT queue || ' ' || priority || ' ' || (args->>'recordingId') FROM river_job WHERE kind = 'thumbnail_generate'")
			require.NoError(t, err)
			queued, err := pgx.CollectRows(rows, pgx.RowTo[string])
			require.NoError(t, err)
			assert.Equal(t, []string{jobs.QueueMedia + " 4 " + recordingID.String()}, queued)
		})
	}
}

// TestAnalyzeWorkerCancels は、録画が消されたか、録画中か、読めるセグメントがなければ、送り直さずに取りやめることを確かめる。
func TestAnalyzeWorkerCancels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		state   string
		prepare func(t *testing.T, f *fixture, recordingID uuid.UUID) uuid.UUID
		wantErr string
	}{
		{
			name: "録画が消された", state: completed,
			prepare: func(*testing.T, *fixture, uuid.UUID) uuid.UUID { return id.New() },
			wantErr: "the recording no longer exists",
		},
		{
			name: "録画中", state: "recording",
			prepare: func(_ *testing.T, _ *fixture, recordingID uuid.UUID) uuid.UUID { return recordingID },
			wantErr: "the recording is still being recorded",
		},
		{
			name: "プールがこのインスタンスにマウントされていない", state: completed,
			prepare: func(t *testing.T, f *fixture, recordingID uuid.UUID) uuid.UUID {
				t.Helper()

				f.store = kago.New(map[string]string{"sub": f.dir}, nil)

				return recordingID
			},
			wantErr: "the recording has no segment on the mounted pools",
		},
		{
			name: "一部のセグメントのプールがこのインスタンスにマウントされていない", state: completed,
			prepare: func(t *testing.T, f *fixture, recordingID uuid.UUID) uuid.UUID {
				t.Helper()

				_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
					VALUES ($1, $2, 'segment', 'sub', 'b.m2ts', 1, $3, $3, $3)`, id.New(), recordingID, f.clock.Now())
				require.NoError(t, err)

				return recordingID
			},
			wantErr: "some segments of the recording are on pools that are not mounted: 1 of 2 segments",
		},
		{
			name: "TS のセグメントがない", state: completed,
			prepare: func(t *testing.T, f *fixture, recordingID uuid.UUID) uuid.UUID {
				t.Helper()

				_, err := f.pool.Exec(t.Context(), "UPDATE recording_files SET kind = 'other' WHERE recording_id = $1", recordingID)
				require.NoError(t, err)

				return recordingID
			},
			wantErr: "the recording has no segment on the mounted pools",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			target := tt.prepare(t, f, f.recording(t, tt.state, loadTestTS(t)))
			err := f.analyze(t, target)
			var cancel *rivertype.JobCancelError
			require.ErrorAs(t, err, &cancel)
			require.ErrorContains(t, err, tt.wantErr)
			var queued int
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM river_job").Scan(&queued))
			assert.Zero(t, queued)
		})
	}
}

// TestAnalyzeWorkerStopsWhenCancelled は、nekonote を止めたら、取り込んだ集計を残したまま何も記録せず、送り直しに任せることを確かめる。
func TestAnalyzeWorkerStopsWhenCancelled(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, completedWithErrors, loadTestTS(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	worker := analysis.NewAnalyzeWorker(f.pool, f.store, f.clock, f.inserter)
	err := worker.Work(ctx, &river.Job[analysis.AnalyzeArgs]{JobRow: new(rivertype.JobRow), Args: analysis.AnalyzeArgs{RecordingID: recordingID}})
	require.ErrorIs(t, err, context.Canceled)
	var cancelErr *rivertype.JobCancelError
	assert.NotErrorAs(t, err, &cancelErr, "the job is retried")

	var drops int64
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT drops FROM recordings WHERE id = $1", recordingID).Scan(&drops))
	assert.Equal(t, int64(2), drops, "the imported stats are kept")
	var indexes int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM recording_files WHERE kind = 'index'").Scan(&indexes))
	assert.Zero(t, indexes)
}
