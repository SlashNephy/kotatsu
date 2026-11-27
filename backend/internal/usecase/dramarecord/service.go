package dramarecord

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/logattr"
	"github.com/SlashNephy/kotatsu/internal/lib/secretbox"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/usersettings"
)

const (
	// appSealPurpose は、管理者が登録したアプリのクライアントシークレットを暗号化する用途 (設計書 8.4)。
	appSealPurpose = "kotatsu/drama-record-app/v1"
	// connectionSealPurpose は、ユーザーのアクセストークンと更新のトークンを暗号化する用途 (設計書 8.4)。
	connectionSealPurpose = "kotatsu/drama-record-token/v1"
	// callbackPathPrefix は、記録先から戻ってくるリダイレクト URI のパスの前半。後ろに記録先の名前と /callback を付ける (#1906)。
	callbackPathPrefix = "/internal/auth/"
	// authorizationTTL は、認可を始めてから戻ってくるまでの時間の上限。
	authorizationTTL = 10 * time.Minute
	// stateBytes は、state の乱数の長さ。
	stateBytes = 32
	// maxClientIDLength と maxClientSecretLength は、管理者が入れるクライアント ID とシークレットの長さの上限。
	maxClientIDLength     = 256
	maxClientSecretLength = 1024
	// refreshMargin は、アクセストークンの期限のどれだけ前から更新するか。Simkl は、期限の 1 日ほど前に更新するよう勧めている。
	refreshMargin = 24 * time.Hour
	// auditTargetApp と auditTargetConnection は、監査ログの対象の種類。
	auditTargetApp        = "drama_record_app"
	auditTargetConnection = "drama_record_connection"
	summaryDestination    = "destination"
)

var (
	// ErrUnavailable は、管理者が TMDB との連携を無効にしているか、アプリを登録していないことを表す。
	ErrUnavailable = errors.New("drama records are not available")
	// ErrInvalidApp は、管理者が入れたアプリの値が正しくないことを表す。
	ErrInvalidApp = errors.New("invalid drama record app")
	// ErrSessionRequired は、ブラウザーのセッションでない (API トークンや MCP のクライアントの) 操作であることを表す。
	ErrSessionRequired = errors.New("a browser session is required")
	// ErrAuthorizationExpired は、認可の途中の状態が見つからないか、期限を過ぎたことを表す。
	ErrAuthorizationExpired = errors.New("the authorization has expired")
	// ErrAuthorizationMismatch は、認可を始めたセッションや記録先と、戻ってきたリクエストが違うことを表す。
	ErrAuthorizationMismatch = errors.New("the authorization does not match the session")
	// ErrAccessDenied は、利用者が記録先で認可を断ったことを表す。
	ErrAccessDenied = errors.New("the authorization was denied")
	// ErrNotConnected は、記録先と連携していないことを表す。
	ErrNotConnected = errors.New("the destination is not connected")
)

var _ auth.UserDataEraser = new(Service)

// Service は、ドラマの記録先のアプリと、ユーザーごとの連携を扱う。
type Service struct {
	clock        clock.Clock
	recorder     audit.Recorder
	appBox       *secretbox.Box
	tokenBox     *secretbox.Box
	destinations map[string]Destination
	// order は、画面に並べる記録先の順。
	order  []string
	logger *slog.Logger
	// inserter は、連携したときに、ウォッチリストを取り込むジョブを入れる River のクライアント。nil なら入れない。
	inserter anime.Inserter
	// requestMu は、記録先への問い合わせを、このプロセスの中で 1 つずつにする。requestLast は、前の問い合わせの時刻。
	// Simkl は、書き込みを 1 秒に 1 回まで、読み取りを 1 秒に 10 回までとし、超えた書き込みはしばらく止める。
	requestMu       sync.Mutex
	requestLast     time.Time
	requestInterval time.Duration
}

// Options は NewService の設定。
type Options struct {
	Clock    clock.Clock
	Recorder audit.Recorder
	// AppSecret は、アプリの秘密キー (設計書 8.4)。
	AppSecret    string
	Destinations []Destination
	Logger       *slog.Logger
	// Inserter は、連携したときに、ウォッチリストを取り込むジョブを入れる River のクライアント。nil なら入れない。
	Inserter anime.Inserter
	// RequestInterval は、記録先への問い合わせの間を空ける長さ。0 なら空けない。
	RequestInterval time.Duration
}

// DefaultRequestInterval は、記録先への問い合わせの間を空ける長さ。Simkl の書き込みの上限 (1 秒に 1 回) を超えないようにする。
const DefaultRequestInterval = 1100 * time.Millisecond

// NewService は Service を返す。
func NewService(opts *Options) (*Service, error) {
	appBox, err := secretbox.New(opts.AppSecret, appSealPurpose)
	if err != nil {
		return nil, err
	}
	tokenBox, err := secretbox.New(opts.AppSecret, connectionSealPurpose)
	if err != nil {
		return nil, err
	}
	destinations := make(map[string]Destination, len(opts.Destinations))
	order := make([]string, 0, len(opts.Destinations))
	for _, destination := range opts.Destinations {
		destinations[destination.Name()] = destination
		order = append(order, destination.Name())
	}

	return &Service{
		clock: opts.Clock, recorder: opts.Recorder, appBox: appBox, tokenBox: tokenBox, destinations: destinations, order: order,
		logger: opts.Logger, inserter: opts.Inserter, requestMu: sync.Mutex{}, requestLast: time.Time{}, requestInterval: opts.RequestInterval,
	}, nil
}

// Destinations は、記録先の名前を、画面に並べる順に返す。
func (s *Service) Destinations() []string {
	return append([]string{}, s.order...)
}

// AppStatus は、管理者が記録先に登録したアプリの様子。クライアントシークレットは返さない。
type AppStatus struct {
	Destination string
	// ClientID は、登録したアプリのクライアント ID。登録していなければ空。
	ClientID string
	// Configured は、クライアント ID とシークレットを登録したかどうか。
	Configured bool
	UpdatedAt  *time.Time
}

// Apps は、記録先ごとに、管理者が登録したアプリの様子を返す。
func (s *Service) Apps(ctx context.Context, db sqlc.DBTX) ([]*AppStatus, error) {
	rows, err := sqlc.New(db).ListDramaRecordApps(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the drama record apps: %w", err)
	}
	byName := make(map[string]*sqlc.DramaRecordApp, len(rows))
	for i := range rows {
		byName[rows[i].Destination] = &rows[i]
	}
	apps := make([]*AppStatus, 0, len(s.order))
	for _, name := range s.order {
		status := &AppStatus{Destination: name, ClientID: "", Configured: false, UpdatedAt: nil}
		if row, ok := byName[name]; ok {
			status.ClientID, status.Configured, status.UpdatedAt = row.ClientID, true, new(row.UpdatedAt)
		}
		apps = append(apps, status)
	}

	return apps, nil
}

// AppChanges は、管理者が変えるアプリの値。
type AppChanges struct {
	// ClientID は、クライアント ID。空なら、登録を消す。
	ClientID string
	// ClientSecret は、クライアントシークレット。nil なら、登録したものを変えない。
	ClientSecret *string
}

// Database は、トランザクションを張れる DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// UpdateApp は、記録先 name に登録したアプリを変える。クライアント ID を空にすると、登録を消す。
// 利用者を記録先へ送り出す操作の元になる値なので、呼ぶ側で、直近の再認証を確認する (設計書 9.5)。
// 監査ログには、クライアントシークレットを変えたかどうかだけを残し、値は残さない。
func (s *Service) UpdateApp(ctx context.Context, db Database, actor *audit.Actor, name string, changes *AppChanges) error {
	if _, err := s.destination(name); err != nil {
		return err
	}
	clientID := strings.TrimSpace(changes.ClientID)
	if utf8.RuneCountInString(clientID) > maxClientIDLength {
		return fmt.Errorf("%w: the client ID is too long", ErrInvalidApp)
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if clientID == "" {
			deleted, err := q.DeleteDramaRecordApp(ctx, name)
			if err != nil {
				return fmt.Errorf("failed to delete the drama record app: %w", err)
			}
			if deleted == 0 {
				return nil
			}

			return s.recorder.Record(ctx, tx, &audit.Event{
				Actor: actor, Action: "drama_record_app.delete", TargetType: auditTargetApp, TargetID: name,
				Summary: map[string]any{}, SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
			})
		}
		current, err := q.GetDramaRecordApp(ctx, name)
		exists := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("failed to get the drama record app: %w", err)
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
		if err := q.UpsertDramaRecordApp(ctx, sqlc.UpsertDramaRecordAppParams{
			Destination: name, ClientID: clientID, ClientSecretEncrypted: encrypted, Now: s.clock.Now(),
		}); err != nil {
			return fmt.Errorf("failed to save the drama record app: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: "drama_record_app.update", TargetType: auditTargetApp, TargetID: name,
			Summary: map[string]any{
				"clientId":            map[string]any{"from": current.ClientID, "to": clientID},
				"clientSecretChanged": secretChanged,
			},
			SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
}

// Availability は、記録先ごとに、利用者が連携を始められるかどうか。
type Availability struct {
	// LinkingEnabled は、管理者が TMDB との連携を有効にしているかどうか (設計書 3.9)。
	LinkingEnabled bool
	// Configured は、記録先の名前から、管理者がアプリを登録したかどうかへのマップ。
	Configured map[string]bool
}

// Available は、利用者が記録先と連携を始められるかどうかを返す。
func (s *Service) Available(ctx context.Context, db sqlc.DBTX) (*Availability, error) {
	system, err := settings.Load(ctx, db)
	if err != nil {
		return nil, err
	}
	apps, err := s.Apps(ctx, db)
	if err != nil {
		return nil, err
	}
	configured := make(map[string]bool, len(apps))
	for _, app := range apps {
		configured[app.Destination] = app.Configured
	}

	return &Availability{LinkingEnabled: system.DramaLinkingEnabled, Configured: configured}, nil
}

// CallbackPath は、記録先 name から戻ってくるリダイレクト URI のパス (/internal/auth/<記録先>/callback) を返す。
func CallbackPath(name string) string {
	return callbackPathPrefix + name + "/callback"
}

// RedirectURI は、記録先 name のリダイレクト URI を、接続したオリジン origin から返す。管理者は、これを記録先に登録する。
func RedirectURI(origin, name string) string {
	return origin + CallbackPath(name)
}

// Connection は、ユーザーと記録先の連携。トークンは返さない。
type Connection struct {
	Destination string
	AccountName string
	ConnectedAt time.Time
	// ReauthorizationRequired は、更新のトークンが拒否され、連携し直す必要があるかどうか。
	ReauthorizationRequired bool
	// WatchlistCheckedAt と WatchlistImportedAt は、ウォッチリストを最後に問い合わせた日時と、最後に取り込めた日時。
	WatchlistCheckedAt  *time.Time
	WatchlistImportedAt *time.Time
	// WatchlistError は、最後の取り込みが失敗した理由。取り込めていれば nil。
	WatchlistError *string
}

func connectionOf(row *sqlc.DramaRecordConnection) *Connection {
	return &Connection{
		Destination: row.Destination, AccountName: row.AccountName, ConnectedAt: row.ConnectedAt,
		ReauthorizationRequired: row.ReauthorizationRequired, WatchlistCheckedAt: row.WatchlistCheckedAt,
		WatchlistImportedAt: row.WatchlistImportedAt, WatchlistError: row.WatchlistError,
	}
}

// Connections は、ユーザー userID の連携を、記録先の名前の順に返す。
func (s *Service) Connections(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) ([]*Connection, error) {
	rows, err := sqlc.New(db).ListDramaRecordConnections(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the drama record connections: %w", err)
	}
	connections := make([]*Connection, 0, len(rows))
	for i := range rows {
		connections = append(connections, connectionOf(&rows[i]))
	}

	return connections, nil
}

// Begin は、ログイン中のユーザー principal と記録先 name の連携を始め、利用者を送り出す認可のエンドポイントの URL を返す。
// 戻ってきたときに、始めたセッションのまま戻ってきたことを確認するので、ブラウザーのセッションでだけ始められる。
// origin は、接続したオリジン。リダイレクト URI にする。
func (s *Service) Begin(ctx context.Context, db Database, principal *auth.Principal, name, origin string) (string, error) {
	if principal.SessionID == uuid.Nil() {
		return "", ErrSessionRequired
	}
	destination, err := s.destination(name)
	if err != nil {
		return "", err
	}
	app, err := s.app(ctx, db, name)
	if err != nil {
		return "", err
	}
	state, err := randomString()
	if err != nil {
		return "", err
	}
	verifier := oauth2.GenerateVerifier()
	redirectURI := RedirectURI(origin, name)
	now := s.clock.Now()
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if _, err := q.DeleteExpiredDramaRecordAuthorizations(ctx, now); err != nil {
			return fmt.Errorf("failed to delete expired drama record authorizations: %w", err)
		}
		if err := q.InsertDramaRecordAuthorization(ctx, sqlc.InsertDramaRecordAuthorizationParams{
			ID: id.New(), StateHash: hashState(state), SessionID: principal.SessionID, UserID: principal.UserID, Destination: name,
			CodeVerifier: verifier, RedirectUri: redirectURI, Now: now, ExpiresAt: now.Add(authorizationTTL),
		}); err != nil {
			return fmt.Errorf("failed to save the drama record authorization: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return destination.AuthCodeURL(app, redirectURI, state, verifier), nil
}

// Callback は、記録先から戻ってきたリクエストの内容。
type Callback struct {
	State string
	Code  string
	// Error は、記録先が返したエラー (access_denied など)。
	Error string
	// Issuer は、応答の iss。記録先の認可サーバーからの応答であることを確認する (RFC 9207)。
	Issuer string
}

// Complete は、記録先 name から戻ってきた認可を確認し、ログイン中のユーザー principal と連携する。
// 途中の状態は、確認できなくても使用済みにする。始めたセッションと記録先が、戻ってきたリクエストと同じであることと、
// 応答の iss が記録先の認可サーバーであることを確認する (CSRF、アカウントの取り違え、別の認可サーバーとの取り違えを防ぐ)。
func (s *Service) Complete(ctx context.Context, db Database, principal *auth.Principal, name string, callback *Callback, client *auth.ClientInfo) error {
	destination, err := s.destination(name)
	if err != nil {
		return err
	}
	if callback.State == "" {
		return ErrAuthorizationExpired
	}
	row, err := sqlc.New(db).TakeDramaRecordAuthorization(ctx, sqlc.TakeDramaRecordAuthorizationParams{
		StateHash: hashState(callback.State), Now: s.clock.Now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAuthorizationExpired
	}
	if err != nil {
		return fmt.Errorf("failed to get the drama record authorization: %w", err)
	}
	if principal == nil || principal.SessionID != row.SessionID || principal.UserID != row.UserID || row.Destination != name {
		return ErrAuthorizationMismatch
	}
	if callback.Issuer != destination.Issuer() {
		return fmt.Errorf("%w: the response came from another issuer", ErrAuthorizationMismatch)
	}
	if callback.Error != "" {
		return fmt.Errorf("%w: the destination returned %s", ErrAccessDenied, shorten(callback.Error))
	}
	if callback.Code == "" {
		return fmt.Errorf("%w: no authorization code", ErrAccessDenied)
	}
	app, err := s.app(ctx, db, name)
	if err != nil {
		return err
	}
	token, err := destination.Exchange(ctx, app, row.RedirectUri, callback.Code, row.CodeVerifier)
	if err != nil {
		return err
	}
	account, err := destination.AccountName(ctx, app, token.AccessToken)
	if err != nil {
		return err
	}
	access, err := s.tokenBox.Seal([]byte(token.AccessToken))
	if err != nil {
		return err
	}
	refresh, err := s.tokenBox.Seal([]byte(token.RefreshToken))
	if err != nil {
		return err
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := sqlc.New(tx).UpsertDramaRecordConnection(ctx, sqlc.UpsertDramaRecordConnectionParams{
			UserID: principal.UserID, Destination: name, AccessTokenEncrypted: access, RefreshTokenEncrypted: refresh,
			AccessTokenExpiresAt: s.clock.Now().Add(token.ExpiresIn), AccountName: account, Now: s.clock.Now(),
		}); err != nil {
			return fmt.Errorf("failed to save the drama record connection: %w", err)
		}
		if err := usersettings.Touch(ctx, tx, s.clock, principal.UserID); err != nil {
			return err
		}
		// 連携したら、画面を開くのを待たずに、ウォッチリストを取り込む。
		if err := EnqueueWatchlistImport(ctx, tx, s.inserter, principal.UserID, name); err != nil {
			return err
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: audit.User(principal.UserID), Action: "drama_record.connect", TargetType: auditTargetConnection,
			TargetID: principal.UserID.String() + "/" + name,
			Summary:  map[string]any{summaryDestination: name, "account": account},
			SourceIP: client.SourceIP, UserAgent: client.UserAgent,
		})
	})
}

// Disconnect は、ユーザーと記録先 name の連携を外し、記録先でトークンを取り消す。取り消せなくても、Kotatsu からはトークンを消す。
// 取り込んだウォッチリストの状態 (drama_watchlist_imports) は連携と一緒に消えるが、見ている作品にコピーした作品は、
// Kotatsu の作品の一覧として残す。まだ送っていない記録は、ジョブが連携していないものとして諦める。
func (s *Service) Disconnect(ctx context.Context, db Database, actor *audit.Actor, userID uuid.UUID, name string) error {
	destination, err := s.destination(name)
	if err != nil {
		return err
	}
	var token string
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, err := sqlc.New(tx).DeleteDramaRecordConnection(ctx, sqlc.DeleteDramaRecordConnectionParams{UserID: userID, Destination: name})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotConnected
		}
		if err != nil {
			return fmt.Errorf("failed to delete the drama record connection: %w", err)
		}
		// 更新のトークンを取り消すと、アクセストークンも無効になる。
		if plain, err := s.tokenBox.Open(row.RefreshTokenEncrypted); err == nil {
			token = string(plain)
		}
		if err := usersettings.Touch(ctx, tx, s.clock, userID); err != nil {
			return err
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: "drama_record.disconnect", TargetType: auditTargetConnection, TargetID: userID.String() + "/" + name,
			Summary: map[string]any{summaryDestination: name, "account": row.AccountName}, SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
	if err != nil {
		return err
	}
	if token == "" {
		return nil
	}
	// 連携を無効にしているか、アプリを消した後は、取り消せない。トークンは消したので、取り消せなくても外したことにする。
	app, err := s.app(ctx, db, name)
	if err == nil {
		err = destination.Revoke(ctx, app, token)
	}
	if err != nil {
		s.logger.WarnContext(ctx, "failed to revoke the token", logattr.String("destination", name), slog.Any("error", err))
	}

	return nil
}

// PrepareUserErasure は、消すユーザーの連携のトークンを読み、ユーザーを消した後に記録先で取り消す関数を返す (#1382)。
// 連携の行は外部キーで消える。Disconnect と同じく、取り消せなくてもユーザーの削除は戻さない。
func (s *Service) PrepareUserErasure(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (func(context.Context), error) {
	rows, err := sqlc.New(tx).ListDramaRecordConnections(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the drama record connections: %w", err)
	}
	revocations := make([]func(context.Context) error, 0, len(rows))
	for i := range rows {
		name := rows[i].Destination
		destination, err := s.destination(name)
		if err != nil {
			s.logger.WarnContext(ctx, "failed to revoke the token", logattr.String("destination", name), slog.Any("error", err))

			continue
		}
		plain, err := s.tokenBox.Open(rows[i].RefreshTokenEncrypted)
		if err != nil {
			s.logger.WarnContext(ctx, "failed to revoke the token", logattr.String("destination", name), slog.Any("error", err))

			continue
		}
		app, err := s.app(ctx, tx, name)
		if err != nil {
			s.logger.WarnContext(ctx, "failed to revoke the token", logattr.String("destination", name), slog.Any("error", err))

			continue
		}
		revocations = append(revocations, func(ctx context.Context) error {
			if err := destination.Revoke(ctx, app, string(plain)); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}

			return nil
		})
	}

	return func(ctx context.Context) {
		for _, revoke := range revocations {
			if err := revoke(ctx); err != nil {
				s.logger.WarnContext(ctx, "failed to revoke the token", slog.Any("error", err))
			}
		}
	}, nil
}

// LogSecrets は、ログの報告からマスクする値として、記録先のアプリのクライアントシークレットと、全ユーザーのトークンを返す (#1521)。
func (s *Service) LogSecrets(ctx context.Context, db sqlc.DBTX) ([]string, error) {
	q := sqlc.New(db)
	secrets := []string{}
	apps, err := q.ListDramaRecordApps(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the drama record apps: %w", err)
	}
	for _, app := range apps {
		secret, err := s.appBox.Open(app.ClientSecretEncrypted)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt the drama record app secret: %w", err)
		}
		secrets = append(secrets, string(secret))
	}
	tokens, err := q.ListDramaRecordTokens(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the drama record tokens: %w", err)
	}
	for _, row := range tokens {
		for _, sealed := range [][]byte{row.AccessTokenEncrypted, row.RefreshTokenEncrypted} {
			token, err := s.tokenBox.Open(sealed)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt the drama record token: %w", err)
			}
			secrets = append(secrets, string(token))
		}
	}

	return secrets, nil
}

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

// destination は、名前 name の記録先を返す。
func (s *Service) destination(name string) (Destination, error) {
	destination, ok := s.destinations[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownDestination, name)
	}

	return destination, nil
}

// app は、記録先 name に登録したアプリを返す。TMDB との連携を無効にしているか、登録していなければ ErrUnavailable を返す。
func (s *Service) app(ctx context.Context, db sqlc.DBTX, name string) (*App, error) {
	system, err := settings.Load(ctx, db)
	if err != nil {
		return nil, err
	}
	if !system.DramaLinkingEnabled {
		return nil, ErrUnavailable
	}
	row, err := sqlc.New(db).GetDramaRecordApp(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the drama record app: %w", err)
	}
	secret, err := s.appBox.Open(row.ClientSecretEncrypted)
	if err != nil {
		return nil, err
	}

	return &App{ClientID: row.ClientID, ClientSecret: string(secret)}, nil
}

// wait は、前の問い合わせから間を空けてから戻る。このプロセスの中で 1 つずつ問い合わせるよう、呼んだ側は、戻ったら問い合わせ、
// 終えたら done を呼ぶ。
func (s *Service) wait(ctx context.Context) (func(), error) {
	s.requestMu.Lock()
	if wait := s.requestInterval - s.clock.Now().Sub(s.requestLast); wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			s.requestMu.Unlock()

			return nil, fmt.Errorf("failed to wait for the destination: %w", ctx.Err())
		case <-timer.C:
		}
	}

	return func() {
		s.requestLast = s.clock.Now()
		s.requestMu.Unlock()
	}, nil
}
