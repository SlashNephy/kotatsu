package databroadcastapi

import (
	"context"
	"log/slog"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// MaxBodyBytes と MaxResponseBytes は、文書が送る本文と、中継する応答の大きさの上限。上限を超えたときを確かめるのに使う。
const (
	MaxBodyBytes     = maxBodyBytes
	MaxResponseBytes = maxResponseBytes
)

// NewWith は、接続の前に確かめる control と、接続を確かめるときのポートを決めた Relay を返す。
// httptest のサーバーは 127.0.0.1 で待ち受けるので、それをインターネットのサーバーに見立てるのに使う。
func NewWith(enabled func(context.Context) (bool, error), control Control, confirmPort string) *Relay {
	relay := newRelay(enabled, control, clock.Real{}, slog.New(slog.DiscardHandler))
	relay.confirmPort = confirmPort

	return relay
}
