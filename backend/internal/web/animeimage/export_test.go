package animeimage

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewHandlerWithClient は、画像を取ってくるクライアントを差し替えた Handler を返す。テストの TLS のサーバーは LAN の中にあるため。
func NewHandlerWithClient(pool *pgxpool.Pool, logger *slog.Logger, client *http.Client) *Handler {
	return newHandler(pool, logger, client)
}

// CacheKeys は、大きさの上限 maxBytes の cache に、大きさ sizes の画像を順に覚えさせ、覚えている画像の鍵を、覚えた順に返す。
func CacheKeys(maxBytes int, sizes []int) []string {
	c := newCache(maxBytes)
	for i, size := range sizes {
		c.put(string(rune('a'+i)), &image{data: make([]byte, size), contentType: "image/png"})
	}

	return append([]string{}, c.order...)
}
