package autocaption

import (
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// ThreadsOf は、テストから threadsOf を呼ぶ。
func ThreadsOf(configured, cpus int) int { return threadsOf(configured, cpus) }

// AudioOffset は、テストから audioOffset を呼ぶ。
func AudioOffset(cues *ts.Cues) (time.Duration, error) { return audioOffset(cues) }
