package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"slices"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
)

const (
	// OIDCCookieName は、OIDC でのログインの途中の状態のトークンを入れる Cookie の名前。
	OIDCCookieName = "kotatsu_oidc"
	// OIDCCallbackPath は、IdP から戻ってくるリダイレクト URI のパス。
	OIDCCallbackPath = "/auth/oidc/callback"
	// OIDCLoginTTL は、IdP へ送り出してから、戻ってくるまでに許す時間 (付録 A)。
	OIDCLoginTTL = 10 * time.Minute
	// methodOIDC は、OIDC でのログインを監査ログに残すときの方式。多要素認証は IdP に任せる (設計書 9.4)。
	methodOIDC SecondFactor = "oidc"
	// oidcRandomBytes は、state と nonce の乱数の長さ。
	oidcRandomBytes = 32
	// oidcHTTPTimeout は、IdP との 1 回の通信に許す時間。IdP が応答しないときに、待ち続けないようにする。
	oidcHTTPTimeout = 15 * time.Second
	// summaryIssuer は、監査ログの要約で、IdP の issuer を入れる項目。
	summaryIssuer = "issuer"
	// summarySubject は、監査ログの要約で、IdP のユーザーの subject を入れる項目。
	summarySubject = "subject"
)

var (
	// ErrOIDCDisabled は、OIDC でのログインを設定していないことを表す。
	ErrOIDCDisabled = errors.New("OIDC login is not configured")
	// ErrOIDCUnavailable は、IdP の設定 (discovery) を引けなかったことを表す。
	ErrOIDCUnavailable = errors.New("the OIDC provider is unavailable")
	// ErrOIDCLoginExpired は、ログインを始めていないか、期限が切れたか、使用済みであることを表す。
	ErrOIDCLoginExpired = errors.New("the OIDC login has expired or was not started")
	// ErrOIDCStateMismatch は、IdP から戻ってきた state が、始めたログインと違うことを表す。
	ErrOIDCStateMismatch = errors.New("the OIDC state does not match")
	// ErrOIDCTokenInvalid は、認可コードを引き換えられなかったか、ID トークンを確かめられなかったことを表す。
	// nonce が違うときも含む。
	ErrOIDCTokenInvalid = errors.New("the OIDC ID token is invalid")
	// ErrOIDCAccessDenied は、IdP がログインを拒んだか、Kotatsu のユーザーとしてログインさせないことを表す。
	// 紐付けたユーザーがいない、自動作成の条件を満たさない、ユーザーを無効にした、のどれかを区別しない。
	ErrOIDCAccessDenied = errors.New("the OIDC user is not allowed to log in")
	// ErrOIDCUsernameTaken は、自動で作るユーザーのユーザー名が、ほかのユーザーと重なることを表す。
	ErrOIDCUsernameTaken = errors.New("the username from the OIDC provider is already taken")
	// ErrOIDCIdentityLinked は、IdP のユーザーが、既にほかの Kotatsu のユーザーと紐付いていることを表す。
	ErrOIDCIdentityLinked = errors.New("the OIDC identity is already linked")
)

// OIDCSettings は、OIDC でのログインの設定 (設計書 9.4)。
type OIDCSettings struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	// RedirectURL が空なら、接続したオリジンに OIDCCallbackPath を付けたものにする。
	RedirectURL   string
	Scopes        []string
	DisplayName   string
	UsernameClaim string
	AutoCreate    OIDCAutoCreate
}

// OIDCAutoCreate は、OIDC で初めてログインしたユーザーを自動で作る設定。
type OIDCAutoCreate struct {
	Enabled      bool
	GroupsClaim  string
	AdminGroups  []string
	MemberGroups []string
	ViewerGroups []string
}

// OIDCOptions は OIDC の設定。
type OIDCOptions struct {
	Settings OIDCSettings
	Clock    clock.Clock
	Recorder audit.Recorder
	Logger   *slog.Logger
	// Sessions は、ログインしたユーザーのセッションを作る。
	Sessions *Sessions
	// HTTPClient は、IdP と通信するクライアント。nil なら、oidcHTTPTimeout で打ち切るクライアントにする。
	HTTPClient *http.Client
}

// OIDC は、OpenID Connect の IdP でのログインを扱う (設計書 9.4)。
//
// 認可コードフローに、PKCE (S256)、state、nonce を組み合わせる。ユーザーは (issuer, subject) の組で特定する。
// 多要素認証は IdP に任せるので、ID トークンを確かめたらセッションを作る。
type OIDC struct {
	settings   OIDCSettings
	clock      clock.Clock
	recorder   audit.Recorder
	logger     *slog.Logger
	sessions   *Sessions
	httpClient *http.Client

	mu       sync.Mutex
	provider *oidc.Provider
}

// NewOIDC は OIDC を返す。IdP の設定は、最初に使うときに引く。IdP が落ちていても、Kotatsu は起動できる。
func NewOIDC(opts *OIDCOptions) *OIDC {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: oidcHTTPTimeout}
	}

	return &OIDC{
		settings: opts.Settings, clock: opts.Clock, recorder: opts.Recorder, logger: opts.Logger, sessions: opts.Sessions,
		httpClient: httpClient, mu: sync.Mutex{}, provider: nil,
	}
}

// Enabled は、OIDC でのログインを設定したかどうかを返す。
func (o *OIDC) Enabled() bool {
	return o.settings.Issuer != ""
}

// DisplayName は、ログインの画面に出す IdP の名前を返す。
func (o *OIDC) DisplayName() string {
	return o.settings.DisplayName
}

// OIDCLogin は、IdP へ送り出すログイン。
type OIDCLogin struct {
	// AuthorizationURL は、ブラウザーを送り出す IdP の認可のエンドポイント。
	AuthorizationURL string
	// Token は、戻ってきたときに、ログインを続けるためのトークン。Cookie (kotatsu_oidc) に入れる。
	Token     string
	ExpiresAt time.Time
}

// Begin は、OIDC でのログインを始め、IdP の認可のエンドポイントの URL を返す。
func (o *OIDC) Begin(ctx context.Context, db Database, client *ClientInfo) (*OIDCLogin, error) {
	return o.begin(ctx, db, client, nil)
}

// OIDCCallback は、IdP からリダイレクト URI に戻ってきたときのパラメーター。
type OIDCCallback struct {
	State string
	Code  string
	// Error は、IdP がログインを拒んだときのエラー (access_denied など)。
	Error string
}

// Complete は、IdP から戻ってきたログインを確かめ、セッションを作る。
//
// ログインの途中の状態は、確かめられなくても使用済みにする。state、ID トークン、nonce を確かめた後、
// (issuer, subject) で紐付けたユーザーを引く。いなければ、自動作成の条件を満たすときだけ作る。
// 再認証として始めたものは、ログインとしては完了させない。
func (o *OIDC) Complete(ctx context.Context, db Database, token string, callback *OIDCCallback, client *ClientInfo) (*LoginResult, error) {
	outcome, err := o.complete(ctx, db, token, callback, client, nil)
	if err != nil {
		return nil, err
	}

	return outcome.login, nil
}

// oidcClaims は、ID トークンから読んだ、ユーザーを特定し、作るための値。
type oidcClaims struct {
	issuer  string
	subject string
	all     map[string]any
}

// roleFor は、groups のどれかが当たる、最も強いロールを返す。どれにも当たらなければ false を返す。
func (a *OIDCAutoCreate) roleFor(groups []string) (Role, bool) {
	for _, candidate := range []struct {
		role   Role
		groups []string
	}{
		{role: RoleAdmin, groups: a.AdminGroups},
		{role: RoleMember, groups: a.MemberGroups},
		{role: RoleViewer, groups: a.ViewerGroups},
	} {
		for _, group := range groups {
			if slices.Contains(candidate.groups, group) {
				return candidate.role, true
			}
		}
	}

	return "", false
}

// groupsOf は、グループの claim を文字列の一覧にする。文字列の配列か、1 つの文字列を受け付ける。
func groupsOf(claim any) []string {
	switch v := claim.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []any:
		groups := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				groups = append(groups, s)
			}
		}

		return groups
	default:
		return []string{}
	}
}

// Link は、Kotatsu のユーザー username を、IdP のユーザー subject と紐付ける。issuer は設定の IdP にする。
func (o *OIDC) Link(ctx context.Context, db Database, actor *audit.Actor, username, subject string) error {
	if !o.Enabled() {
		return ErrOIDCDisabled
	}
	if subject == "" {
		return fmt.Errorf("%w: the subject is empty", ErrInvalidUser)
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		user, err := sqlc.New(tx).GetUserByUsername(ctx, username)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to get the user: %w", err)
		}
		if err := o.link(ctx, tx, user.ID, o.settings.Issuer, subject, nil); err != nil {
			return err
		}

		return o.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "user.oidc_linked",
			TargetType: loginAuditTarget,
			TargetID:   user.ID.String(),
			Summary:    map[string]any{summaryIssuer: o.settings.Issuer, summarySubject: subject},
			SourceIP:   netip.Addr{},
			UserAgent:  "",
		})
	})
}

// oidcSessionRequest は、ログイン中のセッションで、IdP での再認証か紐付けを始めるときの、始めたセッションと、終えたときに戻すパス。
type oidcSessionRequest struct {
	sessionID uuid.UUID
	returnTo  string
	// link は、再認証ではなく、ログイン中のユーザーを IdP のユーザーと紐付けるかどうか。
	link bool
}

// begin は、IdP へ送り出すログインを始める。request があれば、ログイン中のセッションでの再認証か紐付けにする。
func (o *OIDC) begin(ctx context.Context, db Database, client *ClientInfo, request *oidcSessionRequest) (*OIDCLogin, error) {
	provider, err := o.loadProvider(ctx)
	if err != nil {
		return nil, err
	}
	redirectURI := o.settings.RedirectURL
	if redirectURI == "" {
		redirectURI = client.Origin() + OIDCCallbackPath
	}
	state, err := randomString()
	if err != nil {
		return nil, err
	}
	nonce, err := randomString()
	if err != nil {
		return nil, err
	}
	token, err := randomString()
	if err != nil {
		return nil, err
	}
	verifier := oauth2.GenerateVerifier()
	now := o.clock.Now()
	options := []oauth2.AuthCodeOption{oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)}
	var sessionID *uuid.UUID
	var returnTo *string
	link := false
	if request != nil {
		if !request.link {
			// IdP に、ログイン中でも、もう一度認証させる (OpenID Connect Core 1.0 3.1.2.1)。
			options = append(options, oauth2.SetAuthURLParam("prompt", "login"), oauth2.SetAuthURLParam("max_age", "0"))
		}
		sessionID, returnTo, link = &request.sessionID, &request.returnTo, request.link
	}
	login := &OIDCLogin{
		AuthorizationURL: o.oauth2Config(provider, redirectURI).AuthCodeURL(state, options...),
		Token:            token,
		ExpiresAt:        now.Add(OIDCLoginTTL),
	}
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if _, err := q.DeleteExpiredOIDCLogins(ctx, now); err != nil {
			return fmt.Errorf("failed to delete expired OIDC logins: %w", err)
		}
		err := q.InsertOIDCLogin(ctx, sqlc.InsertOIDCLoginParams{
			ID: id.New(), TokenHash: hashToken(token), State: state, Nonce: nonce, CodeVerifier: verifier,
			RedirectUri: redirectURI, SessionID: sessionID, ReturnTo: returnTo, Link: link, Now: now, ExpiresAt: login.ExpiresAt,
		})
		if err != nil {
			return fmt.Errorf("failed to save the OIDC login: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return login, nil
}

// oidcOutcome は、IdP から戻ってきたログイン、再認証、紐付けの結果。
type oidcOutcome struct {
	// login は、ログインで作ったセッション。再認証か紐付けなら nil。
	login *LoginResult
	// reauth は、再認証の結果。ログインか紐付けなら nil。
	reauth *OIDCReauthentication
	// linked は、紐付けを終えたかどうか。
	linked bool
	// returnTo は、再認証か紐付けを終えたときに戻すパス。失敗しても、途中の状態を引けたら入れる。
	returnTo string
}

// complete は、IdP から戻ってきたログイン、再認証、紐付けを確かめる。principal は、戻ってきたリクエストのログイン中のユーザー。
// 再認証か紐付けに失敗しても、途中の状態を引けたら、戻すパスを入れた結果を、エラーと一緒に返す。
func (o *OIDC) complete(
	ctx context.Context, db Database, token string, callback *OIDCCallback, client *ClientInfo, principal *Principal,
) (*oidcOutcome, error) {
	provider, err := o.loadProvider(ctx)
	if err != nil {
		return nil, err
	}
	var row sqlc.OidcLogin
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, err = sqlc.New(tx).TakeOIDCLogin(ctx, sqlc.TakeOIDCLoginParams{TokenHash: hashToken(token), Now: o.clock.Now()})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOIDCLoginExpired
		}
		if err != nil {
			return fmt.Errorf("failed to get the OIDC login: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	outcome := &oidcOutcome{login: nil, reauth: nil, linked: false, returnTo: ""}
	if row.ReturnTo != nil {
		outcome.returnTo = *row.ReturnTo
	}
	if subtle.ConstantTimeCompare([]byte(callback.State), []byte(row.State)) != 1 {
		return outcome, ErrOIDCStateMismatch
	}
	if callback.Error != "" {
		return outcome, fmt.Errorf("%w: the provider returned %s", ErrOIDCAccessDenied, callback.Error)
	}
	claims, err := o.verify(ctx, provider, &row, callback.Code)
	if err != nil {
		return outcome, err
	}
	if row.Link {
		err = o.linkSession(ctx, db, &row, claims, principal, client)
		outcome.linked = err == nil

		return outcome, err
	}
	if row.SessionID != nil {
		outcome.reauth, err = o.reauthenticate(ctx, db, &row, claims, principal, client)

		return outcome, err
	}
	outcome.login, err = o.login(ctx, db, claims, client)

	return outcome, err
}

// login は、確かめた ID トークンのユーザーで、セッションを作る。
func (o *OIDC) login(ctx context.Context, db Database, claims *oidcClaims, client *ClientInfo) (*LoginResult, error) {
	var (
		result  *LoginResult
		denyErr error
	)
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		userID, err := o.userFor(ctx, tx, claims, client)
		if errors.Is(err, ErrOIDCAccessDenied) || errors.Is(err, ErrOIDCUsernameTaken) {
			denyErr = err

			// 拒んだことは、ロールバックせずに監査ログに残す。
			return o.recordDenied(ctx, tx, claims, userID, client, err)
		}
		if err != nil {
			return err
		}
		result, err = startSession(ctx, tx, o.sessions, o.recorder, *userID, methodOIDC, client)

		return err
	})
	if err != nil {
		return nil, err
	}
	if denyErr != nil {
		return nil, denyErr
	}

	return result, nil
}

// verify は、認可コードを ID トークンと引き換え、署名、issuer、audience、期限、nonce を確かめる。
func (o *OIDC) verify(ctx context.Context, provider *oidc.Provider, row *sqlc.OidcLogin, code string) (*oidcClaims, error) {
	ctx = oidc.ClientContext(ctx, o.httpClient)
	token, err := o.oauth2Config(provider, row.RedirectUri).Exchange(ctx, code, oauth2.VerifierOption(row.CodeVerifier))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to exchange the code: %w", ErrOIDCTokenInvalid, err)
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok || raw == "" {
		return nil, fmt.Errorf("%w: the token response has no id_token", ErrOIDCTokenInvalid)
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: o.settings.ClientID, Now: o.clock.Now}).Verify(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCTokenInvalid, err)
	}
	// go-oidc は nonce を確かめないので、ここで確かめる。
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(row.Nonce)) != 1 {
		return nil, fmt.Errorf("%w: the nonce does not match", ErrOIDCTokenInvalid)
	}
	all := map[string]any{}
	if err := idToken.Claims(&all); err != nil {
		return nil, fmt.Errorf("%w: failed to read the claims: %w", ErrOIDCTokenInvalid, err)
	}

	return &oidcClaims{issuer: idToken.Issuer, subject: idToken.Subject, all: all}, nil
}

// userFor は、IdP のユーザーに紐付いた Kotatsu のユーザーを返す。いなければ、自動作成の条件を満たすときだけ作る。
func (o *OIDC) userFor(ctx context.Context, tx pgx.Tx, claims *oidcClaims, client *ClientInfo) (*uuid.UUID, error) {
	q := sqlc.New(tx)
	identity, err := q.GetOIDCIdentity(ctx, sqlc.GetOIDCIdentityParams{Issuer: claims.issuer, Subject: claims.subject})
	if err == nil {
		user, err := q.GetUser(ctx, identity.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to get the user: %w", err)
		}
		if user.DisabledAt != nil {
			return &user.ID, fmt.Errorf("%w: the user is disabled", ErrOIDCAccessDenied)
		}
		if err := q.RecordOIDCIdentityUse(ctx, sqlc.RecordOIDCIdentityUseParams{ID: identity.ID, Now: new(o.clock.Now())}); err != nil {
			return nil, fmt.Errorf("failed to record the OIDC identity use: %w", err)
		}

		return &user.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to get the OIDC identity: %w", err)
	}

	return o.autoCreate(ctx, tx, claims, client)
}

// autoCreate は、自動作成を有効にしていて、グループがロールのどれかに当たるユーザーを作る (設計書 9.4)。
func (o *OIDC) autoCreate(ctx context.Context, tx pgx.Tx, claims *oidcClaims, client *ClientInfo) (*uuid.UUID, error) {
	settings := o.settings.AutoCreate
	if !settings.Enabled {
		return nil, fmt.Errorf("%w: no user is linked and auto_create is disabled", ErrOIDCAccessDenied)
	}
	role, ok := settings.roleFor(groupsOf(claims.all[settings.GroupsClaim]))
	if !ok {
		return nil, fmt.Errorf("%w: the user is not in any allowed group", ErrOIDCAccessDenied)
	}
	username, _ := claims.all[o.settings.UsernameClaim].(string)
	if username == "" {
		return nil, fmt.Errorf("%w: the %s claim is missing", ErrOIDCAccessDenied, o.settings.UsernameClaim)
	}
	displayName, _ := claims.all["name"].(string)
	if displayName == "" {
		displayName = username
	}
	// ユーザー名が重なると、INSERT が失敗してトランザクションが使えなくなる。拒んだことを監査ログに残せるよう、
	// セーブポイントの中で作る。
	var created *User
	err := pgx.BeginFunc(ctx, tx, func(sp pgx.Tx) error {
		var err error
		created, err = NewUsers(o.clock).Create(ctx, sp, &NewUser{Username: username, DisplayName: displayName, Role: role, PasswordHash: nil})

		return err
	})
	if errors.Is(err, ErrUsernameTaken) {
		return nil, fmt.Errorf("%w: %s", ErrOIDCUsernameTaken, username)
	}
	if errors.Is(err, ErrInvalidUser) {
		return nil, fmt.Errorf("%w: %w", ErrOIDCAccessDenied, err)
	}
	if err != nil {
		return nil, err
	}
	// 作ったユーザーは、このログインで使ったことにする。
	if err := o.link(ctx, tx, created.ID, claims.issuer, claims.subject, new(o.clock.Now())); err != nil {
		return nil, err
	}
	err = o.recorder.Record(ctx, tx, &audit.Event{
		Actor:      audit.User(created.ID),
		Action:     "user.created_by_oidc",
		TargetType: loginAuditTarget,
		TargetID:   created.ID.String(),
		Summary:    map[string]any{summaryUsername: created.Username, summaryRole: string(role), summaryIssuer: claims.issuer, summarySubject: claims.subject},
		SourceIP:   client.SourceIP,
		UserAgent:  client.UserAgent,
	})
	if err != nil {
		return nil, err
	}

	return &created.ID, nil
}

// link は、userID を IdP のユーザーと紐付ける。lastUsedAt は、紐付けたときにログインに使ったなら、その日時。
func (o *OIDC) link(ctx context.Context, tx pgx.Tx, userID uuid.UUID, issuer, subject string, lastUsedAt *time.Time) error {
	q := sqlc.New(tx)
	if _, err := q.GetOIDCIdentity(ctx, sqlc.GetOIDCIdentityParams{Issuer: issuer, Subject: subject}); err == nil {
		return ErrOIDCIdentityLinked
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("failed to get the OIDC identity: %w", err)
	}
	err := q.InsertOIDCIdentity(ctx, sqlc.InsertOIDCIdentityParams{
		ID: id.New(), UserID: userID, Issuer: issuer, Subject: subject, Now: o.clock.Now(), LastUsedAt: lastUsedAt,
	})
	if err != nil {
		return fmt.Errorf("failed to link the OIDC identity: %w", err)
	}

	return nil
}

func (o *OIDC) recordDenied(ctx context.Context, tx pgx.Tx, claims *oidcClaims, userID *uuid.UUID, client *ClientInfo, reason error) error {
	actor, targetID := audit.System("tenban"), ""
	if userID != nil {
		actor, targetID = audit.User(*userID), userID.String()
	}

	return o.recorder.Record(ctx, tx, &audit.Event{
		Actor:      actor,
		Action:     actionLoginFailed,
		TargetType: loginAuditTarget,
		TargetID:   targetID,
		Summary: map[string]any{
			summaryMethod: string(methodOIDC), summaryIssuer: claims.issuer, summarySubject: claims.subject, "reason": reason.Error(),
		},
		SourceIP:  client.SourceIP,
		UserAgent: client.UserAgent,
	})
}

// loadProvider は、IdP の設定を引く。引けたら覚えておき、引けなければ次に使うときに引き直す。
func (o *OIDC) loadProvider(ctx context.Context) (*oidc.Provider, error) {
	if !o.Enabled() {
		return nil, ErrOIDCDisabled
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider != nil {
		return o.provider, nil
	}
	// IdP が応答しないときに、ロックを持ったまま待ち続けないよう、時間を区切る。
	// go-oidc は、署名の鍵を引き直すときには、この context ではなく、クライアントだけを使う。
	ctx, cancel := context.WithTimeout(ctx, oidcHTTPTimeout)
	defer cancel()
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, o.httpClient), o.settings.Issuer)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOIDCUnavailable, err)
	}
	o.provider = provider

	return provider, nil
}

func (o *OIDC) oauth2Config(provider *oidc.Provider, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     o.settings.ClientID,
		ClientSecret: o.settings.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       o.settings.Scopes,
	}
}

func randomString() (string, error) {
	raw := make([]byte, oidcRandomBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate a random value: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}
