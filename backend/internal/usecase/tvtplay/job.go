package tvtplay

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
)

const (
	// writeMaxAttempts は、チャプターのファイルを書くジョブを試す回数の上限。
	writeMaxAttempts = 3
	// writeTimeout は、チャプターのファイルを書くジョブの時間の上限。小さなファイルを書くだけなので、短くする。
	writeTimeout = time.Minute
	// chaptersDir は、チャプターのファイルを置くフォルダーの名前。TvtPlay は、TS のあるフォルダーにこの名前のフォルダーがあれば、
	// そこの .chapter を最初に読む。参考にした comskip-tvtplay も、ここに書く。
	chaptersDir = "chapters"
	// otherKind は、録画のファイルのうち、その他のファイルの種類。
	otherKind = "other"
)

var (
	// errRecordingNotFound は、録画がなくなったことを表す。
	errRecordingNotFound = errors.New("recording not found")
	// errStillRecording は、録画がまだ終わっていないことを表す。
	errStillRecording = errors.New("the recording is still in progress")
)

// Inserter は、ジョブを投入する。*river.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// WriteArgs は、CM の区間から、TS のセグメントの隣の chapters フォルダーに TvtPlay のチャプターのファイルを書くジョブの引数 (#1737)。
type WriteArgs struct {
	RecordingID uuid.UUID `json:"recordingId"`
}

var _ river.JobArgsWithInsertOpts = new(WriteArgs)

func (WriteArgs) Kind() string { return "recording_tvtplay_chapters" }

func (WriteArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// 小さなファイルを書くだけなので、代表画像などと同じキューで動かす。
		Queue:       jobs.QueueMedia,
		MaxAttempts: writeMaxAttempts,
		// 同じ録画のジョブは、待っている間に重ねて投入しない。動いているジョブも含めるのは、変換した MP4 のチャプター (#1510) と同じく、
		// 動いているジョブが録画ごとのロックを取ってから区間を読むので、新しい区間で書くからである。
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
			rivertype.JobStateRunning, rivertype.JobStateScheduled,
		}},
	}
}

// Enqueuer は、CM の区間を見分け終えたときと、録画のファイルの名前を付け直したときに、管理者が有効にしていれば、
// チャプターのファイルを書くジョブを投入する。
type Enqueuer struct {
	inserter Inserter
}

var _ cmdetect.DetectedHook = new(Enqueuer)

// NewEnqueuer は Enqueuer を返す。
func NewEnqueuer(inserter Inserter) *Enqueuer {
	return &Enqueuer{inserter: inserter}
}

// OnDetected は、CM の区間を書いたトランザクション tx の中で、管理者が有効にしていれば、ジョブを投入する (cmdetect.DetectedHook)。
// CM が見つからなかったときも投入し、前に書いたファイルを消させる。
func (e *Enqueuer) OnDetected(ctx context.Context, tx pgx.Tx, recordingID uuid.UUID) error {
	if enabled, err := chaptersEnabled(ctx, tx); err != nil || !enabled {
		return err
	}
	if _, err := e.inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: WriteArgs{RecordingID: recordingID}, InsertOpts: nil}}); err != nil {
		return fmt.Errorf("failed to enqueue the TvtPlay chapter files: %w", err)
	}

	return nil
}

// OnRenamed は、録画 recordingID のファイルの名前を付け直した後に、ジョブを投入する。チャプターのファイルは、セグメントの名前で探されるので、
// 付け直した名前で書き直し、前の名前のものを消す。
func (e *Enqueuer) OnRenamed(ctx context.Context, db transcode.Database, recordingID uuid.UUID) error {
	if err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		return e.OnDetected(ctx, tx, recordingID)
	}); err != nil {
		return fmt.Errorf("failed to enqueue the TvtPlay chapter files after renaming: %w", err)
	}

	return nil
}

// WriteWorker は、CM の区間から、録画の TS のセグメントごとに、TvtPlay のチャプターのファイルを書く (#1737)。
// ファイルは、セグメントと同じプールの、セグメントのあるフォルダーの chapters フォルダーに、セグメントの拡張子を .chapter に替えた名前で置き、
// その他のファイルとして記録する。記録するので、録画を消すと一緒に消える。CM の区間のないセグメントのファイルと、
// 名前を付け直す前の名前のファイルは消す。ほかの録画のファイルや、記録のないファイル (comskip-tvtplay が書いたものなど) が
// 同じ名前であれば、上書きしない。管理者が無効にしたら、書かない (書いたファイルは消さない)。
type WriteWorker struct {
	river.WorkerDefaults[WriteArgs]

	db    transcode.Database
	store *kago.Store
	clock clock.Clock
}

var _ river.Worker[WriteArgs] = new(WriteWorker)

// NewWriteWorker は WriteWorker を返す。store は、このインスタンスにマウントされたプール。
func NewWriteWorker(db transcode.Database, store *kago.Store, clk clock.Clock) *WriteWorker {
	return &WriteWorker{WorkerDefaults: river.WorkerDefaults[WriteArgs]{}, db: db, store: store, clock: clk}
}

func (w *WriteWorker) Timeout(*river.Job[WriteArgs]) time.Duration {
	return writeTimeout
}

// Work は、録画ごとの advisory lock を取ってから、チャプターのファイルを書く。セグメントをつなぐジョブと同じロックを使うので、
// セグメントを置き換えている間や、CM の区間を書き換えている間には読まない。
func (w *WriteWorker) Work(ctx context.Context, job *river.Job[WriteArgs]) error {
	return transcode.WithRecordingLock(ctx, w.db, job.Args.RecordingID, func() error {
		return w.work(ctx, job.Args.RecordingID)
	})
}

func (w *WriteWorker) work(ctx context.Context, recordingID uuid.UUID) error {
	if enabled, err := chaptersEnabled(ctx, w.db); err != nil || !enabled {
		return err
	}
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
	sections, err := cmdetect.RecordingSections(ctx, w.db, recordingID)
	if err != nil {
		return err
	}

	written := map[string]bool{}
	for i := range files {
		segment := &files[i]
		if segment.Kind != segmentKind || segment.EndedAt == nil {
			continue
		}
		shifted := segmentSections(sections, recording.RecordedFrom, segment)
		tl := &Timeline{RecordedFrom: segment.StartedAt, Gaps: []transcode.Gap{}, Duration: segment.EndedAt.Sub(segment.StartedAt)}
		if len(spans(shifted, tl)) == 0 {
			continue
		}
		rel := ChapterPath(segment.Path)
		ok, err := w.write(ctx, recordingID, segment, rel, Build(shifted, tl), files)
		if err != nil {
			return err
		}
		if ok {
			written[segment.PoolID+"\x00"+rel] = true
		}
	}

	return w.removeStale(ctx, recordingID, files, written)
}

// write は、セグメント segment のチャプターのファイル rel を書き、その他のファイルとして記録する。
// ほかのファイルが同じ名前を使っていれば、書かずに false を返す。
func (w *WriteWorker) write(ctx context.Context, recordingID uuid.UUID, segment *sqlc.RecordingFile, rel string, content []byte, files []sqlc.RecordingFile) (bool, error) {
	ours := slices.ContainsFunc(files, func(f sqlc.RecordingFile) bool {
		return f.Kind == otherKind && f.PoolID == segment.PoolID && f.Path == rel
	})
	if !ours {
		exists, err := w.store.Exists(segment.PoolID, rel)
		if errors.Is(err, kago.ErrPoolNotMounted) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("failed to check the chapter file: %w", err)
		}
		if exists {
			// 記録のないファイル (comskip-tvtplay が書いたものなど) や、ほかの録画のファイルは上書きしない。
			return false, nil
		}
	}
	if err := w.store.WriteFile(segment.PoolID, rel, content); errors.Is(err, kago.ErrPoolNotMounted) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("failed to write the chapter file: %w", err)
	}
	n, err := sqlc.New(w.db).UpsertTvtplayChapterFile(ctx, sqlc.UpsertTvtplayChapterFileParams{
		ID: id.New(), RecordingID: recordingID, PoolID: segment.PoolID, Path: rel, Size: int64(len(content)),
		StartedAt: segment.StartedAt, EndedAt: segment.EndedAt, Now: w.clock.Now(),
	})
	if err != nil {
		return false, fmt.Errorf("failed to record the chapter file: %w", err)
	}
	if n == 0 {
		// 書いている間に、ほかの録画がその名前を記録した。
		return false, nil
	}

	return true, nil
}

// removeStale は、前に書いたチャプターのファイルのうち、今回書かなかったもの (CM の区間がなくなったセグメントのものと、
// 名前を付け直す前の名前のもの) を消し、記録も消す。
func (w *WriteWorker) removeStale(ctx context.Context, recordingID uuid.UUID, files []sqlc.RecordingFile, written map[string]bool) error {
	stale := []uuid.UUID{}
	for _, f := range files {
		if f.Kind != otherKind || !isChapterPath(f.Path) || written[f.PoolID+"\x00"+f.Path] {
			continue
		}
		if err := w.store.Remove(f.PoolID, f.Path, uuid.Nil()); errors.Is(err, kago.ErrPoolNotMounted) {
			continue
		} else if err != nil {
			return fmt.Errorf("failed to remove a stale chapter file: %w", err)
		}
		stale = append(stale, f.ID)
	}
	if len(stale) == 0 {
		return nil
	}
	if err := sqlc.New(w.db).DeleteRecordingFiles(ctx, sqlc.DeleteRecordingFilesParams{RecordingID: recordingID, Ids: stale}); err != nil {
		return fmt.Errorf("failed to delete the records of stale chapter files: %w", err)
	}

	return nil
}

// chaptersEnabled は、管理者がチャプターのファイルを書くようにしているかを返す。設定の行がまだなければ、既定の false にする。
func chaptersEnabled(ctx context.Context, db sqlc.DBTX) (bool, error) {
	settings, err := sqlc.New(db).GetSystemSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get the system settings: %w", err)
	}

	return settings.TvtplayChaptersEnabled, nil
}

// segmentSections は、CM の区間 sections (録画を始めた日時 recordedFrom からの時間) を、セグメント segment の始まりからの時間にずらす。
// セグメントの外にはみ出た分は、Build がセグメントの長さで切り、外に出た区間は捨てる。
func segmentSections(sections []cmdetect.Section, recordedFrom time.Time, segment *sqlc.RecordingFile) []cmdetect.Section {
	shift := segment.StartedAt.Sub(recordedFrom)
	out := make([]cmdetect.Section, 0, len(sections))
	for _, s := range sections {
		out = append(out, cmdetect.Section{Start: s.Start - shift, End: s.End - shift})
	}

	return out
}

// ChapterPath は、プールの中のセグメントのパス segment に対して、チャプターのファイルを置くパスを返す。
// セグメントのあるフォルダーの chapters フォルダーに、拡張子を .chapter に替えた名前で置く。
func ChapterPath(segment string) string {
	dir, name := path.Split(segment)

	return path.Join(dir, chaptersDir, FileName(name))
}

// isChapterPath は、rel が ChapterPath の形 (chapters フォルダーの .chapter) かを返す。
func isChapterPath(rel string) bool {
	dir, name := path.Split(rel)

	return path.Base(dir) == chaptersDir && strings.HasSuffix(name, ".chapter")
}
