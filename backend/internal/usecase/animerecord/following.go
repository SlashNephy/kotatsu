package animerecord

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// FollowingNotifyChannel は、フォロー中のユーザーの視聴状況を問い合わせ終えたことを知らせる変更通知のチャンネル (#1372)。
	FollowingNotifyChannel = "anime_following"
	// FollowingRefreshInterval は、取得した視聴状況を問い合わせ直すまでの間隔 (付録 A)。見たいリストの取り込みと同じ長さにする。
	FollowingRefreshInterval = WishlistRefreshInterval
	// FollowingRetention は、取得した視聴状況を持っておく期間 (付録 A)。表示に要る分だけを、短い間だけ持つ。
	FollowingRetention = 24 * time.Hour
	// followingTimeout は、視聴状況を取得するジョブの時間の上限。見たいリストの取り込みの間は、順番を待つことがある。
	followingTimeout = wishlistTimeout
	// maxFollowingPages と maxFollowingViewings は、記録先ごとに引くページの数と人数の上限。
	maxFollowingPages    = 20
	maxFollowingViewings = 1000
)

// フォロー中のユーザーの、作品の状態 (anime_following_viewings の status)。
const (
	FollowingWatched      = "watched"
	FollowingWatching     = "watching"
	FollowingRepeating    = "repeating"
	FollowingOnHold       = "on_hold"
	FollowingStopWatching = "stop_watching"
	FollowingWannaWatch   = "wanna_watch"
)

// followingRank は、状態の順を返す。1 人が 1 つの TID の複数の作品に状態を付けていたら、順の早い状態を使い、画面にもこの順に並べる。
func followingRank(status string) int {
	return slices.Index([]string{
		FollowingWatched, FollowingWatching, FollowingRepeating, FollowingOnHold, FollowingStopWatching, FollowingWannaWatch,
	}, status)
}

// FollowingViewing は、記録先でフォローしているユーザーの、作品の視聴状況。
type FollowingViewing struct {
	Destination string
	// Username は、記録先でのユーザー名。Name は、画面に出す名前。
	Username string
	Name     string
	// AvatarURL は、記録先が示したアバターの URL。なければ空。
	AvatarURL string
	// Status は、FollowingWatched など。
	Status string
	// Progress は見終えた話数、EpisodesCount は全話数。分からなければ nil。
	Progress      *int32
	EpisodesCount *int32
	// Score は 10 点満点のスコア。付けていなければ nil。
	Score *float64
	// Notes は、ユーザーのメモ。なければ空。
	Notes string
}

// FollowingPage は、視聴状況の 1 ページ。
type FollowingPage struct {
	Viewings []*FollowingViewing
	// Next は、続きを引くときに渡す cursor。続きがなければ空。
	Next string
}

// FollowingViewings は、作品のフォロー中のユーザーの視聴状況。
type FollowingViewings struct {
	// Enabled は、出せるかどうか。管理者が連携を無効にしているか、作品を引ける記録先と連携していなければ false。
	Enabled bool
	// NeedsRefresh は、問い合わせる時期が来ているかどうか。
	NeedsRefresh bool
	// Failed は、最後の問い合わせが、どれかの記録先で失敗したかどうか。
	Failed bool
	// Viewings は、状態の順に、続けて名前の順に並べる。
	Viewings []*FollowingViewing
}

// FollowingArgs は、視聴状況を取得するジョブの引数。トークンは載せず、ジョブが DB から引く。
type FollowingArgs struct {
	UserID uuid.UUID `json:"userId"`
	TID    int32     `json:"tid"`
}

var _ river.JobArgsWithInsertOpts = new(FollowingArgs)

func (FollowingArgs) Kind() string { return "anime_following_import" }

// InsertOpts は、同じ引数のジョブを、終わるまでは 1 つにする。画面を開き直しても、問い合わせを重ねない。
// 取得できなかったら、River では送り直さず、次に画面を開いたときに、問い合わせ直す日時を過ぎていれば取得する。
func (FollowingArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, Queue: jobs.QueueAnimeFollowing, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable, rivertype.JobStateRunning, rivertype.JobStateScheduled,
	}}}
}

// FollowingWorker は、視聴状況を取得するジョブを実行する。
type FollowingWorker struct {
	river.WorkerDefaults[FollowingArgs]

	service *Service
	db      Database
}

var _ river.Worker[FollowingArgs] = new(FollowingWorker)

// NewFollowingWorker は FollowingWorker を作る。
func NewFollowingWorker(service *Service, db Database) *FollowingWorker {
	return &FollowingWorker{WorkerDefaults: river.WorkerDefaults[FollowingArgs]{}, service: service, db: db}
}

// Timeout は、視聴状況を取得するジョブの時間の上限を返す。
func (w *FollowingWorker) Timeout(*river.Job[FollowingArgs]) time.Duration {
	return followingTimeout
}

func (w *FollowingWorker) Work(ctx context.Context, job *river.Job[FollowingArgs]) error {
	return w.service.ImportFollowing(ctx, w.db, job.Args.UserID, job.Args.TID)
}

// followingTarget は、問い合わせる記録先と、その記録先の作品 ID。
type followingTarget struct {
	destination Destination
	workIDs     []int32
	due         bool
	failed      bool
}

// followingTargets は、ユーザーが連携した記録先のうち、作品 tid を引けるものを返す。管理者が連携を無効にしていれば、空を返す。
func (s *Service) followingTargets(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, tid int32) ([]*followingTarget, error) {
	system, err := settings.Load(ctx, db)
	if err != nil {
		return nil, err
	}
	if !system.AnimeLinkingEnabled {
		return []*followingTarget{}, nil
	}
	works, err := anime.ListWorks(ctx, db, []int32{tid})
	if err != nil {
		return nil, err
	}
	rows, err := sqlc.New(db).ListAnimeFollowingFetches(ctx, sqlc.ListAnimeFollowingFetchesParams{SyobocalTid: tid, UserID: userID})
	if err != nil {
		return nil, fmt.Errorf("failed to list the following fetches: %w", err)
	}
	now := s.clock.Now()
	targets := make([]*followingTarget, 0, len(rows))
	for _, row := range rows {
		destination, err := s.destination(row.Destination)
		if err != nil {
			continue
		}
		workIDs := destination.WorkIDs(works[tid])
		if len(workIDs) == 0 {
			continue
		}
		targets = append(targets, &followingTarget{
			destination: destination, workIDs: workIDs,
			due:    followingDue(row.CheckedAt, row.FetchedAt, row.Error, row.RetryAt, now),
			failed: row.Error != nil,
		})
	}

	return targets, nil
}

// followingDue は、問い合わせる時期が来ているかどうかを返す。まだ問い合わせていないか、取り込めてから FollowingRefreshInterval が
// 過ぎたか、取り込めなかったときは、問い合わせ直す日時を過ぎていれば true を返す。
func followingDue(checkedAt, fetchedAt *time.Time, reason *string, retryAt *time.Time, now time.Time) bool {
	if checkedAt == nil {
		return true
	}
	if reason != nil {
		if retryAt != nil {
			return !retryAt.After(now)
		}

		return !checkedAt.Add(WishlistRetryInterval).After(now)
	}

	return fetchedAt == nil || !fetchedAt.Add(FollowingRefreshInterval).After(now)
}

// FollowingViewings は、ユーザー userID がフォローしている人の、作品 tid の視聴状況を、DB から返す。
func (s *Service) FollowingViewings(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, tid int32) (*FollowingViewings, error) {
	result := &FollowingViewings{Enabled: false, NeedsRefresh: false, Failed: false, Viewings: []*FollowingViewing{}}
	targets, err := s.followingTargets(ctx, db, userID, tid)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return result, nil
	}
	result.Enabled = true
	for _, target := range targets {
		result.NeedsRefresh = result.NeedsRefresh || target.due
		result.Failed = result.Failed || target.failed
	}
	rows, err := sqlc.New(db).ListAnimeFollowingViewings(ctx, sqlc.ListAnimeFollowingViewingsParams{
		UserID: userID, SyobocalTid: tid, FetchedAfter: s.clock.Now().Add(-FollowingRetention),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the following viewings: %w", err)
	}
	for _, row := range rows {
		viewing := &FollowingViewing{
			Destination: row.Destination, Username: row.Username, Name: row.Name, AvatarURL: "", Status: row.Status,
			Progress: row.Progress, EpisodesCount: row.EpisodesCount, Score: row.Score, Notes: "",
		}
		if row.AvatarUrl != nil {
			viewing.AvatarURL = *row.AvatarUrl
		}
		if row.Notes != nil {
			viewing.Notes = *row.Notes
		}
		result.Viewings = append(result.Viewings, viewing)
	}
	sortFollowingViewings(result.Viewings)

	return result, nil
}

// sortFollowingViewings は、視聴状況を、状態の順に、続けて名前の順に並べる。
func sortFollowingViewings(viewings []*FollowingViewing) {
	slices.SortStableFunc(viewings, func(a, b *FollowingViewing) int {
		return cmp.Or(
			cmp.Compare(followingRank(a.Status), followingRank(b.Status)),
			cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			cmp.Compare(a.Destination, b.Destination),
			cmp.Compare(a.Username, b.Username),
		)
	})
}

// RefreshFollowing は、問い合わせる時期が来ていれば、視聴状況を取得するジョブを入れ、入れたら true を返す。
func (s *Service) RefreshFollowing(ctx context.Context, db Database, inserter anime.Inserter, userID uuid.UUID, tid int32) (bool, error) {
	if inserter == nil {
		return false, nil
	}
	targets, err := s.followingTargets(ctx, db, userID, tid)
	if err != nil {
		return false, err
	}
	if !slices.ContainsFunc(targets, func(target *followingTarget) bool { return target.due }) {
		return false, nil
	}
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		_, err := inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: FollowingArgs{UserID: userID, TID: tid}, InsertOpts: nil}})

		return err
	})
	if err != nil {
		return false, fmt.Errorf("failed to enqueue the following import: %w", err)
	}

	return true, nil
}

// ImportFollowing は、問い合わせる時期が来た記録先から、ユーザー userID がフォローしている人の、作品 tid の視聴状況を取得する。
// 取得できなければ、前に取得した視聴状況を残し、理由と問い合わせ直す日時を残す。1 つの記録先で失敗しても、ほかの記録先は取得する。
func (s *Service) ImportFollowing(ctx context.Context, db Database, userID uuid.UUID, tid int32) error {
	targets, err := s.followingTargets(ctx, db, userID, tid)
	if err != nil {
		return err
	}
	attempted := false
	for _, target := range targets {
		if !target.due {
			continue
		}
		attempted = true
		if err := s.importFollowing(ctx, db, userID, tid, target); err != nil {
			if ctx.Err() != nil {
				return err
			}
			s.logger.WarnContext(ctx, "failed to import the following viewings",
				slog.String("destination", target.destination.Name()), slog.Any("error", err))
		}
	}
	if !attempted {
		return nil
	}
	// 失敗も画面に伝えるので、どちらでも知らせる。
	if _, err := notify.Publish(context.WithoutCancel(ctx), db, s.clock, FollowingNotifyChannel); err != nil {
		return fmt.Errorf("failed to publish the following viewings change: %w", err)
	}

	return nil
}

// importFollowing は、1 つの記録先の視聴状況を取得し、入れ替える。取得か保存に失敗したら、理由と問い合わせ直す日時を残す。
// ジョブの期限や停止で取り消されたときは、記録先の失敗ではないので残さず、次に画面を開いたときに問い合わせ直す。
func (s *Service) importFollowing(ctx context.Context, db Database, userID uuid.UUID, tid int32, target *followingTarget) error {
	name := target.destination.Name()
	viewings, err := s.fetchFollowing(ctx, db, userID, target)
	if err == nil {
		err = s.replaceFollowing(ctx, db, userID, tid, name, viewings)
	}
	if err == nil || ctx.Err() != nil {
		return err
	}
	reason := shortenError(err)
	now := s.clock.Now()
	if _, saveErr := sqlc.New(db).SaveAnimeFollowingFetch(ctx, sqlc.SaveAnimeFollowingFetchParams{
		SyobocalTid: tid, Now: now, Error: &reason, RetryAt: new(now.Add(retryDelay(err))), UserID: userID, Destination: name,
	}); saveErr != nil {
		return errors.Join(err, fmt.Errorf("failed to save the following import failure: %w", saveErr))
	}

	return err
}

// replaceFollowing は、取得した視聴状況で入れ替え、取得できたことを残す。
func (s *Service) replaceFollowing(ctx context.Context, db Database, userID uuid.UUID, tid int32, name string, viewings []*FollowingViewing) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		// 連携を外した後なら、視聴状況を書かない。
		updated, err := q.SaveAnimeFollowingFetch(ctx, sqlc.SaveAnimeFollowingFetchParams{
			SyobocalTid: tid, Now: s.clock.Now(), Error: nil, RetryAt: nil, UserID: userID, Destination: name,
		})
		if err != nil {
			return fmt.Errorf("failed to save the following import: %w", err)
		}
		if updated == 0 {
			return nil
		}
		if err := q.DeleteAnimeFollowingViewings(ctx, sqlc.DeleteAnimeFollowingViewingsParams{UserID: userID, Destination: name, SyobocalTid: tid}); err != nil {
			return fmt.Errorf("failed to delete the following viewings: %w", err)
		}
		if err := q.InsertAnimeFollowingViewings(ctx, followingViewingsParams(userID, name, tid, viewings)); err != nil {
			return fmt.Errorf("failed to save the following viewings: %w", err)
		}

		return nil
	})
}

// followingViewingsParams は、視聴状況を、まとめて書く引数にする。配列は NULL を持てないので、分からない数は -1 に、
// 分からない文字列は空にする (クエリが NULL に戻す)。
func followingViewingsParams(userID uuid.UUID, name string, tid int32, viewings []*FollowingViewing) sqlc.InsertAnimeFollowingViewingsParams {
	params := sqlc.InsertAnimeFollowingViewingsParams{
		UserID: userID, Destination: name, SyobocalTid: tid,
		Usernames: make([]string, 0, len(viewings)), Names: make([]string, 0, len(viewings)), AvatarUrls: make([]string, 0, len(viewings)),
		Statuses: make([]string, 0, len(viewings)), Progresses: make([]int32, 0, len(viewings)), EpisodesCounts: make([]int32, 0, len(viewings)),
		Scores: make([]float64, 0, len(viewings)), Notes: make([]string, 0, len(viewings)),
	}
	orNone := func(value *int32) int32 {
		if value == nil {
			return -1
		}

		return *value
	}
	for _, viewing := range viewings {
		score := float64(-1)
		if viewing.Score != nil {
			score = *viewing.Score
		}
		params.Usernames = append(params.Usernames, viewing.Username)
		params.Names = append(params.Names, viewing.Name)
		params.AvatarUrls = append(params.AvatarUrls, viewing.AvatarURL)
		params.Statuses = append(params.Statuses, viewing.Status)
		params.Progresses = append(params.Progresses, orNone(viewing.Progress))
		params.EpisodesCounts = append(params.EpisodesCounts, orNone(viewing.EpisodesCount))
		params.Scores = append(params.Scores, score)
		params.Notes = append(params.Notes, viewing.Notes)
	}

	return params
}

// fetchFollowing は、記録先の視聴状況を、ページをたどって引く。問い合わせの間は、見たいリストの取り込みと合わせて空ける。
// 記録先が 429 を返したら、示された長さの間は、ほかのユーザーや作品の分も問い合わせない。
func (s *Service) fetchFollowing(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, target *followingTarget) ([]*FollowingViewing, error) {
	token, err := s.token(ctx, db, userID, target.destination.Name())
	if err != nil {
		return nil, err
	}
	name := target.destination.Name()
	s.wishlistMu.Lock()
	defer s.wishlistMu.Unlock()
	if until := s.followingLimited[name]; time.Now().Before(until) {
		return nil, &RateLimitError{RetryAfter: time.Until(until)}
	}
	viewings := []*FollowingViewing{}
	cursor := ""
	for range maxFollowingPages {
		if err := s.waitWishlistTurn(ctx, name); err != nil {
			return nil, err
		}
		page, err := target.destination.FollowingViewings(ctx, token, target.workIDs, cursor)
		s.wishlistLast[name] = time.Now()
		if err != nil {
			if errors.Is(err, ErrRateLimited) {
				s.followingLimited[name] = time.Now().Add(retryDelay(err))
			}

			return nil, err
		}
		viewings = append(viewings, page.Viewings...)
		if page.Next == "" || len(viewings) >= maxFollowingViewings {
			break
		}
		cursor = page.Next
	}
	if len(viewings) > maxFollowingViewings {
		viewings = viewings[:maxFollowingViewings]
	}

	return mergeFollowingViewings(viewings), nil
}

// mergeFollowingViewings は、1 人に 1 つの視聴状況にまとめる。1 つの TID に複数の作品が対応すると、同じ人が何度も返ってくる。
// 状態の順の早いものを使い、同じ状態なら先に返ってきたものを使う。
func mergeFollowingViewings(viewings []*FollowingViewing) []*FollowingViewing {
	picked := map[string]int{}
	result := make([]*FollowingViewing, 0, len(viewings))
	for _, viewing := range viewings {
		index, ok := picked[viewing.Username]
		switch {
		case !ok:
			picked[viewing.Username] = len(result)
			result = append(result, viewing)
		case followingRank(viewing.Status) < followingRank(result[index].Status):
			result[index] = viewing
		}
	}

	return result
}

// parseFollowingCursor は、AniList の視聴状況の cursor を、作品の添字とページの番号にする。空なら、最初の作品の 1 ページ目を返す。
func parseFollowingCursor(cursor string, works int) (int, int, error) {
	if cursor == "" {
		return 0, 1, nil
	}
	index, page, ok := strings.Cut(cursor, ":")
	work, workErr := strconv.Atoi(index)
	number, pageErr := strconv.Atoi(page)
	if !ok || workErr != nil || pageErr != nil || work < 0 || work >= works || number < 1 {
		return 0, 0, fmt.Errorf("%w: invalid following cursor", ErrPermanent)
	}

	return work, number, nil
}
