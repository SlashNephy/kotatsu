package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
)

const (
	// issuedMark は、API トークンの頭に付ける印。漏れたトークンを、秘密の検出の仕組みで見つけやすくする。
	issuedMark = "kotatsu_"
	// summaryName は、監査ログの要約に残す、トークンの名前の項目。
	summaryName = "name"
	// apiTokenBytes は、API トークンのランダムな部分の長さ。
	apiTokenBytes = 32
	// maxAPITokenName は、API トークンの名前の長さの上限 (文字数)。
	maxAPITokenName = 64
	// apiTokenTouchInterval は、API トークンを使った日時を書き直す間隔。リクエストのたびに書かないようにする。
	apiTokenTouchInterval = time.Minute

	actionAPITokenCreate = "api_token.create"
	actionAPITokenDelete = "api_token.delete"
	targetAPIToken       = "api_token"
)

var (
	// ErrInvalidAPITokenName は、API トークンの名前が空か長すぎることを表す。
	ErrInvalidAPITokenName = errors.New("the API token name must be 1 to 64 characters")
	// ErrAPITokenNotFound は、API トークンがないことを表す。
	ErrAPITokenNotFound = errors.New("API token not found")
)

// APIToken は、発行した API トークンの記録。トークンそのものは持たない。
type APIToken struct {
	ID         uuid.UUID
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// APITokens は、読み取り専用の API トークン (#431、設計書 9.8) を発行し、確かめる。
// トークンは発行したときにだけ返し、DB には SHA-256 だけを保存する。トークンは、発行したユーザーのロールのまま動く。
type APITokens struct {
	pool     *pgxpool.Pool
	clock    clock.Clock
	recorder audit.Recorder
}

// NewAPITokens は APITokens を返す。
func NewAPITokens(pool *pgxpool.Pool, clk clock.Clock, recorder audit.Recorder) *APITokens {
	return &APITokens{pool: pool, clock: clk, recorder: recorder}
}

// Create は、ユーザーに API トークンを発行し、トークンそのものと、その記録を返す。トークンは、ここでしか分からない。
func (a *APITokens) Create(ctx context.Context, actor *audit.Actor, userID uuid.UUID, name string) (string, *APIToken, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxAPITokenName {
		return "", nil, ErrInvalidAPITokenName
	}
	raw := make([]byte, apiTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("failed to generate an API token: %w", err)
	}
	token := issuedMark + base64.RawURLEncoding.EncodeToString(raw)
	record := &APIToken{ID: id.New(), Name: name, CreatedAt: a.clock.Now(), LastUsedAt: nil}
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		err := sqlc.New(tx).CreateAPIToken(ctx, sqlc.CreateAPITokenParams{
			ID: record.ID, UserID: userID, Name: name, TokenHash: hashToken(token), Now: record.CreatedAt,
		})
		if err != nil {
			return fmt.Errorf("failed to save the API token: %w", err)
		}

		return a.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionAPITokenCreate, TargetType: targetAPIToken, TargetID: record.ID.String(),
			Summary: map[string]string{summaryName: name}, SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
	if err != nil {
		return "", nil, err
	}

	return token, record, nil
}

// List は、ユーザーの API トークンの記録を、新しい順に返す。
func (a *APITokens) List(ctx context.Context, userID uuid.UUID) ([]*APIToken, error) {
	rows, err := sqlc.New(a.pool).ListAPITokens(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the API tokens: %w", err)
	}
	tokens := make([]*APIToken, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, &APIToken{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt})
	}

	return tokens, nil
}

// Delete は、ユーザーの API トークンを取り消す。ほかのユーザーのトークンなら ErrAPITokenNotFound を返す。
func (a *APITokens) Delete(ctx context.Context, actor *audit.Actor, userID, tokenID uuid.UUID) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		name, err := sqlc.New(tx).DeleteAPIToken(ctx, sqlc.DeleteAPITokenParams{ID: tokenID, UserID: userID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAPITokenNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to delete the API token: %w", err)
		}

		return a.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionAPITokenDelete, TargetType: targetAPIToken, TargetID: tokenID.String(),
			Summary: map[string]string{summaryName: name}, SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
}

// Authenticate は、API トークンから、発行したユーザーを返す。トークンが違うか、ユーザーを無効にしていれば ErrUnauthenticated を返す。
// 返す Principal は、セッションを持たず、認証した日時も持たないので、再認証の要る操作には使えない。
func (a *APITokens) Authenticate(ctx context.Context, token string) (*Principal, error) {
	if !strings.HasPrefix(token, issuedMark) {
		return nil, ErrUnauthenticated
	}
	q := sqlc.New(a.pool)
	row, err := q.GetAPITokenUser(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up the API token: %w", err)
	}
	role, err := ParseRole(row.Role)
	if err != nil {
		return nil, err
	}
	now := a.clock.Now()
	if row.LastUsedAt == nil || now.Sub(*row.LastUsedAt) >= apiTokenTouchInterval {
		if err := q.TouchAPIToken(ctx, sqlc.TouchAPITokenParams{ID: row.ID, Now: &now}); err != nil {
			return nil, fmt.Errorf("failed to record the API token use: %w", err)
		}
	}

	return &Principal{UserID: row.UserID, Role: role, SessionID: uuid.Nil, AuthenticatedAt: time.Time{}, OAuthClientID: ""}, nil
}

// BearerMiddleware は、Authorization の Bearer の API トークンを確かめ、そのユーザーを context に持たせるミドルウェアを返す。
// トークンがなければ、そのまま next を呼ぶ (権限は、後ろの RequirePermission が確かめる)。トークンが違えば 401 を返す。
func (a *APITokens) BearerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			next.ServeHTTP(w, r)

			return
		}
		principal, err := a.Authenticate(r.Context(), strings.TrimSpace(token))
		if errors.Is(err, ErrUnauthenticated) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kotatsu"`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return
		}
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}
