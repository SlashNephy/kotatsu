// Package graph は、GraphQL のスキーマとリゾルバを持つ (設計書 9.1)。
package graph

import (
	"time"

	"github.com/SlashNephy/kotatsu/internal/clock"
)

// Resolver は、リゾルバが使う依存。
type Resolver struct {
	clock clock.Clock
	// heartbeatInterval は、heartbeat の subscription で時刻を送る間隔。
	heartbeatInterval time.Duration
}
