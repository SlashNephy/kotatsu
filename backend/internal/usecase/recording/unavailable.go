package recording

import (
	"context"
	"log/slog"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
)

// attemptTimeouts は、untilDatabase の 1 回の試みが、DatabaseTimeout の何倍まで DB を待つか。録画の完了のように、いくつも問い合わせる試みがある。
const attemptTimeouts = 4

// untilDatabase は、fn が DB に届かずに失敗する間、RetryInterval ごとにやり直す (#1073)。
// DB に届かないのでなければ、fn のエラーをそのまま返す。stop が終わったら、やり直さずに最後のエラーを返す。
// 1 回の fn には、stop から作った、attemptTimeouts 回分の DatabaseTimeout で切れる context を渡す。DB への接続が詰まっても、やり直せる。
// ログは、届かなくなったときと、届くようになったときに 1 度ずつ出す。
func (r *Recorder) untilDatabase(stop context.Context, logger *slog.Logger, operation string, fn func(ctx context.Context) error) error {
	var since time.Time
	for {
		attemptCtx, cancel := context.WithTimeout(stop, attemptTimeouts*r.opts.DatabaseTimeout)
		err := fn(attemptCtx)
		cancel()
		if err == nil || !database.IsUnavailable(err) || stop.Err() != nil {
			if err == nil && !since.IsZero() {
				logger.InfoContext(stop, "the database is reachable again", slog.String("operation", operation),
					slog.Duration("unreachable", r.clock.Now().Sub(since)))
			}

			return err
		}
		if since.IsZero() {
			since = r.clock.Now()
			logger.WarnContext(stop, "the database is unreachable; retrying until it is reachable", slog.String("operation", operation), slog.Any("error", err))
		}
		timer := time.NewTimer(r.opts.RetryInterval)
		select {
		case <-stop.Done():
			timer.Stop()

			return err
		case <-timer.C:
		}
	}
}
