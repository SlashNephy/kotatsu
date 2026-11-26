package filelib

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/lib/procprio"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
)

const (
	// cmPriority は、ライブラリのファイルの CM の区間を見分けるジョブの優先度。開いたファイルの数だけ入るので、
	// 録画の CM の区間 (既定の優先度) を待たせないよう、後にする。
	cmPriority = 3
	// cmMaxAttempts は、見分けるジョブを試す回数の上限。録画と同じ。
	cmMaxAttempts = 3
	// cmSnooze は、インデックスを作り終えていないファイルを見分けるジョブを、待たせる時間。
	cmSnooze = 30 * time.Second
)

var errNoService = errors.New("the service of the library file is unknown")

// CMArgs は、ライブラリのファイル (#1734) の CM の区間を見分けるジョブの引数。画像 (ImagesArgs) と同じく、ライブラリの名前とパスで指し、
// 見分けるときのファイルの ID を添える。
type CMArgs struct {
	Library string `json:"library"`
	Path    string `json:"path"`
	FileID  string `json:"fileId"`
}

var _ river.JobArgsWithInsertOpts = new(CMArgs)

func (CMArgs) Kind() string { return "library_cm_detect" }

func (CMArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// 録画と同じく、ファイルを読み通すので、CM のキューで 1 つずつ動かす。
		Queue:       jobs.QueueCM,
		MaxAttempts: cmMaxAttempts,
		Priority:    cmPriority,
		// 画像と同じく、同じファイルのジョブは、待っているか動いている間と、取りやめたか諦めた後 (River が行を消すまで) は重ねない。
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable, rivertype.JobStateCancelled, rivertype.JobStateDiscarded, rivertype.JobStatePending,
			rivertype.JobStateRetryable, rivertype.JobStateRunning, rivertype.JobStateScheduled,
		}},
	}
}

// CMWorker は、ライブラリのファイルの CM の区間を見分ける (設計書 5.8)。録画と同じ見分け方 (cmdetect) で、管理者が全体で選んだものを使い、
// 結果をインデックスの保存先に置く。nekonote が、ライブラリとデータディレクトリをマウントして動かす。
type CMWorker struct {
	river.WorkerDefaults[CMArgs]

	libraries   *medialib.Set
	indexer     *Indexer
	source      *Source
	newDetector func(ctx context.Context) (cmdetect.Detector, error)
}

var _ river.Worker[CMArgs] = new(CMWorker)

// NewCMWorker は CMWorker を返す。indexer は、インデックスを作らない (読むだけの) もの。lowerer は、FFmpeg を最低の優先度で動かす。
func NewCMWorker(libraries *medialib.Set, indexer *Indexer, db sqlc.DBTX, lowerer *procprio.Lowerer) *CMWorker {
	return newCMWorker(libraries, indexer, func(ctx context.Context) (cmdetect.Detector, error) {
		return cmdetect.NewSelectedDetector(ctx, db, lowerer)
	})
}

func newCMWorker(libraries *medialib.Set, indexer *Indexer, newDetector func(ctx context.Context) (cmdetect.Detector, error)) *CMWorker {
	return &CMWorker{
		WorkerDefaults: river.WorkerDefaults[CMArgs]{}, libraries: libraries, indexer: indexer,
		source: NewSource(libraries, indexer), newDetector: newDetector,
	}
}

// Timeout は、時間の上限の上限を返す。ファイルの長さに合わせた上限は、インデックスを読んでから Work で掛ける (録画と同じ。#1505)。
func (w *CMWorker) Timeout(*river.Job[CMArgs]) time.Duration {
	return cmdetect.MaxTimeLimit
}

func (w *CMWorker) Work(ctx context.Context, job *river.Job[CMArgs]) error {
	detector, err := w.newDetector(ctx)
	if errors.Is(err, cmdetect.ErrFFmpegNotFound) {
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}
	ref := Ref{Library: job.Args.Library, Path: job.Args.Path}
	// ライブラリをマウントしていない nekonote では、取りやめずにやり直させ、ジョブの画面に理由を残す (画像と同じ)。
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
		return river.JobSnooze(cmSnooze)
	case file.Index.Broadcast == nil:
		// 映像と音声を取り出すサービスが分からない。
		return river.JobCancel(errNoService)
	}
	_, duration, _ := file.Timeline()
	ctx, cancel := context.WithTimeout(ctx, cmdetect.TimeLimit(duration))
	defer cancel()
	input := &cmdetect.Input{
		Path: ref.Library + "/" + ref.Path, ServiceID: file.Index.Broadcast.ServiceID, Progress: nil,
		Program: cmProgram(file.Index.Broadcast), Duration: duration,
		Open: func() (io.ReadSeekCloser, error) { return library.Open(ctx, ref.Path) },
	}
	sections, report, err := cmdetect.Detect(ctx, detector, input)
	if errors.Is(err, cmdetect.ErrNoVideo) {
		// 見分けるものがない。見分けたことは残し、画面に待たせない。
		sections, report, err = []cmdetect.Section{}, &cmdetect.Report{Note: "", Missing: 0}, nil
	}
	if err != nil {
		return err
	}
	result := &CMResult{
		FileID: job.Args.FileID, Detector: detector.Name(), Version: detector.Version(),
		Sections: make([]*CMSection, 0, len(sections)), Note: report.Note,
	}
	for _, s := range sections {
		result.Sections = append(result.Sections, &CMSection{Start: s.Start, End: s.End})
	}

	return w.indexer.WriteCM(ref, result)
}

// cmProgram は、番組のウィンドウ (#1506) に渡す、番組の時刻を返す。番組が始まるはずの時刻は、番組の始まりを、録り始め (再生の 0 秒) からの
// 時間にしたもの。番組の始まりと尺か、録り始めが分からなければ nil。
func cmProgram(broadcast *Broadcast) *cmdetect.Program {
	program := broadcast.Program
	if program == nil || program.Start == nil || program.Duration == nil || broadcast.RecordedFrom == nil || *program.Duration <= 0 {
		return nil
	}

	return &cmdetect.Program{Start: program.Start.Sub(*broadcast.RecordedFrom), Duration: *program.Duration, Fixed: false, Scheduled: 0}
}

// EnqueueCM は、ファイル file の CM の区間をまだ見分けていなければ、見分けるジョブを入れる。同じファイルのジョブが待っていれば、重ねない。
// インデックスを作り終えていないか、書いている途中か、サービスが分からないファイルには入れない。
func EnqueueCM(ctx context.Context, inserter JobInserter, indexer *Indexer, file *File) error {
	if !file.Index.Complete || file.Writing || file.Index.Broadcast == nil {
		return nil
	}
	if _, ok := indexer.CM(file.Ref, file.ID()); ok {
		return nil
	}
	if _, err := inserter.Insert(ctx, CMArgs{Library: file.Ref.Library, Path: file.Ref.Path, FileID: file.ID()}, nil); err != nil {
		return fmt.Errorf("failed to enqueue the library CM job: %w", err)
	}

	return nil
}
