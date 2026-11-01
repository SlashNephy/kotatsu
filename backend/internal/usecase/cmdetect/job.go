package cmdetect

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
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
	// foreignKeyViolation は、PostgreSQL の外部キーの違反のエラーコード。見分けている間に録画が消されたときに返る。
	foreignKeyViolation = "23503"
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

// Inserter は、ジョブをトランザクションの中で投入する。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// Enqueuer は、録画の完了のイベントを配るときに、CM の区間を見分けるジョブを投入する。
type Enqueuer struct {
	inserter Inserter
}

var _ notification.Hook = new(Enqueuer)

// NewEnqueuer は Enqueuer を返す。
func NewEnqueuer(inserter Inserter) *Enqueuer {
	return &Enqueuer{inserter: inserter}
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
	if _, err := e.inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: DetectArgs{RecordingID: recordingID}, InsertOpts: nil}}); err != nil {
		return fmt.Errorf("failed to enqueue the CM detection: %w", err)
	}

	return nil
}

// shouldDetect は、録画の完了のイベントの材料 payload から、録画の CM の区間を見分けるかどうかを決める。
// 見分けるかどうかは、ここだけで決める。いまは、TS のセグメントを書いた録画はすべて見分ける。
func shouldDetect(payload map[string]any) bool {
	return segmentCount(payload) > 0
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

// DetectWorker は、録画のセグメントごとに CM の区間を見分け、録画を始めた日時からの時間にそろえて DB に書く。
type DetectWorker struct {
	river.WorkerDefaults[DetectArgs]

	db    Database
	store *kago.Store
	clock clock.Clock
	// newDetector は、見分けに使う Detector を作る。テストでは差し替える。
	newDetector func() (Detector, error)
}

var _ river.Worker[DetectArgs] = new(DetectWorker)

// NewDetectWorker は DetectWorker を返す。store は、このインスタンスにマウントされたプール。
func NewDetectWorker(db Database, store *kago.Store, clk clock.Clock) *DetectWorker {
	return newDetectWorker(db, store, clk, func() (Detector, error) { return NewFFmpegDetector() })
}

func newDetectWorker(db Database, store *kago.Store, clk clock.Clock, newDetector func() (Detector, error)) *DetectWorker {
	return &DetectWorker{WorkerDefaults: river.WorkerDefaults[DetectArgs]{}, db: db, store: store, clock: clk, newDetector: newDetector}
}

func (w *DetectWorker) Timeout(*river.Job[DetectArgs]) time.Duration {
	return detectTimeout
}

// Work は、録画ごとの advisory lock を取ってから、CM の区間を見分ける。
// セグメントをつなぐジョブと同じロックを使い、ほかの nekonote がファイルをつないだり消したりしている間は読まず、後でやり直す。
func (w *DetectWorker) Work(ctx context.Context, job *river.Job[DetectArgs]) error {
	detector, err := w.newDetector()
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

	return w.work(ctx, detector, job.Args.RecordingID)
}

func (w *DetectWorker) work(ctx context.Context, detector Detector, recordingID uuid.UUID) error {
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

	sections := []Section{}
	for _, segment := range segments {
		found, err := detector.Detect(ctx, &Input{Path: segment.path, ServiceID: serviceID(sid)})
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

	return w.record(ctx, detector, recordingID, mergeSections(sections))
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
		segments = append(segments, segment{path: path, startedAt: file.StartedAt})
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

		return nil
	})
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return river.JobCancel(errRecordingNotFound)
	}

	return err
}

// RecordingSections は、録画 recordingID の CM の区間を、始まりの順に返す。見分けていなければ空のスライスを返す。
// 区間は、録画を始めた日時からの時間。見分け方を足したら、どの見分け方の区間を返すかをここで決める。
func RecordingSections(ctx context.Context, db sqlc.DBTX, recordingID uuid.UUID) ([]Section, error) {
	rows, err := sqlc.New(db).ListRecordingCMSections(ctx, sqlc.ListRecordingCMSectionsParams{RecordingID: recordingID, Detector: FFmpegDetectorName})
	if err != nil {
		return nil, fmt.Errorf("failed to list the CM sections of the recording: %w", err)
	}
	sections := make([]Section, 0, len(rows))
	for _, row := range rows {
		sections = append(sections, Section{Start: time.Duration(row.StartOffsetMs) * time.Millisecond, End: time.Duration(row.EndOffsetMs) * time.Millisecond})
	}

	return sections, nil
}
