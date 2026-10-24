// Package analysis は、録画を終えた後に録画ファイルを解析するジョブを扱う (設計書 6.1、6.3)。
//
// EPGStation から取り込んだ録画には、Kotatsu が録画中に作る索引やドロップの集計がない。
// このジョブは、取り込んだ TS を読み直して、セグメントの索引を作り、ドロップ、エラー、スクランブルを数え直す。
// 索引ができたら、代表画像とプレビュー画像を作るジョブを投入する。
package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/jobs"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/notify"
	"github.com/SlashNephy/kotatsu/internal/thumbnail"
)

const (
	// kindSegment は、recording_files の、TS のセグメントの種類。
	kindSegment = "segment"
	// indexTempSuffix は、作り直している途中の索引の一時ファイルの接尾辞。
	indexTempSuffix = ".tmp"
	// stateRecording は、録画中の録画の状態。
	stateRecording = "recording"

	// analyzeMaxAttempts は、解析するジョブを試す回数の上限。
	analyzeMaxAttempts = 3
	// analyzeTimeout は、解析するジョブの時間の上限。数 GB の TS を読み通すので、長めにとる。
	analyzeTimeout = 30 * time.Minute
	// lockedSnooze は、同じ録画のファイルをほかでつないでいるときに、やり直すまで待つ時間。
	lockedSnooze = 10 * time.Minute
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
		// 解析するのは、取り込んだ録画と、頼まれてやり直す録画 (#476)、セグメントをつないだ録画。取り込んだ分は、新しく録った録画のジョブより後に回す。
		// 頼まれてやり直すときと、セグメントをつないだ録画では、投入するときに優先度を上げる。
		Priority: jobs.PriorityImported,
		// 同じ録画を解析するジョブは、待っている間に重ねて投入しない。River の既定は、終えたジョブ (completed) にも重ねないので、
		// 待っている状態だけにする。終えたジョブに重ねないと、解析してから 1 日 (River が終えたジョブを残す間) はやり直せない (#897)。
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

// Database は、解析に使う DB。*pgxpool.Pool が満たす。
// 録画ごとの advisory lock を持つのに、接続を借りる。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
	Acquire(ctx context.Context) (*pgxpool.Conn, error)
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
	segment *sqlc.RecordingFile
	// pidStats は、PID ごとの品質の集計の JSON (#473)。
	pidStats []byte
}

// Work は、録画ごとの advisory lock を取ってから、録画を解析する。
// セグメントをつなぐジョブと同じロックを使い、ほかの nekonote がファイルをつないだり消したりしている間は読まず、後でやり直す (#476)。
func (w *AnalyzeWorker) Work(ctx context.Context, job *river.Job[AnalyzeArgs]) error {
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

	return w.work(ctx, job)
}

func (w *AnalyzeWorker) work(ctx context.Context, job *river.Job[AnalyzeArgs]) error {
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
	results := make([]*analyzed, len(segments))
	total := new(ts.PIDStats)
	events := make([][]*ts.EventAudio, len(segments))
	// 引き継ぎで重なった区間は、後のセグメントでも録っているので、前のセグメントの集計からは除く (#525)。
	// 重なりの始まりは、後のセグメントの最初の I フレームの PTS で決めるので、後ろのセグメントから読む。
	var nextFirstPTS *int64
	for i := len(segments) - 1; i >= 0; i-- {
		segment := segments[i]
		var cutoff *int64
		if i+1 < len(segments) && overlaps(segment, segments[i+1]) {
			cutoff = nextFirstPTS
		}
		result, stats, err := w.analyze(ctx, segment, cutoff)
		if err != nil {
			return err
		}
		results[i], events[i], nextFirstPTS = result, stats.Events, stats.FirstPTS
		counted := stats.PIDs
		if stats.PIDsBeforeCutoff != nil {
			counted = stats.PIDsBeforeCutoff
		}
		// PID ごとの内訳も、重なった区間を除いた集計で記録し直す (#473)。
		if result.pidStats, err = json.Marshal(ts.PIDQualities(stats, counted)); err != nil {
			return fmt.Errorf("failed to encode the PID stats: %w", err)
		}
		for _, pid := range counted {
			total.Drops += pid.Drops
			total.Errors += pid.Errors
			total.Scrambled += pid.Scrambled
		}
	}

	// EPG から音声の構成を写せなかった録画 (取り込んだ録画や時刻予約) は、TS の EIT[p/f] から読んだもので埋める (設計書 6.1)。
	audios, ok, err := ts.RecordingAudios(ts.MergeEvents(events...), recording.ServiceID, recording.EventID)
	if err != nil {
		return fmt.Errorf("failed to encode the audio components: %w", err)
	}
	if !ok {
		audios = nil
	}

	return w.record(ctx, job.Args.RecordingID, job.Priority, results, total, audios)
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

// overlaps は、後のセグメント next が、segment を書き始めた後、書き終える前に始まったかどうか。引き継ぎで重ねて録ったときに起きる。
// EPGStation から取り込んだ録画は、どのセグメントも番組の始まりを始まりとするので、重なりとはしない。
func overlaps(segment, next *sqlc.RecordingFile) bool {
	return segment.EndedAt != nil && next.StartedAt.After(segment.StartedAt) && next.StartedAt.Before(*segment.EndedAt)
}

// analyze は、セグメントを読み通して、データディレクトリに索引を書き、統計を返す。
// cutoff を渡せば、PTS がそこに届いたときの統計も返す。索引は作り直すたびに切り詰めて書くので、何度実行しても同じ索引になる。
func (w *AnalyzeWorker) analyze(ctx context.Context, segment *sqlc.RecordingFile, cutoff *int64) (*analyzed, *ts.Stats, error) {
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
	if _, err := w.store.CreateSegmentDir(segment.ID); err != nil {
		return nil, nil, err
	}
	index := kago.IndexLocation(segment.ID)
	indexPath, err := w.store.Path(index.PoolID, index.Path)
	if err != nil {
		return nil, nil, err
	}
	// 索引は一時ファイルに書き、読み終えてから置き換える。途中で止まっても、読む側に書きかけの索引を見せない。
	tmpPath := indexPath + indexTempSuffix
	output, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create the index file: %w", err)
	}
	defer func() {
		_ = output.Close()
		_ = os.Remove(tmpPath)
	}()
	writer, err := ts.NewIndexWriter(output)
	if err != nil {
		return nil, nil, err
	}
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer, CutoffPTS: cutoff})
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
	if err := os.Rename(tmpPath, indexPath); err != nil {
		return nil, nil, fmt.Errorf("failed to replace the index file: %w", err)
	}

	return &analyzed{segment: segment, pidStats: nil}, stats, nil
}

// record は、集計を記録し、同じトランザクションで、代表画像とプレビュー画像を作るジョブを投入する。
// audios は、TS から読んだ音声の構成の JSON。読めなければ nil。
func (w *AnalyzeWorker) record(
	ctx context.Context, recordingID uuid.UUID, priority int, results []*analyzed, total *ts.PIDStats, audios []byte,
) error {
	now := w.clock.Now()
	err := pgx.BeginFunc(ctx, w.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		for _, result := range results {
			if err := q.SetRecordingFilePIDStats(ctx, sqlc.SetRecordingFilePIDStatsParams{PIDStats: result.pidStats, ID: result.segment.ID}); err != nil {
				return fmt.Errorf("failed to record the PID stats: %w", err)
			}
		}
		err := q.SetRecordingStats(ctx, sqlc.SetRecordingStatsParams{
			Drops: total.Drops, Errors: total.Errors, Scrambled: total.Scrambled, Now: now, ID: recordingID,
		})
		if err != nil {
			return fmt.Errorf("failed to record the stats: %w", err)
		}
		// 録画の一覧にドロップの数を出すので、録画の変更として知らせる (#868)。
		if _, err := notify.Publish(ctx, tx, w.clock, notify.RecordingsChannel); err != nil {
			return err
		}
		if audios != nil {
			if err := q.SetRecordingAudiosIfUnknown(ctx, sqlc.SetRecordingAudiosIfUnknownParams{Audios: audios, Now: now, ID: recordingID}); err != nil {
				return fmt.Errorf("failed to record the audio components: %w", err)
			}
		}
		// 画像は、解析と同じ優先度で作る。取り込んだ録画の画像は、新しく録った録画の画像より後に回し、
		// 頼まれてやり直す録画と、セグメントをつないだ録画の画像は、取り込んだ分を待たせない。キューと一意性は、GenerateArgs に従う。
		params := []river.InsertManyParams{{
			Args:       thumbnail.GenerateArgs{RecordingID: recordingID},
			InsertOpts: &river.InsertOpts{Priority: priority},
		}}
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
