package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

const (
	// oidcAuthTimeLeeway は、IdP と Kotatsu の時計のずれとして、ID トークンの auth_time が、再認証を始めた日時より前でも許す幅。
	oidcAuthTimeLeeway = time.Minute
	// maxReturnToLength は、再認証を終えたときに戻すパスの長さの上限。
	maxReturnToLength = 2048
)

var (
	// ErrOIDCNotLinked は、ログイン中のユーザーが、IdP のユーザーと紐付いていないので、IdP で再認証できないことを表す。
	ErrOIDCNotLinked = errors.New("the user is not linked to the OIDC provider")
	// ErrOIDCStaleAuthentication は、IdP が auth_time を返さないか、再認証を始めるより前の認証の日時を返したことを表す。
	// IdP が、もう一度認証させずに、前のログインで ID トークンを発行したかもしれないので、再認証として扱わない。
	ErrOIDCStaleAuthentication = errors.New("the OIDC provider did not authenticate the user again")
	// ErrOIDCReauthenticationMismatch は、再認証を始めたセッションと、戻ってきたリクエストのセッションか、
	// IdP で認証したユーザーと、ログイン中のユーザーが違うことを表す。
	ErrOIDCReauthenticationMismatch = errors.New("the OIDC reauthentication does not match the session")
	// ErrInvalidReturnTo は、再認証を終えたときに戻すパスが、同じオリジンの絶対パスでないことを表す。
	ErrInvalidReturnTo = errors.New("the return path must be an absolute path on this site")
)

// OIDCReauthentication は、IdP での再認証の結果。
type OIDCReauthentication struct {
	// AuthenticatedAt は、更新したセッションの認証の日時。
	AuthenticatedAt time.Time
	// ReturnTo は、再認証を始めたときに渡した、戻すパス。
	ReturnTo string
}

// BeginReauthentication は、ログイン中のユーザーに、IdP でもう一度認証させる再認証を始め、IdP の認可のエンドポイントの URL を返す
// (設計書 9.5)。パスワードもパスキーもない、OIDC だけでログインするユーザーが、重要な操作の前に再認証するのに使う。
// returnTo は、再認証を終えたときに戻す、画面のパス。空なら / にする。
func (o *OIDC) BeginReauthentication(
	ctx context.Context, db Database, principal *Principal, returnTo string, client *ClientInfo,
) (*OIDCLogin, error) {
	if !o.Enabled() {
		return nil, ErrOIDCDisabled
	}
	returnTo, err := normalizeReturnTo(returnTo)
	if err != nil {
		return nil, err
	}
	linked, err := o.linkedToIssuer(ctx, db, principal.UserID)
	if err != nil {
		return nil, err
	}
	if !linked {
		return nil, ErrOIDCNotLinked
	}

	return o.begin(ctx, db, client, &oidcSessionRequest{sessionID: principal.SessionID, returnTo: returnTo, link: false})
}

// reauthenticate は、IdP で認証し直したユーザーが、再認証を始めたセッションのユーザーであることを確かめ、セッションの認証の日時を更新する。
func (o *OIDC) reauthenticate(
	ctx context.Context, db Database, row *sqlc.OidcLogin, claims *oidcClaims, principal *Principal, client *ClientInfo,
) (*OIDCReauthentication, error) {
	// 再認証を始めたセッションのまま戻ってきたかを確かめる。ほかの端末やセッションで、途中の状態を使わせない。
	if principal == nil || principal.SessionID != *row.SessionID {
		return nil, ErrOIDCReauthenticationMismatch
	}
	authTime, ok := authTimeOf(claims.all)
	if !ok || authTime.Before(row.CreatedAt.Add(-oidcAuthTimeLeeway)) {
		return nil, ErrOIDCStaleAuthentication
	}
	now := o.clock.Now()
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		identity, err := q.GetOIDCIdentity(ctx, sqlc.GetOIDCIdentityParams{Issuer: claims.issuer, Subject: claims.subject})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOIDCReauthenticationMismatch
		}
		if err != nil {
			return fmt.Errorf("failed to get the OIDC identity: %w", err)
		}
		if identity.UserID != principal.UserID {
			return ErrOIDCReauthenticationMismatch
		}
		if err := q.RecordOIDCIdentityUse(ctx, sqlc.RecordOIDCIdentityUseParams{ID: identity.ID, Now: &now}); err != nil {
			return fmt.Errorf("failed to record the OIDC identity use: %w", err)
		}

		return markSessionReauthenticated(ctx, tx, o.recorder, principal, methodOIDC, client, now)
	})
	if err != nil {
		return nil, err
	}

	// 再認証として始めたものには、戻すパスを必ず入れているが、なければ / に戻す。
	returnTo := "/"
	if row.ReturnTo != nil {
		returnTo = *row.ReturnTo
	}

	return &OIDCReauthentication{AuthenticatedAt: now, ReturnTo: returnTo}, nil
}

// authTimeOf は、ID トークンの auth_time (ユーザーが IdP で認証した UNIX 時刻) を返す。なければ false を返す。
func authTimeOf(claims map[string]any) (time.Time, bool) {
	value, ok := claims["auth_time"].(float64)
	if !ok || value <= 0 {
		return time.Time{}, false
	}

	return time.Unix(int64(value), 0), true
}

// normalizeReturnTo は、再認証を終えたときに戻すパスを確かめる。ほかのサイトに戻させないよう、同じオリジンの絶対パスに限る。
func normalizeReturnTo(returnTo string) (string, error) {
	if returnTo == "" {
		return "/", nil
	}
	// //example.com や /\\example.com は、ブラウザーがほかのオリジンとして扱うので拒む。
	if len(returnTo) > maxReturnToLength || !strings.HasPrefix(returnTo, "/") ||
		strings.HasPrefix(returnTo, "//") || strings.ContainsAny(returnTo, "\\\r\n\t") {
		return "", ErrInvalidReturnTo
	}
	u, err := url.Parse(returnTo)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return "", ErrInvalidReturnTo
	}

	return returnTo, nil
}
