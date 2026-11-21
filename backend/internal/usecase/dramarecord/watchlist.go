package dramarecord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// WatchlistRefreshInterval は、画面を開いたときに、ウォッチリストを問い合わせ直すまでの間隔 (付録 A)。
	// Simkl は、タイマーで問い合わせ続けず、利用者の操作をきっかけに、15〜30 分に 1 回までにするよう求めている。
	WatchlistRefreshInterval = 30 * time.Minute
	// watchlistRetryInterval は、取り込めなかったときに、問い合わせ直すまでの間隔。
	watchlistRetryInterval = time.Hour
	// maxRetryAfter は、記録先が 429 で示した待ち時間の上限。
	maxRetryAfter = time.Hour
	// watchlistTimeout は、ウォッチリストを取り込むジョブの時間の上限。
	watchlistTimeout = 5 * time.Minute
	// WatchlistNotifyChannel は、ウォッチリストを取り込んで、見ている作品が変わったことを知らせるチャンネル。
	// ドラマの視聴管理の画面が、読み直すのに使う。
	WatchlistNotifyChannel = "kotatsu_drama_watchlist"
	// 見ている作品の状態 (drama_watchlist_entries の status。dramalibrary.StatusWatching と StatusWannaWatch と同じ)。
	entryWatching   = "watching"
	entryWannaWatch = "wanna_watch"
)

// entryStatus は、Simkl のウォッチリストの状態を、見ている作品の状態にする。見ている作品にしない状態なら空を返す。
// 「見ている」は「見ている」に、「見たい」(plantowatch) は「見たい」にする。中断 (hold)、見た、やめた作品は、見ている作品にしない。
func entryStatus(status string) string {
	switch status {
	case StatusWatching:
		return entryWatching
	case StatusPlanToWatch:
		return entryWannaWatch
	default:
		return ""
	}
}

// WatchlistArgs は、ユーザーのウォッチリストを取り込むジョブの引数。
type WatchlistArgs struct {
	UserID      uuid.UUID `json:"userId"`
	Destination string    `json:"destination"`
}

var _ river.JobArgsWithInsertOpts = new(WatchlistArgs)

func (WatchlistArgs) Kind() string { return "drama_watchlist_import" }

// InsertOpts は、同じユーザーと記録先のジョブを、終わるまでは 1 つにする。取り込めなかったら、River では送り直さず、
// 問い合わせ直す日時の後に画面を開いたときに任せる。
func (WatchlistArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable, rivertype.JobStateRunning, rivertype.JobStateScheduled,
	}}}
}

// WatchlistWorker は、ウォッチリストを取り込むジョブを実行する。
type WatchlistWorker struct {
	river.WorkerDefaults[WatchlistArgs]

	service *Service
	db      Database
}

var _ river.Worker[WatchlistArgs] = new(WatchlistWorker)

// NewWatchlistWorker は WatchlistWorker を作る。
func NewWatchlistWorker(service *Service, db Database) *WatchlistWorker {
	return &WatchlistWorker{WorkerDefaults: river.WorkerDefaults[WatchlistArgs]{}, service: service, db: db}
}

// Timeout は、ウォッチリストを取り込むジョブの時間の上限を返す。
func (w *WatchlistWorker) Timeout(*river.Job[WatchlistArgs]) time.Duration {
	return watchlistTimeout
}

func (w *WatchlistWorker) Work(ctx context.Context, job *river.Job[WatchlistArgs]) error {
	return w.service.ImportWatchlist(ctx, w.db, job.Args.UserID, job.Args.Destination)
}

// EnqueueWatchlistImport は、ユーザー userID の記録先 name のウォッチリストを取り込むジョブを tx で入れる。inserter が nil なら何もしない。
func EnqueueWatchlistImport(ctx context.Context, tx pgx.Tx, inserter anime.Inserter, userID uuid.UUID, name string) error {
	if inserter == nil {
		return nil
	}
	if _, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{
		{Args: WatchlistArgs{UserID: userID, Destination: name}, InsertOpts: nil},
	}); err != nil {
		return fmt.Errorf("failed to enqueue the drama watchlist import: %w", err)
	}

	return nil
}

// RequestWatchlistImport は、ユーザー userID の連携のうち、問い合わせる時期が来たものの、ウォッチリストを取り込むジョブを入れる。
// ドラマの視聴管理の画面を開いたときと、利用者が取り込み直しを押したとき (force) に呼ぶ。Simkl はタイマーで問い合わせ続けることを
// 断っているので、定期のジョブでは取り込まない。force でなければ、前に問い合わせてから 30 分たった連携だけを取り込む。
// 取り込めなかった連携は、force でも、問い合わせ直す日時 (429 の Retry-After を含む) まで待つ。ジョブを入れたら true を返す。
// 管理者が TMDB との連携を無効にしていれば、何もしない。
func (s *Service) RequestWatchlistImport(ctx context.Context, db Database, inserter anime.Inserter, userID uuid.UUID, force bool) (bool, error) {
	system, err := settings.Load(ctx, db)
	if err != nil {
		return false, err
	}
	if !system.DramaLinkingEnabled || inserter == nil {
		return false, nil
	}
	enqueued := false
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		rows, err := q.ListDramaRecordConnections(ctx, userID)
		if err != nil {
			return fmt.Errorf("failed to list the drama record connections: %w", err)
		}
		now := s.clock.Now()
		for i := range rows {
			if !watchlistDue(&rows[i], now, force) {
				continue
			}
			if _, err := q.ClaimDramaWatchlistImport(ctx, sqlc.ClaimDramaWatchlistImportParams{
				Now: &now, UserID: userID, Destination: rows[i].Destination,
			}); err != nil {
				return fmt.Errorf("failed to claim the drama watchlist import: %w", err)
			}
			if err := EnqueueWatchlistImport(ctx, tx, inserter, userID, rows[i].Destination); err != nil {
				return err
			}
			enqueued = true
		}

		return nil
	})

	return enqueued, err
}

// watchlistDue は、連携 row のウォッチリストを、今 now に問い合わせてよいかを返す。
func watchlistDue(row *sqlc.DramaRecordConnection, now time.Time, force bool) bool {
	if row.ReauthorizationRequired {
		return false
	}
	if row.WatchlistRetryAt != nil && now.Before(*row.WatchlistRetryAt) {
		return false
	}
	if force || row.WatchlistCheckedAt == nil {
		return true
	}

	return now.Sub(*row.WatchlistCheckedAt) >= WatchlistRefreshInterval
}

// ImportWatchlist は、ユーザー userID の記録先 name のウォッチリストを取り込み、見ている作品に写す。
// まず /sync/activities で、前に取り込んでから変わったかを確かめ、変わっていなければウォッチリストを引かない (Simkl の求める手順)。
// 初めてならすべてを、そうでなければ前に取り込んだ後に変わった作品だけを引き、作品を外したことが示されたら、すべての作品の ID を
// 引いて、なくなった作品を見分ける。取り込めなかったら、前に取り込んだ状態を残し、理由と問い合わせ直す日時を連携に残す。
func (s *Service) ImportWatchlist(ctx context.Context, db Database, userID uuid.UUID, name string) error {
	row, err := sqlc.New(db).GetDramaRecordConnection(ctx, sqlc.GetDramaRecordConnectionParams{UserID: userID, Destination: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get the drama record connection: %w", err)
	}
	if err := s.importWatchlist(ctx, db, &row); err != nil {
		if ctx.Err() != nil {
			return err
		}
		retryAt := s.clock.Now().Add(watchlistRetryInterval)
		if limited := (*RateLimitError)(nil); errors.As(err, &limited) && limited.RetryAfter > 0 {
			retryAt = s.clock.Now().Add(min(limited.RetryAfter, maxRetryAfter))
		}
		reason := failureReason(err)
		if failErr := sqlc.New(db).FailDramaWatchlistImport(context.WithoutCancel(ctx), sqlc.FailDramaWatchlistImportParams{
			Error: &reason, RetryAt: &retryAt, UserID: userID, Destination: name,
		}); failErr != nil {
			return fmt.Errorf("failed to save the drama watchlist import error: %w", failErr)
		}
		s.logger.WarnContext(ctx, "failed to import the drama watchlist", slog.String("destination", name), slog.Any("error", err))

		return nil
	}

	return nil
}

func (s *Service) importWatchlist(ctx context.Context, db Database, row *sqlc.DramaRecordConnection) error {
	c, err := s.open(ctx, db, row.UserID, row.Destination)
	if err != nil {
		return err
	}
	var activities *Activities
	if err := c.do(ctx, func(ctx context.Context, access string) error {
		activities, err = c.destination.Activities(ctx, c.app, access)

		return err
	}); err != nil {
		return err
	}
	previous := ""
	if row.WatchlistActivity != nil {
		previous = *row.WatchlistActivity
	}
	changed := previous == "" || activities.All != previous
	var items []*WatchlistItem
	var current []int64
	if changed {
		if err := c.do(ctx, func(ctx context.Context, access string) error {
			items, err = c.destination.Watchlist(ctx, c.app, access, previous)

			return err
		}); err != nil {
			return err
		}
		removed := previous == "" || row.WatchlistRemovedActivity == nil || activities.RemovedFromList != *row.WatchlistRemovedActivity
		// 初めての取り込みは、すべての作品を引いたので、その一覧で、なくなった作品を見分けられる。
		switch {
		case previous == "":
			current = make([]int64, 0, len(items))
			for _, item := range items {
				current = append(current, item.SimklID)
			}
		case removed && activities.RemovedFromList != "":
			if err := c.do(ctx, func(ctx context.Context, access string) error {
				current, err = c.destination.WatchlistIDs(ctx, c.app, access)

				return err
			}); err != nil {
				return err
			}
		}
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		changes, err := s.mergeWatchlist(ctx, tx, row, items, current)
		if err != nil {
			return err
		}
		applied, err := s.applyWatchlist(ctx, tx, row.UserID, row.Destination)
		if err != nil {
			return err
		}
		if err := sqlc.New(tx).FinishDramaWatchlistImport(ctx, sqlc.FinishDramaWatchlistImportParams{
			Activity: &activities.All, RemovedActivity: &activities.RemovedFromList, Now: new(s.clock.Now()),
			UserID: row.UserID, Destination: row.Destination,
		}); err != nil {
			return fmt.Errorf("failed to finish the drama watchlist import: %w", err)
		}
		if changes+applied == 0 {
			return nil
		}
		_, err = notify.Publish(ctx, tx, s.clock, WatchlistNotifyChannel)

		return err
	})
}

// mergeWatchlist は、引いた作品 items の状態を、取り込んだ状態 (drama_watchlist_imports) に写し、見ている作品から外すものを外す。
// current が nil でなければ、ウォッチリストにある作品の ID のすべてで、ない作品はウォッチリストから外したものとする。
// 見ている作品に足すのは、applyWatchlist に任せる。見ている作品を変えた数を返す。
func (s *Service) mergeWatchlist(ctx context.Context, tx pgx.Tx, row *sqlc.DramaRecordConnection, items []*WatchlistItem, current []int64) (int, error) {
	changes, err := s.mergeItems(ctx, tx, row, items)
	if err != nil || current == nil {
		return changes, err
	}
	removed, err := s.mergeRemovals(ctx, tx, row, current)

	return changes + removed, err
}

// mergeItems は、引いた作品 items の状態を、取り込んだ状態に写す。変わった作品の、前に写した見ている作品を外し、外した数を返す。
func (s *Service) mergeItems(ctx context.Context, tx pgx.Tx, row *sqlc.DramaRecordConnection, items []*WatchlistItem) (int, error) {
	q := sqlc.New(tx)
	now := s.clock.Now()
	changes := 0
	for _, item := range items {
		if item.TMDBID <= 0 {
			// TMDB の ID のない作品は、Kotatsu の作品と結びつけられない。
			continue
		}
		previous, err := q.GetDramaWatchlistImport(ctx, sqlc.GetDramaWatchlistImportParams{
			UserID: row.UserID, Destination: row.Destination, TmdbSeriesID: item.TMDBID,
		})
		exists := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("failed to get the drama watchlist import: %w", err)
		}
		if exists && previous.Status == item.Status {
			// 状態が変わっていなければ、見ている作品は変えない (Kotatsu で外した作品を足し直さない)。ID だけを埋める。
			if err := q.UpsertDramaWatchlistImport(ctx, sqlc.UpsertDramaWatchlistImportParams{
				UserID: row.UserID, Destination: row.Destination, TmdbSeriesID: item.TMDBID, SimklID: &item.SimklID,
				Status: item.Status, Applied: previous.Applied, Now: now,
			}); err != nil {
				return 0, fmt.Errorf("failed to save the drama watchlist import: %w", err)
			}

			continue
		}
		if exists {
			n, err := s.releaseEntry(ctx, q, row.UserID, &previous)
			if err != nil {
				return 0, err
			}
			changes += n
		}
		if err := q.UpsertDramaWatchlistImport(ctx, sqlc.UpsertDramaWatchlistImportParams{
			UserID: row.UserID, Destination: row.Destination, TmdbSeriesID: item.TMDBID, SimklID: &item.SimklID,
			Status: item.Status, Applied: false, Now: now,
		}); err != nil {
			return 0, fmt.Errorf("failed to save the drama watchlist import: %w", err)
		}
	}

	return changes, nil
}

// mergeRemovals は、ウォッチリストにある作品の ID のすべて current にない作品を、ウォッチリストから外したものとし、取り込んだ状態を消す。
// 前に写した見ている作品を外し、外した数を返す。
func (s *Service) mergeRemovals(ctx context.Context, tx pgx.Tx, row *sqlc.DramaRecordConnection, current []int64) (int, error) {
	q := sqlc.New(tx)
	changes := 0
	known, err := q.ListDramaWatchlistImportSimklIDs(ctx, sqlc.ListDramaWatchlistImportSimklIDsParams{UserID: row.UserID, Destination: row.Destination})
	if err != nil {
		return 0, fmt.Errorf("failed to list the drama watchlist imports: %w", err)
	}
	for _, k := range known {
		if k.SimklID == nil || slices.Contains(current, *k.SimklID) {
			continue
		}
		previous, err := q.GetDramaWatchlistImport(ctx, sqlc.GetDramaWatchlistImportParams{
			UserID: row.UserID, Destination: row.Destination, TmdbSeriesID: k.TmdbSeriesID,
		})
		if err != nil {
			return 0, fmt.Errorf("failed to get the drama watchlist import: %w", err)
		}
		n, err := s.releaseEntry(ctx, q, row.UserID, &previous)
		if err != nil {
			return 0, err
		}
		changes += n
		if err := q.DeleteDramaWatchlistImport(ctx, sqlc.DeleteDramaWatchlistImportParams{
			UserID: row.UserID, Destination: row.Destination, TmdbSeriesID: k.TmdbSeriesID,
		}); err != nil {
			return 0, fmt.Errorf("failed to delete the drama watchlist import: %w", err)
		}
	}

	return changes, nil
}

// releaseEntry は、取り込んだ状態 previous から写した見ている作品を、Simkl で状態が変わったか外したので、外す。
// 写した後に Kotatsu で状態を変えた作品と、Kotatsu で先に足していた作品 (写していない作品) は外さない。外した数を返す。
func (s *Service) releaseEntry(ctx context.Context, q *sqlc.Queries, userID uuid.UUID, previous *sqlc.DramaWatchlistImport) (int, error) {
	status := entryStatus(previous.Status)
	if !previous.Applied || status == "" {
		return 0, nil
	}
	n, err := q.DeleteDramaWatchlistEntryIfStatus(ctx, sqlc.DeleteDramaWatchlistEntryIfStatusParams{
		UserID: userID, TmdbSeriesID: previous.TmdbSeriesID, Status: status,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to remove the drama from the watchlist: %w", err)
	}

	return int(n), nil
}

// applyWatchlist は、まだ写していない作品のうち、番組か録画に結びついた作品を、見ている作品に足すか、状態を変える。
// 結びつきのない作品は、名前が分からず、放送も録画も並べられないので、結びつくまで写さない (次の取り込みで確かめ直す)。
// 見ている作品にしない状態 (中断、見た、やめた) の作品は、写したことにする。足すか変えた数を返す。
func (s *Service) applyWatchlist(ctx context.Context, tx pgx.Tx, userID uuid.UUID, name string) (int, error) {
	q := sqlc.New(tx)
	pending, err := q.ListPendingDramaWatchlistImports(ctx, sqlc.ListPendingDramaWatchlistImportsParams{UserID: userID, Destination: name})
	if err != nil {
		return 0, fmt.Errorf("failed to list the pending drama watchlist imports: %w", err)
	}
	now := s.clock.Now()
	applied := 0
	for _, item := range pending {
		status := entryStatus(item.Status)
		if status != "" {
			if !item.Linked {
				continue
			}
			if err := q.UpsertDramaWatchlistEntry(ctx, sqlc.UpsertDramaWatchlistEntryParams{
				UserID: userID, TmdbSeriesID: item.TmdbSeriesID, Status: status, Now: now,
			}); err != nil {
				return 0, fmt.Errorf("failed to save the drama watchlist: %w", err)
			}
			applied++
		}
		if err := q.MarkDramaWatchlistImportApplied(ctx, sqlc.MarkDramaWatchlistImportAppliedParams{
			UserID: userID, Destination: name, TmdbSeriesID: item.TmdbSeriesID,
		}); err != nil {
			return 0, fmt.Errorf("failed to mark the drama watchlist import: %w", err)
		}
	}

	return applied, nil
}
