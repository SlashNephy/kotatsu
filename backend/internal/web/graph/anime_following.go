package graph

import (
	"context"
	"math"
	"net/url"

	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/web/animeimage"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// followingTID は、GraphQL の tid を確かめて返す。
func followingTID(tid int) (int32, error) {
	if tid <= 0 || tid > math.MaxInt32 {
		return 0, NewError(CodeBadUserInput, "invalid tid")
	}

	return int32(tid), nil
}

// animeFollowingViewings は、ログイン中のユーザーがフォローしている人の、作品の視聴状況を返す (#1372)。DB にあるものだけを返し、外部には問い合わせない。
func (r *Resolver) animeFollowingViewings(ctx context.Context, tid int) (*model.AnimeFollowingViewings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	work, err := followingTID(tid)
	if err != nil {
		return nil, err
	}
	service, err := r.animeRecordService()
	if err != nil {
		return nil, err
	}
	following, err := service.FollowingViewings(ctx, r.db, principal.UserID, work)
	if err != nil {
		return nil, err
	}
	result := &model.AnimeFollowingViewings{
		Enabled: following.Enabled, NeedsRefresh: following.NeedsRefresh, Failed: following.Failed,
		Viewings: make([]*model.AnimeFollowingViewing, 0, len(following.Viewings)),
	}
	for _, viewing := range following.Viewings {
		item := &model.AnimeFollowingViewing{
			Destination: destinationOf(viewing.Destination), Username: viewing.Username, Name: viewing.Name, AvatarURL: nil,
			ProfileURL: followingProfileURL(viewing.Destination, viewing.Username), Status: followingStatusOf(viewing.Status),
			Progress: intOf(viewing.Progress), EpisodesCount: intOf(viewing.EpisodesCount), Score: viewing.Score, Notes: nil,
		}
		if viewing.AvatarURL != "" {
			item.AvatarURL = new(animeimage.FollowingAvatarPath(viewing.Destination, viewing.Username, viewing.AvatarURL))
		}
		if viewing.Notes != "" {
			item.Notes = new(viewing.Notes)
		}
		result.Viewings = append(result.Viewings, item)
	}

	return result, nil
}

// refreshAnimeFollowingViewings は、問い合わせる時期が来ていれば、作品の視聴状況を取得するジョブを入れる (#1372)。
func (r *Resolver) refreshAnimeFollowingViewings(ctx context.Context, tid int) (bool, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}
	work, err := followingTID(tid)
	if err != nil {
		return false, err
	}
	service, err := r.animeRecordService()
	if err != nil {
		return false, err
	}
	if r.jobs == nil {
		return false, nil
	}

	return service.RefreshFollowing(ctx, r.db, r.jobs, principal.UserID, work)
}

// followingProfileURL は、記録先のユーザーのページを返す。
func followingProfileURL(destination, username string) string {
	if destination == animerecord.DestinationAniList {
		return "https://anilist.co/user/" + url.PathEscape(username)
	}

	return "https://annict.com/@" + url.PathEscape(username)
}

// followingStatusOf は、フォロー中のユーザーの作品の状態を、GraphQL の形にする。
func followingStatusOf(status string) model.AnimeFollowingStatus {
	switch status {
	case animerecord.FollowingWatched:
		return model.AnimeFollowingStatusWatched
	case animerecord.FollowingRepeating:
		return model.AnimeFollowingStatusRepeating
	case animerecord.FollowingOnHold:
		return model.AnimeFollowingStatusOnHold
	case animerecord.FollowingStopWatching:
		return model.AnimeFollowingStatusStopWatching
	case animerecord.FollowingWannaWatch:
		return model.AnimeFollowingStatusWannaWatch
	default:
		return model.AnimeFollowingStatusWatching
	}
}
