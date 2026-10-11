// Package analysis は、録画を終えた後に録画ファイルを解析するジョブを扱う (設計書 6.1、6.3)。
//
// EPGStation から取り込んだ録画には、Kotatsu が録画中に作る索引やドロップの集計がない。
// このジョブは、取り込んだ TS を読み直して、セグメントの索引を作り、ドロップ、エラー、スクランブルを数え直す。
// 索引ができたら、代表画像とプレビュー画像を作るジョブを投入する。
package analysis

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	"github.com/SlashNephy/kotatsu/internal/thumbnail"
	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	// kindSegment は、recording_files の、TS のセグメントの種類。
	kindSegment = "segment"
	// stateRecording は、録画中の録画の状態。
	stateRecording = "recording"

	// analyzeMaxAttempts は、解析するジョブを試す回数の上限。
	analyzeMaxAttempts = 3
	// analyzeTimeout は、解析するジョブの時間の上限。数 GB の TS を読み通すので、長めにとる。
	analyzeTimeout = 30 * time.Minute
	// foreignKeyViolation は、PostgreSQL の外部キーの違反のエラーコード。解析している間に録画が消されたときに返る。
	foreignKeyViolation = "23503"
)

var (
	errRecordingNotFound = errors.New("the recording no longer exists")
	errStillRecording    = errors.New("the recording is still being recorded")
	errNoSegment         = errors.New("the recording has no segment on the mounted pools")
	errPartiallyMounted  = errors.New("some segments of the recording are on pools that are not mounted")
)

// AnalyzeArgs は、録画を終えた後に録画ファイルを解析するジョブの引数。
type AnalyzeArgs struct {
	RecordingID uuid.UUID `json:"recordingId"`
}

var _ river.JobArgsWithInsertOpts = new(AnalyzeArgs)

func (AnalyzeArgs) Kind() string { return "recording_analyze" }

func (AnalyzeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// 録画ファイルを読み通すので、FFmpeg を動かすジョブと同じく、同時に 1 つだけ動かす。
		Queue:       jobs.QueueMedia,
		MaxAttempts: analyzeMaxAttempts,
		// 同じ録画を解析するジョブは、待っている間に重ねて投入しない。
		UniqueOpts: river.UniqueOpts{ByArgs: true},
	}
}

// Inserter は、ジョブをトランザクションの中で投入する。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// Database は、解析に使う DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// AnalyzeWorker は、録画のセグメントを読み、索引を作り、ドロップ、エラー、スクランブルを数え直す。
// 索引と集計を記録するトランザクションで、代表画像とプレビュー画像を作るジョブを投入する。
type AnalyzeWorker struct {
	river.WorkerDefaults[AnalyzeArgs]

	db       Database
	store    *kago.Store
	clock    clock.Clock
	inserter Inserter
}

var _ river.Worker[AnalyzeArgs] = new(AnalyzeWorker)

// NewAnalyzeWorker は AnalyzeWorker を返す。store は、このインスタンスにマウントされたプール。
func NewAnalyzeWorker(db Database, store *kago.Store, clk clock.Clock, inserter Inserter) *AnalyzeWorker {
	return &AnalyzeWorker{WorkerDefaults: river.WorkerDefaults[AnalyzeArgs]{}, db: db, store: store, clock: clk, inserter: inserter}
}

func (w *AnalyzeWorker) Timeout(*river.Job[AnalyzeArgs]) time.Duration {
	return analyzeTimeout
}

// analyzed は、解析したセグメントと、その索引。
type analyzed struct {
	segment   *sqlc.RecordingFile
	indexPath string
	indexSize int64
}

func (w *AnalyzeWorker) Work(ctx context.Context, job *river.Job[AnalyzeArgs]) error {
	q := sqlc.New(w.db)
	recording, err := q.GetRecording(ctx, job.Args.RecordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(errRecordingNotFound)
	}
	if err != nil {
		return fmt.Errorf("failed to get the recording: %w", err)
	}
	// 録画中の録画は、録画が索引と集計を作っている。
	if recording.State == stateRecording {
		return river.JobCancel(errStillRecording)
	}
	files, err := q.ListRecordingFiles(ctx, job.Args.RecordingID)
	if err != nil {
		return fmt.Errorf("failed to list the recording files: %w", err)
	}

	segments, err := w.segments(files)
	if err != nil {
		return err
	}
	results := make([]*analyzed, 0, len(segments))
	total := new(ts.PIDStats)
	events := []*ts.EventAudio{}
	for _, segment := range segments {
		result, stats, err := w.analyze(ctx, segment)
		if err != nil {
			return err
		}
		results = append(results, result)
		events = append(events, stats.Events...)
		for _, pid := range stats.PIDs {
			total.Drops += pid.Drops
			total.Errors += pid.Errors
			total.Scrambled += pid.Scrambled
		}
	}

	// EPG から音声の構成を写せなかった録画 (取り込んだ録画や時刻予約) は、TS の EIT[p/f] から読んだもので埋める (設計書 6.1)。
	audios, ok, err := ts.RecordingAudios(events, recording.ServiceID, recording.EventID)
	if err != nil {
		return fmt.Errorf("failed to encode the audio components: %w", err)
	}
	if !ok {
		audios = nil
	}

	return w.record(ctx, job.Args.RecordingID, results, total, audios)
}

// segments は、録画のファイルから TS のセグメントを選ぶ。
// 一部のセグメントだけで集計を置き換えると、残りのセグメントのドロップが集計から消えるので、
// このインスタンスにマウントされていないプールのセグメントがあれば、取りやめる。
func (w *AnalyzeWorker) segments(files []sqlc.RecordingFile) ([]*sqlc.RecordingFile, error) {
	segments := []*sqlc.RecordingFile{}
	unmounted := 0
	for i := range files {
		file := &files[i]
		if file.Kind != kindSegment {
			continue
		}
		if _, err := w.store.Path(file.PoolID, file.Path); errors.Is(err, kago.ErrPoolNotMounted) {
			unmounted++

			continue
		}
		segments = append(segments, file)
	}
	if len(segments) == 0 {
		return nil, river.JobCancel(errNoSegment)
	}
	if unmounted > 0 {
		return nil, river.JobCancel(fmt.Errorf("%w: %d of %d segments", errPartiallyMounted, unmounted, unmounted+len(segments)))
	}

	return segments, nil
}

// analyze は、セグメントを読み通して、付属ファイルのディレクトリに索引を書き、統計を返す。
// 索引は作り直すたびに切り詰めて書くので、何度実行しても同じ索引になる。
func (w *AnalyzeWorker) analyze(ctx context.Context, segment *sqlc.RecordingFile) (*analyzed, *ts.Stats, error) {
	segmentPath, err := w.store.Path(segment.PoolID, segment.Path)
	if err != nil {
		return nil, nil, err
	}
	input, err := os.Open(segmentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open the segment: %w", err)
	}
	defer func() {
		_ = input.Close()
	}()
	if _, err := w.store.CreateSidecarDir(segment.PoolID, segment.Path); err != nil {
		return nil, nil, err
	}
	indexRel := kago.IndexPath(segment.Path)
	indexPath, err := w.store.Path(segment.PoolID, indexRel)
	if err != nil {
		return nil, nil, err
	}
	output, err := os.OpenFile(indexPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create the index file: %w", err)
	}
	defer func() {
		_ = output.Close()
	}()
	writer, err := ts.NewIndexWriter(output)
	if err != nil {
		return nil, nil, err
	}
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer})
	// 数 GB のセグメントを読む間でも、nekonote を止めたら読むのをやめる。
	reader := readerFunc(func(p []byte) (int, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}

		return input.Read(p)
	})
	if _, err := io.Copy(analyzer, reader); err != nil {
		return nil, nil, fmt.Errorf("failed to read the segment %s/%s: %w", segment.PoolID, segment.Path, err)
	}
	stats := analyzer.Stats()
	// 解析器は、索引を書けなくても解析を続けるので、ここで確かめる。
	if stats.IndexError != nil {
		return nil, nil, fmt.Errorf("failed to write the index: %w", stats.IndexError)
	}
	if err := output.Close(); err != nil {
		return nil, nil, fmt.Errorf("failed to close the index file: %w", err)
	}
	info, err := os.Stat(indexPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to inspect the index file: %w", err)
	}

	return &analyzed{segment: segment, indexPath: indexRel, indexSize: info.Size()}, stats, nil
}

// record は、索引と集計を記録し、同じトランザクションで、代表画像とプレビュー画像を作るジョブを投入する。
// audios は、TS から読んだ音声の構成の JSON。読めなければ nil。
func (w *AnalyzeWorker) record(ctx context.Context, recordingID uuid.UUID, results []*analyzed, total *ts.PIDStats, audios []byte) error {
	now := w.clock.Now()
	err := pgx.BeginFunc(ctx, w.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		for _, result := range results {
			err := q.UpsertRecordingIndex(ctx, sqlc.UpsertRecordingIndexParams{
				ID: id.New(), RecordingID: recordingID, PoolID: result.segment.PoolID, Path: result.indexPath, Size: result.indexSize,
				StartedAt: result.segment.StartedAt, EndedAt: result.segment.EndedAt, Now: now,
			})
			if err != nil {
				return fmt.Errorf("failed to record the index: %w", err)
			}
		}
		err := q.SetRecordingStats(ctx, sqlc.SetRecordingStatsParams{
			Drops: total.Drops, Errors: total.Errors, Scrambled: total.Scrambled, Now: now, ID: recordingID,
		})
		if err != nil {
			return fmt.Errorf("failed to record the stats: %w", err)
		}
		if audios != nil {
			if err := q.SetRecordingAudiosIfUnknown(ctx, sqlc.SetRecordingAudiosIfUnknownParams{Audios: audios, Now: now, ID: recordingID}); err != nil {
				return fmt.Errorf("failed to record the audio components: %w", err)
			}
		}
		params := []river.InsertManyParams{{Args: thumbnail.GenerateArgs{RecordingID: recordingID}, InsertOpts: nil}}
		if _, err := w.inserter.InsertManyTx(ctx, tx, params); err != nil {
			return fmt.Errorf("failed to enqueue the thumbnail generation: %w", err)
		}

		return nil
	})
	if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return river.JobCancel(errRecordingNotFound)
	}

	return err
}

// readerFunc は、関数を io.Reader にする。
type readerFunc func(p []byte) (int, error)

var _ io.Reader = readerFunc(nil)

func (f readerFunc) Read(p []byte) (int, error) {
	return f(p)
}
