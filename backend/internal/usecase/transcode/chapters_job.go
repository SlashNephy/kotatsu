package transcode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

const (
	// chaptersMaxAttempts は、チャプターを書くジョブを試す回数の上限。
	chaptersMaxAttempts = 3
	// chaptersTimeout は、チャプターを書くジョブの時間の上限。再エンコードせずに写すだけなので、ディスクを読み書きする時間で済む。
	chaptersTimeout = 30 * time.Minute
	// headScanLimit は、最初のセグメントの最初の映像の PTS を探すのに読む長さの上限。PAT と PMT は 0.1 秒ごとに届くので、ずっと手前で見つかる。
	headScanLimit = 32 << 20
	// metadataSuffix は、チャプターを書いた ffmetadata の一時ファイルに付ける拡張子。
	metadataSuffix = ".ffmetadata"
)

// ChaptersArgs は、録画を変換した MP4 に、CM の区間の境目をチャプターとして書くジョブの引数 (#1510)。
type ChaptersArgs struct {
	RecordingID uuid.UUID `json:"recordingId"`
}

var _ river.JobArgsWithInsertOpts = new(ChaptersArgs)

func (ChaptersArgs) Kind() string { return "recording_chapters" }

func (ChaptersArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// 再エンコードせずに写し直すだけで軽いので、変換 (transcode) の後ろで待たせず、代表画像などと同じキューで動かす。
		Queue:       jobs.QueueMedia,
		MaxAttempts: chaptersMaxAttempts,
		// 同じ録画のジョブは、待っている間に重ねて投入しない。River は running も含めることを求めるが、
		// 投入するのは CM の区間を見分けるジョブと変換のジョブが録画ごとのロックを持っている間で、このジョブはそのロックを取ってから
		// 区間とファイルを読むので、重ねずに捨てた投入の分も、動いているジョブが (ロックを取れずに後でやり直して) 新しい区間で書く。
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
			rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
			rivertype.JobStateRunning, rivertype.JobStateScheduled,
		}},
	}
}

// ChaptersEnqueuer は、CM の区間を見分け終えたときに、変換した MP4 があれば、チャプターを書くジョブを投入する。
// CM の区間を書いたトランザクションの中で呼ばれる (cmdetect.DetectedHook)。
type ChaptersEnqueuer struct {
	inserter Inserter
}

var _ cmdetect.DetectedHook = new(ChaptersEnqueuer)

// NewChaptersEnqueuer は ChaptersEnqueuer を返す。
func NewChaptersEnqueuer(inserter Inserter) *ChaptersEnqueuer {
	return &ChaptersEnqueuer{inserter: inserter}
}

// OnDetected は、録画 recordingID に変換した MP4 があれば、チャプターを書くジョブを投入する。
// CM が見つからなかったときも投入し、前に書いたチャプターを外させる。
func (e *ChaptersEnqueuer) OnDetected(ctx context.Context, tx pgx.Tx, recordingID uuid.UUID) error {
	files, err := sqlc.New(tx).ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return fmt.Errorf("failed to list the recording files: %w", err)
	}
	if len(transcodedFiles(files)) == 0 {
		return nil
	}

	return insertChapters(ctx, e.inserter, tx, recordingID)
}

func insertChapters(ctx context.Context, inserter Inserter, tx pgx.Tx, recordingID uuid.UUID) error {
	if _, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: ChaptersArgs{RecordingID: recordingID}, InsertOpts: nil}}); err != nil {
		return fmt.Errorf("failed to enqueue the chapters of the transcoded files: %w", err)
	}

	return nil
}

// transcodedFiles は、録画のファイル files から、Kotatsu が変換した MP4 を返す。最初のセグメントの名前から、画質ごとに作る名前 (outputPath) の
// その他のファイルだけを選ぶ。EPGStation から取り込んだ MP4 は、録画の時間軸とそろっているか分からないので、書き換えない。
func transcodedFiles(files []sqlc.RecordingFile) []sqlc.RecordingFile {
	first := slices.IndexFunc(files, func(f sqlc.RecordingFile) bool { return f.Kind == kindSegment })
	if first < 0 {
		return []sqlc.RecordingFile{}
	}
	out := []sqlc.RecordingFile{}
	for _, preset := range domain.TranscodePresets() {
		rel := outputPath(files[first].Path, preset)
		i := slices.IndexFunc(files, func(f sqlc.RecordingFile) bool {
			return f.Kind == kindOther && f.PoolID == files[first].PoolID && f.Path == rel
		})
		if i >= 0 {
			out = append(out, files[i])
		}
	}

	return out
}

// ChaptersWorker は、録画を変換した MP4 に、CM の区間と本編を交互に並べたチャプターを書き直す (#1510)。
// ファイルは切り詰めず、再エンコードもせずに写し直す。CM の区間がなければ、チャプターを外す。
type ChaptersWorker struct {
	river.WorkerDefaults[ChaptersArgs]

	db     Database
	store  *kago.Store
	clock  clock.Clock
	source *stream.RecordingSource
	// newChapterer は、写し直すのに使う Chapterer を作る。テストでは差し替える。
	newChapterer func() (Chapterer, error)
}

var _ river.Worker[ChaptersArgs] = new(ChaptersWorker)

// NewChaptersWorker は ChaptersWorker を返す。store は、このインスタンスにマウントされたプール。
func NewChaptersWorker(db Database, store *kago.Store, clk clock.Clock, newChapterer func() (Chapterer, error)) *ChaptersWorker {
	return &ChaptersWorker{
		WorkerDefaults: river.WorkerDefaults[ChaptersArgs]{}, db: db, store: store, clock: clk,
		source: stream.NewRecordingSource(db, store, 0), newChapterer: newChapterer,
	}
}

func (w *ChaptersWorker) Timeout(*river.Job[ChaptersArgs]) time.Duration {
	return chaptersTimeout
}

// Work は、録画ごとの advisory lock を取ってから、チャプターを書き直す。
// 変換と CM の区間の見分けと同じロックを使うので、MP4 を置き換えている間や、区間を書き換えている間には読まない。
func (w *ChaptersWorker) Work(ctx context.Context, job *river.Job[ChaptersArgs]) error {
	chapterer, err := w.newChapterer()
	if errors.Is(err, ErrFFmpegNotFound) {
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}

	return withRecordingLock(ctx, w.db, job.Args.RecordingID, func() error {
		return w.work(ctx, chapterer, job.Args.RecordingID)
	})
}

// target は、チャプターを書き直す MP4。
type target struct {
	file    *sqlc.RecordingFile
	path    string
	partial string
}

func (w *ChaptersWorker) work(ctx context.Context, chapterer Chapterer, recordingID uuid.UUID) error {
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
	transcoded := transcodedFiles(files)
	if len(transcoded) == 0 {
		return nil
	}
	sections, err := cmdetect.RecordingSections(ctx, w.db, recordingID)
	if err != nil {
		return err
	}
	tl := &chapterTimeline{recordedFrom: recording.RecordedFrom, gaps: []Gap{}, head: 0, duration: 0}
	if err := json.Unmarshal(recording.Gaps, &tl.gaps); err != nil {
		return fmt.Errorf("failed to decode the recording gaps: %w", err)
	}
	var lead time.Duration
	if len(sections) > 0 {
		if lead, err = w.headLead(ctx, recordingID, files, tl); err != nil {
			return err
		}
	}

	// 一部の MP4 だけが新しくなる状態を残さないよう、全部を一時ファイルに書いてから、まとめて置き換える。
	targets := make([]*target, 0, len(transcoded))
	defer func() {
		for _, t := range targets {
			_ = os.Remove(t.partial)
		}
	}()
	for i := range transcoded {
		file := &transcoded[i]
		path, err := w.store.Path(file.PoolID, file.Path)
		if err != nil {
			return river.JobCancel(fmt.Errorf("%w: %w", errNoSegment, err))
		}
		info, err := chapterer.Probe(ctx, path)
		if err != nil {
			return err
		}
		tl.head, tl.duration = lead-info.VideoStart, info.Duration
		chapters := buildChapters(sections, tl)
		if len(chapters) == 0 && info.Chapters == 0 {
			// 外すチャプターもなければ、写し直さない。
			continue
		}
		t, err := w.rewrite(ctx, chapterer, path, chapters)
		if t != nil {
			t.file = file
			targets = append(targets, t)
		}
		if err != nil {
			return err
		}
	}

	return w.replace(ctx, targets)
}

// headLead は、CM の区間の時間軸のうち、変換が流し始める最初の I フレームより前にあって MP4 に入らない長さを返す。
//
// CM の区間は、セグメントごとに、そのファイルの最初の映像の PES (ts.CueScanner の FirstVideoPTS) を、セグメントの始まりの日時に置いて数える。
// 変換は、録画を最初の I フレームから流す (stream.Recording.Seek(0))。GOP の途中から録り始めたセグメントでは、それより前の映像と音声を捨てる。
// MP4 の時間は、その I フレームを MP4 の最初の映像のコマ (MP4Info.VideoStart) に置いて数える。そこで、MP4 の時間は、
// 再生の時間から、最初のセグメントの始まりの再生の時間と、最初の映像から最初の I フレームまでの長さを引き、VideoStart を足して求める。
// ここでは、VideoStart を足す前の分を返す。索引がなく、最初の I フレームが分からなければ、最初の映像から流したものとみなす。
func (w *ChaptersWorker) headLead(ctx context.Context, recordingID uuid.UUID, files []sqlc.RecordingFile, tl *chapterTimeline) (time.Duration, error) {
	first := slices.IndexFunc(files, func(f sqlc.RecordingFile) bool { return f.Kind == kindSegment && f.Size > 0 })
	if first < 0 {
		return 0, river.JobCancel(errNoSegment)
	}
	lead := playbackPosition(tl.recordedFrom, tl.gaps, files[first].StartedAt)
	opened, err := w.source.Open(ctx, recordingID)
	if errors.Is(err, stream.ErrNotMounted) || errors.Is(err, stream.ErrNoSegment) || errors.Is(err, stream.ErrRecordingNotFound) {
		return 0, river.JobCancel(fmt.Errorf("failed to open the recording: %w", err))
	}
	if err != nil {
		return 0, fmt.Errorf("failed to open the recording: %w", err)
	}
	points := opened.KeyframePoints()
	if len(points) == 0 || points[0].PTS < 0 {
		return lead, nil
	}
	sid, err := sqlc.New(w.db).GetRecordingSID(ctx, recordingID)
	if err != nil {
		return 0, fmt.Errorf("failed to get the service of the recording: %w", err)
	}
	path, err := w.store.Path(files[first].PoolID, files[first].Path)
	if err != nil {
		return 0, river.JobCancel(fmt.Errorf("%w: %w", errNoSegment, err))
	}
	firstVideo, err := firstVideoPTS(path, sid)
	if err != nil || firstVideo < 0 {
		return lead, err
	}
	skipped := time.Duration(ts.PTSDiff(firstVideo, points[0].PTS)) * time.Second / ts.PTSClock

	return lead + skipped, nil
}

// firstVideoPTS は、セグメント path の、サービス sid の最初の映像の PES の PTS を返す。CM の区間を見分けるときと同じく
// ts.CueScanner で読む。headScanLimit までに見つからなければ -1 を返す。
func firstVideoPTS(path string, sid int64) (int64, error) {
	file, err := sharedfile.Open(path)
	if err != nil {
		return 0, fmt.Errorf("failed to open the first segment: %w", err)
	}
	defer func() { _ = file.Close() }()
	var serviceID uint16
	if sid > 0 && sid <= 0xffff {
		serviceID = uint16(sid)
	}
	scanner := ts.NewCueScanner(serviceID)
	buf := make([]byte, 1<<20)
	for read := 0; read < headScanLimit; {
		n, err := file.Read(buf)
		_, _ = scanner.Write(buf[:n])
		read += n
		if pts := scanner.Cues().FirstVideoPTS; pts != nil {
			return *pts, nil
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("failed to read the first segment: %w", err)
		}
	}

	return -1, nil
}

// rewrite は、path の MP4 を、chapters に替えて、ジョブ固有の一時ファイルに写す。一時ファイルを作れたら、書けなくても返す (消すのは呼び手)。
func (w *ChaptersWorker) rewrite(ctx context.Context, chapterer Chapterer, path string, chapters []Chapter) (*target, error) {
	dir, base := filepath.Dir(path), filepath.Base(path)
	partialFile, err := os.CreateTemp(dir, base+".*"+partialSuffix)
	if err != nil {
		return nil, fmt.Errorf("failed to create the rewritten file: %w", err)
	}
	t := &target{file: nil, path: path, partial: partialFile.Name()}
	_ = partialFile.Close()
	metadata := ""
	if len(chapters) > 0 {
		metadataFile, err := os.CreateTemp(dir, base+".*"+metadataSuffix)
		if err != nil {
			return t, fmt.Errorf("failed to create the chapters file: %w", err)
		}
		metadata = metadataFile.Name()
		defer func() { _ = os.Remove(metadata) }()
		_, err = metadataFile.Write(ffmetadata(chapters))
		if closeErr := metadataFile.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return t, fmt.Errorf("failed to write the chapters file: %w", err)
		}
	}

	return t, chapterer.Rewrite(ctx, path, metadata, t.partial)
}

// replace は、書き終えた一時ファイルを元の名前に置き換え、大きさを記録し直す。写し直しても映像は変わらないので、コマ数 (#1509) は前の記録のままにする。
func (w *ChaptersWorker) replace(ctx context.Context, targets []*target) error {
	for _, t := range targets {
		if err := os.Rename(t.partial, t.path); err != nil {
			return fmt.Errorf("failed to replace the transcoded file: %w", err)
		}
	}
	q := sqlc.New(w.db)
	for _, t := range targets {
		info, err := os.Stat(t.path)
		if err != nil {
			return fmt.Errorf("failed to stat the transcoded file: %w", err)
		}
		_, err = q.UpsertTranscodedRecordingFile(ctx, sqlc.UpsertTranscodedRecordingFileParams{
			ID: id.New(), RecordingID: t.file.RecordingID, PoolID: t.file.PoolID, Path: t.file.Path, Size: info.Size(),
			StartedAt: t.file.StartedAt, EndedAt: t.file.EndedAt, TranscodeFrameRate: t.file.TranscodeFrameRate, Now: w.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to record the transcoded file: %w", err)
		}
	}

	return nil
}
