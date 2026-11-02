package anime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/workids"
)

// SourceAnnictWorks は、Annict と AniList から作った作品 ID の対応の取り込み元の名前 (anime_imports.source、#1223)。
const SourceAnnictWorks = "annict_works"

const (
	// seasonalInterval は、今期と来期の作品を Annict から引き直す間隔 (付録 A)。
	seasonalInterval = 24 * time.Hour
	// fullPassInterval は、Annict の作品をすべて引き終えてから、また初めから引き直すまでの間隔 (付録 A)。
	fullPassInterval = 30 * 24 * time.Hour
	// fullPassPages は、Annict の作品をすべて引くとき、1 回の取り込みで引くページの数 (付録 A)。全件を数日に分けて引く。
	fullPassPages = 20
	// maxSeasonalPages は、今期と来期の作品を引くページの数の上限。1 期は数百作品なので、足りる。
	maxSeasonalPages = 20
	// aniListRecheckInterval は、AniList の ID を引き直す間隔 (付録 A)。
	aniListRecheckInterval = 30 * 24 * time.Hour
	// aniListBatches は、1 回の取り込みで、AniList に問い合わせる回数 (付録 A)。
	aniListBatches = 20
	// DefaultAnnictRequestInterval と DefaultAniListRequestInterval は、問い合わせの間を空ける長さ (付録 A)。
	// Annict は 1 分に 600 回まで受け付けるが、全件を一度に引かず、間を空けて 1 つずつ送るよう求めている。
	// AniList は 1 分に 90 回 (混んでいるときは 30 回) まで受け付ける。
	DefaultAnnictRequestInterval  = time.Second
	DefaultAniListRequestInterval = 2 * time.Second
)

// WorkIDFetcher は、Annict と AniList から作品 ID を引く。*workids.Client が満たす。
type WorkIDFetcher interface {
	AnnictWorks(ctx context.Context, token string, seasons []string, cursor string) (*workids.AnnictPage, error)
	AniListIDsByMAL(ctx context.Context, malIDs []int32) (map[int32]int32, error)
}

var _ WorkIDFetcher = new(workids.Client)

// TokenSource は、管理者が設定した、作品 ID を引くための Annict のトークンを返す。設定していなければ空を返す。
// *animerecord.Service が満たす。
type TokenSource interface {
	AnnictImportToken(ctx context.Context, db sqlc.DBTX) (string, error)
}

// WorkIDOptions は、作品 ID の対応を自前で作るための設定 (#1223)。
type WorkIDOptions struct {
	Fetcher WorkIDFetcher
	Tokens  TokenSource
	// AnnictInterval と AniListInterval は、問い合わせの間を空ける長さ。
	AnnictInterval  time.Duration
	AniListInterval time.Duration
}

// WithWorkIDs は、Annict と AniList から作品 ID の対応を作るようにした Importer を返す (#1223)。
func (i *Importer) WithWorkIDs(opts *WorkIDOptions) *Importer {
	i.workIDs = opts

	return i
}

// importAnnictWorks は、Annict と AniList から作品 ID を引いて、作品 ID の対応 (anime_works の annict) を作り直す (#1223)。
// 管理者が Annict のトークンを設定していなければ、何もしない。
//
// Annict の負荷を抑えるため、今期と来期の作品は 1 日に 1 回、それ以外は、1 回に fullPassPages ページずつ数日かけて引き、
// 引き終えたら 30 日後にまた初めから引く。引いた作品は annict_works に持ち、引いたところまでを annict_work_sync に覚えるので、
// 途中で失敗しても、次は続きから引く。AniList の ID は、MyAnimeList の ID から 50 件ずつ引く。
// 問い合わせに失敗したら、理由を result と anime_imports に残して nil を返し、1 時間後に問い合わせ直す。
// 429 の Retry-After も、1 時間までなら、これで守れる。
func (i *Importer) importAnnictWorks(ctx context.Context, force bool, result *ImportResult) error {
	if i.workIDs == nil {
		return nil
	}
	token, err := i.workIDs.Tokens.AnnictImportToken(ctx, i.db)
	if err != nil {
		return err
	}
	if token == "" {
		return nil
	}
	previous, err := sqlc.New(i.db).GetAnimeImport(ctx, SourceAnnictWorks)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("failed to read the import status of %s: %w", SourceAnnictWorks, err)
	}
	found := err == nil
	if found && !force && previous.Error != nil && !i.due(&previous) {
		return nil
	}

	changed, err := i.fetchAnnictWorks(ctx, token, force)
	if err == nil {
		var resolved bool
		resolved, err = i.resolveAniListIDs(ctx)
		changed = changed || resolved
	}
	if err != nil {
		return i.recordAnnictWorksFailure(ctx, err, result)
	}
	// 前の取り込みが途中で失敗していれば、そこまでに引いた作品を、まだ対応に写していない。
	if !changed && found && previous.Error == nil && previous.ImportedAt != nil {
		result.Unchanged = append(result.Unchanged, SourceAnnictWorks)
		if err := sqlc.New(i.db).RecordAnimeImport(ctx, sqlc.RecordAnimeImportParams{
			Source: SourceAnnictWorks, Etag: nil, Now: i.clock.Now(), ItemCount: previous.ItemCount,
		}); err != nil {
			return fmt.Errorf("failed to record the import status of %s: %w", SourceAnnictWorks, err)
		}

		return nil
	}
	count, err := i.rebuildAnnictWorks(ctx)
	if err != nil {
		return err
	}
	result.Imported[SourceAnnictWorks] = count

	return nil
}

// recordAnnictWorksFailure は、作品 ID を引けなかった理由を、result と anime_imports に残す。
func (i *Importer) recordAnnictWorksFailure(ctx context.Context, cause error, result *ImportResult) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	i.logger.WarnContext(ctx, "failed to import anime data", slog.String("source", SourceAnnictWorks), slog.Any("error", cause))
	reason := truncate(cause.Error(), maxErrorLength)
	result.Failed[SourceAnnictWorks] = reason
	if err := sqlc.New(i.db).RecordAnimeImportFailure(ctx, sqlc.RecordAnimeImportFailureParams{
		Source: SourceAnnictWorks, Now: i.clock.Now(), Error: &reason,
	}); err != nil {
		return fmt.Errorf("failed to record the import status of %s: %w", SourceAnnictWorks, err)
	}

	return nil
}

// fetchAnnictWorks は、引く時期の来た Annict の作品を引いて、annict_works に書く。作品を書いたかどうかを返す。
func (i *Importer) fetchAnnictWorks(ctx context.Context, token string, force bool) (bool, error) {
	sync, err := sqlc.New(i.db).GetAnnictWorkSync(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get the Annict work sync: %w", err)
	}
	now := i.clock.Now()
	changed := false
	// 今期と来期の作品。放送が始まる前に、TID が付いていくので、毎日引き直す。
	if force || sync.SeasonalAt == nil || !now.Before(sync.SeasonalAt.Add(seasonalInterval)) {
		if err := i.fetchSeasonalWorks(ctx, token, now); err != nil {
			return false, err
		}
		changed = true
		sync.SeasonalAt = new(now)
		if err := i.saveAnnictWorkSync(ctx, &sync); err != nil {
			return changed, err
		}
	}
	// すべての作品。引き終えてから fullPassInterval が過ぎたら、初めから引き直す。
	if sync.FullCompletedAt != nil && !now.Before(sync.FullCompletedAt.Add(fullPassInterval)) {
		sync.FullCompletedAt, sync.FullCursor = nil, nil
	}
	if sync.FullCompletedAt != nil {
		return changed, nil
	}
	fetched, err := i.fetchAllWorks(ctx, token, &sync, now)

	return changed || fetched, err
}

// fetchSeasonalWorks は、今期と来期の作品をすべて引いて、annict_works に書く。
func (i *Importer) fetchSeasonalWorks(ctx context.Context, token string, now time.Time) error {
	cursor := ""
	for range maxSeasonalPages {
		page, err := i.annictPage(ctx, token, seasonsOf(now), cursor)
		if err != nil {
			return err
		}
		if err := i.upsertAnnictWorks(ctx, page.Works); err != nil {
			return err
		}
		if cursor = page.Next; cursor == "" {
			break
		}
	}

	return nil
}

// fetchAllWorks は、すべての作品を、前に引いたところの続きから fullPassPages ページまで引いて、annict_works に書く。
// 作品を書いたかどうかを返す。
func (i *Importer) fetchAllWorks(ctx context.Context, token string, sync *sqlc.AnnictWorkSync, now time.Time) (bool, error) {
	changed := false
	for range fullPassPages {
		cursor := ""
		if sync.FullCursor != nil {
			cursor = *sync.FullCursor
		}
		page, err := i.annictPage(ctx, token, nil, cursor)
		if err != nil {
			return changed, err
		}
		if err := i.upsertAnnictWorks(ctx, page.Works); err != nil {
			return changed, err
		}
		changed = true
		sync.FullCursor = optional(page.Next)
		if page.Next == "" {
			sync.FullCompletedAt = new(now)
		}
		// 途中で失敗しても、次は続きから引けるよう、ページごとに覚える。
		if err := i.saveAnnictWorkSync(ctx, sync); err != nil {
			return changed, err
		}
		if page.Next == "" {
			break
		}
	}

	return changed, nil
}

// saveAnnictWorkSync は、Annict の作品を引いたところまでを覚える。
func (i *Importer) saveAnnictWorkSync(ctx context.Context, sync *sqlc.AnnictWorkSync) error {
	if err := sqlc.New(i.db).UpdateAnnictWorkSync(ctx, sqlc.UpdateAnnictWorkSyncParams{
		SeasonalAt: sync.SeasonalAt, FullCursor: sync.FullCursor, FullCompletedAt: sync.FullCompletedAt,
	}); err != nil {
		return fmt.Errorf("failed to update the Annict work sync: %w", err)
	}

	return nil
}

// annictPage は、間を空けてから、Annict の作品の 1 ページを引く。
func (i *Importer) annictPage(ctx context.Context, token string, seasons []string, cursor string) (*workids.AnnictPage, error) {
	if err := i.waitWorkIDs(ctx, i.workIDs.AnnictInterval); err != nil {
		return nil, err
	}
	page, err := i.workIDs.Fetcher.AnnictWorks(ctx, token, seasons, cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the works from Annict: %w", err)
	}

	return page, nil
}

// upsertAnnictWorks は、Annict の作品を annict_works に書く。TID のない作品も書き、Annict で TID が外れたら、対応からも外す。
func (i *Importer) upsertAnnictWorks(ctx context.Context, works []*workids.AnnictWork) error {
	params := sqlc.UpsertAnnictWorksParams{AnnictIds: []int32{}, SyobocalTids: []int32{}, MalAnimeIds: []int32{}, Now: i.clock.Now()}
	for _, work := range works {
		params.AnnictIds = append(params.AnnictIds, work.AnnictID)
		params.SyobocalTids = append(params.SyobocalTids, positiveOrZero(work.SyobocalTID))
		params.MalAnimeIds = append(params.MalAnimeIds, positiveOrZero(work.MALAnimeID))
	}
	if len(params.AnnictIds) == 0 {
		return nil
	}
	if err := sqlc.New(i.db).UpsertAnnictWorks(ctx, params); err != nil {
		return fmt.Errorf("failed to save the Annict works: %w", err)
	}

	return nil
}

// resolveAniListIDs は、MyAnimeList の ID を持つ作品の AniList の ID を、まだ引いていないか、引いてから時間の経ったものから引く。
// AniList の ID を書いたかどうかを返す。
func (i *Importer) resolveAniListIDs(ctx context.Context) (bool, error) {
	q := sqlc.New(i.db)
	changed := false
	for range aniListBatches {
		rows, err := q.ListAnnictWorksToResolve(ctx, sqlc.ListAnnictWorksToResolveParams{
			CheckedBefore: i.clock.Now().Add(-aniListRecheckInterval), MaxCount: workids.AniListBatchSize,
		})
		if err != nil {
			return changed, fmt.Errorf("failed to list the works to resolve on AniList: %w", err)
		}
		if len(rows) == 0 {
			return changed, nil
		}
		malIDs := make([]int32, 0, len(rows))
		for _, row := range rows {
			malIDs = append(malIDs, row.MalAnimeID)
		}
		if err := i.waitWorkIDs(ctx, i.workIDs.AniListInterval); err != nil {
			return changed, err
		}
		found, err := i.workIDs.Fetcher.AniListIDsByMAL(ctx, malIDs)
		if err != nil {
			return changed, fmt.Errorf("failed to fetch the works from AniList: %w", err)
		}
		params := sqlc.UpdateAnnictWorksAniListParams{AnnictIds: make([]int32, 0, len(rows)), AnilistIds: make([]int32, 0, len(rows)), Now: i.clock.Now()}
		for _, row := range rows {
			params.AnnictIds = append(params.AnnictIds, row.AnnictID)
			params.AnilistIds = append(params.AnilistIds, found[row.MalAnimeID])
		}
		if err := q.UpdateAnnictWorksAniList(ctx, params); err != nil {
			return changed, fmt.Errorf("failed to save the AniList IDs: %w", err)
		}
		changed = true
	}

	return changed, nil
}

// rebuildAnnictWorks は、annict_works から作品 ID の対応 (anime_works の annict) を作り直し、その件数を返す。
func (i *Importer) rebuildAnnictWorks(ctx context.Context) (int, error) {
	var count int64
	err := pgx.BeginFunc(ctx, i.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.LockAnimeImport(ctx, SourceAnnictWorks); err != nil {
			return fmt.Errorf("failed to lock the import of %s: %w", SourceAnnictWorks, err)
		}
		if err := q.DeleteAnimeWorks(ctx, workSourceAnnict); err != nil {
			return fmt.Errorf("failed to delete the works of %s: %w", workSourceAnnict, err)
		}
		var err error
		if count, err = q.InsertAnnictAnimeWorks(ctx); err != nil {
			return fmt.Errorf("failed to insert the works of %s: %w", workSourceAnnict, err)
		}
		n, err := itemCount(int(count))
		if err != nil {
			return err
		}

		return q.RecordAnimeImport(ctx, sqlc.RecordAnimeImportParams{Source: SourceAnnictWorks, Etag: nil, Now: i.clock.Now(), ItemCount: n})
	})
	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// workSourceAnnict は、自前で作った作品 ID の対応の anime_works.source。arm の対応より優先する。
const workSourceAnnict = "annict"

// waitWorkIDs は、問い合わせの間を d だけ空ける。
func (i *Importer) waitWorkIDs(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// seasonsOf は、日本時間で now を含む期と、その次の期を、Annict の形 (2026-autumn) で返す。
func seasonsOf(now time.Time) []string {
	names := [...]string{"winter", "spring", "summer", "autumn"}
	jst := now.In(time.FixedZone("JST", 9*60*60))
	year, index := jst.Year(), (int(jst.Month())-1)/3
	next, nextYear := index+1, year
	if next == len(names) {
		next, nextYear = 0, year+1
	}

	return []string{fmt.Sprintf("%d-%s", year, names[index]), fmt.Sprintf("%d-%s", nextYear, names[next])}
}
