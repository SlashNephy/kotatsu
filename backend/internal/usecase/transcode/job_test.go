package transcode_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
)

const (
	poolID    = "main"
	serviceID = 3273601024
	// segmentName は、テストの録画のセグメントの名前。
	segmentName = "anime/foo.m2ts"
	// outputName は、segmentName を 720p で変換した MP4 の名前。
	outputName = "anime/foo.720p.mp4"
	preset720p = "720p"
	// filesKey は、録画の完了のイベントの、セグメントの一覧のキー。
	filesKey = "files"
	// presetKey は、録画の完了のイベントの、変換の画質のキー。
	presetKey = "transcodePreset"
)

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
	clk := clocktest.NewFake(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $2, $2)`, serviceID, clk.Now())
	require.NoError(t, err)
	dir := t.TempDir()

	return &fixture{pool: pool, clock: clk, dir: dir, store: kago.New(map[string]string{poolID: dir}, nil).WithDataDir(t.TempDir()), inserter: inserter}
}

// recording は、segmentName のセグメント (中身は content) を 1 つ持つ、録り終えた録画を作る。
func (f *fixture) recording(t *testing.T, content []byte) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	recordedFrom := f.clock.Now().Add(-time.Hour)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, drops, errors, scrambled, recorded_from, recorded_until, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, '録画', '', $3, $4, 'completed', false, 0, 0, 0, $3, $4, $3, $3)`,
		recordingID, serviceID, recordedFrom, recordedFrom.Add(30*time.Minute))
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(f.dir, filepath.Dir(segmentName)), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, segmentName), content, 0o600))
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'segment', $3, $4, $5, $6, $7, $6)`,
		id.New(), recordingID, poolID, segmentName, len(content), recordedFrom, recordedFrom.Add(30*time.Minute))
	require.NoError(t, err)

	return recordingID
}

// copyEncoder は、読んだ TS をそのまま書く Encoder。渡された画質を覚える。
type copyEncoder struct {
	presets []string
}

var _ transcode.Encoder = new(copyEncoder)

func (e *copyEncoder) Encode(_ context.Context, src io.Reader, preset *kyusu.Preset, out string) error {
	e.presets = append(e.presets, preset.Name)
	b, err := io.ReadAll(src)
	if err != nil {
		return err
	}

	return os.WriteFile(out, b, 0o600)
}

type otherFile struct {
	path string
	size int64
}

func (f *fixture) otherFiles(t *testing.T, recordingID uuid.UUID) []otherFile {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT path, size FROM recording_files WHERE recording_id = $1 AND kind = 'other' ORDER BY path", recordingID)
	require.NoError(t, err)
	defer rows.Close()
	files := []otherFile{}
	for rows.Next() {
		var file otherFile
		require.NoError(t, rows.Scan(&file.path, &file.size))
		files = append(files, file)
	}
	require.NoError(t, rows.Err())

	return files
}

func work(t *testing.T, f *fixture, encoder transcode.Encoder, recordingID uuid.UUID, preset string) error {
	t.Helper()

	worker := transcode.NewWorker(f.pool, f.store, f.clock, func() (transcode.Encoder, error) { return encoder, nil })

	return worker.Work(t.Context(), &river.Job[transcode.Args]{JobRow: new(rivertype.JobRow), Args: transcode.Args{RecordingID: recordingID, Preset: preset}})
}

// tsPackets は、n 個の TS のパケット (同期バイトのあとを 0 で埋めたもの) を返す。
func tsPackets(n int) []byte {
	packet := make([]byte, 188)
	packet[0] = 0x47

	return bytes.Repeat(packet, n)
}

// TestWorker は、録画を 1 本の TS として読んで変換し、最初のセグメントの隣に MP4 を書き、その他のファイルとして記録することを確かめる (#1253)。
// 変換し直したら、同じファイルの記録を書き直す。
func TestWorker(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	content := tsPackets(10)
	recordingID := f.recording(t, content)
	encoder := new(copyEncoder)

	require.NoError(t, work(t, f, encoder, recordingID, preset720p))
	assert.Equal(t, []string{preset720p}, encoder.presets)
	written, err := os.ReadFile(filepath.Join(f.dir, outputName))
	require.NoError(t, err)
	assert.Equal(t, content, written, "the recording is read from the beginning to the end")
	assert.Equal(t, []otherFile{{path: outputName, size: int64(len(content))}}, f.otherFiles(t, recordingID))
	_, err = os.Stat(filepath.Join(f.dir, outputName+".part"))
	require.ErrorIs(t, err, os.ErrNotExist, "the partial file is renamed")

	// 変換し直しても、記録は 1 つのまま。
	require.NoError(t, work(t, f, encoder, recordingID, preset720p))
	assert.Len(t, f.otherFiles(t, recordingID), 1)
}

// TestWorkerCancels は、変換できない録画では、やり直さずに取りやめることを確かめる (#1253)。
func TestWorkerCancels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		setup  func(t *testing.T, f *fixture) uuid.UUID
		preset string
	}{
		{
			name: "unknown preset", preset: "1080p60",
			setup: func(t *testing.T, f *fixture) uuid.UUID {
				t.Helper()

				return f.recording(t, []byte("ts"))
			},
		},
		{name: "deleted recording", preset: preset720p, setup: func(*testing.T, *fixture) uuid.UUID { return id.New() }},
		{
			name: "still recording", preset: preset720p,
			setup: func(t *testing.T, f *fixture) uuid.UUID {
				t.Helper()

				recordingID := f.recording(t, []byte("ts"))
				_, err := f.pool.Exec(t.Context(), "UPDATE recordings SET state = 'recording', recorded_until = NULL WHERE id = $1", recordingID)
				require.NoError(t, err)

				return recordingID
			},
		},
		{
			name: "untracked file at the output path", preset: preset720p,
			setup: func(t *testing.T, f *fixture) uuid.UUID {
				t.Helper()

				recordingID := f.recording(t, []byte("ts"))
				// 利用者が手で置いたファイルは、DB に記録がなくても上書きしない。
				require.NoError(t, os.WriteFile(filepath.Join(f.dir, outputName), []byte("mine"), 0o600))

				return recordingID
			},
		},
		{
			name: "output path used by another recording", preset: preset720p,
			setup: func(t *testing.T, f *fixture) uuid.UUID {
				t.Helper()

				recordingID := f.recording(t, []byte("ts"))
				other := id.New()
				_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
					state, protected, drops, errors, scrambled, recorded_from, recorded_until, created_at, updated_at)
					SELECT $1, NULL, service_id, NULL, '別の録画', '', start_at, end_at, state, false, 0, 0, 0, recorded_from, recorded_until, created_at, updated_at
					FROM recordings WHERE id = $2`, other, recordingID)
				require.NoError(t, err)
				_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
					VALUES ($1, $2, 'other', $3, $4, 1, $5, $5, $5)`, id.New(), other, poolID, outputName, f.clock.Now())
				require.NoError(t, err)

				return recordingID
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := tt.setup(t, f)
			encoder := new(copyEncoder)
			var cancel *river.JobCancelError
			require.ErrorAs(t, work(t, f, encoder, recordingID, tt.preset), &cancel)
			assert.Empty(t, encoder.presets, "nothing is encoded")
			if written, err := os.ReadFile(filepath.Join(f.dir, outputName)); err == nil {
				assert.NotEqual(t, []byte("ts"), written, "an existing file is not overwritten")
			}
		})
	}
}

// TestEnqueuer は、変換の画質を選んだ録画を終えたときだけ、変換のジョブを入れることを確かめる (#1253)。
func TestEnqueuer(t *testing.T) {
	t.Parallel()

	oneSegment := func() []map[string]string { return []map[string]string{{"poolId": poolID, "path": segmentName}} }
	tests := []struct {
		name    string
		kind    domain.NotificationKind
		payload map[string]any
		want    bool
	}{
		{name: "preset", kind: domain.NotificationKindRecordingCompleted, payload: map[string]any{filesKey: oneSegment(), presetKey: preset720p}, want: true},
		{name: "no transcoding", kind: domain.NotificationKindRecordingCompleted, payload: map[string]any{filesKey: oneSegment(), presetKey: ""}, want: false},
		{name: "older mikan", kind: domain.NotificationKindRecordingCompleted, payload: map[string]any{filesKey: oneSegment()}, want: false},
		{name: "no segment", kind: domain.NotificationKindRecordingCompleted, payload: map[string]any{filesKey: []map[string]string{}, presetKey: preset720p}, want: false},
		{name: "failed recording", kind: domain.NotificationKindRecordingFailed, payload: map[string]any{filesKey: oneSegment(), presetKey: preset720p}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := id.New()
			tt.payload["recordingId"] = recordingID.String()
			require.NoError(t, notification.Emit(t.Context(), f.pool, f.clock, tt.kind, tt.payload))
			_, err := notification.NewDispatcher(f.pool, f.clock, transcode.NewEnqueuer(f.inserter)).DispatchOnce(t.Context())
			require.NoError(t, err)

			rows, err := f.pool.Query(t.Context(), "SELECT queue, args FROM river_job WHERE kind = 'recording_transcode'")
			require.NoError(t, err)
			defer rows.Close()
			queued := []transcode.Args{}
			for rows.Next() {
				var queue string
				var raw json.RawMessage
				require.NoError(t, rows.Scan(&queue, &raw))
				assert.Equal(t, jobs.QueueTranscode, queue)
				var args transcode.Args
				require.NoError(t, json.Unmarshal(raw, &args))
				queued = append(queued, args)
			}
			require.NoError(t, rows.Err())
			if tt.want {
				assert.Equal(t, []transcode.Args{{RecordingID: recordingID, Preset: preset720p}}, queued)
			} else {
				assert.Empty(t, queued)
			}
		})
	}
}
