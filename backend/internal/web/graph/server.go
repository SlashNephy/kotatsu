package graph

import (
	"context"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/SlashNephy/kotatsu/internal/infra/jikkyoclient"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/update"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/jikkyo"
	"github.com/SlashNephy/kotatsu/internal/usecase/library"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
	"github.com/SlashNephy/kotatsu/internal/usecase/oauth"
	"github.com/SlashNephy/kotatsu/internal/usecase/player"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
	"github.com/SlashNephy/kotatsu/internal/usecase/thumbnail"
	"github.com/SlashNephy/kotatsu/internal/usecase/timeshift"
	"github.com/SlashNephy/kotatsu/internal/usecase/webhook"
	"github.com/SlashNephy/kotatsu/internal/usecase/webpush"
	"github.com/SlashNephy/kotatsu/internal/web/graph/generated"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	// DefaultMaxDepth と DefaultMaxComplexity は、クエリの深さと複雑度の上限 (設計書 9.1)。
	// 画面が使うクエリが収まり、入れ子の深い悪意あるクエリを拒める値にする (付録 A)。
	DefaultMaxDepth      = 12
	DefaultMaxComplexity = 1000
	// DefaultHeartbeatInterval は、heartbeat の subscription で時刻を送る間隔。
	// リバースプロキシの無通信のタイムアウト (60 秒が多い) より短くする。
	DefaultHeartbeatInterval = 30 * time.Second
	// heavyFieldComplexity は、番組の検索のような重いフィールドの複雑度。DefaultMaxComplexity の中に 3 つまでしか並べられない。
	heavyFieldComplexity = 300
	// maxBodyBytes は、リクエストの本文の大きさの上限。
	maxBodyBytes = 1 << 20
)

// Options は Handler の設定。0 の項目は既定値にする。
type Options struct {
	DB *pgxpool.Pool
	// Recorder は、ルールや予約の変更を残す監査ログ。nil なら DB に記録する。
	Recorder audit.Recorder
	// SecondFactors は、第二要素の登録と削除に使う。nil なら、第二要素の操作は内部のエラーにする。
	SecondFactors *auth.SecondFactors
	// Passkeys は、パスキーの一覧、名前の変更、削除に使う。nil なら、パスキーの操作は内部のエラーにする。
	Passkeys *auth.Passkeys
	// APITokens は、読み取り専用の API トークンを発行する (#431)。nil なら、API トークンを扱わない。
	APITokens *auth.APITokens
	// OAuth は、MCP のクライアントにトークンを発行する認可サーバー (#963)。nil なら、同意の操作を扱わない。
	OAuth *oauth.Server
	// Players は、再生中のプレイヤーを扱う (#459)。nil なら、プレイヤーを扱わない。
	Players *player.Service
	// UserAdmin は、管理者によるユーザーの管理に使う。nil なら、ユーザーの管理は内部のエラーにする。
	UserAdmin *auth.UserAdmin
	// Passwords は、ログイン中のユーザーがパスワードを変えるのに使う。nil なら、内部のエラーにする。
	Passwords *auth.Passwords
	// Sessions は、ログイン中のユーザーのセッションの一覧と失効に使う。nil なら、内部のエラーにする。
	Sessions *auth.Sessions
	// URLUsers は、署名付き URL を発行したユーザーの、覚えた認可の結果を忘れるのに使う (#1084)。nil なら、何もしない。
	URLUsers *auth.URLUsers
	// Settings は、全体の設定を変えるのに使う。nil なら、Recorder と Clock で作る。
	Settings *settings.Store
	// Webhooks は、Webhook の管理に使う。nil なら、Webhook の操作は内部のエラーにする。
	Webhooks *webhook.Store
	// WebPush は、Web Push の購読に使う。nil なら、Web Push の操作は内部のエラーにする。
	WebPush *webpush.Service
	// StreamSigner は、外部プレイヤー用の映像の URL に署名する。nil なら、URL の発行は内部のエラーにする。
	StreamSigner *stream.Signer
	// JikkyoTable と JikkyoClient は、サービスの実況のチャンネルを引くのに使う。どちらかが nil なら、実況のチャンネルは null にする。
	JikkyoTable  *jikkyo.Table
	JikkyoClient *jikkyoclient.Client
	// Changes は、ロール間の変更の通知を、subscription で知らせるのに使う。nil なら、変更の subscription は内部のエラーにする。
	Changes Changes
	// Pools は、保存先プールの ID からマウント先への対応。システムの状態で示す。
	Pools map[string]string
	// DataDir は、データディレクトリ。
	DataDir string
	// TimeshiftDir は、タイムシフトのバッファを置くディレクトリ。空なら、タイムシフトのチャンネルを設定できない。
	TimeshiftDir string
	// FreeSpace は、保存先プールの空き容量を調べる。nil なら、ファイルシステムに問い合わせる。
	FreeSpace kago.FreeSpaceFunc
	// Jobs は、ジョブを入れる River のクライアント。録画の画像を作り直すのに使う。nil なら、作り直せない。
	Jobs thumbnail.Inserter
	// TotalSpace は、保存先プールのファイルシステムの容量を調べる。nil なら、ファイルシステムに問い合わせる。
	TotalSpace kago.TotalSpaceFunc
	Clock      clock.Clock
	Logger     *slog.Logger
	// Location は、番組の曜日と時間帯を判定する、表示のタイムゾーン。nil なら UTC。
	Location          *time.Location
	MaxDepth          int
	MaxComplexity     int
	HeartbeatInterval time.Duration
	// Updates は、新しいリリースを確かめた結果 (#506)。確かめない設定なら nil。
	Updates Updates
}

// Schema は、GraphQL のスキーマを返す。MCP のツールの入力の型を作るのに使う (#964)。
func Schema() *ast.Schema {
	var config generated.Config

	return generated.NewExecutableSchema(config).Schema()
}

// Updates は、動いている版より新しいリリースを返す。
type Updates interface {
	Latest() *update.Release
}

// NewHandler は、POST /graphql のハンドラーを返す (設計書 9.1)。
//
// ログイン中のユーザーは、手前のミドルウェア (auth.Sessions.Middleware) が context に入れておく。
// 次のリクエストは、GraphQL の処理に入る前に拒否する。
//
//   - ログインしていないリクエスト (401)
//   - POST 以外のリクエスト (405)。GET でのクエリは、CSRF を防ぐため受け付けない (設計書 9.5)。
//   - Content-Type が application/json でないリクエスト (415)。フォームの送信による CSRF を防ぐ (設計書 9.5)。
//
// subscription は、同じエンドポイントで、Accept: text/event-stream のリクエストに SSE で配信する。
func NewHandler(opts *Options) http.Handler {
	var o Options
	if opts != nil {
		o = *opts
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	if o.Clock == nil {
		o.Clock = new(clock.Real)
	}
	if o.Recorder == nil {
		o.Recorder = audit.NewRecorder(o.Clock)
	}
	if o.Location == nil {
		o.Location = time.UTC
	}
	if o.MaxDepth <= 0 {
		o.MaxDepth = DefaultMaxDepth
	}
	if o.MaxComplexity <= 0 {
		o.MaxComplexity = DefaultMaxComplexity
	}
	if o.HeartbeatInterval <= 0 {
		o.HeartbeatInterval = DefaultHeartbeatInterval
	}

	var config generated.Config
	resolver := &Resolver{
		searchSlots: make(chan struct{}, maxConcurrentSearches),
		queries:     query.New(o.DB),
		db:          o.DB, clock: o.Clock, location: o.Location, heartbeatInterval: o.HeartbeatInterval,
		rules:        rule.NewRules(o.Clock, o.Recorder),
		reservations: reserve.NewReservations(o.Clock, o.Recorder),
		factors:      o.SecondFactors,
		passkeys:     o.Passkeys,
		apiTokens:    o.APITokens,
		oauth:        o.OAuth,
		players:      o.Players,
		userAdmin:    o.UserAdmin,
		passwords:    o.Passwords,
		sessions:     o.Sessions,
		urlUsers:     o.URLUsers,
		recorder:     o.Recorder,
		settings:     o.Settings,
		timeshift:    timeshift.NewChannels(o.Clock, o.Recorder),
		inbox:        notification.NewInbox(o.Clock),
		webhooks:     o.Webhooks,
		webPush:      o.WebPush,
		streamSigner: o.StreamSigner,
		jikkyoTable:  o.JikkyoTable,
		jikkyoClient: o.JikkyoClient,
		logger:       o.Logger,
		changes:      o.Changes,
		pools:        o.Pools,
		freeSpace:    o.FreeSpace,
		jobs:         o.Jobs,
		totalSpace:   o.TotalSpace,
		store:        kago.New(o.Pools, o.FreeSpace).WithDataDir(o.DataDir).WithTimeshiftDir(o.TimeshiftDir),
		library:      library.New(o.Clock, o.Recorder, kago.New(o.Pools, o.FreeSpace).WithDataDir(o.DataDir), o.Logger),
		updates:      o.Updates,
	}
	if resolver.freeSpace == nil {
		resolver.freeSpace = kago.FreeSpace
	}
	if resolver.totalSpace == nil {
		resolver.totalSpace = kago.TotalSpace
	}
	if resolver.settings == nil {
		resolver.settings = settings.NewStore(o.Clock, o.Recorder)
	}
	config.Resolvers = resolver
	// 番組の検索とルールのプレビューは、候補を照合し終えるまで最長 15 秒動くので、重く数える。
	// どのフィールドも 1 と数えると、エイリアスで並べて、1 回のリクエストで多数実行させられる (#541)。
	config.Complexity.Query.SearchPrograms = func(childComplexity int, _ model.ProgramConditionsInput, _ *model.ArchiveRangeInput, _ int, _ *string) int {
		return heavyFieldComplexity + childComplexity
	}
	config.Complexity.Query.PreviewRule = func(childComplexity int, _ model.RuleInput, _ int, _ *string) int {
		return heavyFieldComplexity + childComplexity
	}
	server := handler.New(generated.NewExecutableSchema(config))
	// SSE は Accept で POST と見分けるので、POST より先に置く。
	server.AddTransport(transport.SSE{})
	server.AddTransport(transport.POST{})
	server.Use(extension.Introspection{})
	server.Use(introspectionForViewer{})
	server.Use(depthLimit{max: o.MaxDepth})
	server.Use(extension.FixedComplexityLimit(o.MaxComplexity))
	server.SetErrorPresenter(errorPresenter(o.Logger))
	server.SetRecoverFunc(recoverFunc(o.Logger))

	handler := withLoaders(o.DB, resolver.queries, resolver.clock, resolver.rules)(server)
	if o.Sessions != nil {
		handler = revalidateSubscriptions(handler, o.Sessions, o.DB, o.HeartbeatInterval, o.Logger)
	}

	return guard(handler)
}

// revalidateSubscriptions は、接続中の subscription (SSE) のセッションを、interval ごとに確かめ直し、
// 使えなくなっていたら (失効させた、ユーザーを無効にした、など) 切る。
// 接続を始めたときに一度確かめるだけだと、失効させた後も、接続が続く間は届き続けるからである。
func revalidateSubscriptions(next http.Handler, sessions *auth.Sessions, db *pgxpool.Pool, interval time.Duration, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal := auth.PrincipalFrom(r.Context())
		if principal == nil || !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			next.ServeHTTP(w, r)

			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				usable, err := sessions.Usable(ctx, db, principal.SessionID)
				if err != nil {
					// 一時的に確かめられなくても、切らない。
					if ctx.Err() == nil {
						logger.WarnContext(ctx, "failed to revalidate the session of a subscription", slog.Any("error", err))
					}

					continue
				}
				if !usable {
					cancel()

					return
				}
			}
		}()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// guard は、GraphQL の処理に入る前に、ログインしていないリクエストと、POST の JSON でないリクエストを拒否する。
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.PrincipalFrom(r.Context()) == nil {
			writeError(w, http.StatusUnauthorized, CodeUnauthenticated, "login required")

			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed, CodeBadUserInput, "only POST is allowed")

			return
		}
		if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, CodeBadUserInput, "Content-Type must be application/json")

			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		// 監査ログに残すため、送信元の User-Agent を context に入れておく。
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userAgentKey{}, r.UserAgent())))
	})
}

type errorResponse struct {
	Errors []errorBody `json:"errors"`
}

type errorBody struct {
	Message    string         `json:"message"`
	Extensions errorExtension `json:"extensions"`
}

type errorExtension struct {
	Code Code `json:"code"`
}

// writeError は、GraphQL のエラーの形で、エラーを書く。
func writeError(w http.ResponseWriter, status int, code Code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := errorResponse{Errors: []errorBody{{Message: message, Extensions: errorExtension{Code: code}}}}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// ヘッダーは送ってしまったので、これ以上は伝えられない。
		return
	}
}
