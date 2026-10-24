// Package avatar は、ユーザーのアイコンの画像を、IdP から取って中継する (#778、設計書 10.2)。
// 画像の URL は、ログインのときに ID トークンの picture のクレームから保存する。
// ブラウザに IdP の画像を直に読ませると、CSP の img-src を IdP ごとに緩める必要があるので、tenban が同じオリジンで返す。
package avatar

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/lib/publicnet"
	"github.com/SlashNephy/kotatsu/internal/query"
)

const (
	// fetchTimeout は、IdP から画像を取ってくるのを待つ長さ。
	fetchTimeout = 5 * time.Second
	// maxImageBytes は、中継する画像の大きさの上限。アイコンなので、大きな画像は断る。
	maxImageBytes = 1 << 20
	// cacheControl は、ブラウザに画像を覚えさせる長さ。IdP で画像を変えても、1 時間で反映される。
	cacheControl = "private, max-age=3600"
)

var (
	errUpstreamStatus = errors.New("the image server returned an unexpected status")
	errTooLarge       = errors.New("the image is too large")
	errNotImage       = errors.New("the response is not an image")
)

// Handler は、ユーザーのアイコンの画像を中継する (GET /media/users/{id}/avatar)。
//
// picture の URL は IdP が返すが、IdP によってはユーザーが自由に書き換えられる。そのため、接続先を限り、LAN の中のサーバーへ
// リクエストを送らせないようにする。IdP の issuer と同じホストの URL だけは、IdP と同じく信じて、LAN の中でも取りに行く。
// 自宅で動かす IdP は、LAN の中にあることが多いからである。リダイレクトには従わない。
type Handler struct {
	queries *query.Queries
	logger  *slog.Logger
	// issuerHost は、IdP の issuer のホスト (ポートを含む)。OIDC を使わなければ空。
	issuerHost string
	// public は、グローバルなアドレスにだけ接続するクライアント。issuer は、IdP のホストへ接続するクライアント。
	public *http.Client
	issuer *http.Client
}

var _ http.Handler = new(Handler)

// NewHandler は Handler を返す。issuer は、OIDC の issuer の URL。OIDC を使わなければ空にする。
func NewHandler(pool *pgxpool.Pool, issuer string, logger *slog.Logger) *Handler {
	issuerHost := ""
	if u, err := url.Parse(issuer); err == nil {
		issuerHost = u.Host
	}
	noRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	dialer := &net.Dialer{Timeout: fetchTimeout, Control: publicnet.Control}

	return &Handler{
		queries: query.New(pool), logger: logger, issuerHost: issuerHost,
		public: &http.Client{
			Transport:     &http.Transport{Proxy: nil, DialContext: dialer.DialContext, ForceAttemptHTTP2: true, TLSHandshakeTimeout: fetchTimeout},
			Timeout:       fetchTimeout,
			CheckRedirect: noRedirect,
		},
		issuer: &http.Client{Transport: http.DefaultTransport, Timeout: fetchTimeout, CheckRedirect: noRedirect},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 失敗の応答は覚えさせない。画像を出せないときは、画面が表示名の頭文字を出す。
	w.Header().Set("Cache-Control", "no-store")
	userID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid user ID", http.StatusBadRequest)

		return
	}
	picture, err := h.queries.GetUserPicture(r.Context(), userID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "the user has no avatar", http.StatusNotFound)

		return
	}
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to get the avatar URL", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}
	data, contentType, err := h.fetch(r.Context(), picture)
	if err != nil {
		if r.Context().Err() == nil {
			h.logger.WarnContext(r.Context(), "failed to fetch the avatar", slog.String("user_id", userID.String()), slog.Any("error", err))
		}
		http.Error(w, "failed to fetch the avatar", http.StatusBadGateway)

		return
	}

	sum := sha256.Sum256(data)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:16])+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}

// fetch は、画像を取ってきて、中身と、中身から決めた Content-Type を返す。
func (h *Handler) fetch(ctx context.Context, picture string) ([]byte, string, error) {
	u, err := url.Parse(picture)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse the avatar URL: %w", err)
	}
	client := h.public
	if h.issuerHost != "" && strings.EqualFold(u.Host, h.issuerHost) {
		client = h.issuer
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody) //nolint:gosec // IdP の picture を中継するのが目的。接続先は publicnet.Control と issuer のホストに限り、リダイレクトに従わない (#778)。
	if err != nil {
		return nil, "", fmt.Errorf("failed to build the request: %w", err)
	}
	req.Header.Set("Accept", "image/*")
	resp, err := client.Do(req) //nolint:gosec // IdP の picture を中継するのが目的。接続先は publicnet.Control と issuer のホストに限り、リダイレクトに従わない (#778)。
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch the avatar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("%w: %d", errUpstreamStatus, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("failed to read the avatar: %w", err)
	}
	if len(data) > maxImageBytes {
		return nil, "", errTooLarge
	}
	// 上流の Content-Type は信じず、中身から決める。SVG はスクリプトを含められるので、DetectContentType が画像とみなさないまま断る。
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("%w: %s", errNotImage, contentType)
	}

	return data, contentType, nil
}
