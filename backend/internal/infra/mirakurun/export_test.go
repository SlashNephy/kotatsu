package mirakurun

import "github.com/SlashNephy/kotatsu/internal/domain/tunerserver"

// ToEvent は toEvent をテストから使う。
func ToEvent(raw *RawEvent) (*tunerserver.Event, bool) {
	return toEvent(raw)
}
