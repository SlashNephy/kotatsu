// Package segjoin は、引き継ぎや再起動で分かれた録画のファイル (セグメント) を、録画が終わったら 1 つにつなぐ (設計書 5.4)。
//
// つなぐのは 2 つのジョブに分ける。
// JoinWorker は、つないだものを一時ファイルに書き、DB のファイルの記録を置き換えて、FinalizeWorker のジョブを同じトランザクションで投入する。
// FinalizeWorker は、一時ファイルを最初のセグメントの名前に置き換え、つないだ元のファイルを消して、索引を作り直させる。
// どちらで止まっても、ジョブをやり直せば同じ結果になる。
package segjoin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/analysis"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/jobs"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/thumbnail"
	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	kindSegment = "segment"
	kindIndex   = "index"
	// stateRecording は、録画中の録画の状態。
	stateRecording = "recording"
	stateFailed    = "failed"
	// tempSuffix は、つないでいる途中のファイルの名前に付ける接尾辞。
	tempSuffix = ".joining"
	// joinTimeout は、つなぐジョブの時間の上限。数十 GB を読み書きするので、長めにとる。
	joinTimeout = 2 * time.Hour
	// maxAttempts は、ジョブを試す回数。
	maxAttempts = 3
	// finalizeMaxAttempts は、つないだファイルに置き換えるジョブを試す回数。DB はつないだ後の内容で確定しているので、
	// ファイルを置き換えるまで、NFS などが止まっていても諦めずにやり直す (#514)。River の既定の間隔では、2 週間ほどにわたる。
	finalizeMaxAttempts = 25
	// unmountedSnooze は、プールがマウントされていないときに、置き換えをやり直すまで待つ時間。
	unmountedSnooze = 5 * time.Minute
	// lockedSnooze は、同じ録画をほかでつないでいるときに、やり直すまで待つ時間。
	lockedSnooze = 10 * time.Minute
	// foreignKeyViolation は、外部キーの制約に反したときの PostgreSQL のエラーコード。
	foreignKeyViolation = "23503"
)

var (
	errRecordingNotFound = errors.New("the recording no longer exists")
	errNotFinished       = errors.New("the recording has not finished")
)

// Inserter は、ジョブをトランザクションの中で投入する。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// JoinArgs は、録画のセグメントをつなぐジョブの引数。
type JoinArgs struct {
	RecordingID uuid.UUID `json:"recordingId"`
}

var _ river.JobArgsWithInsertOpts = new(JoinArgs)

func (JoinArgs) Kind() string { return "recording_join" }

func (JoinArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// 索引やサムネイルのジョブと同じキューで、1 つずつ動かす。録画のファイルを読み書きするのが重いため。
		Queue:       jobs.QueueMedia,
		MaxAttempts: maxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// FinalizeArgs は、つないだファイルを最初のセグメントの名前に置き換え、つないだ元のファイルを消すジョブの引数。
type FinalizeArgs struct {
	RecordingID uuid.UUID `json:"recordingId"`
	// PoolID と Path は、つないだファイルの置き場所 (最初のセグメントの置き場所)。一時ファイルは Path に tempSuffix を付けたもの。
	PoolID string `json:"poolId"`
	Path   string `json:"path"`
	// Removed は、消す、つないだ元のセグメント。付属ファイルのディレクトリも一緒に消す。
	Removed []File `json:"removed"`
}

// File は、プールとプール内の相対パスで表すファイル。
type File struct {
	PoolID string `json:"poolId"`
	Path   string `json:"path"`
}

var _ river.JobArgsWithInsertOpts = new(FinalizeArgs)

func (FinalizeArgs) Kind() string { return "recording_join_finalize" }

func (FinalizeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: jobs.QueueMedia, MaxAttempts: finalizeMaxAttempts}
}

// Enqueuer は、録画の完了のイベントを配るときに、録画のファイルを 1 つにつなぐジョブか、画像を作るジョブを投入する。
// つなぐときは、つないだあとで索引と画像を作り直すので、画像のジョブはここでは投入しない。
type Enqueuer struct {
	inserter Inserter
}

var _ notification.Hook = new(Enqueuer)

// NewEnqueuer は Enqueuer を返す。
func NewEnqueuer(inserter Inserter) *Enqueuer {
	return &Enqueuer{inserter: inserter}
}

func (e *Enqueuer) OnEvent(ctx context.Context, tx pgx.Tx, event *notification.Event) error {
	if event.Kind != notification.KindRecordingCompleted {
		return nil
	}
	recordingID, ok := recordingIDOf(event.Payload)
	if !ok {
		// 録画の ID のないイベントでは、何もできない。ほかの通知は配りたいので、失敗にはしない。
		return nil
	}
	var args river.JobArgs = thumbnail.GenerateArgs{RecordingID: recordingID}
	if join, _ := event.Payload["joinSegments"].(bool); join && segmentCount(event.Payload) > 1 {
		args = JoinArgs{RecordingID: recordingID}
	}
	if _, err := e.inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: args, InsertOpts: nil}}); err != nil {
		return fmt.Errorf("failed to enqueue the job after the recording: %w", err)
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

// JoinWorker は、録画のセグメントを 1 つにつなぐ。
type JoinWorker struct {
	river.WorkerDefaults[JoinArgs]

	db       sqlc.DBTX
	pool     txBeginner
	conns    connAcquirer
	store    *kago.Store
	inserter Inserter
	logger   *slog.Logger
	window   int
}

// txBeginner は、トランザクションを始められる DB。*pgxpool.Pool が満たす。
type txBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// connAcquirer は、接続を借りられる DB。*pgxpool.Pool が満たす。セッションの advisory lock を持つのに使う。
type connAcquirer interface {
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
}

var _ river.Worker[JoinArgs] = new(JoinWorker)

// NewJoinWorker は JoinWorker を返す。store は、このインスタンスにマウントされたプール。
// window は重なりを探す範囲 (バイト) で、0 なら ts.DefaultJoinWindow にする。
func NewJoinWorker(db interface {
	sqlc.DBTX
	txBeginner
	connAcquirer
}, store *kago.Store, inserter Inserter, logger *slog.Logger, window int,
) *JoinWorker {
	return &JoinWorker{
		WorkerDefaults: river.WorkerDefaults[JoinArgs]{},
		db:             db, pool: db, conns: db, store: store, inserter: inserter, logger: logger, window: window,
	}
}

func (w *JoinWorker) Timeout(*river.Job[JoinArgs]) time.Duration {
	return joinTimeout
}

// Work は、録画ごとの advisory lock を取ってから、録画のセグメントをつなぐ。
// 時間の上限を超えたジョブが救出され、ほかの nekonote が同じ録画をつなぎ始めても、同じ一時ファイルを開き直して壊さないよう、
// ロックが取れなければ、つながずに後でやり直す (#515)。
func (w *JoinWorker) Work(ctx context.Context, job *river.Job[JoinArgs]) error {
	conn, err := w.conns.Acquire(ctx)
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

	return w.work(ctx, job)
}

func (w *JoinWorker) work(ctx context.Context, job *river.Job[JoinArgs]) error {
	q := sqlc.New(w.db)
	recording, err := q.GetRecording(ctx, job.Args.RecordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(errRecordingNotFound)
	}
	if err != nil {
		return fmt.Errorf("failed to get the recording: %w", err)
	}
	if recording.State == stateRecording || recording.State == stateFailed {
		return river.JobCancel(errNotFinished)
	}
	files, err := q.ListRecordingFiles(ctx, recording.ID)
	if err != nil {
		return fmt.Errorf("failed to list the recording files: %w", err)
	}
	segments, indexes := split(files)
	logger := w.logger.With(slog.String("recording_id", recording.ID.String()))
	if len(segments) < 2 {
		// つなぐものがなければ、画像だけを作る。
		return w.enqueue(ctx, thumbnail.GenerateArgs{RecordingID: recording.ID})
	}

	first := segments[0]
	room, err := w.hasRoom(ctx, first.PoolID, segments)
	if err != nil {
		return err
	}
	if !room {
		// つないでいる間は、全長分をもう 1 本使う。容量の管理に古い録画を消させたり、並行する録画を失敗させたりしないよう、
		// 書いても閾値を下回らない空きがなければ、分けたまま残す (#515)。
		logger.WarnContext(ctx, "kept the recording split; the storage pool does not have room for the joined file", slog.String("pool_id", first.PoolID))

		return w.enqueue(ctx, thumbnail.GenerateArgs{RecordingID: recording.ID})
	}
	outPath, err := w.store.Path(first.PoolID, first.Path+tempSuffix)
	if err != nil {
		return river.JobCancel(fmt.Errorf("failed to locate the joined file: %w", err))
	}
	size, err := w.join(segments, outPath)
	if errors.Is(err, ts.ErrNoOverlap) {
		// 重なりのパケットが違えば、つなぐと録画が壊れるので、分けたまま残す (設計書 5.4)。
		logger.WarnContext(ctx, "kept the recording split; the segments do not overlap packet by packet", slog.Any("error", err))
		_ = os.Remove(outPath)

		return w.enqueue(ctx, thumbnail.GenerateArgs{RecordingID: recording.ID})
	}
	if err != nil {
		_ = os.Remove(outPath)

		return err
	}

	if err := w.commit(ctx, logger, recording.ID, segments, indexes, size, outPath); err != nil {
		return err
	}
	logger.InfoContext(ctx, "joined the recording segments", slog.Int("segments", len(segments)), slog.Int64("size", size))

	return nil
}

// commit は、DB のファイルの記録を、つないだ後の内容に置き換え、同じトランザクションで置き換えのジョブを投入する。
func (w *JoinWorker) commit(
	ctx context.Context, logger *slog.Logger, recordingID uuid.UUID, segments, indexes []*sqlc.RecordingFile, size int64, outPath string,
) error {
	first := segments[0]
	removed := make([]File, 0, len(segments)-1)
	ids := make([]uuid.UUID, 0, len(segments)-1+len(indexes))
	for _, segment := range segments[1:] {
		removed = append(removed, File{PoolID: segment.PoolID, Path: segment.Path})
		ids = append(ids, segment.ID)
	}
	for _, index := range indexes {
		ids = append(ids, index.ID)
	}
	last := segments[len(segments)-1]
	err := pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		n, err := q.SetJoinedRecordingFile(ctx, sqlc.SetJoinedRecordingFileParams{Size: size, EndedAt: last.EndedAt, ID: first.ID})
		if err != nil {
			return fmt.Errorf("failed to update the joined segment: %w", err)
		}
		// つないでいる間に録画が消された。外部キーの違反にはならないので、行の数で確かめる (#514)。
		if n == 0 {
			return errRecordingNotFound
		}
		if err := q.DeleteRecordingFiles(ctx, sqlc.DeleteRecordingFilesParams{RecordingID: recordingID, Ids: ids}); err != nil {
			return fmt.Errorf("failed to delete the joined segments: %w", err)
		}
		args := FinalizeArgs{RecordingID: recordingID, PoolID: first.PoolID, Path: first.Path, Removed: removed}
		if _, err := w.inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: args, InsertOpts: nil}}); err != nil {
			return fmt.Errorf("failed to enqueue the finalization: %w", err)
		}

		return nil
	})
	if pgErr := (*pgconn.PgError)(nil); errors.Is(err, errRecordingNotFound) || errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		_ = os.Remove(outPath)

		return river.JobCancel(errRecordingNotFound)
	}
	if err != nil {
		return w.recoverCommit(ctx, logger, err, recordingID, first.ID, size, outPath)
	}

	return nil
}

// recoverCommit は、つないだ結果を確定するトランザクションが失敗したときに、確定したかどうかを DB で確かめる。
// コミットの結果があいまい (実際には確定したのに、エラーが返った) なら、置き換えのジョブも確定しているので、
// つないだファイルを残す。消すと、置き換えで元のセグメントを消して、録画の後半を失う (#514)。
// 確定していなければ、つないだファイルを消して、やり直す。確かめられなければ、残してやり直す。
func (w *JoinWorker) recoverCommit(ctx context.Context, logger *slog.Logger, cause error, recordingID, firstID uuid.UUID, size int64, outPath string) error {
	if pgErr := (*pgconn.PgError)(nil); errors.As(cause, &pgErr) {
		// PostgreSQL が返したエラーなら、確定していない。
		_ = os.Remove(outPath)

		return cause
	}
	row, err := sqlc.New(w.db).CountRecordingSegments(ctx, sqlc.CountRecordingSegmentsParams{FirstID: firstID, RecordingID: recordingID})
	if err != nil {
		return errors.Join(cause, fmt.Errorf("failed to check whether the join was committed: %w", err))
	}
	if row.Segments == 1 && row.FirstSize == size {
		logger.WarnContext(ctx, "the join was committed although the commit returned an error", slog.Any("error", cause))

		return nil
	}
	_ = os.Remove(outPath)

	return cause
}

// hasRoom は、セグメントの大きさの合計を書いても、プール poolID の空き容量が閾値を下回らないかを返す。
func (w *JoinWorker) hasRoom(ctx context.Context, poolID string, segments []*sqlc.RecordingFile) (bool, error) {
	total := uint64(0)
	for _, segment := range segments {
		total += uint64(max(segment.Size, 0))
	}
	free, err := w.store.PoolFreeSpace(poolID)
	if err != nil {
		return false, fmt.Errorf("failed to get the free space of pool %s: %w", poolID, err)
	}
	threshold, err := sqlc.New(w.db).GetStoragePoolThreshold(ctx, poolID)
	if err != nil {
		return false, fmt.Errorf("failed to get the threshold of pool %s: %w", poolID, err)
	}

	return free >= total && free-total >= uint64(max(threshold, 0)), nil
}

// join は、segments を順につないで outPath に書き、書いた大きさを返す。
// 前のセグメントと時間が重なるセグメントは、重なりを除いてつなぎ、重ならないものはそのまま後ろにつなぐ。
func (w *JoinWorker) join(segments []*sqlc.RecordingFile, outPath string) (int64, error) {
	out, err := os.OpenFile(outPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("failed to create the joined file: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	joiner := ts.NewJoiner(out, w.window)
	for i, segment := range segments {
		if err := w.appendSegment(joiner, segments, i); err != nil {
			return 0, fmt.Errorf("failed to join segment %s: %w", segment.Path, err)
		}
	}
	if err := out.Sync(); err != nil {
		return 0, fmt.Errorf("failed to sync the joined file: %w", err)
	}

	return joiner.Written(), nil
}

// appendSegment は、segments[i] を joiner の後ろにつなぐ。
func (w *JoinWorker) appendSegment(joiner *ts.Joiner, segments []*sqlc.RecordingFile, i int) error {
	segment := segments[i]
	in, err := w.store.Open(segment.PoolID, segment.Path)
	if err != nil {
		return err
	}
	defer func() {
		_ = in.Close()
	}()
	if i > 0 && overlaps(segments[i-1], segment) {
		return joiner.AppendOverlapping(in)
	}

	return joiner.Append(in)
}

// overlaps は、next が、前のセグメント prev の終わりより前に始まっているかを返す。
// 引き継ぎでは、新しい mikan が録り始めてから、古い mikan が録り終えるので、時間が重なる。
func overlaps(prev, next *sqlc.RecordingFile) bool {
	return prev.EndedAt != nil && next.StartedAt.Before(*prev.EndedAt)
}

// split は、録画のファイルから、書いたものがあるセグメント (始まりの順) と、索引を選ぶ。
func split(files []sqlc.RecordingFile) ([]*sqlc.RecordingFile, []*sqlc.RecordingFile) {
	segments := []*sqlc.RecordingFile{}
	indexes := []*sqlc.RecordingFile{}
	for i := range files {
		switch file := &files[i]; {
		case file.Kind == kindSegment && file.Size > 0:
			segments = append(segments, file)
		case file.Kind == kindIndex:
			indexes = append(indexes, file)
		}
	}

	return segments, indexes
}

// enqueue は、ジョブを投入する。
func (w *JoinWorker) enqueue(ctx context.Context, args river.JobArgs) error {
	return pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		if _, err := w.inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: args, InsertOpts: nil}}); err != nil {
			return fmt.Errorf("failed to enqueue %s: %w", args.Kind(), err)
		}

		return nil
	})
}

// FinalizeWorker は、つないだファイルを最初のセグメントの名前に置き換え、つないだ元のファイルを消して、索引を作り直させる。
type FinalizeWorker struct {
	river.WorkerDefaults[FinalizeArgs]

	pool     txBeginner
	store    *kago.Store
	inserter Inserter
}

var _ river.Worker[FinalizeArgs] = new(FinalizeWorker)

// NewFinalizeWorker は FinalizeWorker を返す。
func NewFinalizeWorker(pool txBeginner, store *kago.Store, inserter Inserter) *FinalizeWorker {
	return &FinalizeWorker{WorkerDefaults: river.WorkerDefaults[FinalizeArgs]{}, pool: pool, store: store, inserter: inserter}
}

func (w *FinalizeWorker) Work(ctx context.Context, job *river.Job[FinalizeArgs]) error {
	path, err := w.store.Path(job.Args.PoolID, job.Args.Path)
	if errors.Is(err, kago.ErrPoolNotMounted) {
		// DB はつないだ後の内容で確定しているので、取りやめずに、マウントされるのを待つ (#514)。
		return river.JobSnooze(unmountedSnooze)
	}
	if err != nil {
		return river.JobCancel(fmt.Errorf("failed to locate the joined file: %w", err))
	}
	exists, err := w.recordingExists(ctx, job.Args.RecordingID)
	if err != nil {
		return err
	}
	if !exists {
		// つないだ後に録画が消された。録画の削除は、DB にある最初のセグメントしか消さないので、
		// つないだファイルと元のセグメントを消す。置き換えると、消した録画のファイルを作り直してしまう (#514)。
		return w.removeAll(&job.Args, path)
	}
	// やり直したときは、一時ファイルはもう置き換えてある。
	if err := os.Rename(path+tempSuffix, path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to replace the first segment with the joined file: %w", err)
	}
	for _, file := range job.Args.Removed {
		if err := w.store.Remove(file.PoolID, file.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove the joined segment %s: %w", file.Path, err)
		}
	}

	// つないだファイルの索引と集計、画像を作り直す。解析のジョブが、終わったら画像のジョブを投入する。
	return pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		params := []river.InsertManyParams{{Args: analysis.AnalyzeArgs{RecordingID: job.Args.RecordingID}, InsertOpts: nil}}
		if _, err := w.inserter.InsertManyTx(ctx, tx, params); err != nil {
			return fmt.Errorf("failed to enqueue the analysis: %w", err)
		}

		return nil
	})
}

// recordingExists は、録画がまだあるかを返す。
func (w *FinalizeWorker) recordingExists(ctx context.Context, recordingID uuid.UUID) (bool, error) {
	exists := false
	err := pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		_, err := sqlc.New(tx).GetRecording(ctx, recordingID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to get the recording: %w", err)
		}
		exists = true

		return nil
	})

	return exists, err
}

// removeAll は、消された録画の、つないだファイルと、つないだ元のセグメントを消す。
func (w *FinalizeWorker) removeAll(args *FinalizeArgs, path string) error {
	if err := os.Remove(path + tempSuffix); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove the joined file of a deleted recording: %w", err)
	}
	for _, file := range args.Removed {
		if err := w.store.Remove(file.PoolID, file.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove a segment of a deleted recording %s: %w", file.Path, err)
		}
	}

	return nil
}
