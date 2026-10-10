package recording

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/assign"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/ts"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	// DefaultTuningTime は、チューニングにかかる時間の見込み (付録 A)。予定の開始よりこれだけ早くストリームを開く。
	DefaultTuningTime = 5 * time.Second
	// DefaultTick は、予定の時刻が来たかを確かめる間隔。予約の予定の時刻も、この間隔で読み直す。
	DefaultTick = 500 * time.Millisecond
	// writeBufferSize は、録画ファイルへの書き込みをまとめる大きさ (設計書 5.2)。
	writeBufferSize = 4 << 20
	// estimatedBytesPerSecond は、保存先を選ぶときに見込む、録画の 1 秒あたりの大きさ。地上波の HD の TS (約 17 Mbps) に余裕を持たせる。
	estimatedBytesPerSecond = 3_000_000
	// segmentExt は、録画ファイルの拡張子。
	segmentExt = ".m2ts"
	// indexFileName は、付属ファイルのディレクトリに置く、索引のファイルの名前。
	indexFileName = "index"
)

// FailureReason は、録画に失敗した理由 (設計書 5.5)。
type FailureReason string

const (
	FailureTunerUnavailable       FailureReason = "tuner_unavailable"
	FailureTunerServerUnreachable FailureReason = "tuner_server_unreachable"
	FailureStreamInterrupted      FailureReason = "stream_interrupted"
	FailureStorageUnavailable     FailureReason = "storage_unavailable"
	FailureStorageFull            FailureReason = "storage_full"
	FailureProgramNotFound        FailureReason = "program_not_found"
	FailureCancelled              FailureReason = "cancelled"
)

// State は、録画の状態 (設計書 5.5)。
type State string

const (
	StateRecording           State = "recording"
	StateCompleted           State = "completed"
	StateCompletedWithErrors State = "completed_with_errors"
	StatePartial             State = "partial"
	StateFailed              State = "failed"
)

// StreamOpener は、サービスのストリームを開く。*mirakurun.Client が満たす。
type StreamOpener interface {
	OpenServiceStream(ctx context.Context, serviceID int64, opts *mirakurun.StreamOptions) (io.ReadCloser, error)
}

var _ StreamOpener = new(mirakurun.Client)

// RecorderOptions は Recorder の設定。0 の項目は既定値にする。
type RecorderOptions struct {
	TuningTime time.Duration
	Tick       time.Duration
}

// Recorder は、割り振りの録画を行う Session を提供する (設計書 5.1、5.2、5.3)。
// 予定の開始からチューニングにかかる時間を引いた時刻にサービスストリームを開き、届いたデータをすべて最終的な保存先に書き込む。
// 書きながら TS を解析し、ドロップを数え、索引を作る。予定の終わりは、録画しながら予約を読み直して決める。
type Recorder struct {
	pool     *pgxpool.Pool
	clock    clock.Clock
	tuner    StreamOpener
	store    *kago.Store
	defaults assign.DefaultsFunc
	location *time.Location
	logger   *slog.Logger
	opts     RecorderOptions
}

// NewRecorder は Recorder を返す。defaults は録画設定の全体の既定値、location はファイル名に使う表示のタイムゾーン。opts は nil でもよい。
func NewRecorder(
	pool *pgxpool.Pool, clk clock.Clock, tuner StreamOpener, store *kago.Store, defaults assign.DefaultsFunc,
	location *time.Location, logger *slog.Logger, opts *RecorderOptions,
) *Recorder {
	if defaults == nil {
		defaults = assign.FixedDefaults(reserve.DefaultSettings())
	}
	o := RecorderOptions{TuningTime: DefaultTuningTime, Tick: DefaultTick}
	if opts != nil {
		if opts.TuningTime > 0 {
			o.TuningTime = opts.TuningTime
		}
		if opts.Tick > 0 {
			o.Tick = opts.Tick
		}
	}

	return &Recorder{pool: pool, clock: clk, tuner: tuner, store: store, defaults: defaults, location: location, logger: logger, opts: o}
}

// Session は、Worker に渡す Session を返す。
func (r *Recorder) Session() Session {
	return r.record
}

// record は、割り振りの予約を録画する。録画しなくてよくなったときと、録画を終えたときに nil を返す。
// 録画の失敗は、録画の状態と理由に記録し、割り振りは終わったことにする。
func (r *Recorder) record(ctx context.Context, assignment *Assignment) error {
	reservation, ok, err := r.waitUntilOpen(ctx, assignment)
	if err != nil || !ok {
		return err
	}
	logger := r.logger.With(slog.String("reservation_id", reservation.ID.String()), slog.String("name", reservation.Name))

	q := sqlc.New(r.pool)
	now := r.clock.Now()
	recording, err := q.StartRecording(ctx, sqlc.StartRecordingParams{
		ID: id.New(), ReservationID: &reservation.ID, ServiceID: reservation.ServiceID, EventID: reservation.EventID,
		Name: reservation.Name, Description: reservation.Description, StartAt: reservation.StartAt, EndAt: reservation.EndAt, Now: now,
	})
	if err != nil {
		return fmt.Errorf("failed to start the recording: %w", err)
	}
	logger = logger.With(slog.String("recording_id", recording.ID.String()))

	stream, err := r.tuner.OpenServiceStream(ctx, reservation.ServiceID, &mirakurun.StreamOptions{Decode: true, Priority: assignment.Priority})
	if err != nil {
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		logger.WarnContext(ctx, "failed to open the service stream", slog.Any("error", err))

		return r.fail(ctx, &recording, streamFailure(err))
	}
	defer stream.Close()

	segment, reason, err := r.createSegment(ctx, &recording, &reservation)
	if err != nil {
		logger.WarnContext(ctx, "failed to create a recording file", slog.Any("error", err))

		return r.fail(ctx, &recording, reason)
	}
	logger.InfoContext(ctx, "started recording", slog.String("pool_id", segment.poolID), slog.String("path", segment.path))

	stats, copyErr := r.copyUntilEnd(ctx, assignment, stream, segment)
	closeErr := r.closeSegment(context.WithoutCancel(ctx), &recording, segment, stats)
	switch {
	case ctx.Err() != nil:
		// リースを失ったか、止めるときは、録画を録画中のまま残し、続きを録る mikan に任せる。
		return errors.Join(context.Cause(ctx), closeErr)
	case closeErr != nil:
		logger.WarnContext(ctx, "failed to close the recording file", slog.Any("error", closeErr))

		return r.fail(ctx, &recording, FailureStorageUnavailable)
	case copyErr != nil:
		logger.WarnContext(ctx, "the stream was interrupted", slog.Any("error", copyErr))

		return r.fail(ctx, &recording, FailureStreamInterrupted)
	}

	state := StateCompleted
	if drops(stats) > 0 {
		state = StateCompletedWithErrors
	}
	if err := q.FinishRecording(ctx, sqlc.FinishRecordingParams{State: string(state), FailureReason: nil, Now: new(r.clock.Now()), ID: recording.ID}); err != nil {
		return fmt.Errorf("failed to finish the recording: %w", err)
	}
	logger.InfoContext(ctx, "finished recording", slog.String("state", string(state)), slog.Int64("bytes", stats.Bytes))

	return nil
}

// waitUntilOpen は、予定の開始からチューニングにかかる時間を引いた時刻まで待ち、そのときの予約を返す。
// 待つ間に予約を無効にしたか、予定の終わりが過ぎたら、録画しないので false を返す。
func (r *Recorder) waitUntilOpen(ctx context.Context, assignment *Assignment) (sqlc.GetReservationForRecordingRow, bool, error) {
	ticker := time.NewTicker(r.opts.Tick)
	defer ticker.Stop()
	for {
		reservation, err := sqlc.New(r.pool).GetReservationForRecording(ctx, assignment.ReservationID)
		if err != nil {
			return reservation, false, fmt.Errorf("failed to get the reservation: %w", err)
		}
		if !reservation.Enabled || reservation.PlannedStartAt == nil || reservation.PlannedEndAt == nil {
			return reservation, false, nil
		}
		now := r.clock.Now()
		if !now.Before(*reservation.PlannedEndAt) {
			return reservation, false, nil
		}
		if !now.Before(reservation.PlannedStartAt.Add(-r.opts.TuningTime)) {
			return reservation, true, nil
		}
		select {
		case <-ctx.Done():
			return reservation, false, context.Cause(ctx)
		case <-ticker.C:
		}
	}
}

// segment は、書いている録画ファイルと、その索引。
type segment struct {
	poolID    string
	path      string
	indexPath string
	fileID    uuid.UUID
	indexID   uuid.UUID
	file      *os.File
	indexFile *os.File
	buffer    *bufio.Writer
	analyzer  *ts.Analyzer
	startedAt time.Time
}

// createSegment は、保存先を選んで録画ファイルと索引を作り、DB に記録する。失敗したら、録画の失敗の理由も返す。
func (r *Recorder) createSegment(
	ctx context.Context, recording *sqlc.Recording, reservation *sqlc.GetReservationForRecordingRow,
) (*segment, FailureReason, error) {
	settings, err := r.settings(ctx, reservation)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	pools := []*kago.Pool{}
	if settings.PoolID != "" {
		pools = append(pools, &kago.Pool{ID: settings.PoolID, Priority: 0, MinFreeBytes: 0})
	} else {
		for _, poolID := range r.store.PoolIDs() {
			pools = append(pools, &kago.Pool{ID: poolID, Priority: 0, MinFreeBytes: 0})
		}
	}
	estimated := uint64(max(reservation.PlannedEndAt.Sub(*reservation.PlannedStartAt), 0)/time.Second) * estimatedBytesPerSecond
	pool, err := r.store.Select(pools, estimated)
	if errors.Is(err, kago.ErrNoPoolFits) {
		return nil, FailureStorageFull, err
	}
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}

	tmpl, err := reserve.ParseFileNameTemplate(settings.FileNameTemplate)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	name, err := kago.RenderName(tmpl, &reserve.FileNameData{
		StartAt: reservation.StartAt.In(r.location), EndAt: reservation.EndAt.In(r.location), ServiceName: reservation.ServiceName,
		ChannelType: reservation.ChannelType, Channel: reservation.Channel, Name: reservation.Name, EventID: valueOr(reservation.EventID),
	}, segmentExt)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	file, rel, err := r.store.Create(pool.ID, "", name)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	// 索引は、録画ファイルの横の付属ファイルのディレクトリに置く。作れなくても、録画は続ける。
	analyzer, indexFile, indexPath, indexErr := r.createIndex(pool.ID, rel)
	if indexErr != nil {
		r.logger.WarnContext(ctx, "failed to create the index; recording without it", slog.String("path", rel), slog.Any("error", indexErr))
	}
	seg := &segment{
		poolID: pool.ID, path: rel, indexPath: indexPath, fileID: uuid.Nil, indexID: uuid.Nil, file: file, indexFile: indexFile,
		buffer: bufio.NewWriterSize(file, writeBufferSize), analyzer: analyzer, startedAt: r.clock.Now(),
	}

	q := sqlc.New(r.pool)
	seg.fileID = id.New()
	err = q.InsertRecordingFile(ctx, sqlc.InsertRecordingFileParams{
		ID: seg.fileID, RecordingID: recording.ID, Kind: "segment", PoolID: pool.ID, Path: rel, StartedAt: seg.startedAt, Now: seg.startedAt,
	})
	if err != nil {
		_ = file.Close()

		return nil, FailureStorageUnavailable, fmt.Errorf("failed to save the recording file: %w", err)
	}
	if seg.indexFile != nil {
		seg.indexID = id.New()
		err = q.InsertRecordingFile(ctx, sqlc.InsertRecordingFileParams{
			ID: seg.indexID, RecordingID: recording.ID, Kind: "index", PoolID: pool.ID, Path: seg.indexPath, StartedAt: seg.startedAt, Now: seg.startedAt,
		})
		if err != nil {
			_ = file.Close()
			_ = seg.indexFile.Close()

			return nil, FailureStorageUnavailable, fmt.Errorf("failed to save the index file: %w", err)
		}
	}

	return seg, "", nil
}

// createIndex は、録画ファイル rel の付属ファイルのディレクトリに索引を作り、索引を書く解析器を返す。
// 索引を作れなければ、索引を書かない解析器と、そのエラーを返す。
func (r *Recorder) createIndex(poolID, rel string) (*ts.Analyzer, *os.File, string, error) {
	dir, err := r.store.CreateSidecarDir(poolID, rel)
	if err != nil {
		return ts.NewAnalyzer(nil), nil, "", err
	}
	indexRel := dir + "/" + indexFileName
	path, err := r.store.Path(poolID, indexRel)
	if err != nil {
		return ts.NewAnalyzer(nil), nil, "", err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return ts.NewAnalyzer(nil), nil, "", fmt.Errorf("failed to create the index file: %w", err)
	}
	writer, err := ts.NewIndexWriter(file)
	if err != nil {
		_ = file.Close()

		return ts.NewAnalyzer(nil), nil, "", err
	}

	return ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer}), file, indexRel, nil
}

// copyUntilEnd は、予定の終わりが来るまで、ストリームを録画ファイルと解析器に書く。
// 予定の終わりは、予約を読み直して決める。予定の終わりが来て止めたときは nil を返す。
func (r *Recorder) copyUntilEnd(ctx context.Context, assignment *Assignment, stream io.ReadCloser, seg *segment) (*ts.Stats, error) {
	ended := make(chan struct{})
	watchCtx, stopWatch := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		if r.waitUntilEnd(watchCtx, assignment) {
			close(ended)
			// 読み込み中のストリームを閉じて、書き込みを止める。
			_ = stream.Close()
		}
	}()

	_, err := io.Copy(io.MultiWriter(seg.buffer, seg.analyzer), stream)
	stopWatch()
	<-watchDone
	stats := seg.analyzer.Stats()
	select {
	case <-ended:
		return stats, nil
	default:
	}
	if err == nil {
		err = io.ErrUnexpectedEOF
	}

	return stats, err
}

// waitUntilEnd は、予定の終わりが来るか、予約を無効にしたら true を返す。ctx が終わったら false を返す。
func (r *Recorder) waitUntilEnd(ctx context.Context, assignment *Assignment) bool {
	ticker := time.NewTicker(r.opts.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
		reservation, err := sqlc.New(r.pool).GetReservationForRecording(ctx, assignment.ReservationID)
		if err != nil {
			// 読めなければ、録画を続け、次の時刻にもう一度読む。
			if ctx.Err() == nil {
				r.logger.WarnContext(ctx, "failed to reload the reservation", slog.Any("error", err))
			}

			continue
		}
		if !reservation.Enabled || reservation.PlannedEndAt == nil || !r.clock.Now().Before(*reservation.PlannedEndAt) {
			return true
		}
	}
}

// closeSegment は、録画ファイルと索引を書き終え、DB に大きさと時間の範囲を記録し、ドロップを録画の集計に足す。
func (r *Recorder) closeSegment(ctx context.Context, recording *sqlc.Recording, seg *segment, stats *ts.Stats) error {
	now := r.clock.Now()
	errs := []error{}
	// 書いたデータを確実に残すため、閉じる前に fsync する (設計書 5.2)。
	if err := seg.buffer.Flush(); err != nil {
		errs = append(errs, fmt.Errorf("failed to flush the recording file: %w", err))
	}
	if err := seg.file.Sync(); err != nil {
		errs = append(errs, fmt.Errorf("failed to sync the recording file: %w", err))
	}
	if err := seg.file.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close the recording file: %w", err))
	}
	q := sqlc.New(r.pool)
	if err := q.CloseRecordingFile(ctx, sqlc.CloseRecordingFileParams{Size: stats.Bytes, EndedAt: &now, ID: seg.fileID}); err != nil {
		errs = append(errs, fmt.Errorf("failed to save the recording file size: %w", err))
	}
	if seg.indexFile != nil {
		// 索引は、なくても再生できるので、書けなくても録画の失敗にしない。
		var indexSize int64
		if err := seg.indexFile.Sync(); err != nil {
			r.logger.WarnContext(ctx, "failed to sync the index file", slog.Any("error", err))
		}
		if info, err := seg.indexFile.Stat(); err == nil {
			indexSize = info.Size()
		}
		if err := seg.indexFile.Close(); err != nil {
			r.logger.WarnContext(ctx, "failed to close the index file", slog.Any("error", err))
		}
		if err := q.CloseRecordingFile(ctx, sqlc.CloseRecordingFileParams{Size: indexSize, EndedAt: &now, ID: seg.indexID}); err != nil {
			errs = append(errs, fmt.Errorf("failed to save the index file size: %w", err))
		}
	}
	var errorsCount, scrambled int64
	for _, pid := range stats.PIDs {
		errorsCount += pid.Errors
		scrambled += pid.Scrambled
	}
	err := q.AddRecordingStats(ctx, sqlc.AddRecordingStatsParams{Drops: drops(stats), Errors: errorsCount, Scrambled: scrambled, Now: now, ID: recording.ID})
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to save the recording stats: %w", err))
	}

	return errors.Join(errs...)
}

// fail は、録画を失敗にし、理由を記録する。割り振りは終わったことにするので、nil を返す。
func (r *Recorder) fail(ctx context.Context, recording *sqlc.Recording, reason FailureReason) error {
	err := sqlc.New(r.pool).FinishRecording(ctx, sqlc.FinishRecordingParams{
		State: string(StateFailed), FailureReason: new(string(reason)), Now: new(r.clock.Now()), ID: recording.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to record the recording failure: %w", err)
	}

	return nil
}

// settings は、予約の録画設定を、全体の既定値に、ルール予約ならルールの上書き、さらに予約自体の上書きを重ねて求める。
func (r *Recorder) settings(ctx context.Context, reservation *sqlc.GetReservationForRecordingRow) (*reserve.Settings, error) {
	q := sqlc.New(r.pool)
	settings, err := r.defaults(ctx, r.pool)
	if err != nil {
		return nil, err
	}
	if reserve.Kind(reservation.Kind) == reserve.KindRule {
		rows, err := q.ListFirstRuleSettings(ctx, []uuid.UUID{reservation.ID})
		if err != nil {
			return nil, fmt.Errorf("failed to list rule settings: %w", err)
		}
		for _, row := range rows {
			overrides, err := reserve.DecodeOverrides(row.Settings)
			if err != nil {
				return nil, err
			}
			settings = settings.Resolve(overrides)
		}
	}
	overrides, err := reserve.DecodeOverrides(reservation.Settings)
	if err != nil {
		return nil, err
	}

	return settings.Resolve(overrides), nil
}

// streamFailure は、ストリームを開けなかったエラーを、録画の失敗の理由にする。
func streamFailure(err error) FailureReason {
	if apiErr, ok := errors.AsType[*mirakurun.APIError](err); ok {
		if apiErr.StatusCode == http.StatusServiceUnavailable {
			return FailureTunerUnavailable
		}

		return FailureStreamInterrupted
	}

	return FailureTunerServerUnreachable
}

// drops は、全 PID のドロップの合計を返す。
func drops(stats *ts.Stats) int64 {
	var total int64
	for _, pid := range stats.PIDs {
		total += pid.Drops
	}

	return total
}

func valueOr(v *int64) int64 {
	if v == nil {
		return 0
	}

	return *v
}
