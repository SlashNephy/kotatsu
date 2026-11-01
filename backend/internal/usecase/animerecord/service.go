package animerecord

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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/secretbox"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/usersettings"
)

const (
	// appSealPurpose は、管理者が登録したアプリのクライアントシークレットを暗号化する用途 (設計書 8.4)。
	appSealPurpose = "kotatsu/anime-record-app/v1"
	// connectionSealPurpose は、ユーザーのアクセストークンを暗号化する用途 (設計書 8.4)。
	connectionSealPurpose = "kotatsu/anime-record-token/v1"
	// CallbackPathPrefix は、記録先から戻ってくるリダイレクト URI のパスの前半。後ろに記録先の名前と /callback を付ける。
	CallbackPathPrefix = "/auth/anime-records/"
	// authorizationTTL は、認可を始めてから戻ってくるまでの時間の上限。
	authorizationTTL = 10 * time.Minute
	// stateBytes は、state の乱数の長さ。
	stateBytes = 32
	// maxClientIDLength と maxClientSecretLength は、管理者が入れるクライアント ID とシークレットの長さの上限。
	maxClientIDLength     = 256
	maxClientSecretLength = 1024
	// DefaultThresholdPercent は、番組の長さのうち、見たら記録する割合の既定値 (付録 A)。TVTestAnnictRecorder と同じ。
	DefaultThresholdPercent = 20
	// auditTargetApp と auditTargetConnection は、監査ログの対象の種類。
	auditTargetApp        = "anime_record_app"
	auditTargetConnection = "anime_record_connection"
	summaryDestination    = "destination"
)

var (
	// ErrUnavailable は、管理者がしょぼいカレンダー、Annict、AniList との連携を無効にしているか、アプリを登録していないことを表す。
	ErrUnavailable = errors.New("anime records are not available")
	// ErrInvalidApp は、管理者が入れたアプリの値が正しくないことを表す。
	ErrInvalidApp = errors.New("invalid anime record app")
	// ErrInvalidSettings は、記録の設定の値が正しくないことを表す。
	ErrInvalidSettings = errors.New("invalid anime record settings")
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

// Service は、記録先のアプリと、ユーザーごとの連携と設定を扱う。
type Service struct {
	clock        clock.Clock
	recorder     audit.Recorder
	appBox       *secretbox.Box
	tokenBox     *secretbox.Box
	destinations map[string]Destination
	// order は、画面に並べる記録先の順。
	order  []string
	logger *slog.Logger
	// inserter は、連携したときに、見たいリストを取り込むジョブを入れる River のクライアント。nil なら入れない。
	inserter anime.Inserter
	// wishlistMu は、見たいリストを引く問い合わせを、このプロセスの中で 1 つずつにする。wishlistLast は、記録先ごとの前の問い合わせの時刻。
	wishlistMu       sync.Mutex
	wishlistLast     map[string]time.Time
	wishlistInterval time.Duration
}

// Options は NewService の設定。
type Options struct {
	Clock    clock.Clock
	Recorder audit.Recorder
	// AppSecret は、アプリの秘密キー (設計書 8.4)。
	AppSecret    string
	Destinations []Destination
	Logger       *slog.Logger
	// Inserter は、連携したときに、見たいリストを取り込むジョブを入れる River のクライアント (#928)。nil なら入れない。
	Inserter anime.Inserter
	// WishlistInterval は、見たいリストを引く問い合わせの間を空ける長さ (#928)。0 なら空けない。
	WishlistInterval time.Duration
}

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
		clock: opts.Clock, recorder: opts.Recorder, appBox: appBox, tokenBox: tokenBox,
		destinations: destinations, order: order, logger: opts.Logger, inserter: opts.Inserter,
		wishlistMu: sync.Mutex{}, wishlistLast: map[string]time.Time{}, wishlistInterval: opts.WishlistInterval,
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
	rows, err := sqlc.New(db).ListAnimeRecordApps(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime record apps: %w", err)
	}
	byName := make(map[string]*sqlc.AnimeRecordApp, len(rows))
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

// UpdateApp は、記録先 name に登録したアプリを変える。クライアント ID を空にすると、登録を消す。
// 利用者を記録先へ送り出す操作の元になる値なので、呼ぶ側で、直近の再認証を確かめる (設計書 9.5)。
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
			deleted, err := q.DeleteAnimeRecordApp(ctx, name)
			if err != nil {
				return fmt.Errorf("failed to delete the anime record app: %w", err)
			}
			if deleted == 0 {
				return nil
			}

			return s.recorder.Record(ctx, tx, &audit.Event{
				Actor: actor, Action: "anime_record_app.delete", TargetType: auditTargetApp, TargetID: name,
				Summary: map[string]any{}, SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
			})
		}
		current, err := q.GetAnimeRecordApp(ctx, name)
		exists := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("failed to get the anime record app: %w", err)
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
		if err := q.UpsertAnimeRecordApp(ctx, sqlc.UpsertAnimeRecordAppParams{
			Destination: name, ClientID: clientID, ClientSecretEncrypted: encrypted, Now: s.clock.Now(),
		}); err != nil {
			return fmt.Errorf("failed to save the anime record app: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: "anime_record_app.update", TargetType: auditTargetApp, TargetID: name,
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
	// LinkingEnabled は、管理者がしょぼいカレンダー、Annict、AniList との連携を有効にしているかどうか (設計書 3.8)。
	LinkingEnabled bool
	// Configured は、記録先の名前から、管理者がアプリを登録したかどうかへの表。
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

	return &Availability{LinkingEnabled: system.AnimeLinkingEnabled, Configured: configured}, nil
}

// RedirectURI は、記録先 name のリダイレクト URI を、接続したオリジン origin から返す。管理者は、これを記録先に登録する。
func RedirectURI(origin, name string) string {
	return origin + CallbackPathPrefix + name + "/callback"
}

// Connection は、ユーザーと記録先の連携。トークンは返さない。
type Connection struct {
	Destination string
	AccountName string
	ConnectedAt time.Time
}

// Connections は、ユーザー userID の連携を、記録先の名前の順に返す。
func (s *Service) Connections(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) ([]*Connection, error) {
	rows, err := sqlc.New(db).ListAnimeRecordConnections(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime record connections: %w", err)
	}
	connections := make([]*Connection, 0, len(rows))
	for _, row := range rows {
		connections = append(connections, &Connection{Destination: row.Destination, AccountName: row.AccountName, ConnectedAt: row.ConnectedAt})
	}

	return connections, nil
}

// Database は、トランザクションを張れる DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Begin は、ログイン中のユーザー principal と記録先 name の連携を始め、利用者を送り出す認可のエンドポイントの URL を返す。
// 戻ってきたときに、始めたセッションのまま戻ってきたことを確かめるので、ブラウザーのセッションでだけ始められる。
// origin は、接続したオリジン。リダイレクト URI にする。
func (s *Service) Begin(ctx context.Context, db Database, principal *auth.Principal, name, origin string) (string, error) {
	if principal.SessionID == uuid.Nil {
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
		if _, err := q.DeleteExpiredAnimeRecordAuthorizations(ctx, now); err != nil {
			return fmt.Errorf("failed to delete expired anime record authorizations: %w", err)
		}
		if err := q.InsertAnimeRecordAuthorization(ctx, sqlc.InsertAnimeRecordAuthorizationParams{
			ID: id.New(), StateHash: hashState(state), SessionID: principal.SessionID, UserID: principal.UserID, Destination: name,
			CodeVerifier: verifier, RedirectUri: redirectURI, Now: now, ExpiresAt: now.Add(authorizationTTL),
		}); err != nil {
			return fmt.Errorf("failed to save the anime record authorization: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return destination.AuthCodeURL(app, redirectURI, state, verifier), nil
}

// Callback は、記録先から戻ってきたリクエストの中身。
type Callback struct {
	State string
	Code  string
	// Error は、記録先が返したエラー (access_denied など)。
	Error string
}

// Complete は、記録先 name から戻ってきた認可を確かめ、ログイン中のユーザー principal と連携する。
// 途中の状態は、確かめられなくても使用済みにする。state は、SHA-256 で引くので、知らない値では何も引けない。
// 始めたセッションと記録先が、戻ってきたリクエストと同じであることを確かめる。ほかの端末やアカウントで始めた認可を、
// 別のユーザーに繋がせないため (CSRF とアカウントの取り違えを防ぐ)。
func (s *Service) Complete(ctx context.Context, db Database, principal *auth.Principal, name string, callback *Callback, client *auth.ClientInfo) error {
	destination, err := s.destination(name)
	if err != nil {
		return err
	}
	if callback.State == "" {
		return ErrAuthorizationExpired
	}
	row, err := sqlc.New(db).TakeAnimeRecordAuthorization(ctx, sqlc.TakeAnimeRecordAuthorizationParams{
		StateHash: hashState(callback.State), Now: s.clock.Now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAuthorizationExpired
	}
	if err != nil {
		return fmt.Errorf("failed to get the anime record authorization: %w", err)
	}
	if principal == nil || principal.SessionID != row.SessionID || principal.UserID != row.UserID || row.Destination != name {
		return ErrAuthorizationMismatch
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
	account, err := destination.AccountName(ctx, token)
	if err != nil {
		return err
	}
	encrypted, err := s.tokenBox.Seal([]byte(token))
	if err != nil {
		return err
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := sqlc.New(tx).UpsertAnimeRecordConnection(ctx, sqlc.UpsertAnimeRecordConnectionParams{
			UserID: principal.UserID, Destination: name, AccessTokenEncrypted: encrypted, AccountName: account, Now: s.clock.Now(),
		}); err != nil {
			return fmt.Errorf("failed to save the anime record connection: %w", err)
		}
		if err := usersettings.Touch(ctx, tx, s.clock, principal.UserID); err != nil {
			return err
		}
		// 連携したら、定期ジョブを待たずに、見たいリストを取り込む (#928)。
		if err := EnqueueWishlistImport(ctx, tx, s.inserter, principal.UserID); err != nil {
			return err
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: audit.User(principal.UserID), Action: "anime_record.connect", TargetType: auditTargetConnection,
			TargetID: principal.UserID.String() + "/" + name,
			Summary:  map[string]any{summaryDestination: name, "account": account},
			SourceIP: client.SourceIP, UserAgent: client.UserAgent,
		})
	})
}

// Disconnect は、ユーザーと記録先 name の連携を外す。記録先がトークンを取り消す API を持てば、取り消す。
// 取り消せなくても、Kotatsu からはトークンを消す。まだ送っていない記録は、ジョブが連携していないものとして諦める。
func (s *Service) Disconnect(ctx context.Context, db Database, actor *audit.Actor, userID uuid.UUID, name string) error {
	destination, err := s.destination(name)
	if err != nil {
		return err
	}
	var token string
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, err := sqlc.New(tx).DeleteAnimeRecordConnection(ctx, sqlc.DeleteAnimeRecordConnectionParams{UserID: userID, Destination: name})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotConnected
		}
		if err != nil {
			return fmt.Errorf("failed to delete the anime record connection: %w", err)
		}
		if plain, err := s.tokenBox.Open(row.AccessTokenEncrypted); err == nil {
			token = string(plain)
		}
		if err := usersettings.Touch(ctx, tx, s.clock, userID); err != nil {
			return err
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: "anime_record.disconnect", TargetType: auditTargetConnection, TargetID: userID.String() + "/" + name,
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
		s.logger.WarnContext(ctx, "failed to revoke the token", slog.String("destination", name), slog.Any("error", err))
	}

	return nil
}

// UserSettings は、ユーザーごとの記録の設定。
type UserSettings struct {
	// ThresholdPercent は、番組の長さのうち、何 % を見たら記録するか (1〜100)。
	ThresholdPercent int
	Rules            *StatusRules
}

// LoadSettings は、ユーザー userID の記録の設定を返す。変えていなければ既定値を返す。
func LoadSettings(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) (*UserSettings, error) {
	row, err := sqlc.New(db).GetAnimeRecordSettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return &UserSettings{
			ThresholdPercent: DefaultThresholdPercent,
			Rules:            &StatusRules{WatchingOnFirst: false, WatchingOnEpisode: false, WatchedOnLast: false},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the anime record settings: %w", err)
	}

	return settingsOf(&row), nil
}

func settingsOf(row *sqlc.AnimeRecordSetting) *UserSettings {
	return &UserSettings{
		ThresholdPercent: int(row.ThresholdPercent),
		Rules:            &StatusRules{WatchingOnFirst: row.WatchingOnFirst, WatchingOnEpisode: row.WatchingOnEpisode, WatchedOnLast: row.WatchedOnLast},
	}
}

// SaveSettings は、ユーザー userID の記録の設定を変える。
func (s *Service) SaveSettings(ctx context.Context, db Database, userID uuid.UUID, changes *UserSettings) (*UserSettings, error) {
	if changes.ThresholdPercent < 1 || changes.ThresholdPercent > 100 {
		return nil, fmt.Errorf("%w: the threshold must be between 1 and 100", ErrInvalidSettings)
	}
	threshold := int16(changes.ThresholdPercent)
	var saved *UserSettings
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, err := sqlc.New(tx).UpsertAnimeRecordSettings(ctx, sqlc.UpsertAnimeRecordSettingsParams{
			UserID: userID, ThresholdPercent: threshold,
			WatchingOnFirst: changes.Rules.WatchingOnFirst, WatchingOnEpisode: changes.Rules.WatchingOnEpisode,
			WatchedOnLast: changes.Rules.WatchedOnLast, Now: s.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to save the anime record settings: %w", err)
		}
		saved = settingsOf(&row)

		return usersettings.Touch(ctx, tx, s.clock, userID)
	})
	if err != nil {
		return nil, err
	}

	return saved, nil
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

// app は、記録先 name に登録したアプリを返す。連携を無効にしているか、登録していなければ ErrUnavailable を返す。
func (s *Service) app(ctx context.Context, db sqlc.DBTX, name string) (*App, error) {
	system, err := settings.Load(ctx, db)
	if err != nil {
		return nil, err
	}
	if !system.AnimeLinkingEnabled {
		return nil, ErrUnavailable
	}
	row, err := sqlc.New(db).GetAnimeRecordApp(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the anime record app: %w", err)
	}
	secret, err := s.appBox.Open(row.ClientSecretEncrypted)
	if err != nil {
		return nil, err
	}

	return &App{ClientID: row.ClientID, ClientSecret: string(secret)}, nil
}

// token は、ユーザー userID の、記録先 name のアクセストークンを返す。連携していなければ ErrNotConnected を返す。
func (s *Service) token(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, name string) (string, error) {
	row, err := sqlc.New(db).GetAnimeRecordConnection(ctx, sqlc.GetAnimeRecordConnectionParams{UserID: userID, Destination: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotConnected
	}
	if err != nil {
		return "", fmt.Errorf("failed to get the anime record connection: %w", err)
	}
	plain, err := s.tokenBox.Open(row.AccessTokenEncrypted)
	if err != nil {
		return "", err
	}

	return string(plain), nil
}
