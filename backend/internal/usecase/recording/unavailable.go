package recording

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
)

// attemptTimeouts は、untilDatabase の 1 回の試みが、DatabaseTimeout の何倍まで DB を待つか。録画の完了のように、いくつも問い合わせる試みがある。
const attemptTimeouts = 4

// DefaultPersistentErrorRetry は、DB に届いた後のエラーを、untilDatabase がやり直し続ける時間の上限の既定値 (#2195)。
// ローリング更新で古いバイナリと新しいスキーマが重なる間や、ロックの待ちの打ち切り、問い合わせの時間の上限のように、
// DB に届いていても、待てば通るエラーがある。録り終えた録画を、それで失敗にしない。上限を過ぎたら、エラーを返す。
const DefaultPersistentErrorRetry = 5 * time.Minute

// untilDatabase は、fn が DB のエラーで失敗する間、RetryInterval ごとにやり直す。
// DB に届かずに失敗する間は、届くまでやり直す (#1073)。DB に届いた後のエラー (PostgreSQL が返したエラー) は、
// PersistentErrorRetry の間だけやり直し、過ぎたら fn のエラーをそのまま返す (#2195)。DB のエラーでなければ、やり直さずに返す。
// stop が終わったら、やり直さずに最後のエラーを返す。
// 1 回の fn には、stop から作った、attemptTimeouts 回分の DatabaseTimeout で切れる context を渡す。DB への接続が詰まっても、やり直せる。
// ログは、失敗し始めたときと、通るようになったときに 1 度ずつ出す。
func (r *Recorder) untilDatabase(stop context.Context, logger *slog.Logger, operation string, fn func(ctx context.Context) error) error {
	var since time.Time
	// failingSince は、DB に届いた後のエラーが続き始めた時刻。上限は、時計を進めるテストでも過ぎるよう、実際の時間で数える。
	var failingSince time.Time
	for {
		attemptCtx, cancel := context.WithTimeout(stop, attemptTimeouts*r.opts.DatabaseTimeout)
		err := fn(attemptCtx)
		cancel()
		if err == nil || stop.Err() != nil || (!database.IsUnavailable(err) && !isDatabaseError(err)) {
			if err == nil && !since.IsZero() {
				logger.InfoContext(stop, "the database operation succeeded again", slog.String("operation", operation),
					slog.Duration("failing", r.clock.Now().Sub(since)))
			}

			return err
		}
		if !database.IsUnavailable(err) {
			if failingSince.IsZero() {
				failingSince = time.Now()
			} else if time.Since(failingSince) >= r.opts.PersistentErrorRetry {
				logger.ErrorContext(stop, "the database operation kept failing; giving up", slog.String("operation", operation), slog.Any("error", err))

				return err
			}
		} else {
			failingSince = time.Time{}
		}
		if since.IsZero() {
			since = r.clock.Now()
			logger.WarnContext(stop, "the database operation failed; retrying", slog.String("operation", operation), slog.Any("error", err))
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

// isDatabaseError は、err が、PostgreSQL が返したエラーかどうかを返す。
func isDatabaseError(err error) bool {
	_, ok := errors.AsType[*pgconn.PgError](err)

	return ok
}
