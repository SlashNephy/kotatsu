package autocaption

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ffprogress"
	"github.com/SlashNephy/kotatsu/internal/usecase/jobprogress"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/segjoin"
)

const (
	// kindSegment は、recording_files の、TS のセグメントの種類。
	kindSegment = "segment"
	// stateRecording は、録画中の録画の状態。
	stateRecording = "recording"
	// maxAttempts は、字幕を生成するジョブを試す回数の上限。
	maxAttempts = 3
	// timeout は、字幕を生成するジョブの時間の上限。CPU だけで動かすと、録画の尺の 3 割ほどかかる (#1682 の計測、8 スレッド)。
	// 長い録画 (映画や特番) と、遅い CPU や少ないスレッドのために、録画の変換 (#1253) と同じく長くとる。
	timeout = 6 * time.Hour
	// lockedSnooze は、同じ録画のファイルをほかでつないでいるときに、やり直すまで待つ時間。
	lockedSnooze = 10 * time.Minute
	// finalizingSnooze は、つないだファイルへの置き換えが終わっていないときに、やり直すまで待つ時間。
	finalizingSnooze = time.Minute
	// foreignKeyViolation は、PostgreSQL の外部キーの違反のエラーコード。生成している間に録画が消されたときに返る。
	foreignKeyViolation = "23503"
	// generateKey は、録画の完了のイベントの、字幕を生成するかどうかのキー。
	generateKey = "generateCaptions"
	// GeneratorVersion は、生成の仕方の版。音声の取り出し方や、時刻のそろえ方を変えたら上げる。
	GeneratorVersion = 1
)

var (
	errRecordingNotFound = errors.New("the recording no longer exists")
	errStillRecording    = errors.New("the recording is still being recorded")
	errNoSegment         = errors.New("the recording has no segment on the mounted pools")
	errPartiallyMounted  = errors.New("some segments of the recording are on pools that are not mounted")
	// errSegmentsChanged は、音声を取り出した後に、録画のセグメントが変わったこと (つないだ、名前を変えた) を表す。やり直す。
	errSegmentsChanged = errors.New("the segments of the recording changed while generating the captions")
)

// Args は、録画の音声から字幕を生成するジョブの引数。
type Args struct {
	RecordingID uuid.UUID `json:"recordingId"`
}

var _ river.JobArgsWithInsertOpts = new(Args)

func (Args) Kind() string { return "recording_auto_captions" }

func (Args) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// 音声の認識は CPU を長く使うので、同時に 1 つだけ動かす。CM の区間の見分けや変換を待たせないよう、別のキューにする。
		Queue:       jobs.QueueCaptions,
		MaxAttempts: maxAttempts,
		// 同じ録画のジョブは、待っている間に重ねて投入しない。終えたジョブには重ねてよい (生成し直せるようにする)。
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
			rivertype.JobStateRunning, rivertype.JobStateScheduled,
		}},
	}
}

// Beginner は、トランザクションを始められるもの。*pgxpool.Pool が満たす。
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Inserter は、ジョブをトランザクションの中で投入する。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// Enqueue は、録画 recordingID の字幕を生成するジョブを投入する。録画の詳細から頼まれたときに使う (設計書 6.1)。
// 全体や予約の設定にかかわらず生成する。生成し終えた録画も生成し直せ、前の字幕は置き換える。同じ録画のジョブを待っていれば、入れずに false を返す。
// 入れたら、録画の詳細が生成を待っていると出せるよう、録画の変更を知らせる。
func Enqueue(ctx context.Context, db Beginner, inserter Inserter, clk clock.Clock, recordingID uuid.UUID) (bool, error) {
	inserted := false
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		var err error
		inserted, err = insert(ctx, tx, inserter, clk, recordingID)

		return err
	})
	if err != nil {
		return false, err
	}

	return inserted, nil
}

// insert は、tx の中でジョブを入れ、入れたら録画の変更を知らせる。待っているジョブがあれば、入れずに false を返す。
func insert(ctx context.Context, tx pgx.Tx, inserter Inserter, clk clock.Clock, recordingID uuid.UUID) (bool, error) {
	results, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: Args{RecordingID: recordingID}, InsertOpts: nil}})
	if err != nil {
		return false, fmt.Errorf("failed to insert the caption generation job: %w", err)
	}
	if len(results) != 1 || results[0].UniqueSkippedAsDuplicate {
		return false, nil
	}
	if _, err := notify.Publish(ctx, tx, clk, notify.RecordingsChannel); err != nil {
		return false, err
	}

	return true, nil
}

// Enqueuer は、録画の完了のイベントを配るときに、字幕を生成するジョブを投入する。
type Enqueuer struct {
	inserter Inserter
	clock    clock.Clock
}

var _ notification.Hook = new(Enqueuer)

// NewEnqueuer は Enqueuer を返す。
func NewEnqueuer(inserter Inserter, clk clock.Clock) *Enqueuer {
	return &Enqueuer{inserter: inserter, clock: clk}
}

func (e *Enqueuer) OnEvent(ctx context.Context, tx pgx.Tx, event *notification.Event) error {
	if event.Kind != domain.NotificationKindRecordingCompleted {
		return nil
	}
	recordingID, ok := recordingIDOf(event.Payload)
	if !ok || !shouldGenerate(event.Payload) {
		// 録画の ID のないイベントでは、何もできない。ほかの通知は配りたいので、失敗にはしない。
		return nil
	}
	if _, err := insert(ctx, tx, e.inserter, e.clock, recordingID); err != nil {
		return err
	}

	return nil
}

// shouldGenerate は、録画の完了のイベントの材料 payload から、字幕を生成するかどうかを決める。
// 生成するかどうかは、ここだけで決める。録画の設定の generateCaptions は、録画を終えるときに、全体の既定値に、ルールと予約の上書きを重ねて求めてある
// (CM の区間の見分けと同じ、設計書 4.7)。TS のセグメントを書いた録画のうち、generateCaptions が true のものだけを生成する。
// generateCaptions のないイベント (足す前の版の mikan が出したもの) は、既定に合わせて生成しない。
func shouldGenerate(payload map[string]any) bool {
	generate, _ := payload[generateKey].(bool)

	return generate && segmentCount(payload) > 0
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

// Database は、字幕の生成に使う DB。*pgxpool.Pool が満たす。録画ごとの advisory lock を持つのに、接続を借りる。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

// Worker は、録画のセグメントごとに音声を取り出して字幕を作り、録画を始めた日時からの時間にそろえて DB に書く。
type Worker struct {
	river.WorkerDefaults[Args]

	db     Database
	store  *kago.Store
	clock  clock.Clock
	logger *slog.Logger
	// newTranscriber は、字幕を作る Transcriber を作る。whisper-cli かモデルがなければ ErrNotConfigured を返す。
	newTranscriber func() (Transcriber, error)
	// newExtractor は、音声を取り出す Extractor を作る。FFmpeg がなければ ErrFFmpegNotFound を返す。
	newExtractor func() (Extractor, error)
	// workDir は、取り出した WAV を置く一時ディレクトリを作る親。空なら OS の一時ディレクトリ。
	workDir string
}

var _ river.Worker[Args] = new(Worker)

// NewWorker は Worker を返す。store は、このインスタンスにマウントされたプール。newTranscriber と newExtractor は、ジョブごとに呼ぶ。
// 設定やファイルを置き直したら、nekonote を再起動しなくても、次のジョブから使える。workDir は、取り出した WAV を置く一時ディレクトリを作る親。
func NewWorker(
	db Database, store *kago.Store, clk clock.Clock, logger *slog.Logger,
	newTranscriber func() (Transcriber, error), newExtractor func() (Extractor, error), workDir string,
) *Worker {
	return &Worker{
		WorkerDefaults: river.WorkerDefaults[Args]{}, db: db, store: store, clock: clk, logger: logger,
		newTranscriber: newTranscriber, newExtractor: newExtractor, workDir: workDir,
	}
}

func (w *Worker) Timeout(*river.Job[Args]) time.Duration {
	return timeout
}

// segment は、字幕を作るセグメントのファイル。
type segment struct {
	path string
	// key は、音声を取り出した後にセグメントが変わっていないかを確かめる、保存先プールとパスと大きさ。
	key       string
	startedAt time.Time
}

// extracted は、セグメントから取り出した音声。
type extracted struct {
	segment segment
	wav     string
	// offset は、WAV の始まりの、セグメントの再生の時間での位置。
	offset time.Duration
	// size は、WAV の大きさ (バイト)。進み具合の分母に使う。
	size int64
}

// Work は、録画ごとの advisory lock を持つ間に音声を取り出し、ロックを外してから字幕を作る。
// 字幕を作るのは録画の尺の数割の時間がかかるので、その間、セグメントをつなぐジョブや名前の付け直しを待たせない。
// 書く前に、セグメントが変わっていないかを確かめ、変わっていればやり直す。
func (w *Worker) Work(ctx context.Context, job *river.Job[Args]) error {
	transcriber, err := w.newTranscriber()
	if errors.Is(err, ErrNotConfigured) {
		// 設定を直すまでやり直しても同じなので、取りやめる。録画の詳細は、この理由を見分けて、置き方を示す。
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}
	extract, err := w.newExtractor()
	if errors.Is(err, ErrFFmpegNotFound) {
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp(w.workDir, "kotatsu-captions-")
	if err != nil {
		return fmt.Errorf("failed to create a working directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	recording, audios, err := w.extract(ctx, job.Args.RecordingID, extract, dir)
	if err != nil {
		return err
	}
	cues, err := w.transcribe(ctx, job, transcriber, recording.RecordedFrom, audios)
	if err != nil {
		return err
	}

	return w.record(ctx, recording.ID, transcriber.Model(), audios, cues)
}

// extract は、録画ごとのロックを取り、セグメントの音声を dir に取り出す。セグメントをつなぐジョブと同じロックを使い、
// ほかの nekonote がファイルをつないだり消したりしている間は読まず、後でやり直す。
func (w *Worker) extract(ctx context.Context, recordingID uuid.UUID, extract Extractor, dir string) (*sqlc.Recording, []extracted, error) {
	conn, err := w.db.Acquire(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to acquire a connection: %w", err)
	}
	defer conn.Release()
	key := recordingID.String()
	locked, err := sqlc.New(conn).TryLockRecordingJoin(ctx, key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to lock the recording: %w", err)
	}
	if !locked {
		return nil, nil, river.JobSnooze(lockedSnooze)
	}
	defer func() {
		if _, err := sqlc.New(conn).UnlockRecordingJoin(context.WithoutCancel(ctx), key); err != nil {
			// 接続を閉じれば、ロックも外れる。
			_ = conn.Conn().Close(context.WithoutCancel(ctx))
		}
	}()
	// つないだ後、つないだファイルに置き換えるまでは、ディスクの最初のセグメントがまだつなぐ前のままなので、読まない (#1719)。
	if pending, err := segjoin.FinalizePending(ctx, conn, recordingID); err != nil {
		return nil, nil, err
	} else if pending {
		return nil, nil, river.JobSnooze(finalizingSnooze)
	}

	q := sqlc.New(w.db)
	recording, err := q.GetRecording(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, river.JobCancel(errRecordingNotFound)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the recording: %w", err)
	}
	if recording.State == stateRecording {
		return nil, nil, river.JobCancel(errStillRecording)
	}
	sid, err := q.GetRecordingSID(ctx, recordingID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get the service of the recording: %w", err)
	}
	segments, err := w.segments(ctx, recordingID)
	if err != nil {
		return nil, nil, err
	}
	audios := make([]extracted, 0, len(segments))
	for i, s := range segments {
		wav := filepath.Join(dir, strconv.Itoa(i)+".wav")
		offset, err := extract(ctx, s.path, serviceID(sid), wav)
		if errors.Is(err, ErrNoAudio) {
			// 音声のないセグメント (チューナーが音を出す前に閉じたものなど) は、字幕にするものがないので飛ばす。
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		info, err := os.Stat(wav)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to stat the extracted audio: %w", err)
		}
		audios = append(audios, extracted{segment: s, wav: wav, offset: offset, size: info.Size()})
	}

	return &recording, audios, nil
}

// segments は、録画のファイルから TS のセグメントを、始まりの順に選ぶ。
// 一部のセグメントだけで作ると、残りの字幕が抜けたまま記録されるので、マウントされていないプールのセグメントがあれば、取りやめる。
func (w *Worker) segments(ctx context.Context, recordingID uuid.UUID) ([]segment, error) {
	files, err := sqlc.New(w.db).ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recording files: %w", err)
	}
	segments := []segment{}
	unmounted := 0
	for i := range files {
		file := &files[i]
		if file.Kind != kindSegment || file.Size == 0 {
			continue
		}
		path, err := w.store.Path(file.PoolID, file.Path)
		if errors.Is(err, kago.ErrPoolNotMounted) {
			unmounted++

			continue
		}
		if err != nil {
			return nil, err
		}
		segments = append(segments, segment{path: path, key: fmt.Sprintf("%s/%s#%d", file.PoolID, file.Path, file.Size), startedAt: file.StartedAt})
	}
	if len(segments) == 0 {
		return nil, river.JobCancel(errNoSegment)
	}
	if unmounted > 0 {
		return nil, river.JobCancel(fmt.Errorf("%w: %d of %d segments", errPartiallyMounted, unmounted, unmounted+len(segments)))
	}

	return segments, nil
}

// transcribe は、取り出した音声から順に字幕を作り、録画を始めた日時からの時間にそろえて 1 つにまとめる。
// 時刻は、CM の区間と同じく、セグメントを書き始めた日時の、録画を始めた日時からの時間に、セグメントの再生の時間を足したもの。
// 途切れた区間も時間に含めて数え、再生の位置にするのは画面に任せる。
func (w *Worker) transcribe(ctx context.Context, job *river.Job[Args], transcriber Transcriber, recordedFrom time.Time, audios []extracted) ([]Cue, error) {
	progress := w.progress(ctx, job, audios)
	defer progress.reporter.Finish(ctx)
	cues := []Cue{}
	var done int64
	for _, audio := range audios {
		found, err := transcriber.Transcribe(ctx, audio.wav, func(ratio float64) {
			progress.read.Store(done + int64(ratio*float64(audio.size)))
			progress.reporter.Observe(ctx, &ffprogress.Block{Frame: -1, TotalSize: -1, DropFrames: -1, Bitrate: "", Speed: "", End: false})
		})
		if err != nil {
			return nil, err
		}
		done += audio.size
		base := audio.segment.startedAt.Sub(recordedFrom) + audio.offset
		for _, c := range found {
			start, end := max(base+c.Start, 0), base+c.End
			if end <= start {
				// 最初の I フレームより前 (再生に入らない所) で終わる字幕は捨てる。
				continue
			}
			cues = append(cues, Cue{Start: start, End: end, Text: c.Text})
		}
	}
	progress.read.Store(done)
	progress.reporter.Observe(ctx, &ffprogress.Block{Frame: -1, TotalSize: -1, DropFrames: -1, Bitrate: "", Speed: "", End: true})
	// 引き継ぎで重ねて録ったセグメントでは、同じ台詞が両方に入るが、どちらも残す。並びだけを始まりの順にそろえる。
	slices.SortStableFunc(cues, func(a, b Cue) int { return cmp.Compare(a.Start, b.Start) })

	return cues, nil
}

// captionProgress は、字幕を作る進み具合。取り出した WAV の大きさの合計を分母に、whisper-cli が出す割合を、読んだバイト数に換えて数える。
type captionProgress struct {
	reporter *ffprogress.Reporter
	read     atomic.Int64
}

func (w *Worker) progress(ctx context.Context, job *river.Job[Args], audios []extracted) *captionProgress {
	var total int64
	for _, a := range audios {
		total += a.size
	}
	logger := w.logger.With(slog.Int64("job_id", job.ID), slog.String("recording_id", job.Args.RecordingID.String()))
	p := &captionProgress{reporter: nil, read: atomic.Int64{}}
	p.reporter = ffprogress.NewReporter(ffprogress.DefaultConfig(), ffprogress.Estimate{Frames: 0, Bytes: total}, w.clock, p.read.Load,
		jobprogress.Saver(ctx, w.db, w.clock, logger, job.ID, job.Attempt), logger)

	return p
}

// storedCue は、DB の cues (JSONB) に残す字幕の形。
type storedCue struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Text    string `json:"text"`
}

// record は、セグメントが音声を取り出したときのままなら、字幕を前の字幕と置き換えて書く。変わっていれば、やり直す。
func (w *Worker) record(ctx context.Context, recordingID uuid.UUID, model string, audios []extracted, cues []Cue) error {
	stored := make([]storedCue, 0, len(cues))
	for _, c := range cues {
		stored = append(stored, storedCue{StartMs: c.Start.Milliseconds(), EndMs: c.End.Milliseconds(), Text: c.Text})
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to encode the captions: %w", err)
	}
	err = pgx.BeginFunc(ctx, w.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if _, err := q.GetRecording(ctx, recordingID); errors.Is(err, pgx.ErrNoRows) {
			return river.JobCancel(errRecordingNotFound)
		} else if err != nil {
			return fmt.Errorf("failed to get the recording: %w", err)
		}
		if err := w.ensureUnchanged(ctx, tx, recordingID, audios); err != nil {
			return err
		}
		err := q.UpsertRecordingAutoCaptions(ctx, sqlc.UpsertRecordingAutoCaptionsParams{
			RecordingID: recordingID, Model: model, GeneratorVersion: GeneratorVersion, Now: w.clock.Now(), Cues: data,
		})
		if err != nil {
			return fmt.Errorf("failed to record the captions: %w", err)
		}
		// 録画の詳細と、プレイヤーの字幕を読み直させる。
		_, err = notify.Publish(ctx, tx, w.clock, notify.RecordingsChannel)

		return err
	})
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return river.JobCancel(errRecordingNotFound)
	}

	return err
}

// ensureUnchanged は、録画のセグメントが、音声を取り出したときから変わっていないことを確かめる。
// 音声のないセグメントは飛ばしたので、取り出したセグメントがどれも今のセグメントにあれば、変わっていないとみなす。
func (w *Worker) ensureUnchanged(ctx context.Context, db sqlc.DBTX, recordingID uuid.UUID, audios []extracted) error {
	files, err := sqlc.New(db).ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return fmt.Errorf("failed to list the recording files: %w", err)
	}
	keys := make([]string, 0, len(files))
	for _, file := range files {
		if file.Kind == kindSegment {
			keys = append(keys, fmt.Sprintf("%s/%s#%d", file.PoolID, file.Path, file.Size))
		}
	}
	for _, a := range audios {
		if !slices.Contains(keys, a.segment.key) {
			return errSegmentsChanged
		}
	}

	return nil
}

// serviceID は、DB のサービス ID (sid) を、TS の program_number にする。範囲の外なら 0 にし、映像のある最初の番組を読ませる。
func serviceID(sid int64) uint16 {
	if sid < 0 || sid > math.MaxUint16 {
		return 0
	}

	return uint16(sid)
}
