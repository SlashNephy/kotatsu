package timeshift

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

const (
	// WatchNotifyChannel は、ライブ視聴が新しく始まったことを neko に知らせる変更通知のチャンネル。
	WatchNotifyChannel = "timeshift_watches"
	// WatchLease は、視聴が続いているとみなす時間 (付録 A)。最後に配信を要求してからこの間は、録り続ける。
	// kyusu の経路では、一時停止するとセグメントを読まなくなるので、一時停止の間も録れるよう、見直しの間隔より長くする。
	WatchLease = 2 * time.Minute
	// WatchKeep は、視聴が終わってから、録ったバッファを残す時間 (付録 A)。同じサービスに戻ったら、続けて遡れる。
	WatchKeep = 5 * time.Minute
	// watchRefresh は、視聴が続いていることを DB に書く間隔。配信の要求が何度来ても、これより短い間隔では書かない。
	watchRefresh = 15 * time.Second
	// watchSweepSize は、最後に書いた時刻の記録がこの数に達したら、古い記録を捨てる数。サービスの数より十分に大きくする。
	watchSweepSize = 1024
)

// WatchParser は、配信のパスの {id} から、サービス ID を返す。読めなければ false を返す。
type WatchParser func(id string) (int64, bool)

// Watches は、tenban で、ログインしたユーザーのライブ視聴が続いているサービスを記録する (設計書 5.7、#1408)。
// 記録するのはサービスと期限だけで、誰が見ているかは持たない。視聴している間だけ録る設定が切れていれば、何も書かない。
type Watches struct {
	pool    *pgxpool.Pool
	clock   clock.Clock
	logger  *slog.Logger
	refresh time.Duration

	mu sync.Mutex
	// last は、サービスごとの、最後に DB に書いた時刻。
	last map[int64]time.Time
}

// NewWatches は Watches を返す。
func NewWatches(pool *pgxpool.Pool, clk clock.Clock, logger *slog.Logger) *Watches {
	return &Watches{pool: pool, clock: clk, logger: logger, refresh: watchRefresh, mu: sync.Mutex{}, last: map[int64]time.Time{}}
}

// Track は、next の配信を流している間、サービスの視聴が続いていることを記録するハンドラーを返す。
// バッファへシークすると、ライブの配信は閉じてバッファの配信に替わるので、ライブとバッファの両方の配信に付ける。
// 署名付き URL の外部プレイヤーは遡れないので、数えない。
func (w *Watches) Track(parse WatchParser, next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		serviceID, ok := parse(r.PathValue("id"))
		if !ok || r.Method == http.MethodHead || auth.PrincipalFrom(r.Context()) == nil {
			next.ServeHTTP(rw, r)

			return
		}
		w.touch(r.Context(), serviceID)
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		go w.keepTouching(ctx, serviceID)
		next.ServeHTTP(rw, r)
	})
}

// keepTouching は、ctx が終わるまで、視聴が続いていることを書き続ける。TS の配信は 1 つのリクエストのまま何時間も続く。
func (w *Watches) keepTouching(ctx context.Context, serviceID int64) {
	ticker := time.NewTicker(w.refresh)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.touch(ctx, serviceID)
		}
	}
}

// touch は、前に書いてから refresh が過ぎていれば、視聴が続いていることを別の goroutine で書く。
// DB が詰まっても、配信を待たせないためである。書けなければ、次の要求で書き直す。
// HLS の要求はすぐ終わるので、要求が終わっても書き続けられるよう、ctx の取り消しは引き継がない。
func (w *Watches) touch(ctx context.Context, serviceID int64) {
	now := w.clock.Now()
	w.mu.Lock()
	if last, ok := w.last[serviceID]; ok && now.Sub(last) < w.refresh {
		w.mu.Unlock()

		return
	}
	// パスの ID は DB で確かめる前のものなので、存在しない ID を並べられても増え続けないよう、間隔を過ぎた記録を捨てる。
	if len(w.last) >= watchSweepSize {
		maps.DeleteFunc(w.last, func(_ int64, at time.Time) bool { return now.Sub(at) >= w.refresh })
	}
	w.last[serviceID] = now
	w.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), database.FallbackQueryTimeout)
		defer cancel()
		if err := w.write(ctx, serviceID, now); err != nil {
			w.mu.Lock()
			delete(w.last, serviceID)
			w.mu.Unlock()
			w.logger.WarnContext(ctx, "failed to record the live viewing", slog.Int64("service_id", serviceID), slog.Any("error", err))
		}
	}()
}

// write は、視聴が続いていることを書き、視聴が新しく始まったら neko に知らせる。
func (w *Watches) write(ctx context.Context, serviceID int64, now time.Time) error {
	started, err := sqlc.New(w.pool).TouchTimeshiftWatch(ctx, sqlc.TouchTimeshiftWatchParams{ServiceID: serviceID, WatchingUntil: now.Add(WatchLease), Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to touch the timeshift watch: %w", err)
	}
	if !started {
		return nil
	}
	if _, err := notify.Publish(ctx, w.pool, w.clock, WatchNotifyChannel); err != nil {
		return fmt.Errorf("failed to publish the timeshift watch: %w", err)
	}

	return nil
}

// syncWatched は、視聴の記録に合わせて、視聴している間だけ録るチャンネルを足し、消す。視聴が続いているサービスを返す。
func syncWatched(ctx context.Context, q *sqlc.Queries, now time.Time) ([]*Channel, error) {
	if err := q.SyncWatchedTimeshiftChannels(ctx, now); err != nil {
		return nil, fmt.Errorf("failed to sync the watched timeshift channels: %w", err)
	}
	keptAfter := now.Add(-WatchKeep)
	if err := q.DeleteUnwatchedTimeshiftChannels(ctx, keptAfter); err != nil {
		return nil, fmt.Errorf("failed to delete the unwatched timeshift channels: %w", err)
	}
	if err := q.DeleteStaleTimeshiftWatches(ctx, keptAfter); err != nil {
		return nil, fmt.Errorf("failed to delete the stale timeshift watches: %w", err)
	}
	rows, err := q.ListWatchedTimeshiftChannels(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("failed to list the watched timeshift channels: %w", err)
	}
	watched := make([]*Channel, 0, len(rows))
	for _, row := range rows {
		watched = append(watched, &Channel{ServiceID: row.ServiceID, ChannelType: row.ChannelType, Channel: row.Channel})
	}

	return watched, nil
}
