package thumbnail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// ThumbnailFileName、PreviewFileName、ManifestFileName は、データディレクトリの、画像を置くセグメント (ImageSegment) の
	// ディレクトリに置く、代表画像、プレビュー画像、プレビュー画像の説明のファイルの名前 (設計書 5.6)。
	ThumbnailFileName = "thumbnail.jpg"
	PreviewFileName   = "preview.jpg"
	ManifestFileName  = "preview.json"

	// kindSegment は、recording_files の、TS のセグメントの種類。
	kindSegment = "segment"
	// stateRecording は、録画中の録画の状態。
	stateRecording = "recording"

	// generateMaxAttempts は、画像を作るジョブを試す回数の上限。
	generateMaxAttempts = 3
	// generateTimeout は、画像を作るジョブの時間の上限。1 枚ずつ FFmpeg を動かすので、コマの数だけかかる。
	generateTimeout = 10 * time.Minute
)

var (
	errRecordingNotFound = errors.New("the recording no longer exists")
	errNoSegment         = errors.New("the recording has no segment with an index on the mounted pools")
	errNotRecording      = errors.New("the recording is no longer being recorded")
)

// GenerateArgs は、録画の代表画像とプレビュー画像を作るジョブの引数。
type GenerateArgs struct {
	RecordingID uuid.UUID `json:"recordingId"`
	// Live なら、録画中の録画の、今の様子の代表画像だけを作る (#1236)。プレビュー画像は、録画を終えてから作る。
	// 録画を終えた録画では、何もしない (録画を終えたときのジョブが作り直す)。
	Live bool `json:"live,omitempty"`
}

var _ river.JobArgsWithInsertOpts = new(GenerateArgs)

func (GenerateArgs) Kind() string { return "thumbnail_generate" }

func (GenerateArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       jobs.QueueMedia,
		MaxAttempts: generateMaxAttempts,
		// 同じ録画の画像を作るジョブは、待っている間に重ねて投入しない。River の既定は、終えたジョブ (completed) にも重ねないので、
		// 待っている状態だけにする。終えたジョブに重ねないと、セグメントをつないだ後や、手で頼んだときに作り直せない (#476)。
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

// GenerateWorker は、録画の代表画像とプレビュー画像を作り、最初のセグメントの生成物のディレクトリに置く。
// FFmpeg が見つからなければ、理由を残して取りやめる。
type GenerateWorker struct {
	river.WorkerDefaults[GenerateArgs]

	db      sqlc.DBTX
	store   *kago.Store
	clock   clock.Clock
	preview bool
	// newExtractor は、フレームを取り出す Extractor を作る。テストでは差し替える。
	newExtractor func() (Extractor, error)
}

var _ river.Worker[GenerateArgs] = new(GenerateWorker)

// NewGenerateWorker は GenerateWorker を返す。store は、このインスタンスにマウントされたプール。
// preview が false なら、代表画像だけを作る。
func NewGenerateWorker(db sqlc.DBTX, store *kago.Store, clk clock.Clock, preview bool) *GenerateWorker {
	return newGenerateWorker(db, store, clk, preview, func() (Extractor, error) { return NewFFmpeg() })
}

func newGenerateWorker(db sqlc.DBTX, store *kago.Store, clk clock.Clock, preview bool, newExtractor func() (Extractor, error)) *GenerateWorker {
	return &GenerateWorker{
		WorkerDefaults: river.WorkerDefaults[GenerateArgs]{},
		db:             db, store: store, clock: clk, preview: preview, newExtractor: newExtractor,
	}
}

func (w *GenerateWorker) Timeout(*river.Job[GenerateArgs]) time.Duration {
	return generateTimeout
}

func (w *GenerateWorker) Work(ctx context.Context, job *river.Job[GenerateArgs]) error {
	extractor, err := w.newExtractor()
	if errors.Is(err, ErrFFmpegNotFound) {
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}
	q := sqlc.New(w.db)
	if err := w.checkRecording(ctx, q, &job.Args); err != nil {
		return err
	}
	files, err := q.ListRecordingFiles(ctx, job.Args.RecordingID)
	if err != nil {
		return fmt.Errorf("failed to list the recording files: %w", err)
	}
	segments, err := w.segments(files)
	if err != nil {
		return err
	}
	owner, ok := ImageSegment(files)
	if !ok {
		return river.JobCancel(errNoSegment)
	}
	opts := &Options{Preview: w.preview && !job.Args.Live, Interval: 0, Latest: job.Args.Live}
	result, err := NewGenerator(extractor, opts).Generate(ctx, segments)
	if errors.Is(err, ErrNoKeyframe) {
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}
	// 作っている間に録画を終えていたら、録画を終えたときのジョブが作る画像を、今の様子で上書きしない。
	// 録画が消されていたら、書いた後に、下で画像を消す。
	if job.Args.Live {
		if recording, err := q.GetRecording(ctx, job.Args.RecordingID); err == nil && recording.State != stateRecording {
			return river.JobCancel(errNotRecording)
		}
	}
	if err := w.write(owner, result); err != nil {
		return err
	}
	// 作っている間に録画が消されていたら、書いた画像を消す。録画の削除は、そのときにあったファイルしか消さない。
	if _, err := q.GetRecording(ctx, job.Args.RecordingID); errors.Is(err, pgx.ErrNoRows) {
		if err := w.store.RemoveSegmentDir(owner); err != nil {
			return fmt.Errorf("failed to remove the images of a deleted recording: %w", err)
		}

		return river.JobCancel(errRecordingNotFound)
	} else if err != nil {
		return fmt.Errorf("failed to get the recording: %w", err)
	}
	// 録画の一覧にサムネイルを出すので、録画の変更として知らせる (#868)。
	if _, err := notify.Publish(ctx, w.db, w.clock, notify.RecordingsChannel); err != nil {
		return err
	}

	return nil
}

// checkRecording は、録画がまだあるかを確かめる。録画中の録画の画像 (Live) なら、まだ録画中かも確かめる。
// なければ、ジョブを取りやめるエラーを返す。
func (w *GenerateWorker) checkRecording(ctx context.Context, q *sqlc.Queries, args *GenerateArgs) error {
	recording, err := q.GetRecording(ctx, args.RecordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(errRecordingNotFound)
	}
	if err != nil {
		return fmt.Errorf("failed to get the recording: %w", err)
	}
	if args.Live && recording.State != stateRecording {
		return river.JobCancel(errNotRecording)
	}

	return nil
}

// ImageSegment は、録画の代表画像とプレビュー画像を置くセグメントのファイルの ID を返す。
// 画像は、書いたものがある最初のセグメント (始まりの順) の、データディレクトリのディレクトリに置く。セグメントがなければ false を返す。
// 書いている最中のセグメントは、大きさを閉じるときに書くので、大きさが 0 でも、書いたものがあるとみなす (録画中の代表画像。#1236)。
// files は、ListRecordingFiles の順 (始まりの順) に並べて渡す。
func ImageSegment(files []sqlc.RecordingFile) (uuid.UUID, bool) {
	for i := range files {
		if HasContent(files[i].Kind, files[i].Size, files[i].EndedAt) {
			return files[i].ID, true
		}
	}

	return uuid.Nil, false
}

// HasContent は、録画のファイルが、書いたもののある TS のセグメントかどうかを返す。閉じたセグメントは大きさで、
// 書いている最中のセグメント (終わりが NULL) は、大きさを閉じるときに書くので、書いたものがあるとみなす。
func HasContent(kind string, size int64, endedAt *time.Time) bool {
	return kind == kindSegment && (size > 0 || endedAt == nil)
}

// segments は、録画のファイルから、索引のあるセグメントを、始まりの順に読む。このインスタンスにマウントされていないプールのものは使えない。
// 索引は、データディレクトリの、セグメントのファイルの ID のディレクトリにあるものを、そのセグメントの索引とする。
// EPGStation から取り込んだ録画では、複数のセグメントの始まりが同じことがあるので、始まりでは対応付けない。
// コマの時間は、使える最初のセグメントの始まりからの時間にする。
func (w *GenerateWorker) segments(files []sqlc.RecordingFile) ([]Segment, error) {
	segments := []Segment{}
	var startedAt *time.Time
	for i := range files {
		file := &files[i]
		if !HasContent(file.Kind, file.Size, file.EndedAt) {
			continue
		}
		segmentPath, err := w.store.Path(file.PoolID, file.Path)
		if errors.Is(err, kago.ErrPoolNotMounted) {
			continue
		}
		if err != nil {
			return nil, err
		}
		entries, err := readIndex(w.store, kago.IndexLocation(file.ID))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if startedAt == nil {
			startedAt = &file.StartedAt
		}
		segments = append(segments, Segment{Path: segmentPath, Start: file.StartedAt.Sub(*startedAt), Index: entries})
	}
	if len(segments) == 0 {
		return nil, river.JobCancel(errNoSegment)
	}

	return segments, nil
}

// readIndex は、索引を読む。なければ os.ErrNotExist を包んで返す。
func readIndex(store *kago.Store, index *kago.Location) ([]ts.IndexEntry, error) {
	indexPath, err := store.Path(index.PoolID, index.Path)
	if err != nil {
		return nil, err
	}
	data, err := sharedfile.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read the index: %w", err)
	}
	entries, err := ts.ReadIndex(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse the index %s: %w", index.Path, err)
	}

	return entries, nil
}

// output は、データディレクトリに書くファイル。
type output struct {
	name string
	data []byte
}

// write は、画像を、セグメントのファイル owner の、データディレクトリのディレクトリに書く。作り直したときは、上書きする。
func (w *GenerateWorker) write(owner uuid.UUID, result *Result) error {
	dir, err := w.store.CreateSegmentDir(owner)
	if err != nil {
		return fmt.Errorf("failed to create the segment directory: %w", err)
	}
	outputs := []output{{name: ThumbnailFileName, data: result.Thumbnail}}
	if result.Preview != nil {
		manifest, err := json.Marshal(result.Manifest)
		if err != nil {
			return fmt.Errorf("failed to encode the preview manifest: %w", err)
		}
		outputs = append(outputs, output{name: PreviewFileName, data: result.Preview}, output{name: ManifestFileName, data: manifest})
	}
	for _, out := range outputs {
		abs, err := w.store.Path(dir.PoolID, path.Join(dir.Path, out.name))
		if err != nil {
			return err
		}
		if err := writeFile(abs, out.data); err != nil {
			return err
		}
	}

	return nil
}

// writeFile は、一時ファイルに書いてから名前を変え、読む側に書きかけのファイルを見せない。
func writeFile(name string, data []byte) error {
	tmp := name + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", name, err)
	}
	if err := sharedfile.Rename(tmp, name); err != nil {
		return fmt.Errorf("failed to rename %s: %w", tmp, err)
	}

	return nil
}

// Beginner はトランザクションを始められるもの。*pgxpool.Pool と pgx.Tx が満たす。
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// EnqueueGenerate は、録画 recordingID の代表画像とプレビュー画像を作り直すジョブを投入する。
// 待っているジョブにだけ重ねない (GenerateArgs.InsertOpts)。作り終えた録画は、また作り直せる。重ねずに飛ばしたら false を返す。
func EnqueueGenerate(ctx context.Context, db Beginner, inserter Inserter, recordingID uuid.UUID) (bool, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to begin a transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	results, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: GenerateArgs{RecordingID: recordingID}, InsertOpts: nil}})
	if err != nil {
		return false, fmt.Errorf("failed to insert the thumbnail job: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("failed to commit the thumbnail job: %w", err)
	}

	return len(results) == 1 && !results[0].UniqueSkippedAsDuplicate, nil
}
