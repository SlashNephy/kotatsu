package recording

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

const (
	// promoteTimeout は、昇格のジョブの時間の上限。バッファから数 GB を読み書きするので、長めにとる。
	promoteTimeout = 2 * time.Hour
	// promoteMaxAttempts は、昇格のジョブを試す回数。試し尽くしたら、録画を失敗にする。
	promoteMaxAttempts = 5
)

// errPromotionGone は、昇格の行が消えたことを表す。録画を消したか、ほかの nekonote が書き出し終えた。
var errPromotionGone = errors.New("the timeshift promotion no longer exists")

// PromoteArgs は、タイムシフトのバッファにある、放送の終わった番組を、録画に書き出すジョブの引数 (#1057、設計書 5.7)。
type PromoteArgs struct {
	RecordingID uuid.UUID `json:"recordingId"`
}

var _ river.JobArgsWithInsertOpts = new(PromoteArgs)

func (PromoteArgs) Kind() string { return "timeshift_promote" }

func (PromoteArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// 書き出しは数分かかるので、代表画像やセグメントをつなぐジョブ (media のキュー) を待たせないよう、既定のキューで動かす。
		MaxAttempts: promoteMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// PromoteWorker は、昇格のジョブを実行する。バッファの、番組の始まりから終わり (前後のマージンを含む) までを、
// 続いたチャンクの並びごとに録画のセグメントに書き出し、録画を完了にする。
type PromoteWorker struct {
	river.WorkerDefaults[PromoteArgs]

	recorder *Recorder
}

var _ river.Worker[PromoteArgs] = new(PromoteWorker)

// NewPromoteWorker は PromoteWorker を返す。store は、このインスタンスにマウントされたプールとタイムシフトのディレクトリ。
// defaults は録画設定の全体の既定値、location はファイル名に使う表示のタイムゾーン。
func NewPromoteWorker(
	pool *pgxpool.Pool, clk clock.Clock, store *kago.Store, defaults assign.DefaultsFunc, location *time.Location, logger *slog.Logger,
) *PromoteWorker {
	return &PromoteWorker{
		WorkerDefaults: river.WorkerDefaults[PromoteArgs]{},
		recorder:       NewRecorder(pool, clk, nil, store, defaults, location, logger, nil),
	}
}

func (w *PromoteWorker) Timeout(*river.Job[PromoteArgs]) time.Duration {
	return promoteTimeout
}

func (w *PromoteWorker) Work(ctx context.Context, job *river.Job[PromoteArgs]) error {
	r := w.recorder
	logger := r.logger.With(slog.String("recording_id", job.Args.RecordingID.String()))
	reason, err := r.promote(ctx, logger, job.Args.RecordingID)
	switch {
	case errors.Is(err, errPromotionGone):
		return nil
	case err == nil:
		return nil
	case job.Attempt < job.MaxAttempts:
		return err
	}
	// 試し尽くしたら、録画を失敗にして、昇格の行を消す。チャンクを消さずに残し続けないためである。
	// 時間の上限を過ぎて ctx が切れていても記録できるよう、ctx の取り消しを引き継がない。
	logger.ErrorContext(ctx, "gave up promoting the timeshift buffer to a recording", slog.Any("error", err))
	failCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.opts.DatabaseTimeout)
	defer cancel()
	if failErr := r.failPromotion(failCtx, job.Args.RecordingID, reason); failErr != nil {
		return errors.Join(err, failErr)
	}

	return err
}

// ReapPromotionsInterval は、ジョブを失った昇格を片付ける間隔。
const ReapPromotionsInterval = time.Minute

// ReapPromotionsArgs は、ジョブを失った昇格を片付ける定期ジョブの引数。
type ReapPromotionsArgs struct{}

var _ river.JobArgs = new(ReapPromotionsArgs)

func (ReapPromotionsArgs) Kind() string { return "timeshift_promote_reap" }

// ReapPromotionsWorker は、終わっていない昇格のジョブがない昇格を、録画を storage_unavailable で失敗にして片付ける。
// 最後に試している間に nekonote が落ちると、River はジョブを Work を呼ばずに捨てるので、昇格の行が残り続け、
// 録画は録画中のまま、範囲のチャンクも破棄されなくなるためである。
type ReapPromotionsWorker struct {
	river.WorkerDefaults[ReapPromotionsArgs]

	recorder *Recorder
}

var _ river.Worker[ReapPromotionsArgs] = new(ReapPromotionsWorker)

// NewReapPromotionsWorker は ReapPromotionsWorker を返す。
func NewReapPromotionsWorker(pool *pgxpool.Pool, clk clock.Clock, logger *slog.Logger) *ReapPromotionsWorker {
	return &ReapPromotionsWorker{
		WorkerDefaults: river.WorkerDefaults[ReapPromotionsArgs]{},
		recorder:       NewRecorder(pool, clk, nil, kago.New(map[string]string{}, nil), nil, time.UTC, logger, nil),
	}
}

func (w *ReapPromotionsWorker) Work(ctx context.Context, _ *river.Job[ReapPromotionsArgs]) error {
	reaped, err := w.RunOnce(ctx)
	if recordErr := river.RecordOutput(ctx, map[string]int{"reaped": reaped}); recordErr != nil {
		return errors.Join(err, fmt.Errorf("failed to record the job output: %w", recordErr))
	}

	return err
}

// RunOnce は、終わっていない昇格のジョブがない昇格を片付け、片付けた数を返す。
func (w *ReapPromotionsWorker) RunOnce(ctx context.Context) (int, error) {
	r := w.recorder
	// River のテーブルは sqlc のスキーマにないので、直接引く。昇格の行とジョブは同じトランザクションで作るので、
	// 行があってジョブがないのは、ジョブを失ったときだけである。
	rows, err := r.pool.Query(ctx, `SELECT p.recording_id FROM timeshift_promotions p
		WHERE NOT EXISTS (
			SELECT 1 FROM river_job j
			WHERE j.kind = $1 AND j.args->>'recordingId' = p.recording_id::text
				AND j.state NOT IN ('completed', 'cancelled', 'discarded')
		)
		ORDER BY p.created_at`, new(PromoteArgs).Kind())
	if err != nil {
		return 0, fmt.Errorf("failed to list the timeshift promotions without a job: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, fmt.Errorf("failed to read the timeshift promotions: %w", err)
	}
	errs := []error{}
	for _, recordingID := range ids {
		if err := r.failPromotion(ctx, recordingID, FailureStorageUnavailable); err != nil {
			errs = append(errs, fmt.Errorf("recording %s: %w", recordingID, err))

			continue
		}
		r.logger.WarnContext(ctx, "failed the timeshift promotion whose job was lost", slog.String("recording_id", recordingID.String()))
	}

	return len(ids) - len(errs), errors.Join(errs...)
}

// ReapPromotionsPeriodicJob は、ジョブを失った昇格を ReapPromotionsInterval ごとに片付ける定期ジョブを返す。
func ReapPromotionsPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(ReapPromotionsInterval),
		func() (river.JobArgs, *river.InsertOpts) {
			return ReapPromotionsArgs{}, nil
		},
		&river.PeriodicJobOpts{ID: "timeshift_promote_reap", RunOnStart: true},
	)
}

// promote は、昇格する録画をバッファから書き出して完了にする。失敗したら、書いたファイルを消し、録画の失敗の理由も返す。
func (r *Recorder) promote(ctx context.Context, logger *slog.Logger, recordingID uuid.UUID) (FailureReason, error) {
	// ほかの nekonote がタイムシフトのディレクトリをマウントしていれば、そちらでやり直せるよう、エラーにする。
	if !r.store.HasTimeshiftDir() {
		return FailureStorageUnavailable, errors.New("the timeshift directory is not configured on this instance")
	}
	q := sqlc.New(r.pool)
	row, err := q.GetTimeshiftPromotion(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errPromotionGone
	}
	if err != nil {
		return FailureStorageUnavailable, fmt.Errorf("failed to get the timeshift promotion: %w", err)
	}
	promotion, recording := &row.TimeshiftPromotion, &row.Recording
	settings, err := r.defaults(ctx, r.pool)
	if err != nil {
		return FailureStorageUnavailable, fmt.Errorf("failed to get the recording settings: %w", err)
	}
	if len(r.store.PoolIDs()) == 0 {
		return FailureStorageUnavailable, errors.New("no storage pool is mounted on this instance")
	}
	pools, err := q.ListStoragePools(ctx)
	if err != nil {
		return FailureStorageUnavailable, fmt.Errorf("failed to list storage pools: %w", err)
	}
	runs, err := stream.NewTimeshiftSource(r.pool, r.store, 0).OpenRange(ctx, promotion.ServiceID, promotion.FromAt, promotion.ToAt)
	if err != nil {
		return FailureStorageUnavailable, fmt.Errorf("failed to open the timeshift buffer: %w", err)
	}
	if len(runs) == 0 {
		// 番組の時間のチャンクがもうないので、書き出せない。やり直しても変わらない。
		logger.WarnContext(ctx, "nothing to promote; the timeshift buffer has no chunk for the program")

		return FailureProgramNotFound, r.failPromotion(ctx, recordingID, FailureProgramNotFound)
	}

	// ファイル名は、予約の録画と同じく、番組の情報から付ける。
	program := &sqlc.GetReservationForRecordingRow{
		ID: uuid.Nil, Kind: "", ServiceID: recording.ServiceID, EventID: recording.EventID, Name: recording.Name, Description: recording.Description,
		StartAt: recording.StartAt, EndAt: recording.EndAt, Enabled: true, PlannedStartAt: nil, PlannedEndAt: nil, Settings: nil,
		ServiceName: row.ServiceName, ChannelType: row.ChannelType, Channel: row.Channel,
	}
	candidates := poolsFrom(pools, r.store.PoolIDs(), settings.PoolID)
	written := make([]*promotedSegment, 0, len(runs))
	removeAll := func() {
		for _, w := range written {
			r.removeBackfilled(logger, w.segment)
		}
	}
	for _, run := range runs {
		w, reason, err := r.promoteRun(ctx, logger, settings, candidates, program, run)
		if err != nil {
			removeAll()

			return reason, err
		}
		written = append(written, w)
	}
	if err := r.finishPromotion(ctx, logger, recording, promotion, settings, written); err != nil {
		removeAll()
		if errors.Is(err, errPromotionGone) {
			logger.InfoContext(ctx, "the timeshift promotion was finished or deleted elsewhere; discarding the written files")
		}

		return FailureStorageUnavailable, err
	}
	logger.InfoContext(ctx, "promoted the timeshift buffer to a recording", slog.Int("segments", len(written)))

	return "", nil
}

// promotedSegment は、バッファから書き出したセグメント。
type promotedSegment struct {
	segment *segment
	stats   *ts.Stats
	endedAt time.Time
}

// promoteRun は、続いたチャンクの並び run を、録画の新しいセグメントに書き出す。DB には記録しない。
func (r *Recorder) promoteRun(
	ctx context.Context, logger *slog.Logger, settings *domain.RecordingSettings, pools []*kago.Pool,
	program *sqlc.GetReservationForRecordingRow, run *stream.TimeshiftRun,
) (*promotedSegment, FailureReason, error) {
	estimated := uint64(max(run.To.Sub(run.From), 0)/time.Second) * estimatedBytesPerSecond
	seg, reason, err := r.newSegment(ctx, settings, pools, program, run.From, estimated)
	if err != nil {
		return nil, reason, err
	}
	copyErr := run.Write(ctx, io.MultiWriter(writeErrorWriter{w: seg.buffer}, seg.analyzer))
	closeErrs := seg.syncAndClose(ctx, logger)
	if err := errors.Join(append(closeErrs, copyErr)...); err != nil {
		r.removeBackfilled(logger, seg)

		return nil, FailureStorageUnavailable, fmt.Errorf("failed to write the timeshift buffer: %w", err)
	}

	return &promotedSegment{segment: seg, stats: seg.analyzer.Stats(), endedAt: *run.To}, "", nil
}

// finishPromotion は、書き出したセグメントを記録し、録画を完了にする。1 つのトランザクションで、昇格の行を消し、
// セグメントと途切れた区間を記録し、予約の録画と同じく、録画履歴に記録して完了を通知する (設計書 4.4、6.2)。
// 昇格の行がもうなければ、何も記録せずに errPromotionGone を返す。
func (r *Recorder) finishPromotion(
	ctx context.Context, logger *slog.Logger, recording *sqlc.Recording, promotion *sqlc.TimeshiftPromotion,
	settings *domain.RecordingSettings, written []*promotedSegment,
) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		deleted, err := q.DeleteTimeshiftPromotion(ctx, recording.ID)
		if err != nil {
			return fmt.Errorf("failed to delete the timeshift promotion: %w", err)
		}
		if deleted == 0 {
			return errPromotionGone
		}
		now := r.clock.Now()
		recordedUntil, err := r.savePromotedSegments(ctx, logger, q, recording, written, now)
		if err != nil {
			return err
		}
		state, err := r.completePromoted(ctx, q, recording.ID, promotion, recordedUntil, now)
		if err != nil {
			return err
		}

		return r.notifyPromotion(ctx, tx, recording.ID, func(payload map[string]any) error {
			payload["state"] = string(state)
			segments, err := q.ListRecordingSegments(ctx, recording.ID)
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

			return notification.EmitTo(ctx, tx, r.clock, domain.NotificationKindRecordingCompleted, payload, audienceOf(promotion.CreatedBy))
		})
	})
}

// savePromotedSegments は、書き出したセグメントを記録し、録画の集計に足す。録画した時間の終わり (最後のセグメントの終わり) を返す。
func (r *Recorder) savePromotedSegments(
	ctx context.Context, logger *slog.Logger, q *sqlc.Queries, recording *sqlc.Recording, written []*promotedSegment, now time.Time,
) (time.Time, error) {
	// 録り始めた日時は、書き出した最初のセグメントの始まりにする。セグメントを記録すると変えられないので、先に書く。
	err := q.SetRecordingRecordedFrom(ctx, sqlc.SetRecordingRecordedFromParams{At: written[0].segment.startedAt, Now: now, ID: recording.ID})
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to save the start of the recording: %w", err)
	}
	var recordedUntil time.Time
	for _, w := range written {
		if err := insertSegment(w.segment)(ctx, q, recording.ID); err != nil {
			return time.Time{}, err
		}
		if err := r.closeSegmentFile(logger, recording, w.segment, w.stats, w.endedAt)(ctx, q, recording.ID); err != nil {
			return time.Time{}, err
		}
		if err := addSegmentStats(ctx, q, recording.ID, w.segment, w.stats, w.endedAt); err != nil {
			return time.Time{}, err
		}
		recordedUntil = maxTime(recordedUntil, w.endedAt)
	}

	return recordedUntil, nil
}

// completePromoted は、記録したセグメントから途切れた区間を求めて録画を終え、正常に終わってドロップが閾値以下なら録画履歴に記録する。
// 番組の頭がもうバッファになければ冒頭の欠けを、バッファの穴はセグメントの間のすき間を、途切れた区間にする。
func (r *Recorder) completePromoted(
	ctx context.Context, q *sqlc.Queries, recordingID uuid.UUID, promotion *sqlc.TimeshiftPromotion, recordedUntil, now time.Time,
) (State, error) {
	files, err := q.ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return "", fmt.Errorf("failed to list the recording files: %w", err)
	}
	gaps, recorded := gapsOf(files, &promotion.FromAt, promotion.ToAt, true)
	if !recorded {
		return "", errors.New("no segment was written")
	}
	encoded, err := json.Marshal(gaps)
	if err != nil {
		return "", fmt.Errorf("failed to encode the gaps: %w", err)
	}
	if err := q.SetRecordingGaps(ctx, sqlc.SetRecordingGapsParams{Gaps: encoded, Now: now, ID: recordingID}); err != nil {
		return "", fmt.Errorf("failed to save the gaps: %w", err)
	}
	recording, err := q.GetRecording(ctx, recordingID)
	if err != nil {
		return "", fmt.Errorf("failed to get the recording: %w", err)
	}
	state := StateCompleted
	switch {
	case len(gaps) > 0:
		state = StatePartial
	case recording.Drops > 0:
		state = StateCompletedWithErrors
	}
	// 録画した時間の終わりは、書いたセグメントの終わりにする (設計書 5.5)。
	err = q.FinishRecording(ctx, sqlc.FinishRecordingParams{State: string(state), FailureReason: nil, Now: &recordedUntil, ID: recordingID})
	if err != nil {
		return "", fmt.Errorf("failed to finish the recording: %w", err)
	}
	if state == StatePartial || recording.Drops > MaxHistoryDrops {
		return state, nil
	}
	err = q.InsertRecordingHistory(ctx, sqlc.InsertRecordingHistoryParams{
		ID: id.New(), ServiceID: recording.ServiceID, TitleKey: rule.TitleKey(recording.Name), Name: recording.Name,
		StartAt: recording.StartAt, EndAt: recording.EndAt, Now: now,
	})
	if err != nil {
		return "", fmt.Errorf("failed to save the recording history: %w", err)
	}

	return state, nil
}

// failPromotion は、昇格を諦め、録画を reason で失敗にして、昇格の行を消し、失敗を通知する。
// 昇格の行がもうなければ、ほかで終えたので何もしない。
func (r *Recorder) failPromotion(ctx context.Context, recordingID uuid.UUID, reason FailureReason) error {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		row, err := q.GetTimeshiftPromotion(ctx, recordingID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errPromotionGone
		}
		if err != nil {
			return fmt.Errorf("failed to get the timeshift promotion: %w", err)
		}
		if _, err := q.DeleteTimeshiftPromotion(ctx, recordingID); err != nil {
			return fmt.Errorf("failed to delete the timeshift promotion: %w", err)
		}
		now := r.clock.Now()
		failure := string(reason)
		if err := q.FinishRecording(ctx, sqlc.FinishRecordingParams{State: string(StateFailed), FailureReason: &failure, Now: &now, ID: recordingID}); err != nil {
			return fmt.Errorf("failed to finish the recording: %w", err)
		}
		createdBy := row.TimeshiftPromotion.CreatedBy

		return r.notifyPromotion(ctx, tx, recordingID, func(payload map[string]any) error {
			payload["state"] = string(StateFailed)
			payload["reason"] = failure

			return notification.EmitTo(ctx, tx, r.clock, domain.NotificationKindRecordingFailed, payload, audienceOf(createdBy))
		})
	})
	if errors.Is(err, errPromotionGone) {
		return nil
	}

	return err
}

// notifyPromotion は、昇格した録画の通知の材料を作り、emit に渡す。予約がないので、予約の種類は載せず、もとになったルールは空にする。
func (r *Recorder) notifyPromotion(
	ctx context.Context, tx pgx.Tx, recordingID uuid.UUID, emit func(payload map[string]any) error,
) error {
	if _, err := notify.Publish(ctx, tx, r.clock, notify.RecordingsChannel); err != nil {
		return err
	}
	row, err := sqlc.New(tx).GetRecordingWithService(ctx, recordingID)
	if err != nil {
		return fmt.Errorf("failed to get the recording: %w", err)
	}
	payload := recordingPayload(row.Recording.ID, row.Recording.Name, row.ServiceName, row.Recording.StartAt, row.Recording.EndAt)
	payload["rules"] = []notification.ReservationRule{}

	return emit(payload)
}

// audienceOf は、昇格した録画の通知を受け取るユーザーを返す。手動の予約と同じく、昇格したユーザーにだけ届ける (設計書 6.2)。
// ユーザーを消していれば、全員に届ける。
func audienceOf(createdBy *uuid.UUID) *notification.Audience {
	if createdBy == nil {
		return nil
	}

	return &notification.Audience{UserIDs: []uuid.UUID{*createdBy}, ByRule: false}
}
