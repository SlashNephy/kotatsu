// Package jobprogress は、録画の後処理のジョブの進み具合を DB (job_progress) に書く (#1511、設計書 6.1)。
// 書いた進み具合は、ジョブの画面 (/admin/jobs) が、動いているジョブの今の試行のものだけを出す。
package jobprogress

import (
	"context"
	"log/slog"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ffprogress"
)

// retention は、書き直されなくなった進み具合を残す時間。River が完了したジョブを消す (既定は 24 時間) のに合わせる。
// River の表には外部キーを張れないので、終えたジョブの行は、後のジョブが書き始めるときにまとめて消す。
const retention = 24 * time.Hour

// Saver は、River のジョブ jobID の試行 attempt の進み具合を書く ffprogress.SaveFunc を返す。
// 返す前に、終えたジョブの古い進み具合を消す。消せなくても、進み具合は画面に出すだけなので、警告して続ける。
func Saver(ctx context.Context, db sqlc.DBTX, clk clock.Clock, logger *slog.Logger, jobID int64, attempt int) ffprogress.SaveFunc {
	q := sqlc.New(db)
	if _, err := q.DeleteJobProgressBefore(ctx, clk.Now().Add(-retention)); err != nil {
		logger.WarnContext(ctx, "failed to delete the old progress of jobs", slog.Any("error", err))
	}

	return func(ctx context.Context, s *ffprogress.Snapshot) error {
		var remaining *int64
		if s.Remaining != nil {
			remaining = new(s.Remaining.Milliseconds())
		}

		return q.UpsertJobProgress(ctx, sqlc.UpsertJobProgressParams{
			JobID: jobID, Attempt: int64(attempt), Ratio: s.Ratio, RemainingMs: remaining, Now: clk.Now(),
		})
	}
}
