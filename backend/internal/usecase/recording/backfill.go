package recording

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

// backfillDelay は、録画のストリームを開いてから、タイムシフトのバッファの書いている途中のチャンクの大きさを測るまで待つ時間。
// 写した分の終わりが、ライブのストリームで書き始めた位置を確かに越えるようにする。重なった区間は、再生とセグメントをつなぐときに
// 切り詰めるが、つなぐときに重なりを探す範囲 (ts.DefaultJoinWindow、約 25 秒) より長く重なると、つなげないので、短くする。
const backfillDelay = 5 * time.Second

// backfill は、遅れて始めた録画に、予定の開始からの分を、タイムシフトのバッファから写す処理 (#826、設計書 5.7)。
type backfill struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// wait は、写し終えるまで待つ。b が nil なら、何もしない。
func (b *backfill) wait() {
	if b == nil {
		return
	}
	<-b.done
}

// stop は、写すのをやめさせ、やめるまで待つ。書きかけのセグメントは、写す処理が消す。b が nil なら、何もしない。
func (b *backfill) stop() {
	if b == nil {
		return
	}
	b.cancel()
	<-b.done
}

// maybeStartBackfill は、この mikan が録り始めた録画の、最初のセグメントを書き始めたとき、写す処理を始める。
// 落ちた mikan の続きや引き継ぎでは、前の mikan が冒頭を書いているので写さない。DB に届かずに録画を記録できていなければ、
// セグメントの記録を順に書けないので写さない。
func (r *Recorder) maybeStartBackfill(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, tracker *endTracker, recording *sqlc.Recording,
	reservation *sqlc.GetReservationForRecordingRow, created int, pendingStart bool, liveStart time.Time,
) {
	if created > 0 || assignment.HandedOver || pendingStart || tracker.backfill != nil {
		return
	}
	tracker.backfill = r.startBackfill(ctx, logger, assignment, *recording, *reservation, liveStart)
}

// startBackfill は、録画の最初のセグメントを liveStart に書き始めたとき、予定の開始からの分を、タイムシフトのバッファから
// 録画のセグメントに写し始める。ライブのストリームで書くのを遅らせないよう、別のゴルーチンで写す。
// タイムシフトのディレクトリを設定していなければ、写さないので nil を返す。
func (r *Recorder) startBackfill(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, recording sqlc.Recording,
	reservation sqlc.GetReservationForRecordingRow, liveStart time.Time,
) *backfill {
	if !r.store.HasTimeshiftDir() {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	b := &backfill{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(b.done)
		defer cancel()
		r.runBackfill(ctx, logger, assignment, &recording, &reservation, liveStart)
	}()

	return b
}

// runBackfill は、最初のセグメントが予定の開始より LateStartTolerance を超えて遅れて始まったら、予定の開始から今までのバッファを、
// 続いたチャンクの並びごとに、録画のセグメントに写す。並びの間の途切れた区間は、セグメントの間のすき間として、録画の途切れた区間になる。
// 写せなくても録画は続けるので、エラーはログに残すだけにする。
func (r *Recorder) runBackfill(
	ctx context.Context, logger *slog.Logger, assignment *Assignment, recording *sqlc.Recording,
	reservation *sqlc.GetReservationForRecordingRow, liveStart time.Time,
) {
	planned, found, err := r.plannedStart(ctx, assignment, recording.ID)
	if err != nil {
		logger.WarnContext(ctx, "failed to get the planned start; not backfilling from the timeshift buffer", slog.Any("error", err))

		return
	}
	if !found || !liveStart.After(planned.Add(LateStartTolerance)) {
		return
	}
	// 書いている途中のチャンクが、ライブのストリームで書き始めた位置を越えるまで待つ。
	ticker := time.NewTicker(r.opts.Tick)
	defer ticker.Stop()
	for r.clock.Now().Before(liveStart.Add(backfillDelay)) {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
	snapshotAt := r.clock.Now()
	runs, err := stream.NewTimeshiftSource(r.pool, r.store, 0).OpenRuns(ctx, reservation.ServiceID, planned)
	switch {
	case errors.Is(err, stream.ErrTimeshiftNotFound), errors.Is(err, stream.ErrNotMounted):
		return
	case err != nil:
		logger.WarnContext(ctx, "failed to open the timeshift buffer; not backfilling", slog.Any("error", err))

		return
	}
	settings, err := r.settings(ctx, reservation)
	if err != nil {
		logger.WarnContext(ctx, "failed to get the recording settings; not backfilling", slog.Any("error", err))

		return
	}
	rows, err := sqlc.New(r.pool).ListStoragePools(ctx)
	if err != nil {
		logger.WarnContext(ctx, "failed to list storage pools; not backfilling", slog.Any("error", err))

		return
	}
	pools := poolsFrom(rows, r.store.PoolIDs(), settings.PoolID)

	earliest := liveStart
	for _, run := range runs {
		// ライブのストリームで書き始めた後に始まった並びは、ライブのセグメントに入っている。
		if !run.From.Before(liveStart) {
			break
		}
		end := snapshotAt
		if run.To != nil {
			end = *run.To
		}
		if err := r.backfillRun(ctx, logger, recording, reservation, settings, pools, run, end); err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.WarnContext(ctx, "failed to backfill from the timeshift buffer", slog.Time("from", run.From), slog.Any("error", err))

			continue
		}
		logger.InfoContext(ctx, "backfilled from the timeshift buffer", slog.Time("from", run.From), slog.Time("to", end))
		earliest = minTime(earliest, run.From)
	}
	if earliest.Before(liveStart) {
		dbCtx, cancel := context.WithTimeout(ctx, r.opts.DatabaseTimeout)
		defer cancel()
		err := sqlc.New(r.pool).MoveRecordingRecordedFromBack(dbCtx, sqlc.MoveRecordingRecordedFromBackParams{At: earliest, Now: r.clock.Now(), ID: recording.ID})
		if err != nil {
			logger.WarnContext(ctx, "failed to save the start of the backfilled recording", slog.Any("error", err))
		}
	}
}

// backfillRun は、続いたチャンクの並び run を、録画の新しいセグメントに写し、end までを表すセグメントとして記録する。
// 写し終える前にやめるか、失敗したら、書きかけのセグメントを、ファイルと記録ごと消す。
func (r *Recorder) backfillRun(
	ctx context.Context, logger *slog.Logger, recording *sqlc.Recording, reservation *sqlc.GetReservationForRecordingRow,
	settings *domain.RecordingSettings, pools []*kago.Pool, run *stream.TimeshiftRun, end time.Time,
) error {
	estimated := uint64(max(end.Sub(run.From), 0)/time.Second) * estimatedBytesPerSecond
	seg, _, err := r.newSegment(ctx, settings, pools, reservation, run.From, estimated)
	if err != nil {
		return err
	}
	q := sqlc.New(r.pool)
	dbCtx, cancel := context.WithTimeout(ctx, r.opts.DatabaseTimeout)
	err = q.InsertRecordingFile(dbCtx, segmentParams(seg, recording.ID))
	cancel()
	if err != nil {
		seg.closeFiles()
		r.removeBackfilled(ctx, logger, recording.ID, seg)

		return fmt.Errorf("failed to save the recording file: %w", err)
	}
	copyErr := run.Write(ctx, io.MultiWriter(writeErrorWriter{w: seg.buffer}, seg.analyzer))
	closeErrs := seg.syncAndClose(ctx, logger)
	if err := errors.Join(append(closeErrs, copyErr)...); err != nil {
		r.removeBackfilled(ctx, logger, recording.ID, seg)

		return err
	}
	stats := seg.analyzer.Stats()
	dbCtx, cancel = context.WithTimeout(ctx, r.opts.DatabaseTimeout)
	defer cancel()
	if err := r.closeSegmentFile(logger, recording, seg, stats, end)(dbCtx, q, recording.ID); err != nil {
		r.removeBackfilled(ctx, logger, recording.ID, seg)

		return err
	}
	// 書いているセグメントの様子 (#829) は、ライブのセグメントのものなので、0 に戻さない。
	if err := addSegmentStats(dbCtx, q, recording.ID, seg, stats, end); err != nil {
		logger.WarnContext(ctx, "failed to save the stats of the backfilled segment", slog.Any("error", err))
	}

	return nil
}

// removeBackfilled は、写すのをやめたセグメントの記録と、ファイルと索引を消す。消せなくても、ログに残すだけにする。
// 記録が残ると、録画の途切れた区間を数え違えるので、写すのをやめた後でも消す。
func (r *Recorder) removeBackfilled(ctx context.Context, logger *slog.Logger, recordingID uuid.UUID, seg *segment) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.opts.DatabaseTimeout)
	defer cancel()
	if err := sqlc.New(r.pool).DeleteRecordingFiles(ctx, sqlc.DeleteRecordingFilesParams{RecordingID: recordingID, Ids: []uuid.UUID{seg.fileID}}); err != nil {
		logger.WarnContext(ctx, "failed to delete the record of the backfilled segment", slog.Any("error", err))
	}
	if err := r.store.Remove(seg.poolID, seg.path, seg.fileID); err != nil {
		logger.WarnContext(ctx, "failed to remove the backfilled segment", slog.String("path", seg.path), slog.Any("error", err))
	}
}
