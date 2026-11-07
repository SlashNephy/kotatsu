package graph

import (
	"context"
	"errors"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// CreateCalendarFeed is the resolver for the createCalendarFeed field.
func (r *mutationResolver) CreateCalendarFeed(ctx context.Context) (*model.CreatedCalendarFeed, error) {
	if r.calendarFeeds == nil {
		return nil, errCalendarFeedsUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	// URL を持つ人は、そのユーザーとして予約を読めるので、API トークンと同じく直前の再認証を求める (設計書 9.5)。
	if err := auth.RequireRecentAuthentication(ctx, r.clock); err != nil {
		return nil, err
	}
	token, feed, err := r.calendarFeeds.Create(ctx, actorOf(ctx, principal), principal.UserID)
	if err != nil {
		return nil, err
	}

	return &model.CreatedCalendarFeed{Path: calendarFeedPath(token), CalendarFeed: calendarFeedOf(feed)}, nil
}

// DeleteCalendarFeed is the resolver for the deleteCalendarFeed field.
func (r *mutationResolver) DeleteCalendarFeed(ctx context.Context) (bool, error) {
	if r.calendarFeeds == nil {
		return false, errCalendarFeedsUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}
	err = r.calendarFeeds.Delete(ctx, actorOf(ctx, principal), principal.UserID)
	if errors.Is(err, auth.ErrFeedNotFound) {
		return false, NewError(CodeNotFound, "calendar feed not found")
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// CreateIptvFeed is the resolver for the createIptvFeed field.
func (r *mutationResolver) CreateIptvFeed(ctx context.Context) (*model.CreatedIptvFeed, error) {
	if r.iptvFeeds == nil {
		return nil, errIPTVFeedsUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	// ライブ視聴を見られないユーザーには、配信しても見られないので発行しない。
	if !principal.Role.Can(auth.PermissionWatch) {
		return nil, NewError(CodeForbidden, "only users who can watch can create an IPTV feed")
	}
	// URL を持つ人は、そのユーザーとしてライブ視聴を見られるので、API トークンと同じく直前の再認証を求める (設計書 9.5)。
	if err := auth.RequireRecentAuthentication(ctx, r.clock); err != nil {
		return nil, err
	}
	token, feed, err := r.iptvFeeds.Create(ctx, actorOf(ctx, principal), principal.UserID)
	if err != nil {
		return nil, err
	}
	playlist, guide := iptvFeedPaths(token)

	return &model.CreatedIptvFeed{PlaylistPath: playlist, GuidePath: guide, IptvFeed: iptvFeedOf(feed)}, nil
}

// DeleteIptvFeed is the resolver for the deleteIptvFeed field.
func (r *mutationResolver) DeleteIptvFeed(ctx context.Context) (bool, error) {
	if r.iptvFeeds == nil {
		return false, errIPTVFeedsUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return false, err
	}
	err = r.iptvFeeds.Delete(ctx, actorOf(ctx, principal), principal.UserID)
	if errors.Is(err, auth.ErrFeedNotFound) {
		return false, NewError(CodeNotFound, "IPTV feed not found")
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// CalendarFeed is the resolver for the calendarFeed field.
func (r *queryResolver) CalendarFeed(ctx context.Context) (*model.CalendarFeed, error) {
	if r.calendarFeeds == nil {
		return nil, errCalendarFeedsUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	feed, err := r.calendarFeeds.Get(ctx, principal.UserID)
	// 発行していなければ、null を返す。
	if errors.Is(err, auth.ErrFeedNotFound) {
		err = nil
	}
	if err != nil || feed == nil {
		return nil, err
	}

	return calendarFeedOf(feed), nil
}

// IptvFeed is the resolver for the iptvFeed field.
func (r *queryResolver) IptvFeed(ctx context.Context) (*model.IptvFeed, error) {
	if r.iptvFeeds == nil {
		return nil, errIPTVFeedsUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	feed, err := r.iptvFeeds.Get(ctx, principal.UserID)
	// 発行していなければ、null を返す。
	if errors.Is(err, auth.ErrFeedNotFound) {
		err = nil
	}
	if err != nil || feed == nil {
		return nil, err
	}

	return iptvFeedOf(feed), nil
}
