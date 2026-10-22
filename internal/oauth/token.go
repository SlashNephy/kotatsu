package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
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
// 認可コードとリフレッシュトークンを、アクセストークンとリフレッシュトークンに引き換える。
// クライアントは公開クライアントなので、クライアントの認証はせず、client_id と PKCE で確かめる。
func (s *Server) TokenHandler(logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !s.serveEnabled(w, r, logger) {
			return
		}
		form, err := readForm(w, r)
		if err != nil {
			writeTokenError(w, &tokenError{code: errorInvalidRequest, description: err.Error()})

			return
		}
		var resp *tokenResponse
		switch form.Get("grant_type") {
		case grantTypeAuthorizationCode:
			resp, err = s.exchangeCode(r.Context(), form)
		case grantTypeRefreshToken:
			resp, err = s.refresh(r.Context(), form, logger)
		default:
			err = &tokenError{code: errorUnsupportedGrantType, description: "grant_type must be authorization_code or refresh_token"}
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
			ID: grantID, UserID: code.UserID, ClientID: code.ClientID, ClientName: code.ClientName,
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
func (s *Server) refresh(ctx context.Context, form url.Values, logger *slog.Logger) (*tokenResponse, error) {
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
		case form.Get("client_id") != row.ClientID:
			return invalidGrant("client_id does not match the refresh token")
		case row.UsedAt != nil:
			reused = &row.GrantID

			return nil
		case !now.Before(row.ExpiresAt) || row.UserDisabledAt != nil:
			return invalidGrant("the refresh token is expired")
		case form.Has("resource") && !sameResource(form.Get("resource"), row.Resource):
			return &tokenError{code: errorInvalidTarget, description: "resource does not match the grant"}
		}
		// スコープを狭めて求められても、連携のスコープのまま発行する。広げることはできない。
		for scope := range strings.FieldsSeq(form.Get("scope")) {
			if !slices.Contains(row.Scopes, scope) {
				return &tokenError{code: errorInvalidScope, description: "scope exceeds the grant"}
			}
		}
		role, err := auth.ParseRole(row.Role)
		if err != nil {
			return err
		}
		if err := q.MarkOAuthRefreshTokenUsed(ctx, sqlc.MarkOAuthRefreshTokenUsedParams{TokenHash: hashSecret(token), Now: &now}); err != nil {
			return fmt.Errorf("failed to mark the refresh token used: %w", err)
		}
		resp, err = s.issue(ctx, q, row.GrantID, narrowScopes(row.Scopes, role))

		return err
	})
	if err != nil {
		return nil, err
	}
	if reused != nil {
		if err := sqlc.New(s.db).DeleteOAuthGrant(ctx, *reused); err != nil {
			return nil, fmt.Errorf("failed to revoke the OAuth grant: %w", err)
		}
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
func (s *Server) RevokeHandler(logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !s.serveEnabled(w, r, logger) {
			return
		}
		form, err := readForm(w, r)
		if err != nil || form.Get("token") == "" {
			writeTokenError(w, &tokenError{code: errorInvalidRequest, description: "token is missing"})

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
		if err := q.DeleteOAuthGrant(ctx, row.GrantID); err != nil {
			return fmt.Errorf("failed to revoke the OAuth grant: %w", err)
		}

		return nil
	}
	if err := q.DeleteOAuthToken(ctx, hashSecret(token)); err != nil {
		return fmt.Errorf("failed to revoke the OAuth token: %w", err)
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
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.code, "error_description": err.description})
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
