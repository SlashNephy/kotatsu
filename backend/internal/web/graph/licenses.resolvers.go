package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/licenses"
)

// GoModuleLicenses is the resolver for the goModuleLicenses field.
func (r *queryResolver) GoModuleLicenses(ctx context.Context) ([]*licenses.Module, error) {
	// 版は、古い版の脆弱性を狙う手がかりになるので、Version と同じく、ログインしたユーザーにだけ返す。
	if auth.PrincipalFrom(ctx) == nil {
		return nil, NewError(CodeUnauthenticated, "login required")
	}
	list, err := licenses.Load()
	if errors.Is(err, licenses.ErrNotGenerated) {
		// 一覧を書き出さないビルドでは、スキーマのとおり null を返す。
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load the license list: %w", err)
	}

	return list.Modules, nil
}
