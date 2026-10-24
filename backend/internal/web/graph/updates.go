package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// latestRelease は、動いている版より新しいリリースを、管理者にだけ返す (#506)。
func (r *Resolver) latestRelease(ctx context.Context) *model.Release {
	principal := auth.PrincipalFrom(ctx)
	if r.updates == nil || principal == nil || !principal.Role.Can(auth.PermissionAdminister) {
		return nil
	}
	release := r.updates.Latest()
	if release == nil {
		return nil
	}

	return &model.Release{Version: release.Version, URL: release.URL, PublishedAt: release.PublishedAt}
}
