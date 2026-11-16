package filelib

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
)

const (
	// captionsMaxAttempts は、字幕を生成するジョブを試す回数の上限。録画と同じ。
	captionsMaxAttempts = 3
	// captionsTimeout は、字幕を生成するジョブの時間の上限。録画と同じ。
	captionsTimeout = 6 * time.Hour
)

// CaptionsArgs は、ライブラリのファイル (#1734) の音声から字幕を自動生成するジョブの引数。画像 (ImagesArgs) と同じく、ライブラリの名前と
// パスで指し、頼んだときのファイルの ID を添える。
type CaptionsArgs struct {
	Library string `json:"library"`
	Path    string `json:"path"`
	FileID  string `json:"fileId"`
}

var _ river.JobArgsWithInsertOpts = new(CaptionsArgs)

func (CaptionsArgs) Kind() string { return "library_auto_captions" }

func (CaptionsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// 録画と同じく、字幕のキューで 1 つずつ動かす。
		Queue:       jobs.QueueCaptions,
		MaxAttempts: captionsMaxAttempts,
		// 利用者が頼んだときだけ入れるので、録画と同じく、待っている間だけ重ねない。終えたジョブには重ねてよい (生成し直せるようにする)。
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
			rivertype.JobStateRunning, rivertype.JobStateScheduled,
		}},
	}
}

// CaptionsWorker は、ライブラリのファイルの音声から字幕を自動生成する (設計書 5.8)。録画と同じ部品 (autocaption) で、音声を取り出して
// whisper-cli で書き起こし、索引の置き場に置く。nekonote が、ライブラリとデータディレクトリをマウントして動かす。
type CaptionsWorker struct {
	river.WorkerDefaults[CaptionsArgs]

	libraries      *medialib.Set
	indexer        *Indexer
	source         *Source
	clock          clock.Clock
	newTranscriber func() (autocaption.Transcriber, error)
	newExtractor   func() (autocaption.ReaderExtractor, error)
	// workDir は、取り出した WAV を置く一時ディレクトリを作る親。空なら OS の一時ディレクトリ。
	workDir string
}

var _ river.Worker[CaptionsArgs] = new(CaptionsWorker)

// NewCaptionsWorker は CaptionsWorker を返す。indexer は、索引を作らない (読むだけの) もの。newTranscriber と newExtractor は、ジョブごとに呼ぶ。
func NewCaptionsWorker(
	libraries *medialib.Set, indexer *Indexer, clk clock.Clock,
	newTranscriber func() (autocaption.Transcriber, error), newExtractor func() (autocaption.ReaderExtractor, error), workDir string,
) *CaptionsWorker {
	return &CaptionsWorker{
		WorkerDefaults: river.WorkerDefaults[CaptionsArgs]{}, libraries: libraries, indexer: indexer, source: NewSource(libraries, indexer),
		clock: clk, newTranscriber: newTranscriber, newExtractor: newExtractor, workDir: workDir,
	}
}

func (w *CaptionsWorker) Timeout(*river.Job[CaptionsArgs]) time.Duration {
	return captionsTimeout
}

func (w *CaptionsWorker) Work(ctx context.Context, job *river.Job[CaptionsArgs]) error {
	transcriber, err := w.newTranscriber()
	if errors.Is(err, autocaption.ErrNotConfigured) {
		// 設定を直すまでやり直しても同じなので、取りやめる。画面は、この理由を見分けて、置き方を示す。
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}
	extract, err := w.newExtractor()
	if errors.Is(err, autocaption.ErrFFmpegNotFound) {
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}
	ref := Ref{Library: job.Args.Library, Path: job.Args.Path}
	library, file, err := w.open(ctx, ref, job.Args.FileID)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp(w.workDir, "kotatsu-captions-")
	if err != nil {
		return fmt.Errorf("failed to create a working directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	cues, err := w.transcribe(ctx, library, file, extract, transcriber, filepath.Join(dir, "audio.wav"))
	if err != nil {
		return err
	}

	return w.indexer.WriteCaptions(ref, &CaptionsResult{FileID: job.Args.FileID, Model: transcriber.Model(), GeneratedAt: w.clock.Now(), Cues: cues})
}

// open は、ジョブのファイルを開く。頼んだ後に書き換えたファイルと、書いている途中のファイルは取りやめる。
func (w *CaptionsWorker) open(ctx context.Context, ref Ref, fileID string) (medialib.Library, *File, error) {
	// ライブラリをマウントしていない nekonote では、取りやめずにやり直させ、ジョブの画面に理由を残す (画像と同じ)。
	library, ok := w.libraries.Get(ref.Library)
	if !ok {
		return nil, nil, fmt.Errorf("library %s is not mounted on this instance", ref.Library)
	}
	file, err := w.source.Open(ctx, ref)
	if errors.Is(err, medialib.ErrNotFound) || errors.Is(err, ErrNotAFile) {
		return nil, nil, river.JobCancel(err)
	}
	if err != nil {
		return nil, nil, err
	}
	switch {
	case file.ID() != fileID:
		return nil, nil, river.JobCancel(errFileChanged)
	case file.Writing:
		return nil, nil, river.JobCancel(errFileWriting)
	case file.Index.Broadcast == nil:
		// 音声を引くサービスが分からない。
		return nil, nil, river.JobCancel(errNoService)
	}

	return library, file, nil
}

// transcribe は、ファイルの音声を wav に取り出して書き起こし、再生の 0 秒からの時間にそろえた字幕を返す。音声がなければ空を返す。
func (w *CaptionsWorker) transcribe(
	ctx context.Context, library medialib.Library, file *File, extract autocaption.ReaderExtractor, transcriber autocaption.Transcriber, wav string,
) ([]*CaptionCue, error) {
	r, err := library.Open(ctx, file.Ref.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open the library file: %w", err)
	}
	offset, err := extract(ctx, r, file.Index.Broadcast.ServiceID, wav)
	_ = r.Close()
	if errors.Is(err, autocaption.ErrNoAudio) {
		return []*CaptionCue{}, nil
	}
	if err != nil {
		return nil, err
	}
	found, err := transcriber.Transcribe(ctx, wav, nil)
	if err != nil {
		return nil, err
	}
	cues := make([]*CaptionCue, 0, len(found))
	for _, c := range found {
		start, end := max(offset+c.Start, 0), offset+c.End
		if end <= start {
			// 最初の I フレームより前 (再生に入らない所) で終わる字幕は捨てる。
			continue
		}
		cues = append(cues, &CaptionCue{Start: start, End: end, Text: c.Text})
	}
	slices.SortStableFunc(cues, func(a, b *CaptionCue) int { return cmp.Compare(a.Start, b.Start) })

	return cues, nil
}

// EnqueueCaptions は、ファイル file の音声から字幕を生成するジョブを入れる。同じファイルのジョブを待っていれば、入れずに false を返す。
// 前に生成した字幕は、生成し終えたら置き換える。
func EnqueueCaptions(ctx context.Context, inserter JobInserter, file *File) (bool, error) {
	result, err := inserter.Insert(ctx, CaptionsArgs{Library: file.Ref.Library, Path: file.Ref.Path, FileID: file.ID()}, nil)
	if err != nil {
		return false, fmt.Errorf("failed to enqueue the library captions job: %w", err)
	}

	return !result.UniqueSkippedAsDuplicate, nil
}
