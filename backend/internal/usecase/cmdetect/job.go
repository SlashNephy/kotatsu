package cmdetect

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
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
	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
	"github.com/SlashNephy/kotatsu/internal/usecase/jobprogress"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/segjoin"
)

const (
	// kindSegment は、recording_files の、TS のセグメントの種類。
	kindSegment = "segment"
	// stateRecording は、録画中の録画の状態。
	stateRecording = "recording"
	// detectMaxAttempts は、CM の区間を見分けるジョブを試す回数の上限。
	detectMaxAttempts = 3
	// detectTimeout は、CM の区間を見分けるジョブの時間の上限。30 分の録画で 1 分もかからないが、長い録画や遅いディスクのために長めにとる。
	detectTimeout = 30 * time.Minute
	// lockedSnooze は、同じ録画のファイルをほかでつないでいるときに、やり直すまで待つ時間。
	lockedSnooze = 10 * time.Minute
	// finalizingSnooze は、つないだファイルへの置き換えが終わっていないときに、やり直すまで待つ時間。置き換えはすぐ終わるので、短くする (#1719)。
	finalizingSnooze = time.Minute
	// foreignKeyViolation は、PostgreSQL の外部キーの違反のエラーコード。見分けている間に録画が消されたときに返る。
	foreignKeyViolation = "23503"
	// detectKey は、録画の完了のイベントの、CM の区間を見分けるかどうかのキー。
	detectKey = "detectCm"
	// mergeGap は、セグメントをまたいで続く CM の区間を 1 つにまとめる間隔。引き継ぎで重ねて録った区間では、同じ CM が両方に入る。
	mergeGap = time.Second
)

var (
	errRecordingNotFound = errors.New("the recording no longer exists")
	errStillRecording    = errors.New("the recording is still being recorded")
	errNoSegment         = errors.New("the recording has no segment on the mounted pools")
	errPartiallyMounted  = errors.New("some segments of the recording are on pools that are not mounted")
)

// DetectArgs は、録画の CM の区間を見分けるジョブの引数。
type DetectArgs struct {
	RecordingID uuid.UUID `json:"recordingId"`
}

var _ river.JobArgsWithInsertOpts = new(DetectArgs)

func (DetectArgs) Kind() string { return "recording_cm_detect" }

func (DetectArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// 録画を読み通すので、同時に 1 つだけ動かす。画像やセグメントをつなぐジョブ (media のキュー) を待たせないよう、別のキューにする。
		Queue:       jobs.QueueCM,
		MaxAttempts: detectMaxAttempts,
		// 同じ録画のジョブは、待っている間に重ねて投入しない。終えたジョブには重ねてよい (見分け直せるようにする)。
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

// Enqueue は、録画 recordingID の CM の区間を見分けるジョブを投入する。録画の詳細から頼まれたときに使う (設計書 6.1)。
// 全体や予約の設定にかかわらず見分ける。見分け終えた録画も見分け直せ、前の区間は置き換える。同じ録画のジョブを待っていれば、入れずに false を返す。
// 入れたら、録画の詳細が見分けを待っていると出せるよう、録画の変更を知らせる (#1571)。
func Enqueue(ctx context.Context, db Beginner, inserter Inserter, clk clock.Clock, recordingID uuid.UUID) (bool, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to begin a transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	results, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: DetectArgs{RecordingID: recordingID}, InsertOpts: nil}})
	if err != nil {
		return false, fmt.Errorf("failed to insert the CM detection job: %w", err)
	}
	inserted := len(results) == 1 && !results[0].UniqueSkippedAsDuplicate
	if !inserted {
		return false, nil
	}
	if _, err := notify.Publish(ctx, tx, clk, notify.RecordingsChannel); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("failed to commit the CM detection job: %w", err)
	}

	return true, nil
}

// Inserter は、ジョブをトランザクションの中で投入する。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// Enqueuer は、録画の完了のイベントを配るときに、CM の区間を見分けるジョブを投入する。
// 入れたら、録画の詳細が見分けを待っていると出せるよう、録画の変更を知らせる (#1571)。
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
	if !ok || !shouldDetect(event.Payload) {
		// 録画の ID のないイベントでは、何もできない。ほかの通知は配りたいので、失敗にはしない。
		return nil
	}
	results, err := e.inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: DetectArgs{RecordingID: recordingID}, InsertOpts: nil}})
	if err != nil {
		return fmt.Errorf("failed to enqueue the CM detection: %w", err)
	}
	if len(results) != 1 || results[0].UniqueSkippedAsDuplicate {
		return nil
	}
	if _, err := notify.Publish(ctx, tx, e.clock, notify.RecordingsChannel); err != nil {
		return err
	}

	return nil
}

// shouldDetect は、録画の完了のイベントの材料 payload から、録画の CM の区間を見分けるかどうかを決める。
// 見分けるかどうかは、ここだけで決める。録画の設定の detectCm (設計書 6.1) は、録画を終えるときに、全体の既定値に、
// ルールと予約の上書きを重ねて求めてある (予約やルールで選んでいればその値、なければ録画を終えるときの全体の設定)。
// TS のセグメントを書いた録画のうち、detectCm が true のものだけを見分ける。detectCm のないイベント (足す前の版の mikan が出したもの) は、既定に合わせて見分けない。
func shouldDetect(payload map[string]any) bool {
	detect, _ := payload[detectKey].(bool)

	return detect && segmentCount(payload) > 0
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

// Database は、CM の区間を見分けるのに使う DB。*pgxpool.Pool が満たす。録画ごとの advisory lock を持つのに、接続を借りる。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

// DetectedHook は、CM の区間を書いたトランザクションの中で、見分け終えたことを受け取る。
// 変換した MP4 にチャプターを書くジョブ (transcode.ChaptersEnqueuer、#1510) を、区間と一緒に投入するのに使う。
// 区間を書くのは録画ごとのロックを持っている間なので、受け取った側も、そのロックの中にいる。
type DetectedHook interface {
	OnDetected(ctx context.Context, tx pgx.Tx, recordingID uuid.UUID) error
}

// DetectedHooks は、見分け終えたことを、並べた順にどれにも渡す。
type DetectedHooks []DetectedHook

var _ DetectedHook = DetectedHooks{}

// OnDetected は、並べた順に OnDetected を呼ぶ。失敗したら、そこで止めて返す。
func (hooks DetectedHooks) OnDetected(ctx context.Context, tx pgx.Tx, recordingID uuid.UUID) error {
	for _, hook := range hooks {
		if err := hook.OnDetected(ctx, tx, recordingID); err != nil {
			return err
		}
	}

	return nil
}

// DetectWorker は、録画のセグメントごとに CM の区間を見分け、録画を始めた日時からの時間にそろえて DB に書く。
type DetectWorker struct {
	river.WorkerDefaults[DetectArgs]

	db    Database
	store *kago.Store
	clock clock.Clock
	// hook は、見分け終えたことを受け取る。nil なら呼ばない。
	hook   DetectedHook
	logger *slog.Logger
	// newDetector は、見分けに使う Detector を作る。テストでは差し替える。
	newDetector func(ctx context.Context) (Detector, error)
}

var _ river.Worker[DetectArgs] = new(DetectWorker)

// NewDetectWorker は DetectWorker を返す。store は、このインスタンスにマウントされたプール。
// lowerer は、FFmpeg を最低の優先度で動かす (#1511)。hook は、区間を書いたトランザクションの中で呼ぶ。nil なら呼ばない。
// logger には、見分けの進み具合を出す。
// 見分け方は、ジョブごとに、管理者が選んだもの (#1505) にする。
func NewDetectWorker(db Database, store *kago.Store, clk clock.Clock, lowerer *procprio.Lowerer, hook DetectedHook, logger *slog.Logger) *DetectWorker {
	return newDetectWorker(db, store, clk, hook, logger, func(ctx context.Context) (Detector, error) {
		name, err := selectedDetector(ctx, db)
		if err != nil {
			return nil, err
		}
		if name == LogoDetectorName {
			return NewLogoDetector(lowerer)
		}

		return NewFFmpegDetector(lowerer)
	})
}

// selectedDetector は、管理者が選んだ見分け方の名前を返す。設定の行がまだなければ、既定の FFmpegDetector にする。
func selectedDetector(ctx context.Context, db sqlc.DBTX) (string, error) {
	row, err := sqlc.New(db).GetSystemSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return FFmpegDetectorName, nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get the system settings: %w", err)
	}

	return row.CMDetector, nil
}

func newDetectWorker(
	db Database, store *kago.Store, clk clock.Clock, hook DetectedHook, logger *slog.Logger, newDetector func(ctx context.Context) (Detector, error),
) *DetectWorker {
	return &DetectWorker{
		WorkerDefaults: river.WorkerDefaults[DetectArgs]{}, db: db, store: store, clock: clk, hook: hook, logger: logger, newDetector: newDetector,
	}
}

func (w *DetectWorker) Timeout(*river.Job[DetectArgs]) time.Duration {
	return detectTimeout
}

// Work は、録画ごとの advisory lock を取ってから、CM の区間を見分ける。
// セグメントをつなぐジョブと同じロックを使い、ほかの nekonote がファイルをつないだり消したりしている間は読まず、後でやり直す。
func (w *DetectWorker) Work(ctx context.Context, job *river.Job[DetectArgs]) error {
	detector, err := w.newDetector(ctx)
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

	// つないだ後、つないだファイルに置き換えるまでは、ディスクの最初のセグメントがまだつなぐ前のままなので、読まない (#1719)。
	if pending, err := segjoin.FinalizePending(ctx, conn, job.Args.RecordingID); err != nil {
		return err
	} else if pending {
		return river.JobSnooze(finalizingSnooze)
	}

	return w.work(ctx, detector, job)
}

func (w *DetectWorker) work(ctx context.Context, detector Detector, job *river.Job[DetectArgs]) error {
	recordingID := job.Args.RecordingID
	q := sqlc.New(w.db)
	recording, err := q.GetRecording(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(errRecordingNotFound)
	}
	if err != nil {
		return fmt.Errorf("failed to get the recording: %w", err)
	}
	if recording.State == stateRecording {
		return river.JobCancel(errStillRecording)
	}
	sid, err := q.GetRecordingSID(ctx, recordingID)
	if err != nil {
		return fmt.Errorf("failed to get the service of the recording: %w", err)
	}
	files, err := q.ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return fmt.Errorf("failed to list the recording files: %w", err)
	}
	segments, err := w.segments(files)
	if err != nil {
		return err
	}

	progress := w.progress(ctx, job, segments)
	// 最後の FFmpeg が終わったら、間を空けずに書き戻す。止めたり失敗したりした後も、止める前の値のまま書く。
	defer progress.reporter.Finish(ctx)
	sections := []Section{}
	for i, segment := range segments {
		found, err := detector.Detect(ctx, &Input{Path: segment.path, ServiceID: serviceID(sid), Progress: progress.observer(ctx, segments, i)})
		if errors.Is(err, ErrNoVideo) {
			// 映像のないセグメント (チューナーが絵を出す前に閉じたものなど) は、見分けるものがないので飛ばす。
			continue
		}
		if err != nil {
			return err
		}
		// 区間は、録画を始めた日時からの時間にそろえる。途切れた区間も時間に含めて数え、再生の位置にするのは画面に任せる。
		base := segment.startedAt.Sub(recording.RecordedFrom)
		for _, s := range found {
			sections = append(sections, Section{Start: max(base+s.Start, 0), End: max(base+s.End, 0)})
		}
	}
	progress.reporter.Finish(ctx)

	return w.record(ctx, detector, recordingID, mergeSections(sections))
}

// detectProgress は、セグメントを順に見分ける進み具合 (#1511)。
// 見分けの FFmpeg は、シーンの切り替わったコマだけを出力するので、frame は、見込みの出せない数 (シーンの切り替わりの数) になる。
// そこで、読んだバイト数を、セグメントの大きさの合計で割る。I フレームだけを 1 本のスレッドでデコードし、読むのがほぼ律速なので、
// デマクサが先に読み進む分は、パイプとバッファの数十 KB にとどまる。
type detectProgress struct {
	reporter *ffprogress.Reporter
	read     atomic.Int64
}

func (w *DetectWorker) progress(ctx context.Context, job *river.Job[DetectArgs], segments []segment) *detectProgress {
	var total int64
	for _, s := range segments {
		total += s.size
	}
	logger := w.logger.With(slog.Int64("job_id", job.ID), slog.String("recording_id", job.Args.RecordingID.String()))
	p := &detectProgress{reporter: nil, read: atomic.Int64{}}
	p.reporter = ffprogress.NewReporter(ffprogress.DefaultConfig(), ffprogress.Estimate{Frames: 0, Bytes: total}, w.clock, p.read.Load,
		jobprogress.Saver(ctx, w.db, w.clock, logger, job.ID, job.Attempt), logger)

	return p
}

// observer は、segments の i 番目のセグメントの -progress のまとまりを、録画の全体の読み位置にして読む。
// progress=end は、最後のセグメントのものだけを、終わりとみなす。
func (p *detectProgress) observer(ctx context.Context, segments []segment, i int) func(b *ffprogress.Block, read int64) {
	var base int64
	for _, s := range segments[:i] {
		base += s.size
	}
	last := i == len(segments)-1

	return func(b *ffprogress.Block, read int64) {
		block := *b
		block.End = b.End && last
		p.read.Store(base + read)
		p.reporter.Observe(ctx, &block)
	}
}

// serviceID は、DB のサービス ID (sid) を、TS の program_number にする。範囲の外なら 0 にし、
// ts.CueScanner に、映像のある最初の番組を読ませる (FFmpeg が読む最初の映像と音声と同じ)。
func serviceID(sid int64) uint16 {
	if sid < 0 || sid > math.MaxUint16 {
		return 0
	}

	return uint16(sid)
}

// segment は、見分けるセグメントのファイル。
type segment struct {
	path      string
	startedAt time.Time
	// size は、ファイルの大きさ (バイト)。進み具合に使う。
	size int64
}

// segments は、録画のファイルから TS のセグメントを、始まりの順に選ぶ。
// 一部のセグメントだけで見分けると、残りの CM が抜けたまま記録されるので、マウントされていないプールのセグメントがあれば、取りやめる。
func (w *DetectWorker) segments(files []sqlc.RecordingFile) ([]segment, error) {
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
		segments = append(segments, segment{path: path, startedAt: file.StartedAt, size: file.Size})
	}
	if len(segments) == 0 {
		return nil, river.JobCancel(errNoSegment)
	}
	if unmounted > 0 {
		return nil, river.JobCancel(fmt.Errorf("%w: %d of %d segments", errPartiallyMounted, unmounted, unmounted+len(segments)))
	}

	return segments, nil
}

// mergeSections は、区間を始まりの順に並べ、重なるか mergeGap より近い区間を 1 つにまとめる。
func mergeSections(sections []Section) []Section {
	slices.SortFunc(sections, func(a, b Section) int { return cmp.Compare(a.Start, b.Start) })
	merged := make([]Section, 0, len(sections))
	for _, s := range sections {
		if s.End <= s.Start {
			continue
		}
		if n := len(merged); n > 0 && s.Start-merged[n-1].End < mergeGap {
			merged[n-1].End = max(merged[n-1].End, s.End)

			continue
		}
		merged = append(merged, s)
	}

	return merged
}

// record は、見分けた記録と区間を、同じ見分け方の前の区間と置き換えて書く。
func (w *DetectWorker) record(ctx context.Context, detector Detector, recordingID uuid.UUID, sections []Section) error {
	err := pgx.BeginFunc(ctx, w.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		err := q.UpsertRecordingCMDetection(ctx, sqlc.UpsertRecordingCMDetectionParams{
			RecordingID: recordingID, Detector: detector.Name(), DetectorVersion: detector.Version(), Now: w.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to record the CM detection: %w", err)
		}
		if err := q.DeleteRecordingCMSections(ctx, sqlc.DeleteRecordingCMSectionsParams{RecordingID: recordingID, Detector: detector.Name()}); err != nil {
			return fmt.Errorf("failed to delete the previous CM sections: %w", err)
		}
		for _, s := range sections {
			err := q.InsertRecordingCMSection(ctx, sqlc.InsertRecordingCMSectionParams{
				RecordingID: recordingID, Detector: detector.Name(), StartOffsetMs: s.Start.Milliseconds(), EndOffsetMs: s.End.Milliseconds(),
			})
			if err != nil {
				return fmt.Errorf("failed to record a CM section: %w", err)
			}
		}
		// 録画の詳細と、プレイヤーのシークバーの区間を読み直させる (#1571)。
		if _, err := notify.Publish(ctx, tx, w.clock, notify.RecordingsChannel); err != nil {
			return err
		}
		if w.hook == nil {
			return nil
		}

		return w.hook.OnDetected(ctx, tx, recordingID)
	})
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return river.JobCancel(errRecordingNotFound)
	}

	return err
}

// RecordingSections は、録画 recordingID の CM の区間を、始まりの順に返す。見分けていなければ空のスライスを返す。
// 区間は、録画を始めた日時からの時間。見分け方が複数あれば、最後に見分けた見分け方の区間を返す (#1505)。
// 管理者が見分け方を替えた後に見分け直した録画は新しい見分け方の区間に、見分け直していない録画は前の見分け方の区間になる。
func RecordingSections(ctx context.Context, db sqlc.DBTX, recordingID uuid.UUID) ([]Section, error) {
	q := sqlc.New(db)
	latest, err := q.GetLatestRecordingCMDetection(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return []Section{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the latest CM detection of the recording: %w", err)
	}
	rows, err := q.ListRecordingCMSections(ctx, sqlc.ListRecordingCMSectionsParams{RecordingID: recordingID, Detector: latest.Detector})
	if err != nil {
		return nil, fmt.Errorf("failed to list the CM sections of the recording: %w", err)
	}
	sections := make([]Section, 0, len(rows))
	for _, row := range rows {
		sections = append(sections, Section{Start: time.Duration(row.StartOffsetMs) * time.Millisecond, End: time.Duration(row.EndOffsetMs) * time.Millisecond})
	}

	return sections, nil
}
