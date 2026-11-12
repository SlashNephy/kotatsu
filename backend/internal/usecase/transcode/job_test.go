package transcode_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
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
	dataDir  string
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
	dir, dataDir := t.TempDir(), t.TempDir()

	return &fixture{
		pool: pool, clock: clk, dir: dir, dataDir: dataDir, store: kago.New(map[string]string{poolID: dir}, nil).WithDataDir(dataDir),
		inserter: inserter,
	}
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
// 読み終えたら、progress を -progress の出力として書く。ProbeFrameRate は、rate を返す。
type copyEncoder struct {
	presets  []string
	progress string
	rate     float64
}

var _ transcode.Encoder = new(copyEncoder)

func (e *copyEncoder) Encode(_ context.Context, src io.Reader, preset *kyusu.Preset, out string, progress io.Writer) error {
	e.presets = append(e.presets, preset.Name)
	b, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(progress, e.progress); err != nil {
		return err
	}

	return os.WriteFile(out, b, 0o600)
}

func (e *copyEncoder) ProbeFrameRate(context.Context, string) (float64, error) {
	return e.rate, nil
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

	return workJob(t.Context(), f, encoder, &rivertype.JobRow{ID: 1, Attempt: 1}, recordingID, preset)
}

func workJob(ctx context.Context, f *fixture, encoder transcode.Encoder, row *rivertype.JobRow, recordingID uuid.UUID, preset string) error {
	worker := transcode.NewWorker(f.pool, f.store, f.clock, f.inserter, slog.New(slog.DiscardHandler), func() (transcode.Encoder, error) { return encoder, nil })

	return worker.Work(ctx, &river.Job[transcode.Args]{JobRow: row, Args: transcode.Args{RecordingID: recordingID, Preset: preset}})
}

// index は、録画 recordingID のセグメントに、1 秒ごとの n 個の I フレームの索引と、フレームレートが rate の映像の形式を付ける。
// 再生の時間は、最後の I フレームの後の 1 間隔を足して n 秒になるので、見込みのコマ数は n × rate。
func (f *fixture) index(t *testing.T, recordingID uuid.UUID, n int64, rate float64) {
	t.Helper()

	var fileID uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT id FROM recording_files WHERE recording_id = $1 AND kind = 'segment'", recordingID).Scan(&fileID))
	location := kago.IndexLocation(fileID)
	require.NoError(t, os.MkdirAll(filepath.Join(f.dataDir, filepath.Dir(location.Path)), 0o750))
	file, err := os.Create(filepath.Join(f.dataDir, location.Path))
	require.NoError(t, err)
	defer file.Close()
	w, err := ts.NewIndexWriter(file)
	require.NoError(t, err)
	for k := range n {
		require.NoError(t, w.Append(ts.IndexEntry{Offset: k * 188, PTS: k * ts.PTSClock}))
	}
	video, err := json.Marshal(&ts.VideoFormat{Codec: "h264", Width: 1440, Height: 1080, Interlaced: true, FrameRate: rate})
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE recording_files SET video = $2 WHERE id = $1", fileID, video)
	require.NoError(t, err)
}

// progressOf は、ジョブ jobID の進み具合の、試行と割合を返す。まだなければ、試行を 0 にする。
func (f *fixture) progressOf(t *testing.T, jobID int64) (int, float64) {
	t.Helper()

	var attempt int
	var ratio float64
	err := f.pool.QueryRow(t.Context(), "SELECT attempt, ratio FROM job_progress WHERE job_id = $1", jobID).Scan(&attempt, &ratio)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0
	}
	require.NoError(t, err)

	return attempt, ratio
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

// quarterBlock は、1000 コマの見込みの 4 分の 1 をエンコードしたときの -progress のまとまり。
const quarterBlock = "frame=250\nprogress=continue\n"

// TestWorkerProgress は、変換の進み具合を、見込みのコマ数か入力の読み位置で割り、ジョブの試行と一緒に DB に書くことを確かめる (#1511)。
func TestWorkerProgress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// keyframes は、索引の I フレームの数 (1 秒ごと)。0 なら索引を作らない。
		keyframes int64
		// video は、映像の形式のフレームレート。probe は、ffprobe で読むフレームレート。
		video, probe float64
		progress     string
		want         float64
	}{
		{name: "frame rate of the video format", keyframes: 100, video: 10, probe: 0, progress: quarterBlock, want: 0.25},
		// HEVC は、映像の形式にフレームレートがないので、ffprobe で補う。
		{name: "frame rate from ffprobe", keyframes: 100, video: 0, probe: 10, progress: quarterBlock, want: 0.25},
		{name: "end", keyframes: 100, video: 10, probe: 0, progress: quarterBlock + "frame=990\nprogress=end\n", want: 1},
		// 索引がなければ、見込みのコマ数を出せないので、流したバイト数で割る。読み終えていても、progress=end までは頭打ちで止める。
		{name: "read position without the index", keyframes: 0, video: 10, probe: 0, progress: quarterBlock, want: 0.99},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.recording(t, tsPackets(100))
			if tt.keyframes > 0 {
				f.index(t, recordingID, tt.keyframes, tt.video)
			}
			encoder := &copyEncoder{presets: []string{}, progress: tt.progress, rate: tt.probe}
			require.NoError(t, workJob(t.Context(), f, encoder, &rivertype.JobRow{ID: 42, Attempt: 2}, recordingID, preset720p))
			attempt, ratio := f.progressOf(t, 42)
			assert.Equal(t, 2, attempt)
			assert.InDelta(t, tt.want, ratio, 1e-9)
		})
	}
}

// fakeFFmpeg は、-progress の出力を模す偽の FFmpeg のシェルのスクリプト。10% を出したら go のファイルを、20% を出したら
// stop のファイルを待つ。stop の後に、読み損ねた少ないコマ数と progress=end を出して終わる。出力はシェルが書くので、
// FFmpeg (このテストのバイナリ) を止められても、シェルが終わるまで出力が届き続ける (止められてから終わるまで少し待つ FFmpeg を模す)。
const fakeFFmpeg = `
printf 'frame=100\nprogress=continue\n'
while [ ! -e "$FAKE_FFMPEG_DIR/go" ]; do sleep 0.05; done
printf 'frame=200\nprogress=continue\n'
while [ ! -e "$FAKE_FFMPEG_DIR/stop" ]; do sleep 0.05; done
sleep 0.2
printf 'frame=50\nprogress=continue\nframe=1000\nprogress=end\n'
touch "$FAKE_FFMPEG_DIR/done"
`

// TestMain は、このテストのバイナリが ffmpeg の名前で起動されたら、偽の FFmpeg (fakeFFmpeg) として動く。
func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == "ffmpeg" {
		cmd := exec.CommandContext(context.Background(), "sh", "-c", fakeFFmpeg)
		cmd.Stdout = os.Stdout
		if err := cmd.Start(); err != nil {
			os.Exit(1)
		}
		_ = cmd.Wait()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestWorkerStopKeepsProgress は、変換を止めてから FFmpeg が終わり、後片付けをするまでの間に、進み具合が戻らず、
// 100% にもならないことを確かめる (#1511)。t.Setenv で PATH を変えるので、並べて走らせない。
func TestWorkerStopKeepsProgress(t *testing.T) {
	f := newFixture(t)
	recordingID := f.recording(t, tsPackets(100))
	// 見込みのコマ数は 100 秒 × 10fps = 1000 コマ。
	f.index(t, recordingID, 100, 10)
	// 偽の FFmpeg は、このテストのバイナリを ffmpeg の名前で起動したもの (TestMain)。
	bin, marks := t.TempDir(), t.TempDir()
	self, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, os.Symlink(self, filepath.Join(bin, "ffmpeg")))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_FFMPEG_DIR", marks)
	encoder, err := transcode.NewFFmpegEncoder(&config.Kyusu{Encoder: "", VAAPIDevice: "", WorkDir: ""}, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- workJob(ctx, f, encoder, &rivertype.JobRow{ID: 7, Attempt: 1}, recordingID, preset720p)
	}()
	ratioOf := func() float64 {
		_, ratio := f.progressOf(t, 7)

		return ratio
	}
	require.Eventually(t, func() bool { return ratioOf() == 0.1 }, 10*time.Second, 10*time.Millisecond)
	// 書き戻しの間隔を空けてから、20% を出させる。
	f.clock.Advance(time.Minute)
	require.NoError(t, os.WriteFile(filepath.Join(marks, "go"), nil, 0o600))
	require.Eventually(t, func() bool { return ratioOf() == 0.2 }, 10*time.Second, 10*time.Millisecond)

	// 止めた後に届く、少ないコマ数と progress=end では、表示を変えない。
	cancel()
	require.NoError(t, os.WriteFile(filepath.Join(marks, "stop"), nil, 0o600))
	f.clock.Advance(time.Minute)
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(20 * time.Second):
		t.Fatal("the worker did not return after it was stopped")
	}
	_, err = os.Stat(filepath.Join(marks, "done"))
	require.NoError(t, err, "the fake FFmpeg wrote the progress after it was stopped")
	assert.InDelta(t, 0.2, ratioOf(), 1e-9)
}
