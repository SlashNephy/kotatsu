// Package animeimage は、見たいリストの作品の画像を、Annict と AniList から取って中継する (#1235、設計書 3.8)。
// ブラウザに外部の画像を直に読ませると、CSP の img-src を作品の画像のホストごとに緩める必要があり、外部へ通信しない環境もあるので、
// tenban が同じオリジンで返す。
package animeimage

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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/lib/publicnet"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

const (
	// fetchTimeout は、画像を取ってくるのを待つ長さ。
	fetchTimeout = 10 * time.Second
	// maxImageBytes は、中継する画像の大きさの上限。作品の画像 (キービジュアル) は大きいことがあるので、アイコンより大きくする。
	maxImageBytes = 4 << 20
	// maxCacheBytes は、取ってきた画像を tenban のメモリに覚えておく量の上限。超えたら、古く覚えたものから忘れる。
	maxCacheBytes = 64 << 20
	// cacheControl は、ブラウザに画像を覚えさせる長さ。画像の URL が変われば、画面に出す URL も変わる (URL の版)。
	cacheControl = "private, max-age=86400"
)

var (
	errUpstreamStatus = errors.New("the image server returned an unexpected status")
	errTooLarge       = errors.New("the image is too large")
	errNotImage       = errors.New("the response is not an image")
	errInvalidURL     = errors.New("the image URL is not an https URL")
)

// Path は、記録先 destination の作品 workID の画像を中継する URL のパスを返す。imageURL は、記録先が示した画像の URL で、
// 変わったらブラウザに読み直させるため、その版をクエリに添える。
func Path(destination string, workID int32, imageURL string) string {
	sum := sha256.Sum256([]byte(imageURL))

	return "/media/anime-works/" + url.PathEscape(destination) + "/" + strconv.Itoa(int(workID)) + "/image?v=" + hex.EncodeToString(sum[:8])
}

// Handler は、見たいリストの作品の画像を中継する (GET /media/anime-works/{destination}/{workId}/image)。
//
// 画像の URL は記録先が返すが、Annict の画像は作品の公式サイトなど、どのホストにもありうる。そのため、ログイン中のユーザーの
// 見たいリストにある作品の画像に限り、https の、グローバルなアドレスにだけ接続し、リダイレクトには従わない。
// LAN の中のサーバーへリクエストを送らせないためである。
type Handler struct {
	queries *query.Queries
	logger  *slog.Logger
	client  *http.Client
	cache   *cache
}

var _ http.Handler = new(Handler)

// NewHandler は Handler を返す。
func NewHandler(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	dialer := &net.Dialer{Timeout: fetchTimeout, Control: publicnet.Control}

	return newHandler(pool, logger, &http.Client{
		Transport:     &http.Transport{Proxy: nil, DialContext: dialer.DialContext, ForceAttemptHTTP2: true, TLSHandshakeTimeout: fetchTimeout},
		Timeout:       fetchTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	})
}

func newHandler(pool *pgxpool.Pool, logger *slog.Logger, client *http.Client) *Handler {
	return &Handler{queries: query.New(pool), logger: logger, client: client, cache: newCache(maxCacheBytes)}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 失敗の応答は覚えさせない。画像を出せないときは、画面が録画の代表画像に切り替える。
	w.Header().Set("Cache-Control", "no-store")
	principal := auth.PrincipalFrom(r.Context())
	if principal == nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

		return
	}
	workID, err := strconv.ParseInt(r.PathValue("workId"), 10, 32)
	if err != nil {
		http.Error(w, "invalid work ID", http.StatusBadRequest)

		return
	}
	imageURL, err := h.queries.GetAnimeWishlistImageURL(r.Context(), principal.UserID, r.PathValue("destination"), int32(workID))
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "the work has no image", http.StatusNotFound)

		return
	}
	if err != nil {
		h.logger.ErrorContext(r.Context(), "failed to get the image URL of the work", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}
	image, ok := h.cache.get(imageURL)
	if !ok {
		image, err = h.fetch(r.Context(), imageURL)
		if err != nil {
			if r.Context().Err() == nil {
				h.logger.WarnContext(r.Context(), "failed to fetch the image of the work", slog.Any("error", err))
			}
			http.Error(w, "failed to fetch the image", http.StatusBadGateway)

			return
		}
		h.cache.put(imageURL, image)
	}

	sum := sha256.Sum256(image.data)
	w.Header().Set("Content-Type", image.contentType)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:16])+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(image.data))
}

// fetch は、画像を取ってきて、中身と、中身から決めた Content-Type を返す。
func (h *Handler) fetch(ctx context.Context, imageURL string) (*image, error) {
	u, err := url.Parse(imageURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, errInvalidURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody) //nolint:gosec // 作品の画像を中継するのが目的。接続先は https の、publicnet.Control で許したグローバルなアドレスに限り、リダイレクトに従わない (#1235)。
	if err != nil {
		return nil, fmt.Errorf("failed to build the request: %w", err)
	}
	req.Header.Set("Accept", "image/*")
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	resp, err := h.client.Do(req) //nolint:gosec // 作品の画像を中継するのが目的。接続先は https の、publicnet.Control で許したグローバルなアドレスに限り、リダイレクトに従わない (#1235)。
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", errUpstreamStatus, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read the image: %w", err)
	}
	if len(data) > maxImageBytes {
		return nil, errTooLarge
	}
	// 上流の Content-Type は信じず、中身から決める。SVG はスクリプトを含められるので、DetectContentType が画像とみなさないまま断る。
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("%w: %s", errNotImage, contentType)
	}

	return &image{data: data, contentType: contentType}, nil
}

// image は、取ってきた画像。
type image struct {
	data        []byte
	contentType string
}

// cache は、取ってきた画像を、画像の URL ごとに覚える。合わせて max バイトを超えたら、古く覚えたものから忘れる。
type cache struct {
	mu    sync.Mutex
	max   int
	size  int
	items map[string]*image
	order []string
}

func newCache(maxBytes int) *cache {
	return &cache{mu: sync.Mutex{}, max: maxBytes, size: 0, items: map[string]*image{}, order: []string{}}
}

func (c *cache) get(key string) (*image, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[key]

	return item, ok
}

func (c *cache) put(key string, item *image) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; ok || len(item.data) > c.max {
		return
	}
	for c.size+len(item.data) > c.max && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		c.size -= len(c.items[oldest].data)
		delete(c.items, oldest)
	}
	c.items[key] = item
	c.order = append(c.order, key)
	c.size += len(item.data)
}
