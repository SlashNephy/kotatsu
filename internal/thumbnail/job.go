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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/jobs"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	// ThumbnailFileName、PreviewFileName、ManifestFileName は、最初のセグメントの付属ファイルのディレクトリに置く、
	// 代表画像、プレビュー画像、プレビュー画像の説明のファイルの名前 (設計書 5.6)。
	ThumbnailFileName = "thumbnail.jpg"
	PreviewFileName   = "preview.jpg"
	ManifestFileName  = "preview.json"

	// kindSegment、kindIndex、kindThumbnail、kindPreview は、recording_files の種類。
	kindSegment   = "segment"
	kindIndex     = "index"
	kindThumbnail = "thumbnail"
	kindPreview   = "preview"

	// generateMaxAttempts は、画像を作るジョブを試す回数の上限。
	generateMaxAttempts = 3
	// generateTimeout は、画像を作るジョブの時間の上限。1 枚ずつ FFmpeg を動かすので、コマの数だけかかる。
	generateTimeout = 10 * time.Minute
	// foreignKeyViolation は、PostgreSQL の外部キーの違反のエラーコード。作っている間に録画が消されたときに返る。
	foreignKeyViolation = "23503"
)

var (
	errRecordingNotFound = errors.New("the recording no longer exists")
	errNoSegment         = errors.New("the recording has no segment with an index on the mounted pools")
)

// GenerateArgs は、録画の代表画像とプレビュー画像を作るジョブの引数。
type GenerateArgs struct {
	RecordingID uuid.UUID `json:"recordingId"`
}

var _ river.JobArgsWithInsertOpts = new(GenerateArgs)

func (GenerateArgs) Kind() string { return "thumbnail_generate" }

func (GenerateArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       jobs.QueueMedia,
		MaxAttempts: generateMaxAttempts,
		// 同じ録画の画像を作るジョブは、待っている間に重ねて投入しない。
		UniqueOpts: river.UniqueOpts{ByArgs: true},
	}
}

// Inserter は、ジョブをトランザクションの中で投入する。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// Enqueuer は、録画の完了のイベントを配るときに、その録画の画像を作るジョブを投入する。
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
		// 録画の ID のないイベントでは、画像を作れない。ほかの通知は配りたいので、失敗にはしない。
		return nil
	}
	params := []river.InsertManyParams{{Args: GenerateArgs{RecordingID: recordingID}, InsertOpts: nil}}
	if _, err := e.inserter.InsertManyTx(ctx, tx, params); err != nil {
		return fmt.Errorf("failed to enqueue the thumbnail generation: %w", err)
	}

	return nil
}

// recordingIDOf は、録画の完了のイベントの材料から、録画の ID を読む。
func recordingIDOf(payload map[string]any) (uuid.UUID, bool) {
	raw, _ := payload["recordingId"].(string)
	recordingID, err := uuid.Parse(raw)

	return recordingID, err == nil
}

// GenerateWorker は、録画の代表画像とプレビュー画像を作り、最初のセグメントの付属ファイルのディレクトリに置く。
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
	if _, err := q.GetRecording(ctx, job.Args.RecordingID); errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(errRecordingNotFound)
	} else if err != nil {
		return fmt.Errorf("failed to get the recording: %w", err)
	}
	files, err := q.ListRecordingFiles(ctx, job.Args.RecordingID)
	if err != nil {
		return fmt.Errorf("failed to list the recording files: %w", err)
	}
	segments, first, err := w.segments(files)
	if err != nil {
		return err
	}
	result, err := NewGenerator(extractor, &Options{Preview: w.preview, Interval: 0}).Generate(ctx, segments)
	if errors.Is(err, ErrNoKeyframe) {
		return river.JobCancel(err)
	}
	if err != nil {
		return err
	}
	written, err := w.write(first, result)
	if err != nil {
		return err
	}

	return w.record(ctx, job.Args.RecordingID, first, written)
}

// sidecar は、画像を置く、最初のセグメントの付属ファイルのディレクトリ。
type sidecar struct {
	poolID string
	// dir は、プール内の相対パス。
	dir string
	// startedAt は、最初のセグメントの始まり。画像の行に記録する。
	startedAt time.Time
}

// segments は、録画のファイルから、索引のあるセグメントを、始まりの順に読む。このインスタンスにマウントされていないプールのものは使えない。
func (w *GenerateWorker) segments(files []sqlc.RecordingFile) ([]Segment, *sidecar, error) {
	indexes := map[time.Time]*sqlc.RecordingFile{}
	for i := range files {
		if files[i].Kind == kindIndex {
			indexes[files[i].StartedAt] = &files[i]
		}
	}
	var segments []Segment
	var first *sidecar
	for i := range files {
		file := &files[i]
		index, ok := indexes[file.StartedAt]
		if file.Kind != kindSegment || file.Size == 0 || !ok {
			continue
		}
		segmentPath, err := w.store.Path(file.PoolID, file.Path)
		if errors.Is(err, kago.ErrPoolNotMounted) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		entries, err := readIndex(w.store, index)
		if err != nil {
			return nil, nil, err
		}
		if first == nil {
			first = &sidecar{poolID: file.PoolID, dir: "", startedAt: file.StartedAt}
			if first.dir, err = w.store.CreateSidecarDir(file.PoolID, file.Path); err != nil {
				return nil, nil, fmt.Errorf("failed to create the sidecar directory: %w", err)
			}
		}
		segments = append(segments, Segment{Path: segmentPath, Start: file.StartedAt.Sub(first.startedAt), Index: entries})
	}
	if first == nil {
		return nil, nil, river.JobCancel(errNoSegment)
	}

	return segments, first, nil
}

func readIndex(store *kago.Store, index *sqlc.RecordingFile) ([]ts.IndexEntry, error) {
	indexPath, err := store.Path(index.PoolID, index.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read the index: %w", err)
	}
	entries, err := ts.ReadIndex(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse the index %s: %w", index.Path, err)
	}

	return entries, nil
}

// writtenImage は、書いた画像のファイル。
type writtenImage struct {
	kind string
	// path は、プール内の相対パス。
	path string
	size int64
}

// output は、付属ファイルのディレクトリに書くファイル。
type output struct {
	// kind は、録画のファイルとして記録する種類。空なら記録しない。
	kind string
	name string
	data []byte
}

// write は、画像を付属ファイルのディレクトリに書く。作り直したときは、上書きする。
func (w *GenerateWorker) write(first *sidecar, result *Result) ([]writtenImage, error) {
	outputs := []output{{kind: kindThumbnail, name: ThumbnailFileName, data: result.Thumbnail}}
	if result.Preview != nil {
		manifest, err := json.Marshal(result.Manifest)
		if err != nil {
			return nil, fmt.Errorf("failed to encode the preview manifest: %w", err)
		}
		// プレビュー画像の説明は、プレビュー画像と同じディレクトリにあるので、DB には記録しない。
		outputs = append(outputs,
			output{kind: kindPreview, name: PreviewFileName, data: result.Preview},
			output{kind: "", name: ManifestFileName, data: manifest})
	}
	written := make([]writtenImage, 0, len(outputs))
	for _, out := range outputs {
		rel := path.Join(first.dir, out.name)
		abs, err := w.store.Path(first.poolID, rel)
		if err != nil {
			return nil, err
		}
		if err := writeFile(abs, out.data); err != nil {
			return nil, err
		}
		if out.kind != "" {
			written = append(written, writtenImage{kind: out.kind, path: rel, size: int64(len(out.data))})
		}
	}

	return written, nil
}

// writeFile は、一時ファイルに書いてから名前を変え、読む側に書きかけのファイルを見せない。
func writeFile(name string, data []byte) error {
	tmp := name + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", name, err)
	}
	if err := os.Rename(tmp, name); err != nil {
		return fmt.Errorf("failed to rename %s: %w", tmp, err)
	}

	return nil
}

// record は、書いた画像を、録画のファイルとして記録する。作っている間に録画が消されていたら、取りやめる。
func (w *GenerateWorker) record(ctx context.Context, recordingID uuid.UUID, first *sidecar, images []writtenImage) error {
	now := w.clock.Now()
	q := sqlc.New(w.db)
	for _, img := range images {
		err := q.UpsertRecordingImage(ctx, sqlc.UpsertRecordingImageParams{
			ID: id.New(), RecordingID: recordingID, Kind: img.kind, PoolID: first.poolID, Path: img.path, Size: img.size,
			StartedAt: first.startedAt, Now: &now,
		})
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == foreignKeyViolation {
			return river.JobCancel(errRecordingNotFound)
		}
		if err != nil {
			return fmt.Errorf("failed to record the %s: %w", img.kind, err)
		}
	}

	return nil
}
