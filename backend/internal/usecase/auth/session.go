package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/usersettings"
)

const (
	// CookieName は、セッションのトークンを入れる Cookie の名前。
	CookieName = "kotatsu_session"

	// DefaultIdleTimeout と DefaultAbsoluteTimeout は、設計書の付録 A の初期値。
	DefaultIdleTimeout     = 30 * 24 * time.Hour
	DefaultAbsoluteTimeout = 90 * 24 * time.Hour
	// touchInterval は、最後に使われた日時を更新する最短の間隔。リクエストのたびに DB へ書かないようにする。
	touchInterval = time.Minute
	// tokenBytes は、トークンの長さ (256 ビット)。
	tokenBytes = 32
)

var (
	// ErrSessionInvalid は、セッションが無い、失効した、期限が切れた、ユーザーが無効になった、のどれかを表す。
	// どれに当たるかは、攻撃者の手がかりにならないよう区別しない。
	ErrSessionInvalid = errors.New("session is invalid")
	// ErrSessionNotFound は、失効させようとしたセッションが、そのユーザーの有効なセッションにないことを表す。
	ErrSessionNotFound = errors.New("session not found")
)

// Session はログイン中のセッション。
type Session struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	UserAgent  string
	// SourceIP は、ログインしたときの送信元。分からなければゼロ値。
	SourceIP netip.Addr
}

// ClientInfo は、ログインした端末の情報。セッションの一覧で端末を見分けるのに使う。
type ClientInfo struct {
	UserAgent string
	SourceIP  netip.Addr
	// Host は、送信元が接続したホスト名 (ポートを含む)。パスキーの RP ID とオリジンに使う。
	Host string
	// Secure は、送信元が HTTPS でつないでいるかどうか。
	Secure bool
}

// Origin は、送信元が接続したオリジン (scheme://host) を返す。
func (c *ClientInfo) Origin() string {
	scheme := "http"
	if c.Secure {
		scheme = "https"
	}

	return scheme + "://" + c.Host
}

// SessionsOptions は Sessions の設定。
type SessionsOptions struct {
	// IdleTimeout は無操作の期限。0 なら 30 日。
	IdleTimeout time.Duration
	// AbsoluteTimeout は絶対的な期限。0 なら 90 日。
	AbsoluteTimeout time.Duration
	// IsSecure は、リクエストが HTTPS で届いたかどうかを返す。true なら Cookie に Secure を付ける。
	// nil なら、TLS で直接受けたかどうかで判定する。リバースプロキシが HTTPS を終端する構成では、
	// 信頼済みのプロキシの X-Forwarded-Proto を見る関数を渡す。
	IsSecure func(r *http.Request) bool
	Clock    clock.Clock
	// Logger は、DB に届かずに覚えた結果で認証したことと、最後に使われた日時を書けなかったことを出す。nil なら出さない。
	Logger *slog.Logger
}

// Sessions はサーバー側のセッションを扱う。
type Sessions struct {
	idleTimeout     time.Duration
	absoluteTimeout time.Duration
	isSecure        func(r *http.Request) bool
	clock           clock.Clock
	logger          *slog.Logger
	// fallback は、トークンのハッシュごとに、使えると確かめたセッションを覚え、DB に届かない間だけ使う (#1084)。
	fallback *database.Fallback[string, *authenticatedSession]
}

// authenticatedSession は、使えると確かめたセッション。
type authenticatedSession struct {
	principal *Principal
	// expiresAt は、絶対的な期限。覚えた結果で認証するときも、これを過ぎたら使わない。
	expiresAt time.Time
}

// NewSessions は Sessions を作る。opts は nil でもよい。
func NewSessions(opts *SessionsOptions) *Sessions {
	if opts == nil {
		opts = &SessionsOptions{IdleTimeout: 0, AbsoluteTimeout: 0, IsSecure: nil, Clock: nil, Logger: nil}
	}
	s := &Sessions{
		idleTimeout:     opts.IdleTimeout,
		absoluteTimeout: opts.AbsoluteTimeout,
		isSecure:        opts.IsSecure,
		clock:           opts.Clock,
		logger:          opts.Logger,
		fallback:        nil,
	}
	if s.idleTimeout == 0 {
		s.idleTimeout = DefaultIdleTimeout
	}
	if s.absoluteTimeout == 0 {
		s.absoluteTimeout = DefaultAbsoluteTimeout
	}
	if s.isSecure == nil {
		s.isSecure = func(r *http.Request) bool { return r.TLS != nil }
	}
	if s.clock == nil {
		s.clock = new(clock.Real)
	}
	if s.logger == nil {
		s.logger = slog.New(slog.DiscardHandler)
	}
	s.fallback = database.NewFallback[string, *authenticatedSession]("sessions", database.FallbackTTL, s.clock, s.logger)

	return s
}

// Create は、ログインしたユーザーのセッションを作り、Cookie に入れるトークンとセッションを返す。
// トークンは DB に保存しないので、ここでしか得られない。client は nil でもよい。
func (s *Sessions) Create(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, client *ClientInfo) (string, *Session, error) {
	if client == nil {
		client = &ClientInfo{UserAgent: "", SourceIP: netip.Addr{}, Host: "", Secure: false}
	}
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("failed to generate a session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	now := s.clock.Now()
	session := &Session{
		ID:         id.New(),
		UserID:     userID,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(s.absoluteTimeout),
		UserAgent:  client.UserAgent,
		SourceIP:   client.SourceIP,
	}
	var sourceIP *netip.Addr
	if client.SourceIP.IsValid() {
		sourceIP = &client.SourceIP
	}
	err := sqlc.New(db).InsertSession(ctx, sqlc.InsertSessionParams{
		ID:        session.ID,
		TokenHash: hashToken(token),
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: session.ExpiresAt,
		UserAgent: client.UserAgent,
		SourceIP:  sourceIP,
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to create a session: %w", err)
	}

	return token, session, nil
}

// Authenticate は、トークンのセッションが使えるかを確かめ、ログイン中のユーザーを返す。
// 使えなければ ErrSessionInvalid を返す。使えたら、最後に使われた日時を更新する。
// DB に届かなければ、database.FallbackTTL の中で使えると確かめたセッションに限って、覚えた結果で認証する (#1084)。
// DB に届かずに覚えた結果もなければ、DB のエラーを返す (database.IsUnavailable が true になる)。
func (s *Sessions) Authenticate(ctx context.Context, db sqlc.DBTX, token string) (*Principal, error) {
	session, stale, err := s.fallback.Load(ctx, string(hashToken(token)), func(ctx context.Context) (*authenticatedSession, error) {
		return s.lookup(ctx, db, token)
	})
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	if stale {
		if !now.Before(session.expiresAt) {
			return nil, ErrSessionInvalid
		}

		return session.principal, nil
	}

	// 最後に使われた日時は、セッションの一覧と無操作の期限に使うだけなので、書けなくても認証は通す (#1084)。
	touchCtx, cancel := context.WithTimeout(ctx, database.FallbackQueryTimeout)
	defer cancel()
	err = sqlc.New(db).TouchSession(touchCtx, sqlc.TouchSessionParams{Now: now, ID: session.principal.SessionID, StaleBefore: now.Add(-touchInterval)})
	if err != nil && ctx.Err() == nil {
		s.logger.WarnContext(ctx, "failed to touch a session", slog.Any("error", err))
	}

	return session.principal, nil
}

// ForgetUser は、ユーザーのセッションについて覚えた結果を、keep のセッションのほかは忘れる。keep は nil でもよい。
// ユーザーを無効にしたときや、ロールやパスワードを変えたときに呼び、その直後に DB が止まっても、覚えた結果で前の権限のまま認証しないようにする。
func (s *Sessions) ForgetUser(userID uuid.UUID, keep *uuid.UUID) {
	s.fallback.ForgetIf(func(session *authenticatedSession) bool {
		return session.principal.UserID == userID && (keep == nil || session.principal.SessionID != *keep)
	})
}

// FallbackUsed は、DB に届かずに、覚えた結果で認証した回数を返す。
func (s *Sessions) FallbackUsed() int64 {
	return s.fallback.Used()
}

// Usable は、セッションが今も使えるかを返す。失効させた、期限を過ぎた、ユーザーを無効にした、消したなら false を返す。
// 接続中の subscription (SSE) を、一定の間隔で確かめ直すのに使う (設計書 9.5)。
func (s *Sessions) Usable(ctx context.Context, db sqlc.DBTX, sessionID uuid.UUID) (bool, error) {
	now := s.clock.Now()
	usable, err := sqlc.New(db).GetSessionUsable(ctx, sqlc.GetSessionUsableParams{
		Now: now, IdleSince: now.Add(-s.idleTimeout), ID: sessionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check the session: %w", err)
	}

	return usable, nil
}

// List は、ユーザーの使えるセッションを、最後に使われた順に返す。
func (s *Sessions) List(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) ([]*Session, error) {
	now := s.clock.Now()
	rows, err := sqlc.New(db).ListActiveSessions(ctx, sqlc.ListActiveSessionsParams{
		UserID:    userID,
		Now:       now,
		IdleSince: now.Add(-s.idleTimeout),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	sessions := make([]*Session, 0, len(rows))
	for _, row := range rows {
		session := &Session{
			ID:         row.ID,
			UserID:     row.UserID,
			CreatedAt:  row.CreatedAt,
			LastSeenAt: row.LastSeenAt,
			ExpiresAt:  row.ExpiresAt,
			UserAgent:  row.UserAgent,
			SourceIP:   netip.Addr{},
		}
		if row.SourceIP != nil {
			session.SourceIP = *row.SourceIP
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// RevokeAndRecord は、ユーザー userID のセッション sessionID を失効させ、監査ログに残す (設計書 8.5)。
// 失効と記録を 1 つのトランザクションにまとめ、記録できなければ失効させない。
func (s *Sessions) RevokeAndRecord(ctx context.Context, db Database, recorder audit.Recorder, actor *audit.Actor, userID, sessionID uuid.UUID) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := s.Revoke(ctx, tx, userID, sessionID); err != nil {
			return err
		}

		return recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "user.session_revoked",
			TargetType: "user",
			TargetID:   userID.String(),
			Summary:    map[string]any{summarySessionID: sessionID.String()},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP:  netip.Addr{},
			UserAgent: "",
		})
	})
}

// RevokeOthersAndRecord は、ユーザー userID の、keepID のほかのセッションをまとめて失効させ、監査ログに残す。
// 失効と記録を 1 つのトランザクションにまとめる。失効させた数を返す。
func (s *Sessions) RevokeOthersAndRecord(ctx context.Context, db Database, recorder audit.Recorder, actor *audit.Actor, userID, keepID uuid.UUID) (int64, error) {
	var revoked int64
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		n, err := s.RevokeOthers(ctx, tx, userID, keepID)
		if err != nil {
			return err
		}
		revoked = n

		return recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "user.other_sessions_revoked",
			TargetType: "user",
			TargetID:   userID.String(),
			Summary:    map[string]any{"count": n},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP:  netip.Addr{},
			UserAgent: "",
		})
	})
	if err != nil {
		return 0, err
	}

	return revoked, nil
}

// Revoke は、ユーザーのセッションを失効させ、同じユーザーのほかの端末に、セッションの一覧が変わったことを知らせる。
// 他のユーザーのセッションや、失効済みのセッションを指定したときは ErrSessionNotFound を返す。
func (s *Sessions) Revoke(ctx context.Context, db Database, userID, sessionID uuid.UUID) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		n, err := sqlc.New(tx).RevokeSession(ctx, sqlc.RevokeSessionParams{Now: s.clock.Now(), ID: sessionID, UserID: userID})
		if err != nil {
			return fmt.Errorf("failed to revoke a session: %w", err)
		}
		if n == 0 {
			return ErrSessionNotFound
		}
		s.fallback.ForgetIf(func(session *authenticatedSession) bool { return session.principal.SessionID == sessionID })

		return usersettings.Touch(ctx, tx, s.clock, userID)
	})
}

// RevokeOthers は、ユーザーのセッションのうち、keepID のほかをまとめて失効させ、失効させた数を返す (設計書 9.5)。
// ログイン中の端末の画面で、使わなくなった端末や、覚えのない端末をまとめてログアウトさせるのに使う。
func (s *Sessions) RevokeOthers(ctx context.Context, db Database, userID, keepID uuid.UUID) (int64, error) {
	var revoked int64
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		n, err := sqlc.New(tx).RevokeUserSessions(ctx, sqlc.RevokeUserSessionsParams{Now: s.clock.Now(), UserID: userID, KeepID: &keepID})
		if err != nil {
			return fmt.Errorf("failed to revoke the other sessions: %w", err)
		}
		revoked = n
		s.fallback.ForgetIf(func(session *authenticatedSession) bool {
			return session.principal.UserID == userID && session.principal.SessionID != keepID
		})

		return usersettings.Touch(ctx, tx, s.clock, userID)
	})
	if err != nil {
		return 0, err
	}

	return revoked, nil
}

// SetCookie は、セッションのトークンを Cookie に入れる。
// HttpOnly と SameSite=Lax を付け、HTTPS で届いたリクエストなら Secure も付ける (設計書 9.5)。
func (s *Sessions) SetCookie(w http.ResponseWriter, r *http.Request, token string, session *Session) {
	http.SetCookie(w, s.cookie(r, token, session.ExpiresAt))
}

// ClearCookie は、セッションの Cookie を消す。
func (s *Sessions) ClearCookie(w http.ResponseWriter, r *http.Request) {
	cookie := s.cookie(r, "", time.Unix(0, 0)) //nolint:gosec // 設計書 9.5: HTTP の LAN でも使うため、Secure は HTTPS のときだけ付ける。
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
}

// Middleware は、Cookie のセッションを確かめ、ログイン中のユーザーを context に持たせるミドルウェアを返す。
// セッションが使えなければ、Cookie を消して未ログインとして扱う。認可は RequirePermission で行う。
func (s *Sessions) Middleware(db sqlc.DBTX) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(CookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)

				return
			}

			principal, err := s.Authenticate(r.Context(), db, cookie.Value)
			if errors.Is(err, ErrSessionInvalid) {
				s.ClearCookie(w, r)
				next.ServeHTTP(w, r)

				return
			}
			// DB に届かず、覚えた結果もないセッションは、確かめられないので、DB が戻るのを待ってもらう (#1084)。
			if database.IsUnavailable(err) {
				http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)

				return
			}
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
		})
	}
}

// LogoutHandler は、ログイン中のセッションを失効させ、Cookie を消すハンドラーを返す (POST /auth/logout)。
// ログインしていなくても、Cookie を消して成功を返す。
func (s *Sessions) LogoutHandler(db Database) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal := PrincipalFrom(r.Context()); principal != nil {
			err := s.Revoke(r.Context(), db, principal.UserID, principal.SessionID)
			if err != nil && !errors.Is(err, ErrSessionNotFound) {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

				return
			}
		}
		s.ClearCookie(w, r)
		w.WriteHeader(http.StatusNoContent)
	})
}

// lookup は、トークンのセッションを DB から引き、使えるかを確かめる。使えなければ ErrSessionInvalid を返す。
func (s *Sessions) lookup(ctx context.Context, db sqlc.DBTX, token string) (*authenticatedSession, error) {
	row, err := sqlc.New(db).GetSessionByTokenHash(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessionInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up a session: %w", err)
	}

	now := s.clock.Now()
	session := row.Session
	switch {
	case session.RevokedAt != nil,
		!now.Before(session.ExpiresAt),
		!now.Before(session.LastSeenAt.Add(s.idleTimeout)),
		row.UserDisabledAt != nil:
		return nil, ErrSessionInvalid
	}
	role, err := ParseRole(row.Role)
	if err != nil {
		return nil, err
	}

	return &authenticatedSession{
		principal: &Principal{UserID: session.UserID, Role: role, SessionID: session.ID, AuthenticatedAt: session.AuthenticatedAt.UTC(), OAuthClientID: ""},
		expiresAt: session.ExpiresAt,
	}, nil
}

func (s *Sessions) cookie(r *http.Request, value string, expires time.Time) *http.Cookie {
	return s.namedCookie(r, CookieName, "/", value, expires)
}

// namedCookie は、セッションと同じ属性の Cookie を返す。
func (s *Sessions) namedCookie(r *http.Request, name, path, value string, expires time.Time) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // 設計書 9.5: HTTP の LAN でも使うため、Secure は HTTPS のときだけ付ける。
		Name:     name,
		Value:    value,
		Path:     path,
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.isSecure(r),
		SameSite: http.SameSiteLaxMode,
	}
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))

	return sum[:]
}
