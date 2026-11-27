package aribcaption

import (
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// BuildCues は buildCues をテストから使う。
func BuildCues(captions []ts.Caption, base int64, lastVideoPTS *int64, offset time.Duration) []Cue {
	return buildCues(captions, base, lastVideoPTS, offset)
}
