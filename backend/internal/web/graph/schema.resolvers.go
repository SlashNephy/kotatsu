package graph

import (
	"context"
	"time"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/web/graph/generated"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// Viewer is the resolver for the viewer field.
func (r *queryResolver) Viewer(ctx context.Context) (*model.User, error) {
	principal := auth.PrincipalFrom(ctx)
	if principal == nil {
		return nil, NewError(CodeUnauthenticated, "login required")
	}
	user, err := loadersFrom(ctx).Users.Load(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}

	return toUser(user), nil
}

// Version is the resolver for the version field.
func (r *queryResolver) Version(ctx context.Context) (*model.AppVersion, error) {
	// 版は、古い版の脆弱性を狙う手がかりになるので、Viewer と同じく、ログインしたユーザーにだけ返す。
	if auth.PrincipalFrom(ctx) == nil {
		return nil, NewError(CodeUnauthenticated, "login required")
	}
	build := buildinfo.Get()
	var revision *string
	if build.Revision != "" {
		revision = new(build.Revision)
	}

	return &model.AppVersion{Version: build.Version, Revision: revision, Modified: build.Modified, LatestRelease: r.latestRelease(ctx)}, nil
}

// TimeZone is the resolver for the timeZone field.
func (r *queryResolver) TimeZone(ctx context.Context) (string, error) {
	if auth.PrincipalFrom(ctx) == nil {
		return "", NewError(CodeUnauthenticated, "login required")
	}

	return r.location.String(), nil
}

// Heartbeat is the resolver for the heartbeat field.
func (r *subscriptionResolver) Heartbeat(ctx context.Context) (<-chan *time.Time, error) {
	ch := make(chan *time.Time, 1)
	go func() {
		defer close(ch)
		ticker := time.NewTicker(r.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case ch <- new(r.clock.Now()):
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return ch, nil
}

// Query returns generated.QueryResolver implementation.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// Subscription returns generated.SubscriptionResolver implementation.
func (r *Resolver) Subscription() generated.SubscriptionResolver { return &subscriptionResolver{r} }

type (
	queryResolver        struct{ *Resolver }
	subscriptionResolver struct{ *Resolver }
)
