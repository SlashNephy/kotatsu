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
