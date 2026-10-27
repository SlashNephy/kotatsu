// Package graph は、GraphQL のスキーマとリゾルバを持つ (設計書 9.1)。
package graph

import (
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/jikkyoclient"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
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
)

// Resolver は、リゾルバが使う依存。
type Resolver struct {
	// searchSlots は、同時に照合する検索の数を maxConcurrentSearches までに抑える。
	searchSlots chan struct{}
	// db は、usecase に渡す DB。トランザクションは usecase が張る。
	db *pgxpool.Pool
	// queries は、トランザクションの外の読み取り。
	queries *query.Queries
	clock   clock.Clock
	// location は、番組の曜日と時間帯を判定する、表示のタイムゾーン。
	location *time.Location
	// heartbeatInterval は、heartbeat の subscription で時刻を送る間隔。
	heartbeatInterval time.Duration
	rules             *rule.Rules
	reservations      *reserve.Reservations
	factors           *auth.SecondFactors
	passkeys          *auth.Passkeys
	apiTokens         *auth.APITokens
	oauth             *oauth.Server
	players           *player.Service
	userAdmin         *auth.UserAdmin
	passwords         *auth.Passwords
	sessions          *auth.Sessions
	urlUsers          *auth.URLUsers
	// recorder は、ほかの部品を通さずにリゾルバが直接残す監査ログ (セッションの失効など)。
	recorder     audit.Recorder
	settings     *settings.Store
	timeshift    *timeshift.Channels
	inbox        *notification.Inbox
	webhooks     *webhook.Store
	webPush      *webpush.Service
	streamSigner *stream.Signer
	jikkyoTable  *jikkyo.Table
	jikkyoClient *jikkyoclient.Client
	logger       *slog.Logger
	// store は、録画のファイルと生成物を読むための、このインスタンスにマウントしたプール。
	store     *kago.Store
	library   *library.Library
	changes   Changes
	pools     map[string]string
	freeSpace kago.FreeSpaceFunc
	// jobs は、ジョブを入れる River のクライアント。
	jobs thumbnail.Inserter
	// totalSpace は、保存先プールのファイルシステムの容量を調べる。
	totalSpace kago.TotalSpaceFunc
	// updates は、新しいリリースを確かめた結果。確かめない設定なら nil。
	updates Updates
}
