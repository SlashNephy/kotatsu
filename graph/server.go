package graph

import (
	"context"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"

	"github.com/SlashNephy/kotatsu/graph/generated"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/settings"
)

const (
	// DefaultMaxDepth と DefaultMaxComplexity は、クエリの深さと複雑度の上限 (設計書 9.1)。
	// 設計書に値の定めはない。画面が使うクエリが収まり、入れ子の深い悪意あるクエリを拒める値にする。
	DefaultMaxDepth      = 12
	DefaultMaxComplexity = 1000
	// DefaultHeartbeatInterval は、heartbeat の subscription で時刻を送る間隔。
	// リバースプロキシの無通信のタイムアウト (60 秒が多い) より短くする。
	DefaultHeartbeatInterval = 30 * time.Second
	// maxBodyBytes は、リクエストの本文の大きさの上限。
	maxBodyBytes = 1 << 20
)

// Options は Handler の設定。0 の項目は既定値にする。
type Options struct {
	DB Database
	// Recorder は、ルールや予約の変更を残す監査ログ。nil なら DB に記録する。
	Recorder audit.Recorder
	// SecondFactors は、第二要素の登録と削除に使う。nil なら、第二要素の操作は内部のエラーにする。
	SecondFactors *auth.SecondFactors
	// Passkeys は、パスキーの一覧、名前の変更、削除に使う。nil なら、パスキーの操作は内部のエラーにする。
	Passkeys *auth.Passkeys
	// UserAdmin は、管理者によるユーザーの管理に使う。nil なら、ユーザーの管理は内部のエラーにする。
	UserAdmin *auth.UserAdmin
	// Passwords は、ログイン中のユーザーがパスワードを変えるのに使う。nil なら、内部のエラーにする。
	Passwords *auth.Passwords
	// Sessions は、ログイン中のユーザーのセッションの一覧と失効に使う。nil なら、内部のエラーにする。
	Sessions *auth.Sessions
	// Settings は、全体の設定を変えるのに使う。nil なら、Recorder と Clock で作る。
	Settings *settings.Store
	// Pools は、保存先プールの ID からマウント先への対応。システムの状態で示す。
	Pools  map[string]string
	Clock  clock.Clock
	Logger *slog.Logger
	// Location は、番組の曜日と時間帯を判定する、表示のタイムゾーン。nil なら UTC。
	Location *time.Location
	// MaxSearchCandidates は、番組の検索で 1 回に照合する候補の数の上限。0 なら DefaultMaxSearchCandidates。
	MaxSearchCandidates int
	MaxDepth            int
	MaxComplexity       int
	HeartbeatInterval   time.Duration
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
	if o.MaxSearchCandidates <= 0 {
		o.MaxSearchCandidates = DefaultMaxSearchCandidates
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
		db: o.DB, clock: o.Clock, location: o.Location, maxSearchCandidates: o.MaxSearchCandidates, heartbeatInterval: o.HeartbeatInterval,
		rules:        rule.NewRules(o.Clock, o.Recorder),
		reservations: reserve.NewReservations(o.Clock, o.Recorder),
		factors:      o.SecondFactors,
		passkeys:     o.Passkeys,
		userAdmin:    o.UserAdmin,
		passwords:    o.Passwords,
		sessions:     o.Sessions,
		settings:     o.Settings,
		inbox:        notification.NewInbox(o.Clock),
		pools:        o.Pools,
	}
	if resolver.settings == nil {
		resolver.settings = settings.NewStore(o.Clock, o.Recorder)
	}
	config.Resolvers = resolver
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

	return guard(withLoaders(o.DB, resolver.rules, resolver.reservations)(server))
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
