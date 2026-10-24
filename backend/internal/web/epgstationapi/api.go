// Package epgstationapi は、EPGStation 互換の読み取り専用 API (/api/*) を提供する (#431、設計書 9.1)。
// EPGStation の API で番組表や録画を読むクライアント (kiririn など) から、Kotatsu を使えるようにする。
// クライアントが読む範囲だけを用意し、予約やルールのような書き込みの API は用意しない。
package epgstationapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/query"
)

// Handler は、EPGStation 互換の API のうち、DB から組み立てる応答を返す。映像とロゴは、既存のハンドラーにつなぐ。
type Handler struct {
	queries *query.Queries
	// store は、代表画像があるかを、データディレクトリで確かめるのに使う。
	store  *kago.Store
	logger *slog.Logger
}

// New は Handler を返す。
func New(pool *pgxpool.Pool, store *kago.Store, logger *slog.Logger) *Handler {
	return &Handler{queries: query.New(pool), store: store, logger: logger}
}

type versionResponse struct {
	Version string `json:"version"`
}

// Version は GET /api/version に答える。Kotatsu の版を返す。
func (h *Handler) Version(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, r, &versionResponse{Version: "kotatsu-" + buildinfo.Get().Version})
}

// writeJSON は、v を JSON で書く。書いている途中で切れたら、ログに残すだけにする。
func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.logger.DebugContext(r.Context(), "failed to write the response", slog.String("path", r.URL.Path), slog.Any("error", err))
	}
}

// fail は、エラーをログに残し、500 を返す。
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "failed to serve the EPGStation-compatible API",
		slog.String("path", r.URL.Path), slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
