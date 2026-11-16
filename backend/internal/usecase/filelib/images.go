package filelib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/usecase/thumbnail"
)

const (
	// imagesPriority は、ライブラリのファイルの画像を作るジョブの優先度。開いたファイルの数だけ入るので、録画の画像を待たせないよう、
	// 録画の画像 (既定の優先度) より後にする。River の優先度は、数が小さいほど先。
	imagesPriority = 3
	// imagesMaxAttempts は、画像を作るジョブを試す回数の上限。録画の画像と同じ。
	imagesMaxAttempts = 3
	// imagesTimeout は、画像を作るジョブの時間の上限。1 枚ずつ FFmpeg を動かすので、コマの数だけかかる。録画の画像と同じ。
	imagesTimeout = 10 * time.Minute
	// imagesSnooze は、索引を作り終えていないファイルの画像を作るジョブを、待たせる時間。
	imagesSnooze = 30 * time.Second
)

var (
	errFileChanged = errors.New("the library file was changed after the job was enqueued")
	errFileWriting = errors.New("the library file is still being written")
)

// ImagesArgs は、ライブラリのファイル (録画として登録していない TS。#1734) の代表画像とプレビュー画像を作るジョブの引数。
// ファイルは DB に行を持たないので、ライブラリの名前とパスで指し、作るときのファイルの ID (FileID) を添える。
type ImagesArgs struct {
	Library string `json:"library"`
	Path    string `json:"path"`
	FileID  string `json:"fileId"`
}

var _ river.JobArgsWithInsertOpts = new(ImagesArgs)

func (ImagesArgs) Kind() string { return "library_images_generate" }

func (ImagesArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       jobs.QueueMedia,
		MaxAttempts: imagesMaxAttempts,
		Priority:    imagesPriority,
		// 同じファイル (同じ ID) の画像を作るジョブは、待っているか動いている間は重ねない。作り終えたら、画像の記録 (HasImages) で重ねない。
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
			rivertype.JobStateRunning, rivertype.JobStateScheduled,
		}},
	}
}

// ImagesWorker は、ライブラリのファイルの代表画像とプレビュー画像を作る (設計書 5.8)。録画と同じ部品 (thumbnail.Generator) で、
// 索引の I フレームから作り、データディレクトリの索引の置き場に置く。nekonote が、ライブラリとデータディレクトリをマウントして動かす。
type ImagesWorker struct {
	river.WorkerDefaults[ImagesArgs]

	libraries    *medialib.Set
	indexer      *Indexer
	source       *Source
	preview      bool
	newExtractor func() (thumbnail.Extractor, error)
}

var _ river.Worker[ImagesArgs] = new(ImagesWorker)

// NewImagesWorker は ImagesWorker を返す。indexer は、索引を作らない (読むだけの) もの。preview が false なら、代表画像だけを作る。
func NewImagesWorker(libraries *medialib.Set, indexer *Indexer, preview bool) *ImagesWorker {
	return newImagesWorker(libraries, indexer, preview, func() (thumbnail.Extractor, error) { return thumbnail.NewFFmpeg() })
}

func newImagesWorker(libraries *medialib.Set, indexer *Indexer, preview bool, newExtractor func() (thumbnail.Extractor, error)) *ImagesWorker {
	return &ImagesWorker{
		WorkerDefaults: river.WorkerDefaults[ImagesArgs]{}, libraries: libraries, indexer: indexer,
		source: NewSource(libraries, indexer), preview: preview, newExtractor: newExtractor,
	}
}

func (w *ImagesWorker) Timeout(*river.Job[ImagesArgs]) time.Duration {
	return imagesTimeout
}

func (w *ImagesWorker) Work(ctx context.Context, job *river.Job[ImagesArgs]) error {
	extractor, err := w.newExtractor()
	if errors.Is(err, thumbnail.ErrFFmpegNotFound) {
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}
	ref := Ref{Library: job.Args.Library, Path: job.Args.Path}
	// ライブラリをマウントしていない nekonote では、取りやめずにやり直させ、ジョブの画面に理由を残す (プールの未マウントと同じ扱い)。
	library, ok := w.libraries.Get(ref.Library)
	if !ok {
		return fmt.Errorf("library %s is not mounted on this instance", ref.Library)
	}
	file, err := w.source.Open(ctx, ref)
	if errors.Is(err, medialib.ErrNotFound) || errors.Is(err, ErrNotAFile) {
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}
	switch {
	case file.ID() != job.Args.FileID:
		return river.JobCancel(errFileChanged)
	case file.Writing:
		return river.JobCancel(errFileWriting)
	case !file.Index.Complete:
		// 索引は tenban が作るので、作り終えるまで待つ。
		return river.JobSnooze(imagesSnooze)
	}
	segments := []thumbnail.Segment{{
		Path: ref.Library + "/" + ref.Path, Start: 0, Index: file.Index.Entries,
		Open: func() (io.ReadSeekCloser, error) { return library.Open(ctx, ref.Path) },
	}}
	opts := &thumbnail.Options{Preview: w.preview, Interval: 0, Latest: false, Avoid: nil}
	result, err := thumbnail.NewGenerator(extractor, opts).Generate(ctx, segments)
	if errors.Is(err, thumbnail.ErrNoKeyframe) {
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}

	return w.write(ref, job.Args.FileID, result)
}

// write は、画像を索引の置き場に書き、どのファイルから作ったかを記録する。
func (w *ImagesWorker) write(ref Ref, fileID string, result *thumbnail.Result) error {
	dir, err := w.indexer.ImageDir(ref)
	if err != nil {
		return err
	}
	if err := WriteFile(dir, ThumbnailFileName, result.Thumbnail); err != nil {
		return err
	}
	if result.Preview != nil {
		manifest, err := json.Marshal(result.Manifest)
		if err != nil {
			return fmt.Errorf("failed to encode the preview manifest: %w", err)
		}
		if err := WriteFile(dir, PreviewFileName, result.Preview); err != nil {
			return err
		}
		if err := WriteFile(dir, ManifestFileName, manifest); err != nil {
			return err
		}
	}

	return w.indexer.MarkImages(ref, fileID)
}

// ImagesInserter は、ジョブを入れる。*jobs.Client が満たす。
type ImagesInserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

var _ ImagesInserter = new(jobs.Client)

// EnqueueImages は、ファイル file の画像がまだなければ、作るジョブを入れる。同じファイルのジョブが待っていれば、重ねない。
// 索引を作り終えていないか、書いている途中のファイルには入れない。
func EnqueueImages(ctx context.Context, inserter ImagesInserter, indexer *Indexer, file *File) error {
	if !file.Index.Complete || file.Writing || indexer.HasImages(file.Ref, file.ID()) {
		return nil
	}
	if _, err := inserter.Insert(ctx, ImagesArgs{Library: file.Ref.Library, Path: file.Ref.Path, FileID: file.ID()}, nil); err != nil {
		return fmt.Errorf("failed to enqueue the library images job: %w", err)
	}

	return nil
}
