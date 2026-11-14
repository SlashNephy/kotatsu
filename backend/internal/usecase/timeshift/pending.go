package timeshift

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// databaseTimeout は、読み込みのループから DB に書くときに待つ上限。
const databaseTimeout = 2 * time.Second

// pendingWrites は、DB に届かずに記録できなかったチャンクと番組の書き込みを、順に持つ (#1073)。
// 読み込みのループは DB を待てないので、届くようになったら、見張りの goroutine が順に書く。
// 1 つでも残っている間は、後の書き込みも順を守って積む。チャンクを閉じる記録が、作った記録より先に書かれないようにするため。
// 積むのと残りを確かめるのは mu で守り、書くのは mu の外で行う。DB が詰まっても、読み込みのループを mu で待たせない。
type pendingWrites struct {
	mu     sync.Mutex
	writes []pendingWrite
	// flushing は、書いているところを 1 つにする。同じ書き込みを、2 つの goroutine で重ねて書かない。
	flushing sync.Mutex
	// assignment は、録っている割り振り。書く前に、ほかの mikan が取っていないかを、世代番号で確かめる。
	assignment *recording.Assignment
	// unreachable は、割り振りを確かめようとして DB に届かなかったかどうか。ログを、届かなくなったときと戻ったときに 1 度ずつ出す。
	unreachable atomic.Bool
}

// pendingWrite は、積んだ書き込み。
type pendingWrite struct {
	write func(ctx context.Context, q *sqlc.Queries) error
	// discard は、書かずに捨てるときの後片付け。チャンクを作った記録なら、DB に記録のないチャンクのファイルと索引を消す (#1722)。
	// 後片付けがなければ nil。
	discard func()
}

// newPendingWrites は、割り振り assignment を録る間の pendingWrites を返す。
func newPendingWrites(assignment *recording.Assignment) *pendingWrites {
	return &pendingWrites{mu: sync.Mutex{}, writes: nil, flushing: sync.Mutex{}, assignment: assignment, unreachable: atomic.Bool{}}
}

// discard は、積んだ書き込みを捨て、捨てた数を返す。捨てた書き込みの後片付けもする。
func (p *pendingWrites) discard() int {
	p.mu.Lock()
	writes := slices.Clone(p.writes)
	p.writes = p.writes[:0]
	p.mu.Unlock()
	for _, w := range writes {
		if w.discard != nil {
			w.discard()
		}
	}

	return len(writes)
}

// addIfWaiting は、積んだ書き込みが残っていれば、write を後ろに積んで true を返す。残っていなければ、積まずに false を返す。
func (p *pendingWrites) addIfWaiting(write pendingWrite) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.writes) == 0 {
		return false
	}
	p.writes = append(p.writes, write)

	return true
}

// add は、書き込みを積む。
func (p *pendingWrites) add(write pendingWrite) {
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

// flush は、積んだ書き込みを順に書く。1 つの書き込みは databaseTimeout で打ち切る。
// 失敗したら、そこから後を残して返す。書いている間に積まれたものも残す。書けた数を返す。
func (p *pendingWrites) flush(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	p.flushing.Lock()
	defer p.flushing.Unlock()
	p.mu.Lock()
	writes := slices.Clone(p.writes)
	p.mu.Unlock()
	q := sqlc.New(pool)
	done := 0
	var err error
	for _, write := range writes {
		writeCtx, cancel := context.WithTimeout(ctx, databaseTimeout)
		err = write.write(writeCtx, q)
		cancel()
		if err != nil {
			break
		}
		done++
	}
	p.mu.Lock()
	p.writes = slices.Delete(p.writes, 0, done)
	p.mu.Unlock()

	return done, err
}

// writeOrDefer は、write を DB に書く。DB に届かなければ、pending に積んで nil を返す。届かない間に積んだものが残っていれば、
// 順を守るため、書かずに積む。DB への接続が応答のないまま詰まっても、読み込みのループを長く止めないよう、databaseTimeout で打ち切る。
// discard は、積んだ書き込みを書かずに捨てるときの後片付けで、なければ nil。
func (r *Recorder) writeOrDefer(
	ctx context.Context, logger *slog.Logger, pending *pendingWrites, write func(ctx context.Context, q *sqlc.Queries) error, discard func(),
) error {
	deferred := pendingWrite{write: write, discard: discard}
	if pending.addIfWaiting(deferred) {
		return nil
	}
	writeCtx, cancel := context.WithTimeout(ctx, databaseTimeout)
	defer cancel()
	err := write(writeCtx, sqlc.New(r.pool))
	if err == nil || !database.IsUnavailable(err) {
		return err
	}
	logger.WarnContext(ctx, "the database is unreachable; keep recording and write the records later", slog.Any("error", err))
	pending.add(deferred)

	return nil
}

// flushPending は、積んだ書き込みを書く。書き終えたら、そのことをログに残す。
// 書く前に、割り振りがまだこの mikan のものかを確かめる。DB が戻った直後にほかの mikan が取っていたら、
// その mikan の記録を古い記録で上書きしないよう、書かずに捨てる (#1073)。
func (r *Recorder) flushPending(ctx context.Context, logger *slog.Logger, pending *pendingWrites) {
	if !pending.waiting() {
		return
	}
	checkCtx, cancel := context.WithTimeout(ctx, databaseTimeout)
	defer cancel()
	stream, err := sqlc.New(r.pool).GetTimeshiftAssignmentStream(checkCtx, pending.assignment.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows) || (err == nil && stream.Generation != pending.assignment.Generation):
		// 作ったチャンクの記録を捨てるときは、DB に記録のないチャンクのファイルと索引も消す。破棄のジョブも、DB にないファイルの探索も、
		// タイムシフトのディレクトリを見ないので、残すと誰にも消されない (#1722)。
		n := pending.discard()
		logger.WarnContext(ctx, "the timeshift assignment was taken or stopped; discarding the records deferred while the database was unreachable",
			slog.Int("writes", n))

		return
	case err != nil:
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
