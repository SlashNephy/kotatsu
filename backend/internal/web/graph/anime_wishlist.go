package graph

import (
	"context"
	"errors"
	"math"
	"strconv"

	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// animeWishlist は、ログイン中のユーザーの見たいリストの作品の放送の候補を返す (#928)。
// 管理者が連携を無効にしていれば、何も引かずに、無効であることだけを返す。
func (r *Resolver) animeWishlist(ctx context.Context) (*model.AnimeWishlist, *animerecord.Wishlist, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, nil, err
	}
	empty := &model.AnimeWishlist{Enabled: false, Imports: []*model.AnimeWishlistImport{}, Works: []*model.AnimeWishlistWork{}}
	service, err := r.animeRecordService()
	if err != nil {
		return nil, nil, err
	}
	system, err := settings.Load(ctx, r.db)
	if err != nil {
		return nil, nil, err
	}
	if !system.AnimeLinkingEnabled {
		return empty, nil, nil
	}
	wishlist, err := service.Wishlist(ctx, r.db, principal.UserID)
	if err != nil {
		return nil, nil, err
	}
	result := &model.AnimeWishlist{Enabled: true, Imports: make([]*model.AnimeWishlistImport, 0, len(wishlist.Imports)), Works: []*model.AnimeWishlistWork{}}
	for _, imported := range wishlist.Imports {
		result.Imports = append(result.Imports, &model.AnimeWishlistImport{
			Destination: destinationOf(imported.Destination), CheckedAt: imported.CheckedAt, ImportedAt: imported.ImportedAt,
			Error: imported.Error, EntryCount: imported.Entries,
		})
	}
	programs, err := r.wishlistPrograms(ctx, wishlist.Works)
	if err != nil {
		return nil, nil, err
	}
	for _, work := range wishlist.Works {
		broadcasts := make([]*model.AnimeWishlistBroadcast, 0, len(work.Broadcasts))
		for _, broadcast := range work.Broadcasts {
			// 候補を求めた後に番組が消えていれば、飛ばす。
			program, ok := programs[programKeyOf(broadcast.ServiceID, broadcast.EventID)]
			if !ok {
				continue
			}
			broadcasts = append(broadcasts, &model.AnimeWishlistBroadcast{Program: program, Preferred: broadcast.Preferred})
		}
		if len(broadcasts) == 0 {
			continue
		}
		sources := make([]*model.AnimeWishlistSource, 0, len(work.Sources))
		for _, source := range work.Sources {
			sources = append(sources, &model.AnimeWishlistSource{
				Destination: destinationOf(source.Destination), WorkID: int(source.WorkID),
				Status: wishlistStatusOf(source.Status), URL: wishlistWorkURL(source.Destination, source.WorkID),
			})
		}
		result.Works = append(result.Works, &model.AnimeWishlistWork{
			Tid: int(work.TID), Title: work.Title, Sources: sources, Broadcasts: broadcasts, ReservedBroadcasts: work.ReservedBroadcasts,
			Covered: work.Covered,
		})
	}

	return result, wishlist, nil
}

// wishlistPrograms は、候補の放送の番組を、番組表と同じ形で引く。
func (r *Resolver) wishlistPrograms(ctx context.Context, works []*animerecord.WishlistWork) (map[string]*model.Program, error) {
	serviceIDs, eventIDs := []int64{}, []int64{}
	for _, work := range works {
		for _, broadcast := range work.Broadcasts {
			serviceIDs, eventIDs = append(serviceIDs, broadcast.ServiceID), append(eventIDs, broadcast.EventID)
		}
	}

	return r.programsByKeys(ctx, serviceIDs, eventIDs)
}

func programKeyOf(serviceID, eventID int64) string {
	return strconv.FormatInt(serviceID, 10) + "/" + strconv.FormatInt(eventID, 10)
}

// wishlistStatusOf は、見たいリストの作品の状態を、GraphQL の形にする。
func wishlistStatusOf(status string) model.AnimeWishlistStatus {
	if status == animerecord.WishlistWatching {
		return model.AnimeWishlistStatusWatching
	}

	return model.AnimeWishlistStatusWannaWatch
}

// wishlistWorkURL は、記録先の作品のページを返す。番組の詳細に出す作品のページ (設計書 3.8) と同じ URL にする。
func wishlistWorkURL(destination string, workID int32) string {
	base := "https://annict.com/works/"
	if destination == animerecord.DestinationAniList {
		base = "https://anilist.co/anime/"
	}

	return *workURL(base, &workID)
}

// createAnimeWishlistRule は、ログイン中のユーザーの候補の作品 tid を録るルールを作る (#928)。
// 候補は、ユーザー自身の見たいリストから求めるので、ほかのユーザーの見たいリストの作品からは作れない。
// 予約済みの作品 (予約か、同じキーワードとサービスのルールがある) からは、ルールを重ねて作らない。
func (r *Resolver) createAnimeWishlistRule(ctx context.Context, tid int) (*model.Rule, error) {
	actor, err := scheduler(ctx)
	if err != nil {
		return nil, err
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if tid <= 0 || tid > math.MaxInt32 {
		return nil, NewError(CodeBadUserInput, "tid is out of range")
	}
	service, err := r.animeRecordService()
	if err != nil {
		return nil, err
	}
	system, err := settings.Load(ctx, r.db)
	if err != nil {
		return nil, err
	}
	if !system.AnimeLinkingEnabled {
		return nil, NewError(CodeConflict, "anime linking is disabled")
	}
	created, err := service.CreateWishlistRule(ctx, r.db, r.rules, actor, principal.UserID, int32(tid))
	switch {
	case errors.Is(err, animerecord.ErrWishlistWorkNotFound):
		return nil, NewError(CodeNotFound, "the work is not in your wishlist candidates")
	case errors.Is(err, animerecord.ErrWishlistWorkCovered):
		return nil, NewError(CodeConflict, "the work already has a reservation or a rule")
	case err != nil:
		return nil, err
	}

	return toRuleModel(ctx, created)
}

// refreshAnimeWishlist は、ログイン中のユーザーの見たいリストを、今すぐ取り込み直すジョブを入れる (#928)。
func (r *Resolver) refreshAnimeWishlist(ctx context.Context) (bool, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}
	service, err := r.animeRecordService()
	if err != nil {
		return false, err
	}
	system, err := settings.Load(ctx, r.db)
	if err != nil {
		return false, err
	}
	if !system.AnimeLinkingEnabled {
		return false, NewError(CodeConflict, "anime linking is disabled")
	}
	if r.jobs == nil {
		return false, nil
	}
	if err := service.RefreshWishlist(ctx, r.db, r.jobs, principal.UserID); err != nil {
		return false, err
	}

	return true, nil
}
