package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

var (
	// ErrOIDCAlreadyLinked は、ログイン中のユーザーが、既に設定の IdP のユーザーと紐付いていることを表す。
	ErrOIDCAlreadyLinked = errors.New("the user is already linked to the OIDC provider")
	// ErrOIDCLinkMismatch は、紐付けを始めたセッションと、戻ってきたリクエストのセッションが違うことを表す。
	ErrOIDCLinkMismatch = errors.New("the OIDC link does not match the session")
)

// BeginLink は、ログイン中のユーザーを、IdP のユーザーと紐付けるのを始め、IdP の認可のエンドポイントの URL を返す (設計書 9.4)。
// IdP でログインしたユーザーを、そのまま紐付けるので、重要な操作として、直近の再認証を求める (設計書 9.5)。
// returnTo は、紐付けを終えたときに戻す、画面のパス。空なら / にする。
func (o *OIDC) BeginLink(ctx context.Context, db Database, principal *Principal, returnTo string, client *ClientInfo) (*OIDCLogin, error) {
	if !o.Enabled() {
		return nil, ErrOIDCDisabled
	}
	if o.clock.Now().Sub(principal.AuthenticatedAt) > RecentAuthenticationWindow {
		return nil, ErrReauthenticationRequired
	}
	returnTo, err := normalizeReturnTo(returnTo)
	if err != nil {
		return nil, err
	}
	linked, err := o.linkedToIssuer(ctx, db, principal.UserID)
	if err != nil {
		return nil, err
	}
	if linked {
		return nil, ErrOIDCAlreadyLinked
	}

	return o.begin(ctx, db, client, &oidcSessionRequest{sessionID: principal.SessionID, returnTo: returnTo, link: true})
}

// Linked は、ユーザーが、設定の IdP のユーザーと紐付いているかどうかを返す。OIDC を設定していなければ false を返す。
func (o *OIDC) Linked(ctx context.Context, db Database, userID uuid.UUID) (bool, error) {
	if !o.Enabled() {
		return false, nil
	}

	return o.linkedToIssuer(ctx, db, userID)
}

// linkedToIssuer は、ユーザーが、設定の issuer の IdP のユーザーと紐付いているかどうかを返す。
func (o *OIDC) linkedToIssuer(ctx context.Context, db Database, userID uuid.UUID) (bool, error) {
	identities, err := sqlc.New(db).ListUserOIDCIdentities(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to list the OIDC identities: %w", err)
	}
	for i := range identities {
		if identities[i].Issuer == o.settings.Issuer {
			return true, nil
		}
	}

	return false, nil
}

// linkSession は、紐付けを始めたセッションのまま戻ってきたことを確かめ、ログイン中のユーザーを、IdP で認証したユーザーと紐付ける。
func (o *OIDC) linkSession(
	ctx context.Context, db Database, row *sqlc.OidcLogin, claims *oidcClaims, principal *Principal, client *ClientInfo,
) error {
	// ほかの端末やセッションで、途中の状態を使わせない。
	if principal == nil || row.SessionID == nil || principal.SessionID != *row.SessionID {
		return ErrOIDCLinkMismatch
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		// 始めてから戻ってくるまでの間に、ほかの経路で紐付けたかもしれないので、もう一度確かめる。
		linked, err := o.linkedToIssuer(ctx, tx, principal.UserID)
		if err != nil {
			return err
		}
		if linked {
			return ErrOIDCAlreadyLinked
		}
		if err := o.link(ctx, tx, principal.UserID, claims, new(o.clock.Now())); err != nil {
			return err
		}

		return o.recorder.Record(ctx, tx, &audit.Event{
			Actor:      audit.User(principal.UserID),
			Action:     "user.oidc_linked",
			TargetType: loginAuditTarget,
			TargetID:   principal.UserID.String(),
			Summary:    map[string]any{summaryIssuer: claims.issuer, summarySubject: claims.subject},
			SourceIP:   client.SourceIP,
			UserAgent:  client.UserAgent,
		})
	})
}
