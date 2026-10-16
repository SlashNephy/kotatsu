// Package graph は、GraphQL のスキーマとリゾルバを持つ (設計書 9.1)。
package graph

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/jikkyo"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/library"
	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/settings"
	"github.com/SlashNephy/kotatsu/internal/stream"
	"github.com/SlashNephy/kotatsu/internal/webhook"
	"github.com/SlashNephy/kotatsu/internal/webpush"
)

// Database は、リゾルバが使う DB。書き込みをトランザクションにまとめるので、トランザクションを始められる。
// *pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Resolver は、リゾルバが使う依存。
type Resolver struct {
	// searchSlots は、同時に照合する検索の数を maxConcurrentSearches までに抑える。
	searchSlots chan struct{}
	db          Database
	clock       clock.Clock
	// location は、番組の曜日と時間帯を判定する、表示のタイムゾーン。
	location *time.Location
	// heartbeatInterval は、heartbeat の subscription で時刻を送る間隔。
	heartbeatInterval time.Duration
	rules             *rule.Rules
	reservations      *reserve.Reservations
	factors           *auth.SecondFactors
	passkeys          *auth.Passkeys
	userAdmin         *auth.UserAdmin
	passwords         *auth.Passwords
	sessions          *auth.Sessions
	settings          *settings.Store
	inbox             *notification.Inbox
	webhooks          *webhook.Store
	webPush           *webpush.Service
	streamSigner      *stream.Signer
	jikkyoTable       *jikkyo.Table
	jikkyoClient      *jikkyo.Client
	logger            *slog.Logger
	// store は、録画のファイルと付属ファイルを読むための、このインスタンスにマウントしたプール。
	store     *kago.Store
	library   *library.Library
	changes   Changes
	pools     map[string]string
	freeSpace kago.FreeSpaceFunc
}
