// Package graph は、GraphQL のスキーマとリゾルバを持つ (設計書 9.1)。
package graph

import (
	"time"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// Resolver は、リゾルバが使う依存。
type Resolver struct {
	db    sqlc.DBTX
	clock clock.Clock
	// location は、番組の曜日と時間帯を判定する、表示のタイムゾーン。
	location *time.Location
	// heartbeatInterval は、heartbeat の subscription で時刻を送る間隔。
	heartbeatInterval time.Duration
}
