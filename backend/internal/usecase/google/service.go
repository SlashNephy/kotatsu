// Package google は、管理者が登録した Google の OAuth クライアントと、それで接続した Google Drive (ライブラリ、設計書 5.8、#1902) を扱う。
//
// 管理者は、Google Cloud で OAuth クライアント (ウェブアプリケーション) を作り、Kotatsu のリダイレクト URI
// (/internal/auth/google/callback) を登録して、クライアント ID とシークレットを連携の画面で登録する。リダイレクト URI は、
// ほかの Google の連携でも共有できるよう、何のための連携か (purpose) を認可の途中の状態に持たせ、戻ってきたときに振り分ける。
//
// Google Drive の接続は、サーバーに 1 つ (管理者のアカウント) だけ持つ。スコープは drive.readonly だけにし、認可コードフローに
// PKCE (S256) と state を組み合わせる。リフレッシュトークンは、アプリの秘密キーで暗号化して DB に置き、アクセストークンは
// 各ロール (tenban、kyusu、nekonote) が手元で作り直す。
package google

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/googledrive"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/secretbox"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

const (
	// CallbackPath は、Google から戻ってくるリダイレクト URI のパス。Google の連携で共有するので、パスに何のための連携かを入れない。
	CallbackPath = "/internal/auth/google/callback"
	// DriveScope は、Google Drive を読むのに求めるスコープ。読み取りだけにする。
	DriveScope = "https://www.googleapis.com/auth/drive.readonly"
	// PurposeDrive は、Google Drive をライブラリとして接続する認可。
	PurposeDrive = "drive"
	// appSealPurpose は、クライアントシークレットを暗号化する用途 (設計書 8.4)。
	appSealPurpose = "kotatsu/google-app/v1"
	// driveSealPurpose は、Google Drive のリフレッシュトークンを暗号化する用途 (設計書 8.4)。
	driveSealPurpose = "kotatsu/google-drive-token/v1"
	// authorizationTTL は、認可を始めてから戻ってくるまでの時間の上限。
	authorizationTTL = 10 * time.Minute
	// stateBytes は、state の乱数の長さ。
	stateBytes = 32
	// maxClientIDLength と maxClientSecretLength は、管理者が入れるクライアント ID とシークレットの長さの上限。
	maxClientIDLength     = 256
	maxClientSecretLength = 1024
	// maxErrorLength は、Google が返したエラーを、ログとエラーに残す長さの上限。
	maxErrorLength = 64
	// auditTargetApp と auditTargetDrive は、監査ログの対象の種類。
	auditTargetApp   = "google_app"
	auditTargetDrive = "google_drive_connection"
	// defaultAccountsURL と defaultOAuthURL は、Google の認可の画面と、トークンの API のオリジン。
	defaultAccountsURL = "https://accounts.google.com"
	defaultOAuthURL    = "https://oauth2.googleapis.com"
)

var (
	// ErrInvalidApp は、管理者が入れたクライアントの値が正しくないことを表す。
	ErrInvalidApp = errors.New("invalid Google OAuth client")
	// ErrUnavailable は、管理者が Google の OAuth クライアントを登録していないことを表す。
	ErrUnavailable = errors.New("the Google OAuth client is not configured")
	// ErrSessionRequired は、ブラウザーのセッションでない (API トークンや MCP のクライアントの) 操作であることを表す。
	ErrSessionRequired = errors.New("a browser session is required")
	// ErrAuthorizationExpired は、認可の途中の状態が見つからないか、期限を過ぎたことを表す。
	ErrAuthorizationExpired = errors.New("the authorization has expired")
	// ErrAuthorizationMismatch は、認可を始めたセッションと、戻ってきたリクエストが違うことを表す。
	ErrAuthorizationMismatch = errors.New("the authorization does not match the session")
	// ErrAccessDenied は、管理者が Google で認可を断ったか、求めたスコープを許さなかったことを表す。
	ErrAccessDenied = errors.New("the authorization was denied")
	// ErrNoRefreshToken は、Google がリフレッシュトークンを返さなかったことを表す。
	ErrNoRefreshToken = errors.New("no refresh token was returned")
	// ErrNotConnected は、Google Drive を接続していないか、接続が切れていることを表す。
	ErrNotConnected = errors.New("the Google Drive is not connected")
)

// Database は、トランザクションを張れる DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Service は、Google の OAuth クライアントと、Google Drive の接続を扱う。
type Service struct {
	clock    clock.Clock
	recorder audit.Recorder
	appBox   *secretbox.Box
	driveBox *secretbox.Box
	http     *http.Client
	logger   *slog.Logger
	// accountsURL、oauthURL、apiURL は、Google の認可の画面、トークンの API、Drive API のオリジン。
	accountsURL, oauthURL, apiURL string
	// tokensMu と tokens は、この部品が作った DriveTokens。接続を変えたら、手元で覚えた接続を捨てさせ、すぐに反映する。
	tokensMu sync.Mutex
	tokens   []*DriveTokens
}

// Options は NewService の設定。
type Options struct {
	Clock    clock.Clock
	Recorder audit.Recorder
	// AppSecret は、アプリの秘密キー (設計書 8.4)。
	AppSecret string
	// HTTPClient は、Google のトークンの API と Drive API を呼ぶクライアント。全体のタイムアウトを付けない。
	HTTPClient *http.Client
	Logger     *slog.Logger
	// URL は、Google の認可の画面と API を向ける先のオリジン。空なら本物の Google にする。検証用のサーバーへ向けるときだけ使う。
	URL string
}

// NewService は Service を返す。
func NewService(opts *Options) (*Service, error) {
	appBox, err := secretbox.New(opts.AppSecret, appSealPurpose)
	if err != nil {
		return nil, err
	}
	driveBox, err := secretbox.New(opts.AppSecret, driveSealPurpose)
	if err != nil {
		return nil, err
	}
	accountsURL, oauthURL, apiURL := defaultAccountsURL, defaultOAuthURL, googledrive.DefaultURL
	if opts.URL != "" {
		base := strings.TrimSuffix(opts.URL, "/")
		accountsURL, oauthURL, apiURL = base, base, base
	}

	return &Service{
		clock: opts.Clock, recorder: opts.Recorder, appBox: appBox, driveBox: driveBox, http: opts.HTTPClient, logger: opts.Logger,
		accountsURL: accountsURL, oauthURL: oauthURL, apiURL: apiURL, tokensMu: sync.Mutex{}, tokens: []*DriveTokens{},
	}, nil
}

// RedirectURI は、接続したオリジン origin から、Google に登録するリダイレクト URI を返す。
func RedirectURI(origin string) string {
	return origin + CallbackPath
}

// AppStatus は、管理者が登録した Google の OAuth クライアントの様子。クライアントシークレットは返さない。
type AppStatus struct {
	// ClientID は、登録したクライアント ID。登録していなければ空。
	ClientID   string
	Configured bool
	UpdatedAt  *time.Time
}

// App は、登録した Google の OAuth クライアントの様子を返す。
func (s *Service) App(ctx context.Context, db sqlc.DBTX) (*AppStatus, error) {
	row, err := sqlc.New(db).GetGoogleApp(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return &AppStatus{ClientID: "", Configured: false, UpdatedAt: nil}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the Google OAuth client: %w", err)
	}

	return &AppStatus{ClientID: row.ClientID, Configured: true, UpdatedAt: new(row.UpdatedAt)}, nil
}

// AppChanges は、管理者が変える Google の OAuth クライアントの値。
type AppChanges struct {
	// ClientID は、クライアント ID。空なら、登録を消す。
	ClientID string
	// ClientSecret は、クライアントシークレット。nil なら、登録したものを変えない。
	ClientSecret *string
}

// UpdateApp は、Google の OAuth クライアントを変える。クライアント ID を空にすると、登録を消し、Google Drive の接続も外す
// (クライアントがなければ、リフレッシュトークンを使えないため)。サーバーが Google と通信し始めるかどうかの元になる値なので、
// 呼ぶ側で、直近の再認証を確かめる (設計書 9.5)。監査ログには、シークレットを変えたかどうかだけを残す。
func (s *Service) UpdateApp(ctx context.Context, db Database, actor *audit.Actor, changes *AppChanges) error {
	clientID := strings.TrimSpace(changes.ClientID)
	if utf8.RuneCountInString(clientID) > maxClientIDLength {
		return fmt.Errorf("%w: the client ID is too long", ErrInvalidApp)
	}
	if clientID == "" {
		return s.deleteApp(ctx, db, actor)
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		current, err := q.GetGoogleApp(ctx)
		exists := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("failed to get the Google OAuth client: %w", err)
		}
		encrypted := current.ClientSecretEncrypted
		secretChanged := changes.ClientSecret != nil
		if secretChanged {
			secret := strings.TrimSpace(*changes.ClientSecret)
			if secret == "" || utf8.RuneCountInString(secret) > maxClientSecretLength {
				return fmt.Errorf("%w: the client secret must be 1 to %d characters", ErrInvalidApp, maxClientSecretLength)
			}
			if encrypted, err = s.appBox.Seal([]byte(secret)); err != nil {
				return err
			}
		} else if !exists {
			return fmt.Errorf("%w: the client secret is required", ErrInvalidApp)
		}
		if err := q.UpsertGoogleApp(ctx, sqlc.UpsertGoogleAppParams{ClientID: clientID, ClientSecretEncrypted: encrypted, Now: s.clock.Now()}); err != nil {
			return fmt.Errorf("failed to save the Google OAuth client: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: "google_app.update", TargetType: auditTargetApp, TargetID: "google",
			Summary: map[string]any{
				"clientId":            map[string]any{"from": current.ClientID, "to": clientID},
				"clientSecretChanged": secretChanged,
			},
			SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
}

// DriveConnection は、Google Drive の接続の様子。トークンは返さない。
type DriveConnection struct {
	AccountName string
	ConnectedAt time.Time
	// RevokedAt は、リフレッシュトークンが使えなくなったと分かった日時。使えていれば nil。接続し直すまで、Drive を読まない。
	RevokedAt *time.Time
}

// Drive は、Google Drive の接続の様子を返す。接続していなければ ErrNotConnected を返す。
func (s *Service) Drive(ctx context.Context, db sqlc.DBTX) (*DriveConnection, error) {
	row, err := sqlc.New(db).GetGoogleDriveConnection(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotConnected
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the Google Drive connection: %w", err)
	}

	return &DriveConnection{AccountName: row.AccountName, ConnectedAt: row.ConnectedAt, RevokedAt: row.RevokedAt}, nil
}

// BeginDrive は、管理者 principal のアカウントで Google Drive を接続する認可を始め、ブラウザーを送り出す Google の認可の画面の URL を返す。
// 戻ってきたときに、始めたセッションのまま戻ってきたことを確かめるので、ブラウザーのセッションでだけ始められる。
// origin は、接続したオリジン。リダイレクト URI にする。
func (s *Service) BeginDrive(ctx context.Context, db Database, principal *auth.Principal, origin string) (string, error) {
	if principal.SessionID == uuid.Nil() {
		return "", ErrSessionRequired
	}
	config, err := s.oauthConfig(ctx, db, RedirectURI(origin))
	if err != nil {
		return "", err
	}
	state, err := randomString()
	if err != nil {
		return "", err
	}
	verifier := oauth2.GenerateVerifier()
	now := s.clock.Now()
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if _, err := q.DeleteExpiredGoogleAuthorizations(ctx, now); err != nil {
			return fmt.Errorf("failed to delete expired Google authorizations: %w", err)
		}
		if err := q.InsertGoogleAuthorization(ctx, sqlc.InsertGoogleAuthorizationParams{
			ID: id.New(), StateHash: hashState(state), SessionID: principal.SessionID, UserID: principal.UserID, Purpose: PurposeDrive,
			CodeVerifier: verifier, RedirectUri: config.RedirectURL, Now: now, ExpiresAt: now.Add(authorizationTTL),
		}); err != nil {
			return fmt.Errorf("failed to save the Google authorization: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	// リフレッシュトークンは、access_type=offline のときだけ返る。前に許可したアカウントでも返るよう、同意の画面を出させる。
	return config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("include_granted_scopes", "false"), oauth2.S256ChallengeOption(verifier)), nil
}

// Callback は、Google から戻ってきたリクエストの中身。
type Callback struct {
	State string
	Code  string
	// Error は、Google が返したエラー (access_denied など)。
	Error string
	// Scope は、許されたスコープ (空白区切り)。
	Scope string
}

// Complete は、Google から戻ってきた認可を確かめ、始めたときの purpose に応じて接続する。接続した purpose を返す。
// 途中の状態は、確かめられなくても使用済みにする。始めたセッションが、戻ってきたリクエストと同じであることを確かめる。
// ほかの端末やアカウントで始めた認可を、別のセッションに繋がせないため (CSRF とアカウントの取り違えを防ぐ)。
func (s *Service) Complete(ctx context.Context, db Database, principal *auth.Principal, callback *Callback, client *auth.ClientInfo) (string, error) {
	if callback.State == "" {
		return "", ErrAuthorizationExpired
	}
	row, err := sqlc.New(db).TakeGoogleAuthorization(ctx, sqlc.TakeGoogleAuthorizationParams{StateHash: hashState(callback.State), Now: s.clock.Now()})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAuthorizationExpired
	}
	if err != nil {
		return "", fmt.Errorf("failed to get the Google authorization: %w", err)
	}
	if principal == nil || principal.SessionID != row.SessionID || principal.UserID != row.UserID {
		return row.Purpose, ErrAuthorizationMismatch
	}
	if callback.Error != "" {
		return row.Purpose, fmt.Errorf("%w: the authorization server returned %s", ErrAccessDenied, shorten(callback.Error))
	}
	if callback.Code == "" {
		return row.Purpose, fmt.Errorf("%w: no authorization code", ErrAccessDenied)
	}
	switch row.Purpose {
	case PurposeDrive:
		return row.Purpose, s.completeDrive(ctx, db, principal, &row, callback, client)
	default:
		return row.Purpose, fmt.Errorf("%w: unknown purpose %q", ErrAuthorizationMismatch, row.Purpose)
	}
}

// DisconnectDrive は、Google Drive の接続を外し、リフレッシュトークンを Google で取り消す。取り消せなくても、Kotatsu からは消す。
func (s *Service) DisconnectDrive(ctx context.Context, db Database, actor *audit.Actor) error {
	var token string
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, err := sqlc.New(tx).DeleteGoogleDriveConnection(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotConnected
		}
		if err != nil {
			return fmt.Errorf("failed to delete the Google Drive connection: %w", err)
		}
		if plain, err := s.driveBox.Open(row.RefreshTokenEncrypted); err == nil {
			token = string(plain)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: "google_drive.disconnect", TargetType: auditTargetDrive, TargetID: "google-drive",
			Summary: map[string]any{"account": row.AccountName}, SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
	if err != nil {
		return err
	}
	s.forgetConnections()
	s.revoke(ctx, token)

	return nil
}

// completeDrive は、認可コードをトークンと引き換え、Google Drive の接続を保存する。
func (s *Service) completeDrive(
	ctx context.Context, db Database, principal *auth.Principal, row *sqlc.GoogleAuthorization, callback *Callback, client *auth.ClientInfo,
) error {
	// 細かい同意の画面で、Drive を読むことを許さなかったなら、接続しない。
	if callback.Scope != "" && !strings.Contains(" "+callback.Scope+" ", " "+DriveScope+" ") {
		return fmt.Errorf("%w: the Drive scope was not granted", ErrAccessDenied)
	}
	config, err := s.oauthConfig(ctx, db, row.RedirectUri)
	if err != nil {
		return err
	}
	token, err := config.Exchange(s.httpContext(ctx), callback.Code, oauth2.VerifierOption(row.CodeVerifier))
	if err != nil {
		return fmt.Errorf("failed to exchange the authorization code: %w", tokenError(err))
	}
	if token.RefreshToken == "" {
		return ErrNoRefreshToken
	}
	api := googledrive.NewClient(s.http, s.apiURL, &staticToken{token: token.AccessToken})
	account, err := api.About(ctx)
	if err != nil {
		return fmt.Errorf("failed to get the Google account: %w", err)
	}
	encrypted, err := s.driveBox.Seal([]byte(token.RefreshToken))
	if err != nil {
		return err
	}
	var previous string

	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if current, err := q.GetGoogleDriveConnection(ctx); err == nil {
			if plain, err := s.driveBox.Open(current.RefreshTokenEncrypted); err == nil {
				previous = string(plain)
			}
		}
		if err := q.UpsertGoogleDriveConnection(ctx, sqlc.UpsertGoogleDriveConnectionParams{
			RefreshTokenEncrypted: encrypted, AccountName: account, Now: s.clock.Now(),
		}); err != nil {
			return fmt.Errorf("failed to save the Google Drive connection: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: audit.User(principal.UserID), Action: "google_drive.connect", TargetType: auditTargetDrive, TargetID: "google-drive",
			Summary: map[string]any{"account": account}, SourceIP: client.SourceIP, UserAgent: client.UserAgent,
		})
	})
	if err != nil {
		return err
	}
	s.forgetConnections()
	// 接続し直したら、前のリフレッシュトークンは使わないので取り消す。
	if previous != "" && previous != token.RefreshToken {
		s.revoke(ctx, previous)
	}

	return nil
}

// deleteApp は、Google の OAuth クライアントの登録を消し、Google Drive の接続も外す。リフレッシュトークンは、消す前に Google で取り消す。
func (s *Service) deleteApp(ctx context.Context, db Database, actor *audit.Actor) error {
	var token string
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		deleted, err := q.DeleteGoogleApp(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete the Google OAuth client: %w", err)
		}
		if deleted == 0 {
			return nil
		}
		connection, err := q.DeleteGoogleDriveConnection(ctx)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return fmt.Errorf("failed to delete the Google Drive connection: %w", err)
		default:
			if plain, err := s.driveBox.Open(connection.RefreshTokenEncrypted); err == nil {
				token = string(plain)
			}
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: "google_app.delete", TargetType: auditTargetApp, TargetID: "google",
			Summary: map[string]any{"driveDisconnected": err == nil}, SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
	if err != nil {
		return err
	}
	s.forgetConnections()
	s.revoke(ctx, token)

	return nil
}

// forgetConnections は、この部品が作った DriveTokens に、覚えた接続を捨てさせる。ほかのプロセスは、connectionTTL のうちに気づく。
func (s *Service) forgetConnections() {
	s.tokensMu.Lock()
	defer s.tokensMu.Unlock()
	for _, tokens := range s.tokens {
		tokens.forget()
	}
}

// revoke は、トークンを Google で取り消す。失敗しても、ログに残すだけにする。
func (s *Service) revoke(ctx context.Context, token string) {
	if token == "" {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.oauthURL+"/revoke",
		strings.NewReader(url.Values{"token": {token}}.Encode()))
	if err != nil {
		s.logger.WarnContext(ctx, "failed to revoke the Google token", slog.Any("error", err))

		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := s.http.Do(req)
	if err != nil {
		s.logger.WarnContext(ctx, "failed to revoke the Google token", slog.Any("error", err))

		return
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		s.logger.WarnContext(ctx, "failed to revoke the Google token", slog.Int("status", res.StatusCode))
	}
}

// oauthConfig は、登録した OAuth クライアントの設定を返す。登録していなければ ErrUnavailable を返す。
func (s *Service) oauthConfig(ctx context.Context, db sqlc.DBTX, redirectURI string) (*oauth2.Config, error) {
	row, err := sqlc.New(db).GetGoogleApp(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the Google OAuth client: %w", err)
	}
	secret, err := s.appBox.Open(row.ClientSecretEncrypted)
	if err != nil {
		return nil, err
	}

	return &oauth2.Config{
		ClientID: row.ClientID, ClientSecret: string(secret), RedirectURL: redirectURI, Scopes: []string{DriveScope},
		Endpoint: oauth2.Endpoint{
			AuthURL: s.accountsURL + "/o/oauth2/v2/auth", DeviceAuthURL: "", TokenURL: s.oauthURL + "/token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}, nil
}

// httpContext は、oauth2 がトークンの API を呼ぶときに、外部との通信を記録するクライアントを使わせる。
func (s *Service) httpContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, s.http)
}

// tokenError は、Google のトークンの API のエラーから、トークンなどの中身を除いて、エラーの種類だけを残す。
func tokenError(err error) error {
	if retrieve, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
		code := retrieve.ErrorCode
		if code == "" {
			code = fmt.Sprintf("status %d", retrieve.Response.StatusCode)
		}

		return fmt.Errorf("the token endpoint returned %s", shorten(code))
	}

	return err
}

// staticToken は、引き換えたばかりのアクセストークンをそのまま返す。接続したアカウントを引くのに使う。
type staticToken struct {
	token string
}

func (t *staticToken) Token(context.Context) (string, error) { return t.token, nil }

func (t *staticToken) Invalidate() {}

// hashState は、state を DB に置く形にする。DB が漏れても、戻ってくるリクエストを作れないようにする。
func hashState(state string) []byte {
	sum := sha256.Sum256([]byte(state))

	return sum[:]
}

func randomString() (string, error) {
	raw := make([]byte, stateBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate a random value: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// shorten は、外から来た文字列を、ログとエラーに残す長さに切る。
func shorten(s string) string {
	if utf8.RuneCountInString(s) <= maxErrorLength {
		return s
	}

	return string([]rune(s)[:maxErrorLength])
}
