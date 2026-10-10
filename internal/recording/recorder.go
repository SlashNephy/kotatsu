package recording

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/assign"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/ts"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	// DefaultTuningTime は、チューニングにかかる時間の見込み (付録 A)。予定の開始よりこれだけ早くストリームを開く。
	DefaultTuningTime = 5 * time.Second
	// DefaultTick は、予定の時刻が来たかを確かめる間隔。予約の予定の時刻も、この間隔で読み直す。
	DefaultTick = 500 * time.Millisecond
	// DefaultRetryInterval は、ストリームを開けなかったときや、切れたときに、開き直すまで待つ時間。
	DefaultRetryInterval = 2 * time.Second
	// MaxHistoryDrops は、重複の回避で「録画済み」とみなすドロップの数の上限 (付録 A)。EDCB と同じく 2 件未満にする。
	MaxHistoryDrops = 1
	// ExtensionStep と MaxExtension は、延長中で終了時刻が未定の番組の、録画の延長幅と上限 (付録 A)。
	ExtensionStep = 5 * time.Minute
	MaxExtension  = 3 * time.Hour
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
	TuningTime    time.Duration
	Tick          time.Duration
	RetryInterval time.Duration
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
	o := RecorderOptions{TuningTime: DefaultTuningTime, Tick: DefaultTick, RetryInterval: DefaultRetryInterval}
	if opts != nil {
		if opts.RetryInterval > 0 {
			o.RetryInterval = opts.RetryInterval
		}
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
	recording, err := r.start(ctx, &reservation)
	if err != nil {
		return err
	}
	logger = logger.With(slog.String("recording_id", recording.ID.String()))

	// 引き継ぎや再開で続きを録るときは、前のセグメントを書き終えてから今までを、途切れた区間とする (設計書 5.2)。
	var gapFrom *time.Time
	lastEnd, recorded, err := r.previousEnd(ctx, recording.ID)
	if err != nil {
		return err
	}
	if recorded {
		gapFrom = &lastEnd
	}

	tracker := newEndTracker(r, &recording, &reservation)
	var lastFailure FailureReason
	for {
		stream, err := r.tuner.OpenServiceStream(ctx, reservation.ServiceID, &mirakurun.StreamOptions{Decode: true, Priority: assignment.Priority})
		if err != nil {
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			// チューナーが空くか、チューナーサーバーにつながるまで、予定の終わりまで開き直す。
			lastFailure = streamFailure(err)
			logger.WarnContext(ctx, "failed to open the service stream; retrying", slog.Any("error", err))
			gapFrom = r.gapStart(gapFrom)
			end, err := tracker.sleep(ctx, r.opts.RetryInterval)
			if err != nil || end != endNone {
				return errors.Join(err, r.finishIfEnded(ctx, logger, &recording, end, recorded, gapFrom, lastFailure))
			}

			continue
		}
		if gapFrom != nil {
			if err := q.AddRecordingGap(ctx, sqlc.AddRecordingGapParams{GapFrom: *gapFrom, GapTo: r.clock.Now(), Now: r.clock.Now(), ID: recording.ID}); err != nil {
				_ = stream.Close()

				return fmt.Errorf("failed to record the gap: %w", err)
			}
			gapFrom = nil
		}

		segment, reason, err := r.createSegment(ctx, &recording, &reservation)
		if err != nil {
			_ = stream.Close()
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			logger.WarnContext(ctx, "failed to create a recording file", slog.Any("error", err))

			return r.fail(ctx, &recording, reason)
		}
		logger.InfoContext(ctx, "started recording", slog.String("pool_id", segment.poolID), slog.String("path", segment.path))

		stats, end, copyErr := r.copyUntilEnd(ctx, tracker, stream, segment)
		_ = stream.Close()
		closeErr := r.closeSegment(context.WithoutCancel(ctx), &recording, segment, stats)
		recorded = recorded || stats.Bytes > 0
		switch {
		case ctx.Err() != nil:
			// リースを失ったか、止めるときは、録画を録画中のまま残し、続きを録る mikan に任せる。
			return errors.Join(context.Cause(ctx), closeErr)
		case closeErr != nil:
			logger.WarnContext(ctx, "failed to close the recording file", slog.Any("error", closeErr))

			return r.fail(ctx, &recording, FailureStorageUnavailable)
		case copyErr != nil:
			// ストリームが切れたら、開き直して、新しいセグメントに書く (設計書 5.2)。
			logger.WarnContext(ctx, "the stream was interrupted; reconnecting", slog.Any("error", copyErr))
			lastFailure = FailureStreamInterrupted
			gapFrom = new(r.clock.Now())

			continue
		}

		return r.finish(ctx, logger, &recording, end, recorded, gapFrom, lastFailure)
	}
}

// previousEnd は、録画の前のセグメントを書き終えた時刻を返す。前のセグメントがなければ false を返す。
func (r *Recorder) previousEnd(ctx context.Context, recordingID uuid.UUID) (time.Time, bool, error) {
	lastEnd, err := sqlc.New(r.pool).GetLastRecordingFileEnd(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && lastEnd == nil) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to get the last recording file: %w", err)
	}

	return *lastEnd, true, nil
}

// gapStart は、途切れた区間が始まっていなければ、今から始める。
func (r *Recorder) gapStart(gapFrom *time.Time) *time.Time {
	if gapFrom != nil {
		return gapFrom
	}

	return new(r.clock.Now())
}

// finishIfEnded は、録画を止めるときが来ていれば、録画を終える。
func (r *Recorder) finishIfEnded(
	ctx context.Context, logger *slog.Logger, recording *sqlc.Recording, end endReason, recorded bool, gapFrom *time.Time, lastFailure FailureReason,
) error {
	if end == endNone {
		return nil
	}

	return r.finish(ctx, logger, recording, end, recorded, gapFrom, lastFailure)
}

// finish は、録画を止めた理由と、録れたかどうか、途切れた区間から、録画の最後の状態を決めて記録する (設計書 5.5)。
func (r *Recorder) finish(
	ctx context.Context, logger *slog.Logger, recording *sqlc.Recording, end endReason, recorded bool, gapFrom *time.Time, lastFailure FailureReason,
) error {
	q := sqlc.New(r.pool)
	now := r.clock.Now()
	if gapFrom != nil {
		if err := q.AddRecordingGap(ctx, sqlc.AddRecordingGapParams{GapFrom: *gapFrom, GapTo: now, Now: now, ID: recording.ID}); err != nil {
			return fmt.Errorf("failed to record the gap: %w", err)
		}
	}
	switch {
	case end == endCancelled:
		logger.InfoContext(ctx, "the reservation was disabled while recording")

		return r.fail(ctx, recording, FailureCancelled)
	case !recorded:
		logger.WarnContext(ctx, "nothing was recorded", slog.String("reason", string(lastFailure)))

		return r.fail(ctx, recording, lastFailure)
	}

	// 引き継ぎや再開で、前のセグメントにドロップがあることもあるので、録画全体の集計で決める。
	finished, err := q.GetRecording(ctx, recording.ID)
	if err != nil {
		return fmt.Errorf("failed to get the recording: %w", err)
	}
	var gaps []json.RawMessage
	if err := json.Unmarshal(finished.Gaps, &gaps); err != nil {
		return fmt.Errorf("failed to decode the recording gaps: %w", err)
	}
	state := StateCompleted
	switch {
	case len(gaps) > 0:
		state = StatePartial
	case finished.Drops > 0:
		state = StateCompletedWithErrors
	}
	if err := r.complete(ctx, recording.ID, state, ""); err != nil {
		return err
	}
	logger.InfoContext(ctx, "finished recording", slog.String("state", string(state)))

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
	mounted := r.store.PoolIDs()
	if (settings.PoolID != "" && !slices.Contains(mounted, settings.PoolID)) || len(mounted) == 0 {
		return nil, FailureStorageUnavailable, fmt.Errorf("%w: %q", kago.ErrPoolNotMounted, settings.PoolID)
	}
	pools, err := r.pools(ctx, mounted, settings.PoolID)
	if err != nil {
		return nil, FailureStorageUnavailable, err
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
		if seg.indexFile != nil {
			_ = seg.indexFile.Close()
		}

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
			// 記録したセグメントは、書いていないものとして閉じておく。
			now := r.clock.Now()
			if closeErr := q.CloseRecordingFile(context.WithoutCancel(ctx), sqlc.CloseRecordingFileParams{Size: 0, EndedAt: &now, ID: seg.fileID}); closeErr != nil {
				r.logger.WarnContext(ctx, "failed to close the recording file", slog.Any("error", closeErr))
			}

			return nil, FailureStorageUnavailable, fmt.Errorf("failed to save the index file: %w", err)
		}
	}

	return seg, "", nil
}

// pools は、録画を書く候補のプールを、優先順位と容量の閾値の設定と一緒に返す (設計書 5.6)。
// 録画設定でプールを指定していれば、そのプールだけを返す。設定のないプールは、優先順位 0、容量の閾値 0 とする。
func (r *Recorder) pools(ctx context.Context, mounted []string, poolID string) ([]*kago.Pool, error) {
	rows, err := sqlc.New(r.pool).ListStoragePools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list storage pools: %w", err)
	}
	settings := make(map[string]*kago.Pool, len(rows))
	for i := range rows {
		settings[rows[i].ID] = &kago.Pool{ID: rows[i].ID, Priority: int(rows[i].Priority), MinFreeBytes: uint64(max(rows[i].MinFreeBytes, 0))}
	}
	candidates := mounted
	if poolID != "" {
		candidates = []string{poolID}
	}
	pools := make([]*kago.Pool, 0, len(candidates))
	for _, id := range candidates {
		pool, ok := settings[id]
		if !ok {
			pool = &kago.Pool{ID: id, Priority: 0, MinFreeBytes: 0}
		}
		pools = append(pools, pool)
	}

	return pools, nil
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

// endReason は、録画を止めた理由。
type endReason int

const (
	// endNone は、まだ止めていないことを表す。
	endNone endReason = iota
	// endPlanned は、予定の終わりが来たことを表す。
	endPlanned
	// endCancelled は、録画中に予約を無効にしたことを表す。
	endCancelled
)

// copyUntilEnd は、予定の終わりが来るか、予約を無効にするまで、ストリームを録画ファイルと解析器に書き、止めた理由を返す。
// 予定の終わりは、予約を読み直して決める。その前にストリームが切れたら、エラーを返す。
func (r *Recorder) copyUntilEnd(ctx context.Context, tracker *endTracker, stream io.ReadCloser, seg *segment) (*ts.Stats, endReason, error) {
	ended := make(chan endReason, 1)
	watchCtx, stopWatch := context.WithCancel(ctx)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		if reason := tracker.wait(watchCtx); reason != endNone {
			ended <- reason
			// 読み込み中のストリームを閉じて、書き込みを止める。
			_ = stream.Close()
		}
	}()

	_, err := io.Copy(io.MultiWriter(seg.buffer, seg.analyzer), stream)
	stopWatch()
	<-watchDone
	stats := seg.analyzer.Stats()
	select {
	case reason := <-ended:
		return stats, reason, nil
	default:
	}
	if err == nil {
		err = io.ErrUnexpectedEOF
	}

	return stats, endNone, err
}

// endTracker は、録画中の予約と番組を読み直して、録画を止める時刻を決める (設計書 5.1)。
//   - 予定の終わりは、neko が EPG に合わせて計算し直す予約の予定の終わりにする。
//   - 番組の終了時刻が未定 (延長中) なら、予定の終わりが来るたびに ExtensionStep ずつ延ばす。延ばすのは、録画を始めたときの予定の終わりから MaxExtension まで。
//   - 番組が EPG から消えたら、予定の終わりまで録り続け、録画に番組を見失ったと記録する。
type endTracker struct {
	recorder      *Recorder
	recordingID   uuid.UUID
	reservationID uuid.UUID
	serviceID     int64
	eventID       *int64
	// extendedUntil は、延長中に延ばした終わり。延ばしていなければゼロ値。
	extendedUntil time.Time
	// limit は、延ばせる終わりの上限。
	limit time.Time
	lost  bool
}

func newEndTracker(r *Recorder, recording *sqlc.Recording, reservation *sqlc.GetReservationForRecordingRow) *endTracker {
	return &endTracker{
		recorder: r, recordingID: recording.ID, reservationID: reservation.ID, serviceID: reservation.ServiceID, eventID: reservation.EventID,
		extendedUntil: time.Time{}, limit: reservation.PlannedEndAt.Add(MaxExtension), lost: false,
	}
}

// wait は、録画を止めるときまで待ち、その理由を返す。ctx が終わったら endNone を返す。
func (t *endTracker) wait(ctx context.Context) endReason {
	ticker := time.NewTicker(t.recorder.opts.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return endNone
		case <-ticker.C:
		}
		if reason := t.check(ctx); reason != endNone {
			return reason
		}
	}
}

// sleep は、d の間待つ。その間に録画を止めるときが来たら、その理由を返す。
func (t *endTracker) sleep(ctx context.Context, d time.Duration) (endReason, error) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	ticker := time.NewTicker(t.recorder.opts.Tick)
	defer ticker.Stop()
	for {
		if reason := t.check(ctx); reason != endNone {
			return reason, nil
		}
		select {
		case <-ctx.Done():
			return endNone, context.Cause(ctx)
		case <-timer.C:
			return endNone, nil
		case <-ticker.C:
		}
	}
}

// check は、今が録画を止めるときかを確かめる。読めなければ、録画を続ける。
func (t *endTracker) check(ctx context.Context) endReason {
	r := t.recorder
	q := sqlc.New(r.pool)
	reservation, err := q.GetReservationForRecording(ctx, t.reservationID)
	if err != nil {
		if ctx.Err() == nil {
			r.logger.WarnContext(ctx, "failed to reload the reservation", slog.Any("error", err))
		}

		return endNone
	}
	// 割り当てを消されても (予定の時刻が NULL)、無効にしたのでなければ、取り消しとは扱わない。
	if !reservation.Enabled {
		return endCancelled
	}
	if reservation.PlannedEndAt == nil {
		return endPlanned
	}
	now := r.clock.Now()
	end := *reservation.PlannedEndAt
	if t.eventID != nil {
		program, err := q.GetProgramEnd(ctx, sqlc.GetProgramEndParams{ServiceID: t.serviceID, EventID: *t.eventID})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			t.markLost(ctx)
		case err != nil:
			if ctx.Err() == nil {
				r.logger.WarnContext(ctx, "failed to reload the program", slog.Any("error", err))
			}
		case program.EndUndecided:
			// 延長中は、終わりが来るたびに延ばす。
			if !now.Before(end) && !now.Before(t.extendedUntil) {
				t.extendedUntil = minTime(now.Add(ExtensionStep), t.limit)
				r.logger.InfoContext(ctx, "the program end is undecided; extending the recording",
					slog.String("recording_id", t.recordingID.String()), slog.Time("until", t.extendedUntil))
			}
			end = maxTime(end, t.extendedUntil)
		}
	}
	if !now.Before(end) {
		return endPlanned
	}

	return endNone
}

// markLost は、番組を見失ったことを、録画に一度だけ記録する。
func (t *endTracker) markLost(ctx context.Context) {
	if t.lost {
		return
	}
	r := t.recorder
	if err := sqlc.New(r.pool).MarkRecordingProgramLost(ctx, sqlc.MarkRecordingProgramLostParams{Now: r.clock.Now(), ID: t.recordingID}); err != nil {
		r.logger.WarnContext(ctx, "failed to record that the program was lost", slog.Any("error", err))

		return
	}
	t.lost = true
	r.logger.WarnContext(ctx, "the program disappeared from the EPG; recording until the planned end", slog.String("recording_id", t.recordingID.String()))
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}

	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}

	return b
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
			r.logger.WarnContext(ctx, "failed to save the index file size", slog.Any("error", err))
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
	return r.complete(ctx, recording.ID, StateFailed, reason)
}

// start は、予約の録画を始め、新しく始めたときは、録画の開始を通知する (設計書 6.2)。引き継ぎや再開で続きを録るときは通知しない。
func (r *Recorder) start(ctx context.Context, reservation *sqlc.GetReservationForRecordingRow) (sqlc.Recording, error) {
	var recording sqlc.Recording
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		started, err := sqlc.New(tx).StartRecording(ctx, sqlc.StartRecordingParams{
			ID: id.New(), ReservationID: &reservation.ID, ServiceID: reservation.ServiceID, EventID: reservation.EventID,
			Name: reservation.Name, Description: reservation.Description, StartAt: reservation.StartAt, EndAt: reservation.EndAt, Now: r.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to start the recording: %w", err)
		}
		recording = started.Recording
		if !started.Inserted {
			return nil
		}

		return notification.Emit(ctx, tx, r.clock, notification.KindRecordingStarted, map[string]any{
			"recordingId": recording.ID.String(), "name": recording.Name, "serviceName": reservation.ServiceName,
			"startAt": recording.StartAt, "endAt": recording.EndAt,
		})
	})

	return recording, err
}

// complete は、録画を state で終え、正常に終わってドロップが閾値以下なら録画履歴に記録し (設計書 4.4)、
// 録画の完了か失敗を通知する (設計書 6.2)。完了の通知には、Webhook のためにセグメントのプール内のパスを載せる。
func (r *Recorder) complete(ctx context.Context, recordingID uuid.UUID, state State, reason FailureReason) error {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		now := r.clock.Now()
		var failure *string
		if reason != "" {
			failure = new(string(reason))
		}
		if err := q.FinishRecording(ctx, sqlc.FinishRecordingParams{State: string(state), FailureReason: failure, Now: &now, ID: recordingID}); err != nil {
			return fmt.Errorf("failed to finish the recording: %w", err)
		}
		row, err := q.GetRecordingWithService(ctx, recordingID)
		if err != nil {
			return fmt.Errorf("failed to get the recording: %w", err)
		}
		recording := row.Recording
		payload := map[string]any{
			"recordingId": recording.ID.String(), "name": recording.Name, "serviceName": row.ServiceName,
			"startAt": recording.StartAt, "endAt": recording.EndAt, "state": string(state),
		}
		if state == StateFailed {
			payload["reason"] = string(reason)

			return notification.Emit(ctx, tx, r.clock, notification.KindRecordingFailed, payload)
		}

		if (state == StateCompleted || state == StateCompletedWithErrors) && recording.Drops <= MaxHistoryDrops {
			err := q.InsertRecordingHistory(ctx, sqlc.InsertRecordingHistoryParams{
				ID: id.New(), ServiceID: recording.ServiceID, TitleKey: rule.TitleKey(recording.Name), Name: recording.Name,
				StartAt: recording.StartAt, EndAt: recording.EndAt, Now: now,
			})
			if err != nil {
				return fmt.Errorf("failed to save the recording history: %w", err)
			}
		}
		segments, err := q.ListRecordingSegments(ctx, recordingID)
		if err != nil {
			return fmt.Errorf("failed to list the recording files: %w", err)
		}
		files := make([]map[string]string, 0, len(segments))
		for _, segment := range segments {
			files = append(files, map[string]string{"poolId": segment.PoolID, "path": segment.Path})
		}
		payload["files"] = files

		return notification.Emit(ctx, tx, r.clock, notification.KindRecordingCompleted, payload)
	})
	if err != nil {
		return fmt.Errorf("failed to complete the recording: %w", err)
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
