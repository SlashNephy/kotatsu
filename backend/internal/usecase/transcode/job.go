package transcode

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

const (
	// kindSegment と kindOther は、recording_files の種類。
	kindSegment = "segment"
	kindOther   = "other"
	// presetKey は、録画の完了のイベントの、変換の画質のキー。
	presetKey = "transcodePreset"
	// maxAttempts は、変換のジョブを試す回数の上限。
	maxAttempts = 3
	// timeout は、変換のジョブの時間の上限。CPU でエンコードすると、録画の長さと同じくらいかかることがある。
	timeout = 6 * time.Hour
	// lockedSnooze は、同じ録画のファイルをほかでつないでいるときに、やり直すまで待つ時間。
	lockedSnooze = 10 * time.Minute
	// partialSuffix は、書いている途中の MP4 に付ける拡張子。書き終えたら外す。
	partialSuffix = ".part"
)

var (
	errRecordingNotFound = errors.New("the recording no longer exists")
	errStillRecording    = errors.New("the recording is still being recorded")
	errNoSegment         = errors.New("the recording has no segment on the mounted pools")
	errUnknownPreset     = errors.New("unknown transcode preset")
	errPathTaken         = errors.New("the output path is used by another file")
)

// Args は、録画を変換するジョブの引数。
type Args struct {
	RecordingID uuid.UUID `json:"recordingId"`
	// Preset は、変換の画質 (domain.TranscodePresets のどれか)。
	Preset string `json:"preset"`
}

var _ river.JobArgsWithInsertOpts = new(Args)

func (Args) Kind() string { return "recording_transcode" }

func (Args) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// 録画を読み通してエンコードするので、ほかの後処理を待たせないよう、別のキューで 1 つずつ動かす。
		Queue:       jobs.QueueTranscode,
		MaxAttempts: maxAttempts,
		// 同じ録画と画質のジョブは、待っている間に重ねて投入しない。
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
			rivertype.JobStateRunning, rivertype.JobStateScheduled,
		}},
	}
}

// Inserter は、ジョブをトランザクションの中で投入する。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// Enqueuer は、録画の完了のイベントを配るときに、変換のジョブを投入する。
type Enqueuer struct {
	inserter Inserter
}

var _ notification.Hook = new(Enqueuer)

// NewEnqueuer は Enqueuer を返す。
func NewEnqueuer(inserter Inserter) *Enqueuer {
	return &Enqueuer{inserter: inserter}
}

// OnEvent は、録画の完了のイベントのうち、変換の画質を選んだ録画 (TS のセグメントを書いたもの) に、変換のジョブを投入する。
// 変換するかどうかは、録画を終えるときに、全体の既定値に、ルールと予約の上書きを重ねて求めてある。
// 画質のないイベント (足す前の版の mikan が出したもの) は、既定に合わせて変換しない。
func (e *Enqueuer) OnEvent(ctx context.Context, tx pgx.Tx, event *notification.Event) error {
	if event.Kind != domain.NotificationKindRecordingCompleted {
		return nil
	}
	preset, _ := event.Payload[presetKey].(string)
	recordingID, ok := recordingIDOf(event.Payload)
	if preset == "" || !ok || segmentCount(event.Payload) == 0 {
		return nil
	}
	if _, err := e.inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: Args{RecordingID: recordingID, Preset: preset}, InsertOpts: nil}}); err != nil {
		return fmt.Errorf("failed to enqueue the transcoding: %w", err)
	}

	return nil
}

// recordingIDOf は、録画の完了のイベントの材料から、録画の ID を読む。
func recordingIDOf(payload map[string]any) (uuid.UUID, bool) {
	raw, _ := payload["recordingId"].(string)
	recordingID, err := uuid.Parse(raw)

	return recordingID, err == nil
}

// segmentCount は、録画の完了のイベントに載せたセグメントの数を返す。
// イベントを作ったプロセスの中では []map[string]string のまま、DB から読み直したものは []any で届く。
func segmentCount(payload map[string]any) int {
	switch files := payload["files"].(type) {
	case []map[string]string:
		return len(files)
	case []any:
		return len(files)
	default:
		return 0
	}
}

// Database は、変換に使う DB。*pgxpool.Pool が満たす。録画ごとの advisory lock を持つのに、接続を借りる。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

// Worker は、録画を、引き継ぎで重なった分を除いた 1 本の TS として読み、MP4 に変換して、録画の最初のセグメントの隣に書く。
type Worker struct {
	river.WorkerDefaults[Args]

	db     Database
	store  *kago.Store
	clock  clock.Clock
	source *stream.RecordingSource
	// newEncoder は、変換に使う Encoder を作る。テストでは差し替える。
	newEncoder func() (Encoder, error)
}

var _ river.Worker[Args] = new(Worker)

// NewWorker は Worker を返す。store は、このインスタンスにマウントされたプール。
func NewWorker(db Database, store *kago.Store, clk clock.Clock, newEncoder func() (Encoder, error)) *Worker {
	return &Worker{
		WorkerDefaults: river.WorkerDefaults[Args]{}, db: db, store: store, clock: clk,
		source: stream.NewRecordingSource(db, store, 0), newEncoder: newEncoder,
	}
}

func (w *Worker) Timeout(*river.Job[Args]) time.Duration {
	return timeout
}

// Work は、録画ごとの advisory lock を取ってから、録画を変換する。
// セグメントをつなぐジョブと同じロックを使い、ほかの nekonote がファイルをつないだり消したりしている間は読まず、後でやり直す。
func (w *Worker) Work(ctx context.Context, job *river.Job[Args]) error {
	preset, ok := kyusu.PresetByName(job.Args.Preset)
	if !ok || !slices.Contains(domain.TranscodePresets(), job.Args.Preset) {
		return river.JobCancel(fmt.Errorf("%w: %q", errUnknownPreset, job.Args.Preset))
	}
	encoder, err := w.newEncoder()
	if errors.Is(err, ErrFFmpegNotFound) {
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}
	conn, err := w.db.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire a connection: %w", err)
	}
	defer conn.Release()
	key := job.Args.RecordingID.String()
	locked, err := sqlc.New(conn).TryLockRecordingJoin(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to lock the recording: %w", err)
	}
	if !locked {
		return river.JobSnooze(lockedSnooze)
	}
	defer func() {
		if _, err := sqlc.New(conn).UnlockRecordingJoin(context.WithoutCancel(ctx), key); err != nil {
			// 接続を閉じれば、ロックも外れる。
			_ = conn.Conn().Close(context.WithoutCancel(ctx))
		}
	}()

	return w.work(ctx, encoder, job.Args.RecordingID, preset)
}

func (w *Worker) work(ctx context.Context, encoder Encoder, recordingID uuid.UUID, preset *kyusu.Preset) error {
	q := sqlc.New(w.db)
	recording, err := q.GetRecording(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(errRecordingNotFound)
	}
	if err != nil {
		return fmt.Errorf("failed to get the recording: %w", err)
	}
	if recording.RecordedUntil == nil {
		return river.JobCancel(errStillRecording)
	}
	files, err := q.ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return fmt.Errorf("failed to list the recording files: %w", err)
	}
	first := slices.IndexFunc(files, func(f sqlc.RecordingFile) bool { return f.Kind == kindSegment })
	if first < 0 {
		return river.JobCancel(errNoSegment)
	}
	poolID, rel := files[first].PoolID, outputPath(files[first].Path, preset.Name)
	out, err := w.store.Path(poolID, rel)
	if err != nil {
		return river.JobCancel(fmt.Errorf("%w: %w", errNoSegment, err))
	}
	owner, err := q.GetRecordingFileOwner(ctx, sqlc.GetRecordingFileOwnerParams{PoolID: poolID, Path: rel})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return fmt.Errorf("failed to check the output path: %w", err)
	case owner.RecordingID != recordingID || owner.Kind != kindOther:
		return river.JobCancel(fmt.Errorf("%w: %s/%s", errPathTaken, poolID, rel))
	}

	opened, err := w.source.Open(ctx, recordingID)
	if err != nil {
		return fmt.Errorf("failed to open the recording: %w", err)
	}
	partial := out + partialSuffix
	if err := w.encode(ctx, encoder, opened, preset, partial); err != nil {
		_ = os.Remove(partial)

		return err
	}
	if err := os.Rename(partial, out); err != nil {
		_ = os.Remove(partial)

		return fmt.Errorf("failed to move the transcoded file: %w", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		return fmt.Errorf("failed to stat the transcoded file: %w", err)
	}
	err = q.UpsertTranscodedRecordingFile(ctx, sqlc.UpsertTranscodedRecordingFileParams{
		ID: id.New(), RecordingID: recordingID, PoolID: poolID, Path: rel, Size: info.Size(),
		StartedAt: recording.RecordedFrom, EndedAt: recording.RecordedUntil, Now: w.clock.Now(),
	})
	if err != nil {
		// 変換している間に録画が消されたら、書いたファイルも消す。
		_ = os.Remove(out)

		return fmt.Errorf("failed to record the transcoded file: %w", err)
	}

	return nil
}

// encode は、録画を始まりから流し、encoder で out に書く。
func (w *Worker) encode(ctx context.Context, encoder Encoder, recording *stream.Recording, preset *kyusu.Preset, out string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	written := make(chan error, 1)
	go func() {
		err := recording.Write(ctx, writer, recording.Seek(0))
		_ = writer.CloseWithError(err)
		written <- err
	}()
	err := encoder.Encode(ctx, reader, preset, out)
	_ = reader.Close()
	cancel()
	writeErr := <-written
	if err != nil {
		return err
	}
	if writeErr != nil && !errors.Is(writeErr, io.ErrClosedPipe) && !errors.Is(writeErr, context.Canceled) {
		return fmt.Errorf("failed to read the recording: %w", writeErr)
	}

	return nil
}

// outputPath は、録画の最初のセグメントのパス segment から、変換した MP4 のパスを作る。拡張子を、画質と .mp4 に変える。
// 例: anime/foo.m2ts → anime/foo.720p.mp4
func outputPath(segment, preset string) string {
	return strings.TrimSuffix(segment, path.Ext(segment)) + "." + preset + ".mp4"
}
