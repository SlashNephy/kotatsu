package transcode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ffprogress"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/jobprogress"
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

	db       Database
	store    *kago.Store
	clock    clock.Clock
	source   *stream.RecordingSource
	inserter Inserter
	logger   *slog.Logger
	// progress は、進み具合の割合と残り時間の決め方。
	progress *ffprogress.Config
	// newEncoder は、変換に使う Encoder を作る。テストでは差し替える。
	newEncoder func() (Encoder, error)
}

var _ river.Worker[Args] = new(Worker)

// NewWorker は Worker を返す。store は、このインスタンスにマウントされたプール。
// inserter は、変換し終えたときに、チャプターを書くジョブ (ChaptersArgs) を投入する。logger には、変換の進み具合を出す。
func NewWorker(db Database, store *kago.Store, clk clock.Clock, inserter Inserter, logger *slog.Logger, newEncoder func() (Encoder, error)) *Worker {
	return &Worker{
		WorkerDefaults: river.WorkerDefaults[Args]{}, db: db, store: store, clock: clk,
		source: stream.NewRecordingSource(db, store, 0), inserter: inserter, logger: logger,
		progress: ffprogress.DefaultConfig(), newEncoder: newEncoder,
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

	return withRecordingLock(ctx, w.db, job.Args.RecordingID, func() error {
		return w.work(ctx, encoder, job, preset)
	})
}

// withRecordingLock は、録画 recordingID の advisory lock を取ってから fn を呼ぶ。セグメントをつなぐジョブと同じロックを使い、
// 取れなければ (ほかの nekonote がファイルをつないだり消したり、変換したり CM の区間を書き換えたりしていれば)、後でやり直す。
func withRecordingLock(ctx context.Context, db Database, recordingID uuid.UUID, fn func() error) error {
	conn, err := db.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire a connection: %w", err)
	}
	defer conn.Release()
	key := recordingID.String()
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

	return fn()
}

func (w *Worker) work(ctx context.Context, encoder Encoder, job *river.Job[Args], preset *kyusu.Preset) error {
	recordingID := job.Args.RecordingID
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
	ours, err := checkOutput(ctx, q, recordingID, poolID, rel, out)
	if err != nil {
		return err
	}

	opened, err := w.source.Open(ctx, recordingID)
	if err != nil {
		return fmt.Errorf("failed to open the recording: %w", err)
	}
	// 書いている途中のファイルは、ジョブごとに重ならない名前で、ここで作る (ほかのファイルを書き換えない)。
	partialFile, err := os.CreateTemp(filepath.Dir(out), filepath.Base(out)+".*"+partialSuffix)
	if err != nil {
		return fmt.Errorf("failed to create the transcoded file: %w", err)
	}
	partial := partialFile.Name()
	_ = partialFile.Close()
	defer func() { _ = os.Remove(partial) }()
	est := w.estimate(ctx, encoder, opened, &files[first])
	if err := w.encode(ctx, encoder, opened, preset, partial, w.reporter(ctx, job, recordingID, est)); err != nil {
		return err
	}
	created, err := place(partial, out, ours)
	if err != nil {
		return err
	}
	info, err := os.Stat(out)
	if err != nil {
		return fmt.Errorf("failed to stat the transcoded file: %w", err)
	}
	var upserted int64
	err = pgx.BeginFunc(ctx, w.db, func(tx pgx.Tx) error {
		upserted, err = sqlc.New(tx).UpsertTranscodedRecordingFile(ctx, sqlc.UpsertTranscodedRecordingFileParams{
			ID: id.New(), RecordingID: recordingID, PoolID: poolID, Path: rel, Size: info.Size(),
			StartedAt: recording.RecordedFrom, EndedAt: recording.RecordedUntil, Now: w.clock.Now(),
		})
		if err != nil || upserted == 0 {
			return err
		}

		return w.enqueueChapters(ctx, tx, recordingID)
	})
	if err != nil || upserted == 0 {
		// 変換している間に録画が消されたり、ほかのファイルがそのパスを使い始めたりしたら、ここで置いたファイルだけを消す。
		if created {
			_ = os.Remove(out)
		}
		if err != nil {
			return fmt.Errorf("failed to record the transcoded file: %w", err)
		}

		return river.JobCancel(fmt.Errorf("%w: %s/%s", errPathTaken, poolID, rel))
	}

	return nil
}

// enqueueChapters は、録画の CM の区間を見分けてあれば、変換した MP4 にチャプターを書くジョブを投入する (#1510)。
// 変換したての MP4 にはチャプターがないので、区間がなければ入れない。CM の区間を見分けるジョブとは別々に終わるので、
// 見分ける方も、終えたときに変換した MP4 があれば投入する (ChaptersEnqueuer)。どちらも録画ごとのロックを持っている間に入れる。
func (w *Worker) enqueueChapters(ctx context.Context, tx pgx.Tx, recordingID uuid.UUID) error {
	sections, err := cmdetect.RecordingSections(ctx, tx, recordingID)
	if err != nil || len(sections) == 0 {
		return err
	}

	return insertChapters(ctx, w.inserter, tx, recordingID)
}

// estimate は、変換の進み具合の分母を求める (#1511)。見込みのコマ数は、最初の I フレームから流した再生の時間に、
// 最初のセグメントの映像のフレームレートを掛ける。変換は、最初の I フレームから流すので、それより前の映像 (先頭を捨てた分) は含めない。
// インターレースの映像も、bwdif の send_frame (FFmpegArgs) で 1 コマから 1 コマを作るので、倍にしない
// (選べる画質は 30fps だけ。60fps で出す画質を足すなら、フィールド単位で出すので 2 倍にする)。
// 見込みのコマ数が出せなければ、入力の大きさで割る。
func (w *Worker) estimate(ctx context.Context, encoder Encoder, recording *stream.Recording, first *sqlc.RecordingFile) ffprogress.Estimate {
	est := ffprogress.Estimate{Frames: 0, Bytes: 0}
	if content, err := recording.Content(); err == nil {
		est.Bytes = content.Size()
		_ = content.Close()
	}
	duration, ok := recording.Duration()
	if !ok {
		return est
	}
	var format ts.VideoFormat
	if len(first.Video) > 0 {
		_ = json.Unmarshal(first.Video, &format)
	}
	rate := format.FrameRate
	if rate <= 0 {
		// HEVC は、映像の形式にフレームレートを読んでいない (0) ので、ffprobe で補う。
		if path, err := w.store.Path(first.PoolID, first.Path); err == nil {
			if rate, err = encoder.ProbeFrameRate(ctx, path); err != nil {
				w.logger.WarnContext(ctx, "failed to probe the frame rate; the progress uses the read position", slog.Any("error", err))
			}
		}
	}
	est.Frames = duration.Seconds() * max(rate, 0)

	return est
}

// reporter は、ジョブ job の進み具合を DB に書き戻す ffprogress.Reporter を作る関数を返す。
func (w *Worker) reporter(ctx context.Context, job *river.Job[Args], recordingID uuid.UUID, est ffprogress.Estimate) func(read func() int64) *ffprogress.Reporter {
	logger := w.logger.With(slog.Int64("job_id", job.ID), slog.String("recording_id", recordingID.String()))
	save := jobprogress.Saver(ctx, w.db, w.clock, logger, job.ID, job.Attempt)

	return func(read func() int64) *ffprogress.Reporter {
		return ffprogress.NewReporter(w.progress, est, w.clock, read, save, logger)
	}
}

// countingReader は、読んだバイト数を数える。FFmpeg に流した入力の読み位置を、進み具合の見込みが出せないときに使う。
type countingReader struct {
	r io.Reader
	n atomic.Int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))

	return n, err
}

// encode は、録画を始まりから流し、encoder で out に書く。FFmpeg の -progress の出力を、newReporter の Reporter で書き戻す。
func (w *Worker) encode(ctx context.Context, encoder Encoder, recording *stream.Recording, preset *kyusu.Preset, out string,
	newReporter func(read func() int64) *ffprogress.Reporter,
) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	written := make(chan error, 1)
	go func() {
		err := recording.Write(ctx, writer, recording.Seek(0))
		_ = writer.CloseWithError(err)
		written <- err
	}()
	src := &countingReader{r: reader, n: atomic.Int64{}}
	reporter := newReporter(src.n.Load)
	parser := ffprogress.NewParser(func(b *ffprogress.Block) { reporter.Observe(ctx, b) }, nil)
	err := encoder.Encode(ctx, src, preset, out, parser)
	// FFmpeg が終わったら、間を空けずに書き戻す。止めた後も、止める前の値のまま書く。
	parser.Flush()
	reporter.Finish(ctx)
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

// checkOutput は、出力のパス (プール poolID の rel、ファイルシステムの out) に書いてよいかを確かめ、
// この録画を前に変換した MP4 のものかどうかを返す。ほかのファイルが使っていれば、取りやめる。
func checkOutput(ctx context.Context, q *sqlc.Queries, recordingID uuid.UUID, poolID, rel, out string) (bool, error) {
	owner, err := q.GetRecordingFileOwner(ctx, sqlc.GetRecordingFileOwnerParams{PoolID: poolID, Path: rel})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// DB に記録がなくても、ほかの誰かが置いたファイルがあれば、上書きしない。
		if _, err := os.Lstat(out); err == nil {
			return false, river.JobCancel(fmt.Errorf("%w: %s/%s", errPathTaken, poolID, rel))
		}

		return false, nil
	case err != nil:
		return false, fmt.Errorf("failed to check the output path: %w", err)
	case owner.RecordingID != recordingID || owner.Kind != kindOther:
		return false, river.JobCancel(fmt.Errorf("%w: %s/%s", errPathTaken, poolID, rel))
	default:
		return true, nil
	}
}

// place は、書き終えた partial を out に置き、out を新しく作ったかを返す。
// 前に変換した MP4 (ours) なら置き換える。そうでなければ、置いている間にほかのファイルが現れても上書きしないよう、
// ハードリンクで、まだないときだけ置く。
func place(partial, out string, ours bool) (bool, error) {
	if ours {
		if err := os.Rename(partial, out); err != nil {
			return false, fmt.Errorf("failed to move the transcoded file: %w", err)
		}

		return false, nil
	}
	if err := os.Link(partial, out); errors.Is(err, os.ErrExist) {
		return false, river.JobCancel(fmt.Errorf("%w: %s", errPathTaken, out))
	} else if err != nil {
		return false, fmt.Errorf("failed to move the transcoded file: %w", err)
	}

	return true, nil
}

// outputPath は、録画の最初のセグメントのパス segment から、変換した MP4 のパスを作る。拡張子を、画質と .mp4 に変える。
// 例: anime/foo.m2ts → anime/foo.720p.mp4
func outputPath(segment, preset string) string {
	return strings.TrimSuffix(segment, path.Ext(segment)) + "." + preset + ".mp4"
}
