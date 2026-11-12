package scramble

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// NotifyChannel は、ライブでスクランブルされたまま届くと決めたサービスが増えたことを知らせる変更通知のチャンネル (設計書 7.1、#1517)。
// 決めるのは kyusu と、ブラウザ内での変換のストリームを中継する tenban で、プレイヤーの購読を受ける tenban とは限らないので、DB を通して知らせる。
const NotifyChannel = "live_scrambled"

// reportTimeout は、決めたことを DB に書くのを待つ時間の上限。書けなくても、ストリームは閉じる。
const reportTimeout = 5 * time.Second

// Reporter は、ライブでスクランブルされたまま届くと決めたサービスを記録し、プレイヤーに知らせる (設計書 7.1)。
type Reporter struct {
	db     *pgxpool.Pool
	clock  clock.Clock
	logger *slog.Logger
}

// NewReporter は Reporter を返す。
func NewReporter(db *pgxpool.Pool, clk clock.Clock, logger *slog.Logger) *Reporter {
	return &Reporter{db: db, clock: clk, logger: logger}
}

// Report は、サービス serviceID がスクランブルされたまま届くと決めたことを記録し、版数を上げて知らせる。
func (r *Reporter) Report(ctx context.Context, serviceID int64) error {
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		err := sqlc.New(tx).UpsertLiveScrambleVerdict(ctx, sqlc.UpsertLiveScrambleVerdictParams{ServiceID: serviceID, DecidedAt: r.clock.Now()})
		if err != nil {
			return fmt.Errorf("failed to record the scrambled live stream: %w", err)
		}
		_, err = notify.Publish(ctx, tx, r.clock, NotifyChannel)

		return err
	})
	if err != nil {
		return fmt.Errorf("failed to report the scrambled live stream: %w", err)
	}

	return nil
}

// DecidedAt は、サービス serviceID が、最後にスクランブルされたまま届くと決めた日時を返す。決めたことがなければ found を false にする。
func (r *Reporter) DecidedAt(ctx context.Context, serviceID int64) (time.Time, bool, error) {
	decidedAt, err := sqlc.New(r.db).GetLiveScrambleVerdict(ctx, serviceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to get the scrambled live stream: %w", err)
	}

	return decidedAt, true, nil
}

// Watch は、ライブで流す TS src を、スクランブルされたまま届いているかを判定しながら読む io.Reader にする (設計書 7.1)。
// スクランブルされたままと決めたら、理由を画面に送るため Report で記録してから、読むのを ts.ErrScrambled で止める。
// 返す ScrambleDetector の Finish で、ストリームが判定の前に終わったときに、いちばん新しい窓だけで決められる。
// r が nil なら、src をそのまま返し、ScrambleDetector も nil を返す。
func (r *Reporter) Watch(ctx context.Context, serviceID int64, src io.Reader) (io.Reader, *ts.ScrambleDetector) {
	if r == nil {
		return src, nil
	}
	detector := ts.NewScrambleDetector(ts.ScrambleDetectorOptions{
		WindowPackets: 0, Windows: 0, Hold: 0, MinPartialPackets: 0, Now: r.clock.Now,
		OnScrambled: func() { r.ReportQuietly(ctx, serviceID) },
	})

	return io.TeeReader(src, detector), detector
}

// ReportQuietly は、Report を reportTimeout まで試し、書けなければログに残す。書けなくても、ストリームを閉じるのは止めない。
func (r *Reporter) ReportQuietly(ctx context.Context, serviceID int64) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), reportTimeout)
	defer cancel()
	r.logger.InfoContext(ctx, "the live stream is still scrambled; closing it", slog.Int64("service_id", serviceID))
	if err := r.Report(ctx, serviceID); err != nil {
		r.logger.WarnContext(ctx, "failed to report the scrambled live stream", slog.Int64("service_id", serviceID), slog.Any("error", err))
	}
}
