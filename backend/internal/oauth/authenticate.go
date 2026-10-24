package oauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// ErrWrongAudience は、アクセストークンが、ほかの MCP サーバー宛てに発行したものであることを表す。
var ErrWrongAudience = errors.New("the access token is not issued for this resource")

// Grant は、アクセストークンの連携。MCP のツールが、スコープとクライアントを確かめるのに使う。
type Grant struct {
	ID         uuid.UUID
	ClientID   string
	ClientName string
	Scopes     []string
}

// Authenticate は、アクセストークンから、連携したユーザーと連携を返す。resource は、リクエストを受けた MCP サーバーの URL。
// トークンが違うか、期限を過ぎたか、ユーザーを無効にしたか、MCP が無効なら auth.ErrUnauthenticated を、
// ほかの MCP サーバー宛てのトークンなら ErrWrongAudience を返す。
// 返す Principal は、セッションを持たず、認証した日時も持たないので、再認証の要る操作には使えない。
func (s *Server) Authenticate(ctx context.Context, token, resource string) (*auth.Principal, *Grant, error) {
	if !strings.HasPrefix(token, accessMark) {
		return nil, nil, auth.ErrUnauthenticated
	}
	err := s.requireEnabled(ctx)
	if errors.Is(err, ErrDisabled) {
		return nil, nil, auth.ErrUnauthenticated
	}
	if err != nil {
		return nil, nil, err
	}
	q := sqlc.New(s.db)
	now := s.clock.Now()
	row, err := q.GetOAuthAccessToken(ctx, sqlc.GetOAuthAccessTokenParams{TokenHash: hashSecret(token), Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, auth.ErrUnauthenticated
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to look up the access token: %w", err)
	}
	if !sameResource(resource, row.Resource) {
		return nil, nil, ErrWrongAudience
	}
	role, err := auth.ParseRole(row.Role)
	if err != nil {
		return nil, nil, err
	}
	if row.LastUsedAt == nil || now.Sub(*row.LastUsedAt) >= grantTouchInterval {
		if err := q.TouchOAuthGrant(ctx, sqlc.TouchOAuthGrantParams{ID: row.GrantID, Now: &now}); err != nil {
			return nil, nil, fmt.Errorf("failed to record the OAuth grant use: %w", err)
		}
	}
	principal := &auth.Principal{UserID: row.UserID, Role: role, SessionID: uuid.Nil, AuthenticatedAt: time.Time{}, OAuthClientID: row.ClientID}
	grant := &Grant{ID: row.GrantID, ClientID: row.ClientID, ClientName: row.ClientName, Scopes: narrowScopes(row.Scopes, role)}

	return principal, grant, nil
}
