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
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

const (
	// DefaultTuningTime は、チューニングにかかる時間の見込み (付録 A)。予定の開始よりこれだけ早くストリームを開く。
	DefaultTuningTime = 5 * time.Second
	// DefaultReleaseAfter は、チューナーサーバーにつながらないまま、割り振りを手放すまでの時間 (付録 A)。
	// チューナーサーバーの再起動は、すべての mikan に同時に起きるので、一度つながらないだけでは手放さない。
	DefaultReleaseAfter = 30 * time.Second
	// successorWait と successorPoll は、引き継いで止める古い mikan が、新しい mikan のセグメントができるのを待つ時間と、確かめる間隔 (#525)。
	successorWait = 5 * time.Second
	successorPoll = 100 * time.Millisecond
	// LateStartTolerance は、録画を始めるのが予定の開始より遅れても、冒頭が欠けたとはみなさない時間 (付録 A)。
	// チューナーの確保やストリームを開くのにかかる時間の揺らぎを許す。
	LateStartTolerance = 30 * time.Second
	// DefaultTick は、予定の時刻が来たかを確かめる間隔。予約の予定の時刻も、この間隔で読み直す。
	DefaultTick = 500 * time.Millisecond
	// DefaultRetryInterval は、ストリームを開けなかったときや、切れたときに、開き直すまで待つ時間。
	DefaultRetryInterval = 2 * time.Second
	// DefaultDatabaseTimeout は、録画中に 1 度に DB を待つ上限の既定値 (#1073)。
	DefaultDatabaseTimeout = 5 * time.Second
	// MaxHistoryDrops は、重複の回避で「録画済み」とみなすドロップの数の上限 (付録 A)。EDCB と同じく 2 件未満にする。
	MaxHistoryDrops = 1
	// ExtensionStep と MaxExtension は、延長中で終了時刻が未定の番組の、録画の延長幅と上限 (付録 A)。
	ExtensionStep = 5 * time.Minute
	MaxExtension  = 3 * time.Hour
	// BroadcastStartGrace は、番組が始まるはずの時刻から、EIT[p/f] で予約した番組が次の番組のままでも、
	// 遅れたとはみなさない時間 (付録 A、#1046)。EIT[p/f] の切り替わりが番組の始まりより少し遅れて届いても、時間どおりの番組を待たない。
	BroadcastStartGrace = 30 * time.Second
	// MaxPreroll は、遅れた番組が放送中になるのを待った後、先に書く直近の分の長さの上限 (付録 A、#1046)。
	// 開始マージンが長くても、メモリに溜めるのはこの長さまでにする。
	MaxPreroll = 30 * time.Second
	// writeBufferSize は、録画ファイルへの書き込みをまとめる大きさ (設計書 5.2)。
	writeBufferSize = 4 << 20
	// estimatedBytesPerSecond は、保存先を選ぶときに見込む、録画の 1 秒あたりの大きさ。地上波の HD の TS (約 17 Mbps) に余裕を持たせる。
	estimatedBytesPerSecond = 3_000_000
	// segmentExt は、録画ファイルの拡張子。
	segmentExt = ".m2ts"
	// fileKindSegment は、録画のファイルの種類 (recording_files.kind)。
	fileKindSegment = "segment"
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
	// FailureRecorderUnavailable は、録画を始める時刻に、割り振りを取る mikan がいなかったこと (#517)。
	FailureRecorderUnavailable FailureReason = "recorder_unavailable"
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
	// ReleaseAfter は、チューナーサーバーにつながらないまま、割り振りを手放すまでの時間 (#524)。0 なら DefaultReleaseAfter にする。
	ReleaseAfter time.Duration
	// Peers は、ほかに動いている mikan の数を返す。nil なら、ほかの mikan はいないとみなし、割り振りを手放さない。
	Peers PeersFunc
	// DatabaseTimeout は、録画中に DB を待つ上限。DB への接続が応答のないまま詰まっても、録画を止めない (#1073)。0 なら DefaultDatabaseTimeout にする。
	DatabaseTimeout time.Duration
}

// PeersFunc は、ほかに動いている mikan の数を返す。
type PeersFunc func(ctx context.Context) (int, error)

// ErrRecorderUnavailable は、録画できない理由がこの mikan にある (プールがマウントされていない、チューナーサーバーにつながらない) ので、
// 割り振りを手放して、ほかの mikan に録画させること (#524)。
var ErrRecorderUnavailable = errors.New("this mikan is unable to record")

// Recorder は、割り振りの録画を行う Session を提供する (設計書 5.1、5.2、5.3)。
// 予定の開始からチューニングにかかる時間を引いた時刻にサービスストリームを開き、予定の開始までに届いたデータは捨て (#847)、
// その後に届いたデータをすべて最終的な保存先に書き込む。
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
		defaults = assign.FixedDefaults(domain.DefaultRecordingSettings())
	}
	o := RecorderOptions{
		TuningTime: DefaultTuningTime, Tick: DefaultTick, RetryInterval: DefaultRetryInterval, ReleaseAfter: DefaultReleaseAfter, Peers: nil,
		DatabaseTimeout: DefaultDatabaseTimeout,
	}
	if opts != nil {
		o = RecorderOptions{
			TuningTime: orDefault(opts.TuningTime, o.TuningTime), Tick: orDefault(opts.Tick, o.Tick),
			RetryInterval: orDefault(opts.RetryInterval, o.RetryInterval), ReleaseAfter: orDefault(opts.ReleaseAfter, o.ReleaseAfter),
			Peers: opts.Peers, DatabaseTimeout: orDefault(opts.DatabaseTimeout, o.DatabaseTimeout),
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
	reservation, cached, ok, err := r.waitUntilOpen(ctx, assignment)
	if err != nil {
		return err
	}
	if !ok {
		return r.finishLeftOver(ctx, assignment, &reservation)
	}
	logger := r.logger.With(slog.String("reservation_id", reservation.ID.String()), slog.String("name", reservation.Name))

	startedAt := r.clock.Now()
	// pendingStart は、DB に届かずに録画を記録できていないかどうか。届くようになったら記録する (#1073)。
	recording, pendingStart, err := r.startOrDefer(ctx, logger, &reservation, startedAt)
	if err != nil {
		return err
	}
	logger = logger.With(slog.String("recording_id", recording.ID.String()))
	// created は、録画のセグメントの数。前の mikan が録った分も数える。録画の途中で保存先に書けなくなっても、
	// 録画を失敗にせず、やり直す (#519)。
	created, err := r.prepare(ctx, assignment, &recording, pendingStart)
	if err != nil {
		return err
	}

	startMargin := time.Duration(domain.DefaultMarginSeconds) * time.Second
	endMargin := time.Duration(domain.DefaultMarginSeconds) * time.Second
	if settings, err := r.cachedSettings(ctx, &reservation, cached); err == nil {
		startMargin = time.Duration(settings.StartMarginSeconds) * time.Second
		endMargin = time.Duration(settings.EndMarginSeconds) * time.Second
	}
	tracker := newEndTracker(r, &recording, &reservation, startMargin, endMargin)
	// 録画を終えずにやめるとき (リースを失ったときなど) は、写すのもやめさせ、書きかけのセグメントを消させる。
	defer func() { tracker.backfill.stop() }()
	tracker.pending = &pendingSave{
		recording: &recording, start: pendingStart, closeStale: false, startedAt: startedAt, handedOver: assignment.HandedOver,
		writes: []pendingWrite{}, cache: cached,
	}
	if !pendingStart {
		tracker.savedRecordingID.Store(new(recording.ID))
	}
	result := &sessionResult{lastFailure: ""}
	// unreachableSince は、チューナーサーバーにつながらなくなった時刻。つながればゼロ値に戻す。
	var unreachableSince time.Time
	for {
		opened := r.openForWriting(ctx, logger, assignment, &recording, &reservation, tracker, result, created, &unreachableSince)
		switch {
		case opened.done:
			return opened.err
		case opened.reopen:
			continue
		}
		stream := opened.stream

		segment, reason, err := r.createSegment(ctx, tracker, &reservation, opened.startedAt)
		if err != nil {
			_ = stream.Close()
			if done, err := r.retryCreate(ctx, logger, assignment, &recording, tracker, result, created, reason, err); done {
				return err
			}

			continue
		}
		// 録画が予定の開始より遅れて始まったら、予定の開始からの分を、タイムシフトのバッファから写す (#826、設計書 5.7)。
		r.maybeStartBackfill(ctx, logger, assignment, tracker, &recording, &reservation, created, pendingStart, segment.startedAt)
		created++
		logger.InfoContext(ctx, "started recording", slog.String("pool_id", segment.poolID), slog.String("path", segment.path))
		openedAt := r.clock.Now()

		stats, end, copyErr := r.copyUntilEnd(ctx, tracker, stream, segment)
		_ = stream.Close()
		// stoppedAt は、セグメントに書くのをやめた時刻。DB に届かずに記録が遅れても、この時刻で記録する (#1073)。
		// 予定の終わりで止めたなら、そう判断した時刻にする。DB を待った分だけ遅れないようにする。
		stoppedAt := r.clock.Now()
		if end == endPlanned && !tracker.stoppedAt.IsZero() {
			stoppedAt = minTime(stoppedAt, tracker.stoppedAt)
		}
		if errors.Is(context.Cause(ctx), ErrHandedOver) {
			r.excludeOverlap(context.WithoutCancel(ctx), logger, &recording, segment, stats)
		}
		closeErr := r.closeSegment(ctx, logger, tracker, &recording, segment, stats, stoppedAt)
		switch {
		case ctx.Err() != nil:
			// リースを失ったか、止めるときは、録画を録画中のまま残し、続きを録る mikan に任せる。
			return errors.Join(context.Cause(ctx), closeErr)
		case end == endDelayed:
			// 予約した番組が遅れて始まるので、前の番組しか入っていないセグメントを消し、開き直して放送中になるのを待つ (#1046)。
			// バッファから写していれば、前の番組なので、写すのもやめさせる。
			tracker.backfill.stop()
			tracker.backfill = nil
			created = r.discardSegments(ctx, logger, &recording, tracker, created)

			continue
		case closeErr != nil || copyErr != nil:
			// ストリームが切れたら、開き直して、新しいセグメントに書く (設計書 5.2)。
			// 書き込みのエラー (ディスクが一杯、I/O エラー) でも、録画全体を失敗にせず、新しいセグメントに書き直す (#519)。
			result.lastFailure = interruption(ctx, logger, closeErr, copyErr)
			// 開いてすぐに切れ続けると、セグメントを作り続けるので、開き直すのは RetryInterval に 1 回までにする。
			// 長く録れていたなら、待たずに開き直し、途切れる区間を短くする。
			wait := r.opts.RetryInterval - r.clock.Now().Sub(openedAt)
			if done, err := r.waitToReopen(ctx, logger, assignment, &recording, tracker, result, wait); done {
				return err
			}

			continue
		}

		return r.finish(ctx, logger, assignment, &recording, end, result, false, stoppedAt, tracker)
	}
}

// leftOverGrace は、前の mikan が残した録画を終える前に待つ、RetryInterval の回数。前の mikan が、覚えていた記録を書く間。
const leftOverGrace = 3

// errInterruptedBeforeStart は、予定の開始まで読み捨てている間に、ストリームが切れたこと。
var errInterruptedBeforeStart = errors.New("the stream was interrupted before the planned start")

// openStream は、サービスストリームを開き、予定の開始まで読み捨てる。
// チューニングのために早く開いたストリームを読み捨て、開始前のマージンを設定どおりにする (#847)。
func (r *Recorder) openStream(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, reservation *sqlc.GetReservationForRecordingRow,
) (io.ReadCloser, error) {
	stream, err := r.tuner.OpenServiceStream(ctx, reservation.ServiceID, &mirakurun.StreamOptions{Decode: true, Priority: assignment.Priority})
	if err != nil {
		return nil, err
	}
	// 引き継ぎ待ちの古い mikan は、ストリームを開いたことを見て、自分のストリームを閉じる (設計書 5.4)。
	markCtx, cancel := context.WithTimeout(ctx, r.opts.DatabaseTimeout)
	defer cancel()
	err = sqlc.New(r.pool).MarkRecordingStreamOpened(markCtx, sqlc.MarkRecordingStreamOpenedParams{
		Generation: assignment.Generation, Now: r.clock.Now(), ID: assignment.ID,
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to record that the stream was opened", slog.Any("error", err))
	}
	if err := skipUntil(r.clock, stream, *reservation.PlannedStartAt); err != nil {
		_ = stream.Close()

		return nil, fmt.Errorf("%w: %w", errInterruptedBeforeStart, err)
	}

	return stream, nil
}

// openForWriting は、ストリームを開き、書き始めるストリームと、その最初のデータを読んだ時刻を返す。
// 開けなければ、開き直すか、録画を終えるか、割り振りを手放す (retryOpen)。
// 遅れた番組を待つときは、放送中になるまで書かずに待ち、溜めた直近の分から書く (#1046)。
func (r *Recorder) openForWriting(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, recording *sqlc.Recording, reservation *sqlc.GetReservationForRecordingRow,
	tracker *endTracker, result *sessionResult, created int, unreachableSince *time.Time,
) waitOutcome {
	stream, err := r.openStream(ctx, logger, assignment, reservation)
	if err != nil {
		done, err := r.retryOpen(ctx, logger, assignment, recording, tracker, result, created, unreachableSince, err)

		return waitOutcome{stream: nil, startedAt: time.Time{}, reopen: !done, done: done, err: err}
	}
	*unreachableSince = time.Time{}

	return r.waitBeforeWriting(ctx, logger, assignment, recording, tracker, result, stream, created)
}

// retryOpen は、ストリームを開けなかったときに、開き直すか、録画を終えるか、割り振りを手放すかを決める。録画を終えたか手放したら true を返す。
// チューナーが空くか、チューナーサーバーにつながるまで、予定の終わりまで開き直す。まだ何も録っていないのに、
// チューナーサーバーにつながらないまま ReleaseAfter が過ぎ、ほかに mikan がいれば、割り振りを手放す (#524)。
func (r *Recorder) retryOpen(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, recording *sqlc.Recording, tracker *endTracker, result *sessionResult,
	created int, unreachableSince *time.Time, err error,
) (bool, error) {
	if ctx.Err() != nil {
		return true, context.Cause(ctx)
	}
	result.lastFailure = streamFailure(err)
	if r.unreachableTooLong(ctx, result.lastFailure, created, unreachableSince) {
		logger.WarnContext(ctx, "the tuner server has been unreachable; releasing the assignment", slog.Any("error", err))

		return true, r.release(ctx, recording, result.lastFailure)
	}
	logger.WarnContext(ctx, "failed to open the service stream; retrying", slog.Any("error", err))

	return r.waitToReopen(ctx, logger, assignment, recording, tracker, result, r.opts.RetryInterval)
}

// retryCreate は、セグメントを作れなかったときに、録画を失敗にするか、やり直すかを決める。録画を終えたら true を返す。
// まだ何も作っていなければ失敗にする。録画の途中なら、それまでに録れた分を失敗にしないよう、作れるようになるまでやり直す (#519)。
func (r *Recorder) retryCreate(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, recording *sqlc.Recording, tracker *endTracker, result *sessionResult,
	created int, reason FailureReason, err error,
) (bool, error) {
	if ctx.Err() != nil {
		return true, context.Cause(ctx)
	}
	if created == 0 {
		// プールがこの mikan にマウントされていなければ、ほかの mikan に録画させる (#524)。
		if errors.Is(err, kago.ErrPoolNotMounted) && r.hasPeers(ctx) {
			logger.WarnContext(ctx, "the storage pool is not mounted on this mikan; releasing the assignment", slog.Any("error", err))

			// DB に届かずに録画を記録できていなければ、届くのを待って記録してから、理由を残す (#1073)。
			return true, r.untilDatabase(ctx, logger, "release the recording", func(ctx context.Context) error {
				if err := tracker.pending.save(ctx, r, tracker); err != nil {
					return err
				}

				return r.release(ctx, recording, reason)
			})
		}
		logger.WarnContext(ctx, "failed to create a recording file", slog.Any("error", err))
		// DB に届かずに録画を記録できていなければ、届くのを待って記録してから、失敗にする (#1073)。
		at := r.clock.Now()

		return true, r.untilDatabase(ctx, logger, "fail the recording", func(ctx context.Context) error {
			if err := tracker.pending.save(ctx, r, tracker); err != nil {
				return err
			}

			return r.fail(ctx, assignment, recording, reason, at)
		})
	}
	logger.WarnContext(ctx, "failed to create a recording file; retrying", slog.Any("error", err))
	result.lastFailure = reason

	return r.waitToReopen(ctx, logger, assignment, recording, tracker, result, r.opts.RetryInterval)
}

// hasPeers は、ほかに動いている mikan がいるかを返す。数えられなければ、いないとみなす。
func (r *Recorder) hasPeers(ctx context.Context) bool {
	if r.opts.Peers == nil {
		return false
	}
	n, err := r.opts.Peers(ctx)
	if err != nil {
		r.logger.WarnContext(ctx, "failed to count other mikan instances", slog.Any("error", err))

		return false
	}

	return n > 0
}

// unreachableTooLong は、まだ何も録っていないのに、チューナーサーバーにつながらないまま ReleaseAfter が過ぎ、
// ほかに mikan がいるかを返す (#524)。チューナーサーバーの再起動はすべての mikan に同時に起きるので、一度つながらないだけでは手放さない。
func (r *Recorder) unreachableTooLong(ctx context.Context, reason FailureReason, created int, since *time.Time) bool {
	if reason != FailureTunerServerUnreachable || created > 0 {
		*since = time.Time{}

		return false
	}
	now := r.clock.Now()
	if since.IsZero() {
		*since = now

		return false
	}

	return now.Sub(*since) >= r.opts.ReleaseAfter && r.hasPeers(ctx)
}

// release は、録画できなかった理由を録画の行に残し、割り振りを手放させるために ErrRecorderUnavailable を返す (#524)。
// ほかの mikan も録れなければ、neko がこの理由で録画を失敗にする。
func (r *Recorder) release(ctx context.Context, recording *sqlc.Recording, reason FailureReason) error {
	err := sqlc.New(r.pool).SetRecordingFailureHint(ctx, sqlc.SetRecordingFailureHintParams{FailureReason: new(string(reason)), Now: r.clock.Now(), ID: recording.ID})
	if err != nil {
		return errors.Join(fmt.Errorf("%w: %s", ErrRecorderUnavailable, reason), fmt.Errorf("failed to record the failure reason: %w", err))
	}

	return fmt.Errorf("%w: %s", ErrRecorderUnavailable, reason)
}

// excludeOverlap は、引き継いで止める古い mikan が、新しい mikan のセグメントが始まった後に数えた品質の集計を、
// 自分のセグメントの集計から差し引くよう記録する (#525)。重なった区間は、新しい mikan のセグメントでも数えるので、
// 差し引かないと二重に数え、録画履歴の閾値を超えて、再放送をまた録ることがある。
func (r *Recorder) excludeOverlap(ctx context.Context, logger *slog.Logger, recording *sqlc.Recording, seg *segment, stats *ts.Stats) {
	end := r.clock.Now()
	// 新しい mikan は、ストリームを開いたと記録してから、セグメントを作る。その間に止められたときのために、少し待つ。
	deadline := time.Now().Add(successorWait)
	for {
		takenOver, err := r.successorStart(ctx, recording, seg)
		if err != nil {
			logger.WarnContext(ctx, "failed to find the segment that took over; counting the overlap twice", slog.Any("error", err))

			return
		}
		if takenOver != nil {
			seg.excluded = countsAfter(seg.samples.list(), *takenOver, end, countsOf(stats))

			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(successorPoll)
	}
}

// successorStart は、引き継いだ新しい mikan のセグメントが始まった時刻を返す。まだなければ nil を返す。
func (r *Recorder) successorStart(ctx context.Context, recording *sqlc.Recording, seg *segment) (*time.Time, error) {
	files, err := sqlc.New(r.pool).ListRecordingFiles(ctx, recording.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recording files: %w", err)
	}
	var started *time.Time
	for i := range files {
		file := &files[i]
		if file.Kind != fileKindSegment || file.ID == seg.fileID || !file.StartedAt.After(seg.startedAt) {
			continue
		}
		if started == nil || file.StartedAt.Before(*started) {
			started = &file.StartedAt
		}
	}

	return started, nil
}

// countSegments は、録画の、書いたものがあるセグメントの数を返す。
func (r *Recorder) countSegments(ctx context.Context, recordingID uuid.UUID) (int, error) {
	files, err := sqlc.New(r.pool).ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return 0, fmt.Errorf("failed to list the recording files: %w", err)
	}
	n := 0
	for i := range files {
		if files[i].Kind == fileKindSegment && files[i].Size > 0 {
			n++
		}
	}

	return n, nil
}

// interruption は、セグメントへの書き込みが途中で止まった理由をログに残し、録画が失敗したときに記録する理由を返す。
// 書き込めなかったか、閉じられなかったなら保存先、そうでなければストリームの中断とする。
func interruption(ctx context.Context, logger *slog.Logger, closeErr, copyErr error) FailureReason {
	if closeErr != nil || errors.Is(copyErr, errWrite) {
		logger.WarnContext(ctx, "failed to write the recording file; starting a new one", slog.Any("error", errors.Join(copyErr, closeErr)))

		return FailureStorageUnavailable
	}
	logger.WarnContext(ctx, "the stream was interrupted; reconnecting", slog.Any("error", copyErr))

	return FailureStreamInterrupted
}

// waitToReopen は、ストリームを開き直す前に d だけ待つ。d が 0 以下なら待たない。
// 待つ前か、待つ間に録画を止めるときが来たら、録画を終え、true と、録画を終えた結果を返す。
func (r *Recorder) waitToReopen(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, recording *sqlc.Recording, tracker *endTracker, result *sessionResult, d time.Duration,
) (bool, error) {
	if d <= 0 {
		// 予定の終わりに、チューナーサーバーがストリームを閉じることもあるので、開き直す前に、止めるときかを確かめる。
		// 切れてすぐなので、途切れた区間は足さない。
		end := tracker.check(ctx)
		if end == endNone {
			return false, nil
		}

		return true, r.finish(ctx, logger, assignment, recording, end, result, false, r.clock.Now(), tracker)
	}
	end, err := tracker.sleep(ctx, d)
	if err != nil {
		return true, err
	}
	if end == endNone {
		return false, nil
	}

	// つながらないまま終えるので、最後のセグメントの終わりから今までも、途切れた区間にする。
	return true, r.finish(ctx, logger, assignment, recording, end, result, true, r.clock.Now(), tracker)
}

// sessionResult は、この mikan が録画を担当した間の結果。
type sessionResult struct {
	// lastFailure は、最後にストリームを開けなかったか、切れた理由。
	lastFailure FailureReason
}

// closeStaleFiles は、録画の書きかけのファイルのうち、staleBefore より前から書き込みのないものを、落ちた mikan が残したものとして、
// ファイルの大きさと、最後に書き込んだ時刻で閉じる。
func (r *Recorder) closeStaleFiles(ctx context.Context, recordingID uuid.UUID, staleBefore time.Time) error {
	q := sqlc.New(r.pool)
	files, err := q.ListOpenRecordingFiles(ctx, recordingID)
	if err != nil {
		return fmt.Errorf("failed to list the open recording files: %w", err)
	}
	for i := range files {
		// ファイルを読めなければ、書き込んだ時刻も分からないので、記録している大きさのまま、書き始めた時刻で閉じる。
		size, writtenAt := files[i].Size, files[i].StartedAt
		if path, err := r.store.Path(files[i].PoolID, files[i].Path); err == nil {
			if info, err := os.Stat(path); err == nil {
				size, writtenAt = info.Size(), maxTime(info.ModTime(), files[i].StartedAt)
			}
		}
		if !writtenAt.Before(staleBefore) {
			continue
		}
		if err := q.CloseRecordingFile(ctx, sqlc.CloseRecordingFileParams{Size: size, EndedAt: &writtenAt, ID: files[i].ID}); err != nil {
			return fmt.Errorf("failed to close an abandoned recording file: %w", err)
		}
	}

	return nil
}

// finish は、録画を止めた理由と、セグメントの時間の範囲から、途切れた区間と録画の最後の状態を決めて記録する (設計書 5.2、5.5)。
// 途切れた区間は、書けたセグメントの時間の範囲のすき間とする。引き継ぎで重なった範囲は、途切れていない。
// ストリームにつながらないまま終えるなら (disconnected)、最後のセグメントの終わりから今 (now) までも、途切れた区間とする。
// now は、録画を止めた時刻。DB に届かない間は、届くまでやり直し、止めた時刻で記録する (#1073)。
func (r *Recorder) finish(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, recording *sqlc.Recording, end endReason, result *sessionResult, disconnected bool,
	now time.Time, tracker *endTracker,
) error {
	// バッファから写したセグメントも、録画の途切れた区間と、セグメントをつなぐ処理に入れるため、写し終えてから終える。
	tracker.backfill.wait()

	return r.untilDatabase(ctx, logger, "finish the recording", func(ctx context.Context) error {
		if err := tracker.pending.save(ctx, r, tracker); err != nil {
			return err
		}
		if err := tracker.persist(ctx); err != nil {
			return err
		}

		return r.finishOnce(ctx, logger, assignment, recording, end, result, disconnected, now)
	})
}

// finishOnce は、finish を 1 度だけ試す。
func (r *Recorder) finishOnce(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, recording *sqlc.Recording, end endReason, result *sessionResult, disconnected bool,
	now time.Time,
) error {
	if end == endCancelled {
		logger.InfoContext(ctx, "the reservation was disabled or deleted while recording")

		return r.fail(ctx, assignment, recording, FailureCancelled, now)
	}
	// 落ちた mikan が残した書きかけのファイルを閉じる。止まる mikan は、ストリームを開いたのを見てから、RenewInterval ごとに確かめて止まるので、
	// それより長く書き込みのないファイルは、落ちた mikan のものとする。
	if err := r.closeStaleFiles(ctx, recording.ID, now.Add(-2*DefaultRenewInterval)); err != nil {
		return err
	}
	files, err := sqlc.New(r.pool).ListRecordingFiles(ctx, recording.ID)
	if err != nil {
		return fmt.Errorf("failed to list the recording files: %w", err)
	}
	plannedStart, found, err := r.plannedStart(ctx, assignment, recording.ID)
	if err != nil {
		return err
	}
	var expectedStart *time.Time
	if found {
		expectedStart = &plannedStart
	}
	gaps, recorded := gapsOf(files, expectedStart, now, disconnected)
	if !recorded {
		reason := result.lastFailure
		if reason == "" {
			reason = FailureStreamInterrupted
		}
		logger.WarnContext(ctx, "nothing was recorded", slog.String("reason", string(reason)))

		return r.fail(ctx, assignment, recording, reason, now)
	}
	encoded, err := json.Marshal(gaps)
	if err != nil {
		return fmt.Errorf("failed to encode the recording gaps: %w", err)
	}
	if err := sqlc.New(r.pool).SetRecordingGaps(ctx, sqlc.SetRecordingGapsParams{Gaps: encoded, Now: now, ID: recording.ID}); err != nil {
		return fmt.Errorf("failed to record the gaps: %w", err)
	}

	// 引き継ぎや再開で、前のセグメントにドロップがあることもあるので、録画全体の集計で決める。
	finished, err := sqlc.New(r.pool).GetRecording(ctx, recording.ID)
	if err != nil {
		return fmt.Errorf("failed to get the recording: %w", err)
	}
	finalState := StateCompleted
	switch {
	case len(gaps) > 0:
		finalState = StatePartial
	case finished.Drops > 0:
		finalState = StateCompletedWithErrors
	}
	if err := r.complete(ctx, assignment, recording.ID, finalState, "", now); err != nil {
		return err
	}
	logger.InfoContext(ctx, "finished recording", slog.String("state", string(finalState)))

	return nil
}

// gap は、途切れた区間。
type gap struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// plannedStart は、予約の予定の開始を返す。割り当てが消えていれば、番組の開始を返す。
// 予約が消えていれば、冒頭の欠けは確かめないので、found を false にする。
// 遅れた番組を待った録画では、放送の開始から、先に書く分の長さを引いた時刻にする。遅れた分は、欠けたのではない (#1046)。
func (r *Recorder) plannedStart(ctx context.Context, assignment *Assignment, recordingID uuid.UUID) (time.Time, bool, error) {
	q := sqlc.New(r.pool)
	reservation, err := q.GetReservationForRecording(ctx, assignment.ReservationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to get the reservation: %w", err)
	}
	planned := reservation.StartAt
	if reservation.PlannedStartAt != nil {
		planned = *reservation.PlannedStartAt
	}
	recording, err := q.GetRecording(ctx, recordingID)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to get the recording: %w", err)
	}
	if recording.WaitedForBroadcast && recording.BroadcastStartedAt != nil {
		startMargin := time.Duration(domain.DefaultMarginSeconds) * time.Second
		if settings, err := r.settings(ctx, &reservation); err == nil {
			startMargin = time.Duration(settings.StartMarginSeconds) * time.Second
		}
		planned = maxTime(planned, recording.BroadcastStartedAt.Add(-min(startMargin, MaxPreroll)))
	}

	return planned, true, nil
}

// gapsOf は、書けたセグメントの時間の範囲のすき間を、途切れた区間として返す。書けたセグメントがなければ false を返す。
// 書いている途中のセグメントは、now まで書いているものとする。trailing なら、最後のセグメントの終わりから now までも途切れた区間にする。
// 最初のセグメントが、予定の開始 plannedStart より LateStartTolerance を超えて遅れて始まったら、その間も途切れた区間にする (#517)。
func gapsOf(files []sqlc.RecordingFile, plannedStart *time.Time, now time.Time, trailing bool) ([]gap, bool) {
	type span struct{ from, to time.Time }
	spans := []span{}
	writing := false
	for i := range files {
		file := &files[i]
		switch {
		case file.Kind != fileKindSegment:
		case file.EndedAt == nil:
			writing = true
			spans = append(spans, span{from: file.StartedAt, to: now})
		case file.Size > 0:
			spans = append(spans, span{from: file.StartedAt, to: *file.EndedAt})
		}
	}
	if len(spans) == 0 {
		return []gap{}, false
	}
	slices.SortFunc(spans, func(a, b span) int { return a.from.Compare(b.from) })
	gaps := []gap{}
	if plannedStart != nil && spans[0].from.After(plannedStart.Add(LateStartTolerance)) {
		gaps = append(gaps, gap{From: *plannedStart, To: spans[0].from})
	}
	covered := spans[0].to
	for _, s := range spans[1:] {
		if s.from.After(covered) {
			gaps = append(gaps, gap{From: covered, To: s.from})
		}
		covered = maxTime(covered, s.to)
	}
	if trailing && !writing && covered.Before(now) {
		gaps = append(gaps, gap{From: covered, To: now})
	}

	return gaps, true
}

// waitUntilOpen は、予定の開始からチューニングにかかる時間を引いた時刻まで待ち、そのときの予約を返す。
// 待つ間に予約を無効にしたか、予定の終わりが過ぎたら、録画しないので false を返す。予約を読めなければ、読めるまでやり直す。
func (r *Recorder) waitUntilOpen(ctx context.Context, assignment *Assignment) (sqlc.GetReservationForRecordingRow, *sessionCache, bool, error) {
	ticker := time.NewTicker(r.opts.Tick)
	defer ticker.Stop()
	// cache は、最後に読めた予約と録画設定。開始の前後に DB に届かなくても、これで録画を始める (#1073)。
	var cache *sessionCache
	for {
		reservation, err := sqlc.New(r.pool).GetReservationForRecording(ctx, assignment.ReservationID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// 録画を始める前に予約が消された。無効にしたときと同じく、録画せずに終える。
			return reservation, cache, false, nil
		case err != nil && cache != nil && database.IsUnavailable(err):
			reservation = cache.reservation
		case err != nil:
			// 一時的に読めなくても諦めない。諦めると割り振りが終わったことになり、録画されない (#516)。
			if ctx.Err() == nil {
				r.logger.WarnContext(ctx, "failed to get the reservation; retrying", slog.Any("error", err))
			}
			select {
			case <-ctx.Done():
				return reservation, cache, false, context.Cause(ctx)
			case <-ticker.C:
			}

			continue
		case cache == nil:
			cache = r.loadSessionCache(ctx, &reservation)
		default:
			cache.reservation = reservation
		}
		if !reservation.Enabled || reservation.PlannedStartAt == nil || reservation.PlannedEndAt == nil {
			return reservation, cache, false, nil
		}
		now := r.clock.Now()
		if !now.Before(*reservation.PlannedEndAt) {
			return reservation, cache, false, nil
		}
		if !now.Before(reservation.PlannedStartAt.Add(-r.opts.TuningTime)) {
			// 録画設定は、始める直前に読み直す。読めなければ、前に読めたものを使う。
			if err == nil {
				cache.refresh(ctx, r, &reservation)
			}

			return reservation, cache, true, nil
		}
		select {
		case <-ctx.Done():
			return reservation, cache, false, context.Cause(ctx)
		case <-ticker.C:
		}
	}
}

// segment は、書いている録画ファイルと、その索引。
type segment struct {
	poolID    string
	path      string
	fileID    uuid.UUID
	file      *os.File
	indexFile *os.File
	buffer    *bufio.Writer
	analyzer  *ts.Analyzer
	startedAt time.Time
	// samples は、書いている間に控えた品質の集計。excluded は、集計から差し引く、引き継がれた後の区間の分 (#525)。
	samples  qualitySamples
	excluded qualityCounts
}

// createSegment は、保存先を選んで録画ファイルと索引を作り、DB に記録する。失敗したら、録画の失敗の理由も返す。
// startedAt は、セグメントの最初のデータを読んだ時刻。遅れた番組を待った後は、溜めた分を読み始めた時刻になる (#1046)。
func (r *Recorder) createSegment(
	ctx context.Context, tracker *endTracker, reservation *sqlc.GetReservationForRecordingRow, startedAt time.Time,
) (*segment, FailureReason, error) {
	dbCtx, cancel := context.WithTimeout(ctx, r.opts.DatabaseTimeout)
	defer cancel()
	settings, err := r.cachedSettings(dbCtx, reservation, tracker.pending.cache)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	mounted := r.store.PoolIDs()
	if (settings.PoolID != "" && !slices.Contains(mounted, settings.PoolID)) || len(mounted) == 0 {
		return nil, FailureStorageUnavailable, fmt.Errorf("%w: %q", kago.ErrPoolNotMounted, settings.PoolID)
	}
	pools, err := r.cachedPools(dbCtx, mounted, settings.PoolID, tracker.pending.cache)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	// 推定サイズは、これから録る残りの時間から求める。途中で開き直すときに、全長で選ぶと終盤ほど選べなくなる (#519)。
	from := maxTime(r.clock.Now(), *reservation.PlannedStartAt)
	estimated := uint64(max(reservation.PlannedEndAt.Sub(from), 0)/time.Second) * estimatedBytesPerSecond
	seg, reason, err := r.newSegment(ctx, settings, pools, reservation, startedAt, estimated)
	if err != nil {
		return nil, reason, err
	}

	// DB に届かなければ、ファイルは残して書き続け、記録は DB に届くようになってからにする (#1073)。
	// 先に記録できていないものがあれば、順を守るため、これも後にする。
	err = tracker.pending.writeOrDefer(ctx, r, insertSegment(seg))
	if err != nil {
		seg.closeFiles()

		return nil, FailureStorageUnavailable, fmt.Errorf("failed to save the recording file: %w", err)
	}

	return seg, "", nil
}

// newSegment は、推定サイズ estimated の入るプールを選んで、録画ファイルと索引を作る。DB には記録しない。
// 失敗したら、録画の失敗の理由も返す。
func (r *Recorder) newSegment(
	ctx context.Context, settings *domain.RecordingSettings, pools []*kago.Pool, reservation *sqlc.GetReservationForRecordingRow,
	startedAt time.Time, estimated uint64,
) (*segment, FailureReason, error) {
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
	name, err := reserve.RenderFileName(tmpl, &reserve.FileNameData{
		StartAt: reservation.StartAt.In(r.location), EndAt: reservation.EndAt.In(r.location), ServiceName: reservation.ServiceName,
		ChannelType: reservation.ChannelType, Channel: reservation.Channel, Name: reservation.Name, EventID: valueOr(reservation.EventID),
	}, segmentExt)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	// ファイル名のテンプレートは、/ でディレクトリに分けられる。
	dir, base := path.Split(name)
	file, rel, err := r.store.Create(pool.ID, strings.TrimSuffix(dir, "/"), base)
	if err != nil {
		return nil, FailureStorageUnavailable, err
	}
	// 索引は、データディレクトリの、セグメントのファイルの ID のディレクトリに置く。作れなくても、録画は続ける。
	fileID := id.New()
	analyzer, indexFile, indexErr := CreateIndex(r.store, fileID)
	if indexErr != nil {
		r.logger.WarnContext(ctx, "failed to create the index; recording without it", slog.String("path", rel), slog.Any("error", indexErr))
	}
	seg := &segment{
		poolID: pool.ID, path: rel, fileID: fileID, file: file, indexFile: indexFile,
		buffer: bufio.NewWriterSize(file, writeBufferSize), analyzer: analyzer, startedAt: startedAt,
		samples: qualitySamples{mu: sync.Mutex{}, samples: []qualitySample{}}, excluded: qualityCounts{Drops: 0, Errors: 0, Scrambled: 0},
	}

	return seg, "", nil
}

// closeFiles は、録画ファイルと索引を、書き出さずに閉じる。記録できなかったセグメントを捨てるときに使う。
func (seg *segment) closeFiles() {
	_ = seg.file.Close()
	if seg.indexFile != nil {
		_ = seg.indexFile.Close()
	}
}

// pools は、録画を書く候補のプールを、優先順位と容量の閾値の設定と一緒に返す (設計書 5.6)。
// 録画設定でプールを指定していれば、そのプールだけを返す。設定のないプールは、優先順位 0、容量の閾値 0 とする。
func poolsFrom(rows []sqlc.StoragePool, mounted []string, poolID string) []*kago.Pool {
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

	return pools
}

// CreateIndex は、store のデータディレクトリの、ファイル fileID のディレクトリに索引を作り、索引を書く解析器を返す。
// 索引を作れなければ、索引を書かない解析器と、そのエラーを返す。タイムシフトのチャンクも、同じ作りの索引を書く。
func CreateIndex(store *kago.Store, fileID uuid.UUID) (*ts.Analyzer, *os.File, error) {
	if _, err := store.CreateSegmentDir(fileID); err != nil {
		return ts.NewAnalyzer(nil), nil, err
	}
	index := kago.IndexLocation(fileID)
	path, err := store.Path(index.PoolID, index.Path)
	if err != nil {
		return ts.NewAnalyzer(nil), nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return ts.NewAnalyzer(nil), nil, fmt.Errorf("failed to create the index file: %w", err)
	}
	writer, err := ts.NewIndexWriter(file)
	if err != nil {
		_ = file.Close()

		return ts.NewAnalyzer(nil), nil, err
	}

	return ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer, CutoffPTS: nil}), file, nil
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
	// endDelayed は、予約した番組が遅れて始まるので、書いているセグメントを消して待つことを表す (#1046)。
	endDelayed
)

// copyUntilEnd は、予定の終わりが来るか、予約を無効にするまで、ストリームを録画ファイルと解析器に書き、止めた理由を返す。
// 予定の終わりは、予約を読み直して決める。その前にストリームが切れたら、エラーを返す。
// errWrite は、録画ファイルへの書き込みに失敗したこと。ストリームの中断と分けて、失敗の理由を保存先にする (#519)。
var errWrite = errors.New("failed to write the recording file")

// writeErrorWriter は、書き込みのエラーに errWrite を付ける。
type writeErrorWriter struct{ w io.Writer }

func (w writeErrorWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if err != nil {
		return n, fmt.Errorf("%w: %w", errWrite, err)
	}

	return n, nil
}

// skipUntil は、clk の until まで stream を読み捨てる。until が過ぎていれば何もしない。
// 読み捨てる長さは最初に決め、経過時間で測る。読み捨てている間に壁時計が戻っても、読み捨て続けて冒頭を失わないようにする。
// 読み捨てた後に書くデータが TS パケットの途中から始まらないよう、パケットの区切りまで読み捨てる。
func skipUntil(clk clock.Clock, stream io.Reader, until time.Time) error {
	stopwatch := clk.StartStopwatch()
	length := until.Sub(clk.Now())
	buf := make([]byte, 64*ts.PacketSize)
	skipped := 0
	for stopwatch.Elapsed() < length {
		n, err := stream.Read(buf)
		skipped += n
		if err != nil {
			return err
		}
	}
	if rest := skipped % ts.PacketSize; rest != 0 {
		if _, err := io.CopyN(io.Discard, stream, int64(ts.PacketSize-rest)); err != nil {
			return err
		}
	}

	return nil
}

func (r *Recorder) copyUntilEnd(ctx context.Context, tracker *endTracker, stream io.ReadCloser, seg *segment) (*ts.Stats, endReason, error) {
	tracker.watch(seg.analyzer)
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

	writer := &flushingWriter{mu: sync.Mutex{}, buffer: seg.buffer, last: time.Now()}
	sampleDone := make(chan struct{})
	go func() {
		defer close(sampleDone)
		ticker := time.NewTicker(r.opts.Tick)
		defer ticker.Stop()
		live := newLiveStatsWriter(r, &tracker.savedRecordingID)
		for {
			stats := seg.analyzer.Stats()
			seg.samples.add(r.clock.Now(), countsOf(stats))
			live.maybeWrite(watchCtx, stats)
			// 書き込みが途絶えても、書いた分をファイルに出す。書き出しのエラーは、閉じるときにも返る。
			_ = writer.flushIfStale()
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	_, err := io.Copy(io.MultiWriter(writeErrorWriter{w: writer}, seg.analyzer), stream)
	stopWatch()
	<-watchDone
	// セグメントを閉じるときにバッファを書き出すので、その前に、書き出すゴルーチンを止める。
	<-sampleDone
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
	// broadcast は、録画中の TS の EIT[p/f] から、予約した番組が実際に放送されているかを追う。番組予約とルール予約だけ (#389)。
	broadcast *broadcastTracker
	// analyzer は、今書いているセグメントの解析器。EIT[p/f] を読む。セグメントを作り直すたびに替わる。
	analyzer atomic.Pointer[ts.Analyzer]
	// serviceID16 は、EIT[p/f] の service_id (サービスの ID の下 5 桁)。
	serviceID16 uint16
	// endMargin は、番組の後ろのマージン。放送が終わってから、これだけ録って止める。
	endMargin time.Duration
	// notBroadcast は、放送されなかったことを記録したかどうか。
	notBroadcast bool
	// expectedOnAir は、予約した番組が放送中になるはずの時刻 (#1046)。実際に録画を始めた時刻と予定の開始の早いほうに、開始マージンを足す。
	// 録り始めた後に EPG で予約の開始が後ろに動いても、録画を始めた時刻で決めるので、変えない。録画を始めるのが遅れても、予定の開始で決める。
	expectedOnAir time.Time
	// startMargin は、番組の前のマージン。遅れた番組を待った後に先に書く長さは、これと MaxPreroll の短いほう。
	startMargin time.Duration
	// startedBefore は、この mikan が録り始める前に、予約した番組がすでに放送中になっていたかどうか (引き継ぎや再開)。
	startedBefore bool
	// waited は、遅れた番組を待ったことを、録画に記録したかどうか。
	waited bool
	// last は、最後に読めた予約。DB に届かない間は、これで止めるときを決める (#1073)。
	last sqlc.GetReservationForRecordingRow
	// unreachable は、予約を読もうとして DB に届かなかったかどうか。ログを、届かなくなったときと戻ったときに 1 度ずつ出す。
	unreachable bool
	// pendingBroadcast は、放送の開始と終了を、DB に届かずに記録できなかったかどうか。録画を終えるときに記録し直す (#1073)。
	pendingBroadcast bool
	// pendingNotBroadcast は、放送されなかったことを、DB に届かずに記録できなかったかどうか。録画を終えるときに記録し直す。
	pendingNotBroadcast bool
	// pending は、DB に届かずに記録できていない録画とセグメント (#1073)。
	pending *pendingSave
	// lastProgram は、最後に読めた番組の終わり。DB に届かない間は、これで延長を続ける (#1073)。
	lastProgram *sqlc.GetProgramEndRow
	// stoppedAt は、予定の終わりだと判断した時刻。録画を止めた時刻は、これにそろえる。まだ判断していなければゼロ値。
	stoppedAt time.Time
	// savedRecordingID は、DB に記録した録画の ID。DB に届かずに仮の録画で始めた間は nil (#1073)。
	// 書いているセグメントの様子 (#829) を書くゴルーチンが読むので、atomic で持つ。
	savedRecordingID atomic.Pointer[uuid.UUID]
	// backfill は、遅れて始めた録画に、予定の開始からの分をタイムシフトのバッファから写す処理 (#826)。写していなければ nil。
	// 録画を終える前に、写し終えるのを待つ。
	backfill *backfill
}

func newEndTracker(
	r *Recorder, recording *sqlc.Recording, reservation *sqlc.GetReservationForRecordingRow, startMargin, endMargin time.Duration,
) *endTracker {
	t := &endTracker{
		recorder: r, recordingID: recording.ID, reservationID: reservation.ID, serviceID: reservation.ServiceID, eventID: reservation.EventID,
		extendedUntil: time.Time{}, limit: reservation.PlannedEndAt.Add(MaxExtension), lost: false,
		broadcast: nil, analyzer: atomic.Pointer[ts.Analyzer]{}, serviceID16: 0,
		endMargin: endMargin, notBroadcast: false,
		expectedOnAir: minTime(recording.RecordedFrom, *reservation.PlannedStartAt).Add(startMargin), startMargin: startMargin, startedBefore: recording.BroadcastStartedAt != nil,
		waited: recording.WaitedForBroadcast, last: *reservation, unreachable: false, pendingBroadcast: false, pendingNotBroadcast: false, lastProgram: nil, stoppedAt: time.Time{},
		pending: &pendingSave{
			recording: recording, start: false, closeStale: false, startedAt: time.Time{}, handedOver: false, writes: []pendingWrite{}, cache: nil,
		},
		savedRecordingID: atomic.Pointer[uuid.UUID]{}, backfill: nil,
	}
	// 時刻予約や、ID が EIT の範囲に収まらない予約では、EIT[p/f] で追わない。
	sid, ok := ts.BroadcastServiceID(reservation.ServiceID)
	if reservation.EventID != nil && ok && *reservation.EventID >= 0 && *reservation.EventID <= 0xFFFF {
		t.serviceID16 = sid
		t.broadcast = newBroadcastTracker(uint16(*reservation.EventID))
	}

	return t
}

// watch は、EIT[p/f] を読む解析器を、今書いているセグメントのものにする。
func (t *endTracker) watch(analyzer *ts.Analyzer) {
	t.analyzer.Store(analyzer)
}

// wait は、録画を止めるときまで待ち、その理由を返す。ctx が終わったら endNone を返す。
// 番組が遅れて始まると分かったら、書いているセグメントを止めて待つよう、endDelayed を返す (#1046)。
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
		// DB に届かない間は、前の番組のセグメントを消せないので、遅れた番組を待ち始めない。届くようになってから待つ (#1073)。
		if !t.unreachable && t.delayed(t.recorder.clock.Now()) {
			return endDelayed
		}
	}
}

// delayed は、番組が始まるはずの時刻から BroadcastStartGrace が過ぎても、予約した番組が一度も放送中にならず、
// EIT[p/f] の次の番組にいるかどうか (#1046)。前の番組 (スポーツ中継など) が延び、予約した番組が遅れて始まるときに当たる。
func (t *endTracker) delayed(now time.Time) bool {
	return !t.startedBefore && t.broadcast.pending() && !now.Before(t.expectedOnAir.Add(BroadcastStartGrace))
}

// mustWait は、ストリームを開いた後、書き始める前に、予約した番組が放送中になるのを待つかどうか (#1046)。
// まだ何も書いていない録画で、番組が始まるはずの時刻から BroadcastStartGrace が過ぎていれば、EIT[p/f] を読んでから決める。
// 遅れた番組を待っている間に、ストリームが切れたときや、ほかの mikan に引き継いだときに当たる。
func (t *endTracker) mustWait(created int, now time.Time) bool {
	return t.broadcast != nil && !t.startedBefore && t.broadcast.startedAt == nil && created == 0 &&
		!now.Before(t.expectedOnAir.Add(BroadcastStartGrace))
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

// reload は、予約を読み直す。録画中に予約が消されたら endCancelled を返す。読めなければ、false を返す。
// DB に届かない間は、最後に読めた予約を返し、予定の終わりに止められるようにする (#1073)。
func (t *endTracker) reload(ctx context.Context) (sqlc.GetReservationForRecordingRow, endReason, bool) {
	r := t.recorder
	reservation, err := sqlc.New(r.pool).GetReservationForRecording(ctx, t.reservationID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// 録画中に予約が消された。無効にしたときと同じく、取り消しとして終える。
		return reservation, endCancelled, false
	case err != nil && errors.Is(ctx.Err(), context.Canceled):
		return reservation, endNone, false
	case err != nil && !database.IsUnavailable(err):
		r.logger.WarnContext(ctx, "failed to reload the reservation", slog.Any("error", err))

		return reservation, endNone, false
	case err != nil:
		if !t.unreachable {
			r.logger.WarnContext(ctx, "failed to reload the reservation; using the last one until the database is reachable", slog.Any("error", err))
			t.unreachable = true
		}

		return t.last, endNone, true
	}
	if t.unreachable {
		r.logger.InfoContext(ctx, "reloaded the reservation again")
		t.unreachable = false
	}
	t.last = reservation
	// DB に届くようになったら、録画が終わるのを待たずに、記録できていない録画とセグメントを記録する。
	if t.pending.waiting() {
		if err := t.pending.save(ctx, r, t); err != nil && ctx.Err() == nil {
			r.logger.WarnContext(ctx, "failed to save the recording", slog.Any("error", err))
		}
	}

	return reservation, endNone, true
}

// check は、今が録画を止めるときかを確かめる。読めなければ、録画を続ける。
func (t *endTracker) check(ctx context.Context) endReason {
	r := t.recorder
	// checkedAt は、DB を待つ前の時刻。DB に届かずに待たされたら、録画を止めた時刻は、これにする (#1073)。
	checkedAt := r.clock.Now()
	// DB への接続が詰まっても、予定の終わりに気づけるよう、DB を待つのは DatabaseTimeout までにする。
	ctx, cancel := context.WithTimeout(ctx, r.opts.DatabaseTimeout)
	defer cancel()
	q := sqlc.New(r.pool)
	reservation, reason, ok := t.reload(ctx)
	if !ok {
		return reason
	}
	// 割り当てを消されても (予定の時刻が NULL)、無効にしたのでなければ、取り消しとは扱わない。
	if !reservation.Enabled {
		return endCancelled
	}
	now := r.clock.Now()
	stoppedAt := now
	if t.unreachable {
		stoppedAt = checkedAt
	}
	if reservation.PlannedEndAt == nil {
		t.stoppedAt = stoppedAt

		return endPlanned
	}
	end := *reservation.PlannedEndAt
	t.observeBroadcast(ctx, now)
	if t.eventID != nil {
		program, err := t.program(ctx, q)
		switch {
		case errors.Is(err, errNoProgramYet):
		case errors.Is(err, pgx.ErrNoRows):
			t.markLost(ctx)
		case err != nil:
			if ctx.Err() == nil {
				r.logger.WarnContext(ctx, "failed to reload the program", slog.Any("error", err))
			}
		case t.broadcast.leftAir():
			// EIT[p/f] で放送の終わりを検知したら、終わりは broadcastEnd で決める。EPG で終了時刻が未定のままでも延ばさない (#643)。
		case program.EndUndecided:
			// 延長中は、終わりが来るたびに延ばす。
			if !now.Before(end) && !now.Before(t.extendedUntil) {
				t.extendedUntil = minTime(now.Add(ExtensionStep), t.limit)
				r.logger.InfoContext(ctx, "the program end is undecided; extending the recording",
					slog.String("recording_id", t.recordingID.String()), slog.Time("until", t.extendedUntil))
			}
			end = maxTime(end, t.extendedUntil)
		case !t.extendedUntil.IsZero():
			// 延長の後に終了時刻が決まったら、決まった終わりと後ろのマージンまで録る (#512)。
			// 予定の終わりは、過ぎると割り当ての対象から外れて更新されないので、番組の終わりから求める。
			end = maxTime(end, minTime(program.EndAt.Add(t.endMargin), t.limit))
		}
	}
	end, notBroadcast := t.broadcastEnd(end, now)
	if !now.Before(end) {
		if notBroadcast {
			t.markNotBroadcast(ctx)
		}
		t.stoppedAt = stoppedAt

		return endPlanned
	}

	return endNone
}

// errNoProgramYet は、DB に届かず、番組の終わりをまだ一度も読めていないこと。
var errNoProgramYet = errors.New("the program end has not been read yet")

// program は、番組の終わりを読む。DB に届かない間は、最後に読めたものを返す。延長中なら、延長を続けられる (#1073)。
func (t *endTracker) program(ctx context.Context, q *sqlc.Queries) (sqlc.GetProgramEndRow, error) {
	if t.unreachable {
		if t.lastProgram == nil {
			return sqlc.GetProgramEndRow{}, errNoProgramYet
		}

		return *t.lastProgram, nil
	}
	program, err := q.GetProgramEnd(ctx, sqlc.GetProgramEndParams{ServiceID: t.serviceID, EventID: *t.eventID})
	switch {
	case err == nil:
		t.lastProgram = &program
	case database.IsUnavailable(err) && t.lastProgram != nil:
		// 予約を読み直した直後に DB に届かなくなっても、最後に読めた番組の終わりで延長を続ける。
		return *t.lastProgram, nil
	}

	return program, err
}

// observeBroadcast は、録画中の TS の EIT[p/f] を読み、放送の開始と終了を記録する (#389)。
func (t *endTracker) observeBroadcast(ctx context.Context, now time.Time) {
	analyzer := t.analyzer.Load()
	if t.broadcast == nil || analyzer == nil {
		return
	}
	r := t.recorder
	q := sqlc.New(r.pool)
	change := t.broadcast.update(analyzer.PresentFollowing(t.serviceID16), now)
	if change.started {
		if err := q.SetRecordingBroadcastStarted(ctx, sqlc.SetRecordingBroadcastStartedParams{At: &now, ID: t.recordingID}); err != nil {
			t.pendingBroadcast = true
			r.logger.WarnContext(ctx, "failed to record the broadcast start", slog.Any("error", err))
		}
	}
	if change.ended {
		// 放送中が空のまま続いて終わったとみなしたときは、空になった時刻を記録する (#637)。
		if err := q.SetRecordingBroadcastEnded(ctx, sqlc.SetRecordingBroadcastEndedParams{At: t.broadcast.endedAt, ID: t.recordingID}); err != nil {
			t.pendingBroadcast = true
			r.logger.WarnContext(ctx, "failed to record the broadcast end", slog.Any("error", err))
		}
		r.logger.InfoContext(ctx, "the program left the air; recording the end margin",
			slog.String("recording_id", t.recordingID.String()), slog.Duration("margin", t.endMargin))
	}
	if change.resumed {
		if err := q.ClearRecordingBroadcastEnded(ctx, sqlc.ClearRecordingBroadcastEndedParams{Now: now, ID: t.recordingID}); err != nil {
			t.pendingBroadcast = true
			r.logger.WarnContext(ctx, "failed to clear the broadcast end", slog.Any("error", err))
		}
		r.logger.InfoContext(ctx, "the program is back on the air; continuing the recording", slog.String("recording_id", t.recordingID.String()))
	}
}

// broadcastEnd は、observeBroadcast で読んだ放送の状態で、EPG から決めた終わり end を決め直す (#389)。
// 放送されなかったなら、notBroadcast を true にする。
func (t *endTracker) broadcastEnd(end, now time.Time) (time.Time, bool) {
	if t.broadcast == nil {
		return end, false
	}

	return t.broadcast.end(end, now, t.endMargin, ExtensionStep, t.limit)
}

// markNotBroadcast は、予約した番組が放送されなかったことと、その時間に放送されていた番組を、録画に一度だけ記録する。
func (t *endTracker) markNotBroadcast(ctx context.Context) {
	if t.notBroadcast {
		return
	}
	r := t.recorder
	if err := t.saveNotBroadcast(ctx); err != nil {
		// DB に届かなければ、録画を終えるときに記録し直す。
		t.pendingNotBroadcast = database.IsUnavailable(err)
		r.logger.WarnContext(ctx, "failed to record that the program was not broadcast", slog.Any("error", err))

		return
	}
	t.notBroadcast = true
	if _, err := notify.Publish(ctx, r.pool, r.clock, notify.RecordingsChannel); err != nil {
		r.logger.WarnContext(ctx, "failed to notify the recording change", slog.Any("error", err))
	}
	r.logger.WarnContext(ctx, "the program was not broadcast during the planned time", slog.String("recording_id", t.recordingID.String()))
}

// saveNotBroadcast は、放送されなかったことと、その時間に放送されていた番組を記録する。
func (t *endTracker) saveNotBroadcast(ctx context.Context) error {
	r := t.recorder
	var aired *int64
	if present := t.broadcast.present; present != nil {
		aired = new(int64(*present))
	}

	return sqlc.New(r.pool).MarkRecordingNotBroadcast(ctx, sqlc.MarkRecordingNotBroadcastParams{AiredEventID: aired, Now: r.clock.Now(), ID: t.recordingID})
}

// persist は、DB に届かずに記録できなかった放送の状態を、記録し直す (#1073)。録画を終える前に呼ぶ。
// 放送の開始と終了は、一度きりの変化を記録しそこねても、最後の状態を書けば足りる。
func (t *endTracker) persist(ctx context.Context) error {
	r := t.recorder
	q := sqlc.New(r.pool)
	if t.pendingBroadcast && t.broadcast != nil {
		if err := t.persistBroadcast(ctx, q); err != nil {
			return err
		}
		t.pendingBroadcast = false
	}
	if t.pendingNotBroadcast {
		if err := t.saveNotBroadcast(ctx); err != nil {
			return fmt.Errorf("failed to record that the program was not broadcast: %w", err)
		}
		t.pendingNotBroadcast = false
		t.notBroadcast = true
	}

	return nil
}

// persistBroadcast は、放送の開始と終了の最後の状態を記録する。
func (t *endTracker) persistBroadcast(ctx context.Context, q *sqlc.Queries) error {
	if started := t.broadcast.startedAt; started != nil {
		if err := q.SetRecordingBroadcastStarted(ctx, sqlc.SetRecordingBroadcastStartedParams{At: started, ID: t.recordingID}); err != nil {
			return fmt.Errorf("failed to record the broadcast start: %w", err)
		}
	}
	if t.broadcast.endedAt != nil {
		if err := q.SetRecordingBroadcastEnded(ctx, sqlc.SetRecordingBroadcastEndedParams{At: t.broadcast.endedAt, ID: t.recordingID}); err != nil {
			return fmt.Errorf("failed to record the broadcast end: %w", err)
		}

		return nil
	}
	if err := q.ClearRecordingBroadcastEnded(ctx, sqlc.ClearRecordingBroadcastEndedParams{Now: t.recorder.clock.Now(), ID: t.recordingID}); err != nil {
		return fmt.Errorf("failed to clear the broadcast end: %w", err)
	}

	return nil
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
	if _, err := notify.Publish(ctx, r.pool, r.clock, notify.RecordingsChannel); err != nil {
		r.logger.WarnContext(ctx, "failed to notify the recording change", slog.Any("error", err))
	}
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
// ctx は Session の context。リースを失った後や止めるときも、1 度は記録を試す。now は、セグメントに書くのをやめた時刻。
func (r *Recorder) closeSegment(
	ctx context.Context, logger *slog.Logger, tracker *endTracker, recording *sqlc.Recording, seg *segment, stats *ts.Stats, now time.Time,
) error {
	stop := ctx
	ctx = context.WithoutCancel(ctx)
	errs := seg.syncAndClose(ctx, logger)
	writes := []pendingWrite{r.closeSegmentFile(logger, recording, seg, stats, now), r.saveSegmentStats(seg, stats, now)}
	if stop.Err() != nil {
		// リースを失ったか、止めるときは、DB に届くのを待たずに、1 度だけ書いてみる。
		if err := tracker.pending.save(ctx, r, tracker); err != nil {
			errs = append(errs, err)
		}
		for _, write := range writes {
			if err := write(ctx, sqlc.New(r.pool), recording.ID); err != nil {
				errs = append(errs, err)
			}
		}

		return errors.Join(errs...)
	}
	// DB に届かなければ、記録は DB に届くようになってからにし、待たずに次のセグメントに進む (#1073)。
	for _, write := range writes {
		if err := tracker.pending.writeOrDefer(ctx, r, write); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// syncAndClose は、録画ファイルと索引を書き出し、fsync してから閉じる (設計書 5.2)。録画ファイルを残せなかったエラーを返す。
// 索引は、なくても再生できるので、書けなくてもエラーにしない。
func (seg *segment) syncAndClose(ctx context.Context, logger *slog.Logger) []error {
	errs := []error{}
	if err := seg.buffer.Flush(); err != nil {
		errs = append(errs, fmt.Errorf("failed to flush the recording file: %w", err))
	}
	if err := seg.file.Sync(); err != nil {
		errs = append(errs, fmt.Errorf("failed to sync the recording file: %w", err))
	}
	if err := seg.file.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close the recording file: %w", err))
	}
	if seg.indexFile != nil {
		if err := seg.indexFile.Sync(); err != nil {
			logger.WarnContext(ctx, "failed to sync the index file", slog.Any("error", err))
		}
		if err := seg.indexFile.Close(); err != nil {
			logger.WarnContext(ctx, "failed to close the index file", slog.Any("error", err))
		}
	}

	return errs
}

// closeSegmentFile は、セグメント seg の大きさと、表す時間の終わり now を記録する書き込みを返す。
// 音声の構成と PID ごとの内訳は、見るためだけなので、書けなくても録画の失敗にしない。
func (r *Recorder) closeSegmentFile(logger *slog.Logger, recording *sqlc.Recording, seg *segment, stats *ts.Stats, now time.Time) pendingWrite {
	return func(ctx context.Context, q *sqlc.Queries, recordingID uuid.UUID) error {
		if err := q.CloseRecordingFile(ctx, sqlc.CloseRecordingFileParams{Size: stats.Bytes, EndedAt: &now, ID: seg.fileID}); err != nil {
			return fmt.Errorf("failed to save the recording file size: %w", err)
		}
		// EPG から音声の構成を写せなかった録画 (時刻予約など) は、TS の EIT[p/f] から読んだもので埋める (設計書 5.3)。
		if audios, ok, err := ts.RecordingAudios(stats.Events, recording.ServiceID, recording.EventID); err != nil {
			logger.WarnContext(ctx, "failed to encode the audio components", slog.Any("error", err))
		} else if ok {
			if err := q.SetRecordingAudiosIfUnknown(ctx, sqlc.SetRecordingAudiosIfUnknownParams{Audios: audios, Now: now, ID: recordingID}); err != nil {
				logger.WarnContext(ctx, "failed to save the audio components", slog.Any("error", err))
			}
		}
		// PID ごとの内訳は、どの PID のドロップかを見るためだけなので、書けなくても録画の失敗にしない (#473)。
		if pidStats, err := json.Marshal(ts.PIDQualities(stats, stats.PIDs)); err != nil {
			logger.WarnContext(ctx, "failed to encode the PID stats", slog.Any("error", err))
		} else if err := q.SetRecordingFilePIDStats(ctx, sqlc.SetRecordingFilePIDStatsParams{PIDStats: pidStats, ID: seg.fileID}); err != nil {
			logger.WarnContext(ctx, "failed to save the PID stats", slog.Any("error", err))
		}

		return nil
	}
}

// saveSegmentStats は、セグメント seg のドロップ、エラー、スクランブルの数を、録画に足す書き込みを返す。
func (r *Recorder) saveSegmentStats(seg *segment, stats *ts.Stats, now time.Time) pendingWrite {
	return func(ctx context.Context, q *sqlc.Queries, recordingID uuid.UUID) error {
		if err := addSegmentStats(ctx, q, recordingID, seg, stats, now); err != nil {
			return err
		}
		// 閉じたセグメントの数は、録画に足したので、書いているセグメントの様子を 0 に戻す (#829)。見るためだけなので、書けなくても止めない。
		_ = q.ResetRecordingLiveStats(ctx, sqlc.ResetRecordingLiveStatsParams{Now: now, RecordingID: recordingID})

		return nil
	}
}

// addSegmentStats は、セグメント seg のドロップ、エラー、スクランブルの数を、録画 recordingID に足す。
func addSegmentStats(ctx context.Context, q *sqlc.Queries, recordingID uuid.UUID, seg *segment, stats *ts.Stats, now time.Time) error {
	var errorsCount, scrambled int64
	for _, pid := range stats.PIDs {
		errorsCount += pid.Errors
		scrambled += pid.Scrambled
	}
	// 引き継がれた後の区間は、新しい mikan のセグメントで数えるので、差し引く (#525)。
	counted := qualityCounts{Drops: drops(stats), Errors: errorsCount, Scrambled: scrambled}.minus(seg.excluded)
	err := q.AddRecordingStats(ctx, sqlc.AddRecordingStatsParams{
		Drops: max(counted.Drops, 0), Errors: max(counted.Errors, 0), Scrambled: max(counted.Scrambled, 0), Now: now, ID: recordingID,
	})
	if err != nil {
		return fmt.Errorf("failed to save the recording stats: %w", err)
	}

	return nil
}

// fail は、録画を失敗にし、理由を記録する。割り振りは終わったことにするので、nil を返す。at は、録画を止めた時刻。
func (r *Recorder) fail(ctx context.Context, assignment *Assignment, recording *sqlc.Recording, reason FailureReason, at time.Time) error {
	return r.complete(ctx, assignment, recording.ID, StateFailed, reason, at)
}

// start は、予約の録画を始め、新しく始めたときは、録画の開始を通知する (設計書 6.2)。引き継ぎや再開で続きを録るときは通知しない。
// at は、録画を始めた時刻。DB に届かずに記録が遅れても、この時刻で記録する (#1073)。
func (r *Recorder) start(ctx context.Context, reservation *sqlc.GetReservationForRecordingRow, at time.Time) (sqlc.Recording, error) {
	var recording sqlc.Recording
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		started, err := sqlc.New(tx).StartRecording(ctx, sqlc.StartRecordingParams{
			ID: id.New(), ReservationID: &reservation.ID, ServiceID: reservation.ServiceID, EventID: reservation.EventID,
			Name: reservation.Name, Description: reservation.Description,
			NameNormalized: new(domain.Normalize(reservation.Name)), DescriptionNormalized: new(domain.Normalize(reservation.Description)),
			StartAt: reservation.StartAt, EndAt: reservation.EndAt,
			RecordedFrom: maxTime(at, *reservation.PlannedStartAt), Now: at,
		})
		if err != nil {
			return fmt.Errorf("failed to start the recording: %w", err)
		}
		recording = started.Recording
		if !started.Inserted {
			return nil
		}
		// 解像度と同じく、録画を始めたときの番組とアニメの作品の結びつきを写す (設計書 3.8)。
		if err := anime.CopyToRecording(ctx, tx, recording.ID, recording.ServiceID, recording.StartAt, at); err != nil {
			return err
		}
		if _, err := notify.Publish(ctx, tx, r.clock, notify.RecordingsChannel); err != nil {
			return err
		}

		target, err := notification.LoadReservationContext(ctx, tx, reservation.ID)
		if err != nil {
			return err
		}
		payload := recordingPayload(recording.ID, recording.Name, reservation.ServiceName, recording.StartAt, recording.EndAt)
		target.AddTo(payload)

		return notification.EmitTo(ctx, tx, r.clock, domain.NotificationKindRecordingStarted, payload, target.Audience)
	})

	return recording, err
}

// prepare は、録画を始める前に、落ちた mikan が残した書きかけのセグメントを閉じ、録画のセグメントの数を返す。
// 落ちた mikan の代わりに取ったなら、書きかけのセグメントを、最後に書き込んだ時刻で閉じる (設計書 5.4)。
// 止まる mikan から引き継いだなら、古い mikan がまだ書いているので、閉じない。
// 録画を記録できていなければ (pendingStart)、DB に届くようになってから閉じ、セグメントはまだないものとする (#1073)。
func (r *Recorder) prepare(ctx context.Context, assignment *Assignment, recording *sqlc.Recording, pendingStart bool) (int, error) {
	if pendingStart {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(ctx, r.opts.DatabaseTimeout)
	defer cancel()
	if !assignment.HandedOver {
		if err := r.closeStaleFiles(ctx, recording.ID, r.clock.Now()); err != nil {
			return 0, err
		}
	}

	return r.countSegments(ctx, recording.ID)
}

// finishLeftOver は、予定の終わりの後に割り振りを取ったとき、前の mikan が録画中のまま残した録画を、予定の終わりで終える (#1073)。
// DB が予定の終わりをまたいで止まり、戻った直後にこの mikan が割り振りを取ると、録り終えた前の mikan は録画を完了にできない
// (世代番号が進んでいるので)。前の mikan が、DB に届かずに覚えていた記録を書く間、少し待ってから終える。
// 落ちた mikan の代わりなら、録れなかった終わりの部分は、途切れた区間になる。
func (r *Recorder) finishLeftOver(ctx context.Context, assignment *Assignment, reservation *sqlc.GetReservationForRecordingRow) error {
	if !reservation.Enabled || reservation.PlannedStartAt == nil || reservation.PlannedEndAt == nil || r.clock.Now().Before(*reservation.PlannedEndAt) {
		return nil
	}
	recording, err := sqlc.New(r.pool).GetRecordingInProgressByReservation(ctx, &reservation.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get the recording in progress: %w", err)
	}
	logger := r.logger.With(slog.String("reservation_id", reservation.ID.String()), slog.String("recording_id", recording.ID.String()))
	logger.InfoContext(ctx, "finishing the recording left in progress after the planned end")
	timer := time.NewTimer(leftOverGrace * r.opts.RetryInterval)
	select {
	case <-ctx.Done():
		timer.Stop()

		return context.Cause(ctx)
	case <-timer.C:
	}
	tracker := newEndTracker(r, &recording, reservation, 0, 0)
	tracker.pending.recording = &recording

	return r.finish(ctx, logger, assignment, &recording, endPlanned, &sessionResult{lastFailure: ""}, true, *reservation.PlannedEndAt, tracker)
}

// startOrDefer は、録画を始めたことを記録する。DB に届かなければ、仮の録画を返し、記録できていないことを true で知らせる (#1073)。
func (r *Recorder) startOrDefer(
	ctx context.Context, logger *slog.Logger, reservation *sqlc.GetReservationForRecordingRow, startedAt time.Time,
) (sqlc.Recording, bool, error) {
	startCtx, cancel := context.WithTimeout(ctx, r.opts.DatabaseTimeout)
	defer cancel()
	recording, err := r.start(startCtx, reservation, startedAt)
	if database.IsUnavailable(err) {
		logger.WarnContext(ctx, "failed to start the recording; keep recording and save it when the database is reachable", slog.Any("error", err))

		return provisionalRecording(reservation, startedAt), true, nil
	}

	return recording, false, err
}

// recordingPayload は、録画の開始、完了、失敗の通知に載せる、録画の項目を返す。
func recordingPayload(recordingID uuid.UUID, name, serviceName string, startAt, endAt time.Time) map[string]any {
	return map[string]any{
		"recordingId": recordingID.String(), "name": name, "serviceName": serviceName, "startAt": startAt, "endAt": endAt,
	}
}

// complete は、録画を state で終え、正常に終わってドロップが閾値以下なら録画履歴に記録し (設計書 4.4)、
// 録画の完了か失敗を通知する (設計書 6.2)。完了の通知には、Webhook のためにセグメントのプール内のパスを載せる。
// at は、録画を止めた時刻。DB に届かずに記録が遅れても、録画の終わりはこの時刻にする。
func (r *Recorder) complete(ctx context.Context, assignment *Assignment, recordingID uuid.UUID, state State, reason FailureReason, at time.Time) error {
	// 録画設定は、トランザクションの外で先に読む。トランザクションの中でプールから別の接続を借りると、接続が足りないときに詰まる。
	settings := r.completionSettings(ctx, assignment)
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		// 引き継ぎの最中に予定の終わりが来ると、古い mikan と新しい mikan の両方が録画を終えようとする。
		// 録画を終えるのは、今の世代の mikan だけにする。
		generation, err := q.LockRecordingAssignmentGeneration(ctx, assignment.ID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return fmt.Errorf("failed to lock the recording assignment: %w", err)
		case generation != assignment.Generation:
			return errSuperseded
		}
		now := at
		var failure *string
		if reason != "" {
			failure = new(string(reason))
		}
		if err := q.FinishRecording(ctx, sqlc.FinishRecordingParams{State: string(state), FailureReason: failure, Now: &now, ID: recordingID}); err != nil {
			return fmt.Errorf("failed to finish the recording: %w", err)
		}
		if _, err := notify.Publish(ctx, tx, r.clock, notify.RecordingsChannel); err != nil {
			return err
		}
		row, err := q.GetRecordingWithService(ctx, recordingID)
		if err != nil {
			return fmt.Errorf("failed to get the recording: %w", err)
		}
		recording := row.Recording
		payload := recordingPayload(recording.ID, recording.Name, row.ServiceName, recording.StartAt, recording.EndAt)
		payload["state"] = string(state)
		target, err := notification.LoadReservationContext(ctx, tx, assignment.ReservationID)
		if err != nil {
			return err
		}
		target.AddTo(payload)
		if state == StateFailed {
			payload["reason"] = string(reason)

			return notification.EmitTo(ctx, tx, r.clock, domain.NotificationKindRecordingFailed, payload, target.Audience)
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
		payload["joinSegments"] = settings.JoinSegments
		payload["detectCm"] = settings.DetectCM
		addNotBroadcast(ctx, q, &recording, payload)

		return notification.EmitTo(ctx, tx, r.clock, domain.NotificationKindRecordingCompleted, payload, target.Audience)
	})
	if errors.Is(err, errSuperseded) {
		r.logger.InfoContext(ctx, "another mikan took over the recording; leaving it to finish", slog.String("recording_id", recordingID.String()))

		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to complete the recording: %w", err)
	}

	return nil
}

// completionSettings は、録画を終えた後の処理 (分かれたセグメントをつなぐか (設計書 5.4)、CM の区間を見分けるか (設計書 6.1)) に使う録画設定を、
// 録画を終えるときの全体の既定値に、ルールと予約の上書きを重ねて求める。録画を終えるときに求めるので、予約の後で全体の既定値を変えても効く。
// 予約が消えていれば、全体の既定値に従う。録画設定を読めなくても、録画の完了は止めず、Go の既定値 (つなぎ、CM は見分けない) にする。
func (r *Recorder) completionSettings(ctx context.Context, assignment *Assignment) *domain.RecordingSettings {
	reservation, err := sqlc.New(r.pool).GetReservationForRecording(ctx, assignment.ReservationID)
	if err == nil {
		settings, err := r.settings(ctx, &reservation)
		if err == nil {
			return settings
		}
	}
	settings, err := r.defaults(ctx, r.pool)
	if err != nil {
		r.logger.WarnContext(ctx, "failed to read the recording settings; using the built-in defaults after the recording", slog.Any("error", err))

		return domain.DefaultRecordingSettings()
	}

	return settings
}

// addNotBroadcast は、予約した番組が放送されなかったら、通知に、そのことと、その時間に放送されていた番組の名前を載せる (#389)。
func addNotBroadcast(ctx context.Context, q *sqlc.Queries, recording *sqlc.Recording, payload map[string]any) {
	if !recording.NotBroadcast {
		return
	}
	payload["notBroadcast"] = true
	if recording.AiredEventID == nil {
		return
	}
	aired, err := q.GetAiredProgramName(ctx, sqlc.GetAiredProgramNameParams{
		ServiceID: recording.ServiceID, EventID: *recording.AiredEventID, WindowStart: recording.StartAt, WindowEnd: recording.EndAt,
	})
	if err == nil {
		payload["airedName"] = aired
	}
}

// errSuperseded は、ほかの mikan が割り振りを取ったので、録画を終えないことを表す。
var errSuperseded = errors.New("the recording assignment was taken over")

// settings は、予約の録画設定を、全体の既定値に、ルール予約ならルールの上書き、さらに予約自体の上書きを重ねて求める。
func (r *Recorder) settings(ctx context.Context, reservation *sqlc.GetReservationForRecordingRow) (*domain.RecordingSettings, error) {
	q := sqlc.New(r.pool)
	settings, err := r.defaults(ctx, r.pool)
	if err != nil {
		return nil, err
	}
	if domain.ReservationKind(reservation.Kind) == domain.ReservationKindRule {
		rows, err := q.ListFirstRuleSettings(ctx, []uuid.UUID{reservation.ID})
		if err != nil {
			return nil, fmt.Errorf("failed to list rule settings: %w", err)
		}
		for _, row := range rows {
			overrides, err := domain.DecodeOverrides(row.Settings)
			if err != nil {
				return nil, err
			}
			settings = settings.Resolve(overrides)
		}
	}
	overrides, err := domain.DecodeOverrides(reservation.Settings)
	if err != nil {
		return nil, err
	}

	return settings.Resolve(overrides), nil
}

// streamFailure は、ストリームを開けなかったエラーを、録画の失敗の理由にする。
func streamFailure(err error) FailureReason {
	// 開いた後に切れたので、チューナーサーバーにはつながっている。
	if errors.Is(err, errInterruptedBeforeStart) {
		return FailureStreamInterrupted
	}
	// チューナーサーバーは、チューナーの空きを待つ間、応答を返さないことがある。つながってはいるので、チューナーが空いていないとみなす。
	if errors.Is(err, mirakurun.ErrStreamOpenTimeout) {
		return FailureTunerUnavailable
	}
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

// liveStatsInterval は、録画中の録画の、書いているセグメントの様子を DB に書く間隔 (#829)。
const liveStatsInterval = 10 * time.Second

// liveStatsWriter は、書いているセグメントの様子 (バイト数、ドロップなど、直近のビットレート) を、liveStatsInterval ごとに
// DB に書く (#829)。システムの状態の画面と /metrics が読む。見るためだけなので、書けなくても録画を止めない。
type liveStatsWriter struct {
	recorder *Recorder
	// recordingID は、DB に記録した録画の ID。仮の録画の間は nil なので、書かない (#1073)。
	recordingID *atomic.Pointer[uuid.UUID]
	stopwatch   clock.Stopwatch
	// lastAt と lastBytes は、前に書いたときの経過時間とバイト数。まだ書いていなければ、lastAt は負。
	lastAt    time.Duration
	lastBytes int64
}

func newLiveStatsWriter(r *Recorder, recordingID *atomic.Pointer[uuid.UUID]) *liveStatsWriter {
	return &liveStatsWriter{recorder: r, recordingID: recordingID, stopwatch: r.clock.StartStopwatch(), lastAt: -1, lastBytes: 0}
}

// maybeWrite は、前に書いてから liveStatsInterval が過ぎていれば、stats を書く。最初は、ビットレートを測るための区切りにする。
func (w *liveStatsWriter) maybeWrite(ctx context.Context, stats *ts.Stats) {
	now := w.stopwatch.Elapsed()
	if w.lastAt < 0 {
		w.lastAt, w.lastBytes = now, stats.Bytes

		return
	}
	elapsed := now - w.lastAt
	if elapsed < liveStatsInterval {
		return
	}
	bitrate := (stats.Bytes - w.lastBytes) * 8 * int64(time.Second) / int64(elapsed)
	w.lastAt, w.lastBytes = now, stats.Bytes
	recordingID := w.recordingID.Load()
	if recordingID == nil {
		return
	}
	counts := countsOf(stats)
	dbCtx, cancel := context.WithTimeout(ctx, w.recorder.opts.DatabaseTimeout)
	defer cancel()
	err := sqlc.New(w.recorder.pool).UpsertRecordingLiveStats(dbCtx, sqlc.UpsertRecordingLiveStatsParams{
		RecordingID: *recordingID, Bytes: stats.Bytes, Drops: counts.Drops, Errors: counts.Errors, Scrambled: counts.Scrambled,
		Bitrate: max(bitrate, 0), Now: w.recorder.clock.Now(),
	})
	if err != nil && ctx.Err() == nil {
		w.recorder.logger.WarnContext(ctx, "failed to save the live recording stats", slog.Any("error", err))
	}
}
