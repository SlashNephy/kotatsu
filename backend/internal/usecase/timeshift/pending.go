package timeshift

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// databaseTimeout は、読み込みのループから DB に書くときに待つ上限。
const databaseTimeout = 2 * time.Second

// pendingWrites は、DB に届かずに記録できなかったチャンクと番組の書き込みを、順に持つ (#1073)。
// 読み込みのループは DB を待てないので、届くようになったら、見張りの goroutine が順に書く。
// 1 つでも残っている間は、後の書き込みも順を守って積む。チャンクを閉じる記録が、作った記録より先に書かれないようにするため。
type pendingWrites struct {
	mu     sync.Mutex
	writes []func(ctx context.Context, q *sqlc.Queries) error
}

// add は、書き込みを積む。
func (p *pendingWrites) add(write func(ctx context.Context, q *sqlc.Queries) error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes = append(p.writes, write)
}

// waiting は、積んだ書き込みが残っているかどうかを返す。
func (p *pendingWrites) waiting() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.writes) > 0
}

// flush は、積んだ書き込みを順に書く。失敗したら、そこから後を残して返す。書けた数を返す。
func (p *pendingWrites) flush(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	q := sqlc.New(pool)
	for i, write := range p.writes {
		if err := write(ctx, q); err != nil {
			p.writes = p.writes[i:]

			return i, err
		}
	}
	n := len(p.writes)
	p.writes = p.writes[:0]

	return n, nil
}

// writeOrDefer は、write を DB に書く。DB に届かなければ、pending に積んで nil を返す。届かない間に積んだものが残っていれば、
// 順を守るため、書かずに積む。DB への接続が応答のないまま詰まっても、読み込みのループを長く止めないよう、databaseTimeout で打ち切る。
func (r *Recorder) writeOrDefer(
	ctx context.Context, logger *slog.Logger, pending *pendingWrites, write func(ctx context.Context, q *sqlc.Queries) error,
) error {
	if pending.waiting() {
		pending.add(write)

		return nil
	}
	writeCtx, cancel := context.WithTimeout(ctx, databaseTimeout)
	defer cancel()
	err := write(writeCtx, sqlc.New(r.pool))
	if err == nil || !database.IsUnavailable(err) {
		return err
	}
	logger.WarnContext(ctx, "the database is unreachable; keep recording and write the records later", slog.Any("error", err))
	pending.add(write)

	return nil
}

// flushPending は、積んだ書き込みを書く。書き終えたら、そのことをログに残す。
func (r *Recorder) flushPending(ctx context.Context, logger *slog.Logger, pending *pendingWrites) {
	if !pending.waiting() {
		return
	}
	n, err := pending.flush(ctx, r.pool)
	switch {
	case err == nil:
		logger.InfoContext(ctx, "wrote the records deferred while the database was unreachable", slog.Int("writes", n))
	case !database.IsUnavailable(err):
		logger.WarnContext(ctx, "failed to write the deferred records", slog.Any("error", err))
	}
}
