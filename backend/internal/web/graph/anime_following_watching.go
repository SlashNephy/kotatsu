package graph

import (
	"context"
	"errors"
	"math"

	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/web/animeimage"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// animeFollowingWatching は、ログイン中のユーザーがフォローしている人が「見てる」にした作品の、これからの放送を返す (#1968)。
// 管理者が連携を無効にしていれば、何も引かずに、無効であることだけを返す。
func (r *Resolver) animeFollowingWatching(ctx context.Context) (*model.AnimeFollowingWatching, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	service, err := r.animeRecordService()
	if err != nil {
		return nil, err
	}
	system, err := settings.Load(ctx, r.db)
	if err != nil {
		return nil, err
	}
	result := &model.AnimeFollowingWatching{Enabled: false, Works: []*model.AnimeFollowingWatchingWork{}}
	if !system.AnimeLinkingEnabled {
		return result, nil
	}
	result.Enabled = true
	works, err := service.FollowingWatchingWorks(ctx, r.db, principal.UserID)
	if err != nil {
		return nil, err
	}
	candidates := make([]*animerecord.WishlistWork, 0, len(works))
	for _, work := range works {
		candidates = append(candidates, work.WishlistWork)
	}
	programs, err := r.wishlistPrograms(ctx, candidates)
	if err != nil {
		return nil, err
	}
	for _, work := range works {
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
		watchers := make([]*model.AnimeFollowingWatcher, 0, len(work.Watchers))
		for _, watcher := range work.Watchers {
			item := &model.AnimeFollowingWatcher{
				Destination: destinationOf(watcher.Destination), Username: watcher.Username, Name: watcher.Name, AvatarURL: nil,
				ProfileURL: followingProfileURL(watcher.Destination, watcher.Username),
			}
			if watcher.AvatarURL != "" {
				item.AvatarURL = new(animeimage.FollowingAvatarPath(watcher.Destination, watcher.Username, watcher.AvatarURL))
			}
			watchers = append(watchers, item)
		}
		result.Works = append(result.Works, &model.AnimeFollowingWatchingWork{
			Tid: int(work.TID), Title: work.Title, Watchers: watchers, Broadcasts: broadcasts, ReservedBroadcasts: work.ReservedBroadcasts,
			Covered: work.Covered,
		})
	}

	return result, nil
}

// createAnimeFollowingWatchingRule は、ログイン中のユーザーがフォローしている人が見ている作品 tid を録るルールを作る (#1968)。
// 候補は、ユーザー自身が取り込んだものから求めるので、ほかのユーザーの候補の作品からは作れない。
func (r *Resolver) createAnimeFollowingWatchingRule(ctx context.Context, tid int) (*model.Rule, error) {
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
	created, err := service.CreateFollowingWatchingRule(ctx, r.db, r.rules, actor, principal.UserID, int32(tid))
	switch {
	case errors.Is(err, animerecord.ErrWishlistWorkNotFound):
		return nil, NewError(CodeNotFound, "the work is not watched by the users you follow")
	case errors.Is(err, animerecord.ErrWishlistWorkCovered):
		return nil, NewError(CodeConflict, "the work already has a reservation or a rule")
	case err != nil:
		return nil, err
	}

	return toRuleModel(ctx, created)
}
