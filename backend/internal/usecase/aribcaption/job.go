// Package aribcaption は、録画の TS から ARIB 字幕の文を取り出して DB に残す (設計書 6.1、7.4、#2217)。
// 残した文は、自動生成の字幕と同じく、字幕の文で録画を探すのに使う (設計書 3.5、#2211)。
package aribcaption

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"slices"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/drcstable"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/segjoin"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// kindSegment は、recording_files の、TS のセグメントの種類。
	kindSegment = "segment"
	// stateRecording は、録画中の録画の状態。
	stateRecording = "recording"
	// maxAttempts は、字幕の文を取り出すジョブを試す回数の上限。
	maxAttempts = 3
	// timeout は、字幕の文を取り出すジョブの時間の上限。録画を読み通すだけなので、CM の区間の見分けより短くとる。
	timeout = time.Hour
	// lockedSnooze は、同じ録画のファイルをほかでつないでいるときに、やり直すまで待つ時間。
	lockedSnooze = 10 * time.Minute
	// finalizingSnooze は、つないだファイルへの置き換えが終わっていないときに、やり直すまで待つ時間。
	finalizingSnooze = time.Minute
	// foreignKeyViolation は、PostgreSQL の外部キーの違反のエラーコード。取り出している間に録画が消されたときに返る。
	foreignKeyViolation = "23503"
	// readBufferSize は、セグメントを読むときのバッファの大きさ。
	readBufferSize = 1 << 20
	// ExtractorVersion は、取り出し方のバージョン。字幕の文の読み方や、区間の決め方を変えたら上げる。
	ExtractorVersion = 1
)

var (
	errRecordingNotFound = errors.New("the recording no longer exists")
	errStillRecording    = errors.New("the recording is still being recorded")
	errNoSegment         = errors.New("the recording has no segment on the mounted pools")
	errPartiallyMounted  = errors.New("some segments of the recording are on pools that are not mounted")
	// errSegmentsChanged は、読んでいる間に、録画のセグメントが変わったこと (つないだ、名前を変えた) を表す。やり直す。
	errSegmentsChanged = errors.New("the segments of the recording changed while extracting the captions")
)

// Args は、録画の TS から ARIB 字幕の文を取り出すジョブの引数。
type Args struct {
	RecordingID uuid.UUID `json:"recordingId"`
}

var _ river.JobArgsWithInsertOpts = new(Args)

func (Args) Kind() string { return "recording_arib_captions" }

func (Args) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// 録画を読み通すので、代表画像やセグメントをつなぐジョブと同じく、重いジョブのキューで 1 つずつ動かす。
		Queue:       jobs.QueueMedia,
		MaxAttempts: maxAttempts,
		// 同じ録画のジョブは、待っている間に重ねて投入しない。終えたジョブには重ねてよい (取り出し直せるようにする)。
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

// Enqueuer は、録画の完了のイベントを配信するときに、字幕の文を取り出すジョブを投入する。
// ARIB 字幕は放送に入っているものを読むだけで、字幕の自動生成と違って CPU を長く使わないので、設定にかかわらず、
// TS のセグメントを書いた録画のすべてで取り出す。
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
	if !ok || segmentCount(event.Payload) == 0 {
		// 録画の ID のないイベントでは、何もできない。ほかの通知は配信したいので、失敗にはしない。
		return nil
	}
	if _, err := e.inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: Args{RecordingID: recordingID}, InsertOpts: nil}}); err != nil {
		return fmt.Errorf("failed to insert the ARIB caption extraction job: %w", err)
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

// Database は、字幕の文の取り出しに使う DB。*pgxpool.Pool が満たす。録画ごとの advisory lock を持つのに、接続を借りる。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

// Worker は、録画のセグメントを読み、ARIB 字幕の文を、録画を始めた日時からの時間にそろえて DB に書く。
type Worker struct {
	river.WorkerDefaults[Args]

	db     Database
	store  *kago.Store
	clock  clock.Clock
	logger *slog.Logger
}

var _ river.Worker[Args] = new(Worker)

// NewWorker は Worker を返す。store は、このインスタンスにマウントされたプール。
func NewWorker(db Database, store *kago.Store, clk clock.Clock, logger *slog.Logger) *Worker {
	return &Worker{WorkerDefaults: river.WorkerDefaults[Args]{}, db: db, store: store, clock: clk, logger: logger}
}

func (w *Worker) Timeout(*river.Job[Args]) time.Duration {
	return timeout
}

// segment は、字幕の文を取り出すセグメントのファイル。
type segment struct {
	path string
	// key は、書く前にセグメントが変わっていないかを確認する、保存先プールとパスと大きさ。
	key       string
	startedAt time.Time
}

// Work は、録画ごとの advisory lock を持つ間にセグメントを読み、字幕の文を書く。
// セグメントをつなぐジョブと同じロックを使い、ほかの nekonote がファイルをつないだり消したりしている間は読まず、後でやり直す。
func (w *Worker) Work(ctx context.Context, job *river.Job[Args]) error {
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

	recording, sid, segments, err := w.load(ctx, job.Args.RecordingID)
	if err != nil {
		return err
	}
	replacements, err := w.replacements(ctx)
	if err != nil {
		return err
	}
	cues := []Cue{}
	for _, s := range segments {
		found, err := w.scan(ctx, s, serviceID(sid), replacements, s.startedAt.Sub(recording.RecordedFrom))
		if err != nil {
			return err
		}
		cues = append(cues, found...)
	}
	// 引き継ぎで重ねて録ったセグメントでは、同じ字幕が両方に入るが、どちらも残す。並びだけを始まりの順にそろえる。
	slices.SortStableFunc(cues, func(a, b Cue) int { return cmp.Compare(a.Start, b.Start) })

	return w.record(ctx, recording.ID, segments, cues)
}

// load は、録画と、録画したサービスのサービス ID と、TS のセグメントを読む。
func (w *Worker) load(ctx context.Context, recordingID uuid.UUID) (*sqlc.Recording, int64, []segment, error) {
	q := sqlc.New(w.db)
	recording, err := q.GetRecording(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, nil, river.JobCancel(errRecordingNotFound)
	}
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to get the recording: %w", err)
	}
	if recording.State == stateRecording {
		return nil, 0, nil, river.JobCancel(errStillRecording)
	}
	sid, err := q.GetRecordingSID(ctx, recordingID)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to get the service of the recording: %w", err)
	}
	segments, err := w.segments(ctx, recordingID)
	if err != nil {
		return nil, 0, nil, err
	}

	return &recording, sid, segments, nil
}

// segments は、録画のファイルから TS のセグメントを、始まりの順に選ぶ。
// 一部のセグメントだけで書くと、残りの字幕が抜けたまま記録されるので、マウントされていないプールのセグメントがあれば、取りやめる。
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
		segments = append(segments, segment{path: path, key: segmentKey(file.PoolID, file.Path, file.Size), startedAt: file.StartedAt})
	}
	if len(segments) == 0 {
		return nil, river.JobCancel(errNoSegment)
	}
	if unmounted > 0 {
		return nil, river.JobCancel(fmt.Errorf("%w: %d of %d segments", errPartiallyMounted, unmounted, unmounted+len(segments)))
	}

	return segments, nil
}

// segmentKey は、セグメントが変わっていないかを確認するための、保存先プールとパスと大きさを返す。
func segmentKey(poolID, path string, size int64) string {
	return fmt.Sprintf("%s/%s#%d", poolID, path, size)
}

// replacements は、字幕の外字の置き換えのテーブルを返す。字幕の履歴と同じく、既定のテーブルに管理者の登録を重ねる (設計書 7.4)。
// 既定のテーブルを作らずにビルドした本体 (テストや E2E のビルド) では、管理者の登録だけを使う。
func (w *Worker) replacements(ctx context.Context) (map[string]string, error) {
	system, err := settings.Load(ctx, w.db)
	if err != nil {
		return nil, err
	}
	defaults, err := drcstable.Default()
	if errors.Is(err, drcstable.ErrNotGenerated) {
		w.logger.WarnContext(ctx, "the default DRCS table is not embedded in this build; using only the registered replacements")
		defaults = map[string]string{}
	} else if err != nil {
		return nil, err
	}

	return drcstable.Merge(defaults, system.DRCSReplacements), nil
}

// scan は、セグメント s を読み、字幕の区間を返す。offset は、録画を始めた日時からのセグメントの始まり。
// 時刻の基準 (最初の I フレーム) は、字幕と同じ TS を ts.CueScanner に読ませて求める。
func (w *Worker) scan(ctx context.Context, s segment, sid uint16, replacements map[string]string, offset time.Duration) ([]Cue, error) {
	file, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("failed to open the segment: %w", err)
	}
	defer file.Close()
	captions := ts.NewCaptionScanner(sid, func(md5 string) (string, bool) {
		text, ok := replacements[md5]

		return text, ok
	})
	timing := ts.NewCueScanner(sid)
	if err := copyWithContext(ctx, io.MultiWriter(captions, timing), file); err != nil {
		return nil, err
	}
	found := captions.Captions()
	if len(found) == 0 {
		return []Cue{}, nil
	}

	return buildCues(found, timingBase(timing.Cues(), found[0].PTS), timing.Cues().LastVideoPTS, offset), nil
}

// timingBase は、再生の時間の 0 にする PTS を返す。字幕の自動生成と同じく、最初の I フレーム、なければ最初の映像の PES、
// 映像がなければ最初の音声の PES にする。どれもなければ、最初の字幕文を 0 にする。
func timingBase(cues *ts.Cues, firstCaption int64) int64 {
	switch {
	case cues.FirstKeyframePTS != nil:
		return *cues.FirstKeyframePTS
	case cues.FirstVideoPTS != nil:
		return *cues.FirstVideoPTS
	case cues.FirstAudioPTS != nil:
		return *cues.FirstAudioPTS
	default:
		return firstCaption
	}
}

// copyWithContext は、r を最後まで読んで w に書く。録画を読み通す間に、ジョブの取り消しと時間の上限を効かせるよう、
// 読むたびに ctx を確認する。
func copyWithContext(ctx context.Context, w io.Writer, r io.Reader) error {
	buf := make([]byte, readBufferSize)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return fmt.Errorf("failed to scan the segment: %w", werr)
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to read the segment: %w", err)
		}
	}
}

// storedCue は、DB の cues (JSONB) に残す字幕の形。字幕の自動生成と同じ形にする。
type storedCue struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Text    string `json:"text"`
}

// record は、セグメントが読んだときのままなら、字幕を前の字幕と置き換えて書く。変わっていれば、やり直す。
func (w *Worker) record(ctx context.Context, recordingID uuid.UUID, segments []segment, cues []Cue) error {
	stored := make([]storedCue, 0, len(cues))
	texts := make([]string, 0, len(cues))
	for _, c := range cues {
		stored = append(stored, storedCue{StartMs: c.Start.Milliseconds(), EndMs: c.End.Milliseconds(), Text: c.Text})
		texts = append(texts, domain.Normalize(c.Text))
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to encode the captions: %w", err)
	}
	err = pgx.BeginFunc(ctx, w.db, func(tx pgx.Tx) error {
		// 読んでいる間に録画が消えたら、セグメントも消えて変わったように見えるので、先に録画があるかを確認し、やり直さずに取りやめる。
		if _, err := sqlc.New(tx).GetRecording(ctx, recordingID); errors.Is(err, pgx.ErrNoRows) {
			return river.JobCancel(errRecordingNotFound)
		} else if err != nil {
			return fmt.Errorf("failed to get the recording: %w", err)
		}
		if err := w.ensureUnchanged(ctx, tx, recordingID, segments); err != nil {
			return err
		}
		err := sqlc.New(tx).UpsertRecordingARIBCaptions(ctx, sqlc.UpsertRecordingARIBCaptionsParams{
			RecordingID: recordingID, ExtractorVersion: ExtractorVersion, Now: w.clock.Now(), Cues: data, SearchGrams: match.TextGrams(texts),
		})
		if err != nil {
			return fmt.Errorf("failed to record the ARIB captions: %w", err)
		}
		_, err = notify.Publish(ctx, tx, w.clock, notify.RecordingsChannel)

		return err
	})
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return river.JobCancel(errRecordingNotFound)
	}

	return err
}

// ensureUnchanged は、録画のセグメントが、読んだときから変わっていないことを確認する。
func (w *Worker) ensureUnchanged(ctx context.Context, db sqlc.DBTX, recordingID uuid.UUID, segments []segment) error {
	files, err := sqlc.New(db).ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return fmt.Errorf("failed to list the recording files: %w", err)
	}
	keys := make([]string, 0, len(files))
	for _, file := range files {
		if file.Kind == kindSegment {
			keys = append(keys, segmentKey(file.PoolID, file.Path, file.Size))
		}
	}
	for _, s := range segments {
		if !slices.Contains(keys, s.key) {
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
