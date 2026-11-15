package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
)

const (
	grantTypeRefreshToken = "refresh_token"

	errorInvalidGrant         = "invalid_grant"
	errorUnsupportedGrantType = "unsupported_grant_type"
	errorUnsupportedTokenType = "unsupported_token_type"

	// maxFormBytes は、トークンと失効のエンドポイントで読むフォームの大きさの上限。
	maxFormBytes = 16 << 10
)

// tokenError は、トークンのエンドポイントのエラー (RFC 6749 5.2)。
type tokenError struct {
	code        string
	description string
}

func (e *tokenError) Error() string {
	return fmt.Sprintf("token error %s: %s", e.code, e.description)
}

func invalidGrant(description string) error {
	return &tokenError{code: errorInvalidGrant, description: description}
}

// tokenResponse は、トークンのエンドポイントの応答 (RFC 6749 5.1)。
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// TokenHandler は、トークンのエンドポイント (POST /oauth/token) のハンドラーを返す。
// 認可コード、端末の device_code (RFC 8628 3.4) とリフレッシュトークンを、アクセストークンとリフレッシュトークンに引き換える。
// クライアントは公開クライアントなので、クライアントの認証はせず、client_id と PKCE (認可コード) か device_code で確かめる。
// 認可コードは MCP を、device_code は EPGStation 互換の API を、管理者が有効にしているときだけ受け付ける。
func (s *Server) TokenHandler(logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		gates, ok := s.serveAnyEnabled(w, r, logger)
		if !ok {
			return
		}
		form, err := readForm(w, r)
		if err != nil {
			writeTokenError(w, &tokenError{code: errorInvalidRequest, description: err.Error()})

			return
		}
		var resp *tokenResponse
		switch grantType := form.Get("grant_type"); {
		case grantType == grantTypeAuthorizationCode && gates.mcp:
			resp, err = s.exchangeCode(r.Context(), form)
		case grantType == grantTypeDeviceCode && gates.device:
			resp, err = s.exchangeDeviceCode(r.Context(), form)
		case grantType == grantTypeRefreshToken:
			resp, err = s.refresh(r.Context(), form, gates, logger)
		default:
			err = &tokenError{code: errorUnsupportedGrantType, description: "grant_type is not supported"}
		}
		if err != nil {
			if tokenErr := (*tokenError)(nil); errors.As(err, &tokenErr) {
				writeTokenError(w, tokenErr)

				return
			}
			logger.ErrorContext(r.Context(), "failed to issue OAuth tokens", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

			return
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

// exchangeCode は、認可コードを、トークンに引き換える。認可コードは、引き換えに失敗しても、一度で使えなくする。
func (s *Server) exchangeCode(ctx context.Context, form url.Values) (*tokenResponse, error) {
	now := s.clock.Now()
	code, err := sqlc.New(s.db).ConsumeOAuthAuthorizationCode(ctx, sqlc.ConsumeOAuthAuthorizationCodeParams{
		CodeHash: hashSecret(form.Get("code")), Now: now,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, invalidGrant("the authorization code is invalid or expired")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to consume the authorization code: %w", err)
	}
	switch {
	case form.Get("client_id") != code.ClientID:
		return nil, invalidGrant("client_id does not match the authorization code")
	case form.Get("redirect_uri") != code.RedirectUri:
		return nil, invalidGrant("redirect_uri does not match the authorization request")
	case !VerifyCodeVerifier(form.Get("code_verifier"), code.CodeChallenge):
		return nil, invalidGrant("code_verifier does not match the code_challenge")
	case form.Has("resource") && !sameResource(form.Get("resource"), code.Resource):
		return nil, &tokenError{code: errorInvalidTarget, description: "resource does not match the authorization request"}
	}
	var resp *tokenResponse
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		grantID := id.New()
		err := q.CreateOAuthGrant(ctx, sqlc.CreateOAuthGrantParams{
			ID: grantID, UserID: code.UserID, Kind: grantKindMCP, ClientID: code.ClientID, ClientName: code.ClientName,
			Scopes: code.Scopes, Resource: code.Resource, Now: now,
		})
		if err != nil {
			return fmt.Errorf("failed to save the OAuth grant: %w", err)
		}
		resp, err = s.issue(ctx, q, grantID, code.Scopes)

		return err
	})
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// refresh は、リフレッシュトークンを入れ替え、新しいトークンを返す。使い済みのリフレッシュトークンがもう一度来たら、
// 盗まれたものが使われたとみなし、連携ごと取り消す (OAuth 2.1 4.3.1、RFC 9700 4.14.2)。
// MCP のクライアントの連携は MCP を、端末の連携は EPGStation 互換の API を、管理者が有効にしているときだけ入れ替える。
func (s *Server) refresh(ctx context.Context, form url.Values, gates *serverGates, logger *slog.Logger) (*tokenResponse, error) {
	var resp *tokenResponse
	var reused *uuid.UUID
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		now := s.clock.Now()
		token := form.Get("refresh_token")
		row, err := q.LockOAuthRefreshToken(ctx, hashSecret(token))
		if errors.Is(err, pgx.ErrNoRows) {
			return invalidGrant("the refresh token is invalid")
		}
		if err != nil {
			return fmt.Errorf("failed to look up the refresh token: %w", err)
		}
		switch {
		case !gates.allows(row.Kind):
			return invalidGrant("the refresh token is invalid")
		case form.Get("client_id") != row.ClientID:
			return invalidGrant("client_id does not match the refresh token")
		case row.UsedAt != nil:
			// ロックしたまま取り消し、見つけたことと取り消すことの間に、ほかの入れ替えを挟ませない。
			if err := revokeGrant(ctx, q, row.Kind, row.GrantID, now); err != nil {
				return err
			}
			reused = &row.GrantID

			return nil
		case !now.Before(row.ExpiresAt) || row.UserDisabledAt != nil:
			return invalidGrant("the refresh token is expired")
		case form.Has("resource") && !sameResource(form.Get("resource"), row.Resource):
			return &tokenError{code: errorInvalidTarget, description: "resource does not match the grant"}
		}
		// スコープを狭めて求められても、連携のスコープのまま発行する。広げることはできない。
		// 書くスコープでは読むツールも使えるので、書くだけの連携に読むスコープを求められても断らない (#1415)。
		for scope := range strings.FieldsSeq(form.Get("scope")) {
			if !HasScope(row.Scopes, scope) {
				return &tokenError{code: errorInvalidScope, description: "scope exceeds the grant"}
			}
		}
		role := row.Role
		if err := q.MarkOAuthRefreshTokenUsed(ctx, sqlc.MarkOAuthRefreshTokenUsedParams{TokenHash: hashSecret(token), Now: &now}); err != nil {
			return fmt.Errorf("failed to mark the refresh token used: %w", err)
		}
		resp, err = s.issue(ctx, q, row.GrantID, grantScopes(row.Kind, row.Scopes, role))

		return err
	})
	if err != nil {
		return nil, err
	}
	if reused != nil {
		logger.WarnContext(ctx, "a used refresh token was presented again; the OAuth grant is revoked", slog.String("grant_id", reused.String()))

		return nil, invalidGrant("the refresh token is invalid")
	}

	return resp, nil
}

// issue は、連携に、アクセストークンとリフレッシュトークンを発行する。
func (s *Server) issue(ctx context.Context, q *sqlc.Queries, grantID uuid.UUID, scopes []string) (*tokenResponse, error) {
	access, err := newSecret(accessMark)
	if err != nil {
		return nil, err
	}
	refresh, err := newSecret(refreshMark)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	for _, token := range []sqlc.CreateOAuthTokenParams{
		{TokenHash: hashSecret(access), GrantID: grantID, Kind: tokenKindAccess, Now: now, ExpiresAt: now.Add(AccessTokenLifetime)},
		{TokenHash: hashSecret(refresh), GrantID: grantID, Kind: tokenKindRefresh, Now: now, ExpiresAt: now.Add(RefreshTokenLifetime)},
	} {
		if err := q.CreateOAuthToken(ctx, token); err != nil {
			return nil, fmt.Errorf("failed to save the OAuth token: %w", err)
		}
	}

	return &tokenResponse{
		AccessToken: access, TokenType: "Bearer", ExpiresIn: int(AccessTokenLifetime.Seconds()),
		RefreshToken: refresh, Scope: strings.Join(scopes, " "),
	}, nil
}

// RevokeHandler は、失効のエンドポイント (POST /oauth/revoke、RFC 7009) のハンドラーを返す。
// リフレッシュトークンなら連携ごと取り消し、アクセストークンならそのトークンだけを消す。
// 知らないトークンでも、ほかのクライアントのトークンでも、成功を返す (RFC 7009 2.2)。
// 取り消しは権限を広げないので、MCP と EPGStation 互換の API のどちらかを有効にしていれば受け付ける。
func (s *Server) RevokeHandler(logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if _, ok := s.serveAnyEnabled(w, r, logger); !ok {
			return
		}
		form, err := readForm(w, r)
		if err != nil || form.Get("token") == "" || form.Get("client_id") == "" {
			// 公開クライアントは、client_id で自分を示す (RFC 6749 3.2.1)。
			writeTokenError(w, &tokenError{code: errorInvalidRequest, description: "token and client_id are required"})

			return
		}
		if hint := form.Get("token_type_hint"); hint != "" && hint != "access_token" && hint != "refresh_token" {
			writeTokenError(w, &tokenError{code: errorUnsupportedTokenType, description: "unsupported token_type_hint"})

			return
		}
		if err := s.revoke(r.Context(), form.Get("token"), form.Get("client_id")); err != nil {
			logger.ErrorContext(r.Context(), "failed to revoke the OAuth token", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)

			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

func (s *Server) revoke(ctx context.Context, token, clientID string) error {
	q := sqlc.New(s.db)
	row, err := q.GetOAuthTokenForRevocation(ctx, sqlc.GetOAuthTokenForRevocationParams{TokenHash: hashSecret(token), ClientID: clientID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to look up the OAuth token: %w", err)
	}
	if row.Kind == tokenKindRefresh {
		return revokeGrant(ctx, q, row.GrantKind, row.GrantID, s.clock.Now())
	}
	if err := q.DeleteOAuthToken(ctx, hashSecret(token)); err != nil {
		return fmt.Errorf("failed to revoke the OAuth token: %w", err)
	}

	return nil
}

// revokeGrant は、連携を取り消す。MCP のクライアントの連携は行を消し、端末の連携は行を残して取り消した日時を書く。
func revokeGrant(ctx context.Context, q *sqlc.Queries, kind string, grantID uuid.UUID, now time.Time) error {
	var err error
	if kind == grantKindDevice {
		err = q.RevokeOAuthDeviceGrant(ctx, sqlc.RevokeOAuthDeviceGrantParams{Now: &now, ID: grantID})
	} else {
		err = q.DeleteOAuthGrant(ctx, grantID)
	}
	if err != nil {
		return fmt.Errorf("failed to revoke the OAuth grant: %w", err)
	}

	return nil
}

// readForm は、本文の application/x-www-form-urlencoded を読む。同じパラメーターを 2 回送ったものは断る。
func readForm(w http.ResponseWriter, r *http.Request) (url.Values, error) {
	if ct, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";"); !strings.EqualFold(strings.TrimSpace(ct), "application/x-www-form-urlencoded") {
		return nil, errors.New("the content type must be application/x-www-form-urlencoded")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		return nil, errors.New("failed to parse the form")
	}
	for key, values := range r.PostForm {
		if len(values) > 1 {
			return nil, fmt.Errorf("duplicated parameter %s", key)
		}
	}

	return r.PostForm, nil
}

func writeTokenError(w http.ResponseWriter, err *tokenError) {
	writeErrorStatus(w, http.StatusBadRequest, err)
}

// writeErrorStatus は、エラーを、RFC 6749 5.2 の形の JSON にして、status で返す。
func writeErrorStatus(w http.ResponseWriter, status int, err *tokenError) {
	writeJSON(w, status, map[string]string{"error": err.code, "error_description": err.description})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}
