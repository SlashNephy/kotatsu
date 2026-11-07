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
	if errors.Is(err, auth.ErrCalendarFeedNotFound) {
		return false, NewError(CodeNotFound, "calendar feed not found")
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
	if errors.Is(err, auth.ErrCalendarFeedNotFound) {
		err = nil
	}
	if err != nil || feed == nil {
		return nil, err
	}

	return calendarFeedOf(feed), nil
}
