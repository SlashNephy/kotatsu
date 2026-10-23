// Package update は、GitHub のリリースを定期的に確かめ、動いている版より新しい版があるかを覚える (設計書 11.1、#506)。
//
// 外部への通信になるので、設定 (update_check.enabled) で入れたときだけ確かめる。
// main のビルド (edge) と、版を埋め込まないビルド (dev) は、リリースと比べられないので確かめない。
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

const (
	// DefaultURL は、最新のリリースを返す GitHub の API。
	DefaultURL = "https://api.github.com/repos/SlashNephy/kotatsu/releases/latest"
	// DefaultInterval は、確かめる間隔。
	DefaultInterval = 24 * time.Hour
	// requestTimeout は、1 回の問い合わせの時間の上限。
	requestTimeout = 30 * time.Second
	// maxResponseBytes は、読むレスポンスの大きさの上限。リリースのノートが長くても足りる。
	maxResponseBytes = 1 << 20
	// userAgent は、問い合わせに付ける User-Agent。GitHub の API は、User-Agent のない問い合わせを断る。
	userAgent = "Kotatsu-UpdateCheck/1"
)

// Release は、新しいリリース。
type Release struct {
	// Version は、v を除いたリリースの版 (1.2.3)。
	Version     string
	URL         string
	PublishedAt time.Time
}

// Checker は、最新のリリースを確かめる。
type Checker struct {
	client   *http.Client
	url      string
	current  string
	interval time.Duration
	logger   *slog.Logger

	mu     sync.RWMutex
	latest *Release
}

// Options は Checker の設定。
type Options struct {
	// Current は、動いている版 (buildinfo の Version)。
	Current string
	// URL と Interval は、空か 0 なら既定値を使う。HTTPClient が nil なら、http.DefaultClient を使う。
	URL        string
	Interval   time.Duration
	HTTPClient *http.Client
	Logger     *slog.Logger
}

// NewChecker は Checker を返す。
func NewChecker(opts *Options) *Checker {
	c := &Checker{client: opts.HTTPClient, url: opts.URL, current: opts.Current, interval: opts.Interval, logger: opts.Logger, mu: sync.RWMutex{}, latest: nil}
	if c.client == nil {
		c.client = http.DefaultClient
	}
	if c.url == "" {
		c.url = DefaultURL
	}
	if c.interval <= 0 {
		c.interval = DefaultInterval
	}
	if c.logger == nil {
		c.logger = slog.New(slog.DiscardHandler)
	}

	return c
}

// Comparable は、動いている版がリリースの版で、新しいリリースと比べられるかを返す。
func Comparable(current string) bool {
	return semver.IsValid(canonical(current))
}

// Run は、すぐに確かめ、その後は間隔ごとに確かめる。ctx が終わるまで戻らない。比べられない版なら、確かめずにすぐ戻る。
// 確かめられなくても、前に確かめた結果を残し、次の間隔でやり直す。
func (c *Checker) Run(ctx context.Context) error {
	if !Comparable(c.current) {
		c.logger.InfoContext(ctx, "skipping the update check for a non-release build", slog.String("version", c.current))

		return nil
	}
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		if err := c.Check(ctx); err != nil && !errors.Is(err, context.Canceled) {
			c.logger.WarnContext(ctx, "failed to check for a new release", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Check は、最新のリリースを 1 回確かめ、動いている版より新しければ覚える。
func (c *Checker) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return fmt.Errorf("failed to build the request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get the latest release: %w", err)
	}
	defer res.Body.Close()
	// リリースを 1 つも公開していなければ、404 が返る。新しい版はないものとする。
	if res.StatusCode == http.StatusNotFound {
		c.mu.Lock()
		c.latest = nil
		c.mu.Unlock()

		return nil
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get the latest release: unexpected status %s", res.Status)
	}
	var body struct {
		TagName     string    `json:"tag_name"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, maxResponseBytes)).Decode(&body); err != nil {
		return fmt.Errorf("failed to decode the latest release: %w", err)
	}
	latest := canonical(body.TagName)
	if !semver.IsValid(latest) {
		return fmt.Errorf("the latest release has an invalid version %q", body.TagName)
	}
	var release *Release
	// 新しいリリースのページだけを開けるよう、GitHub のページでない URL は使わない。
	if semver.Compare(latest, canonical(c.current)) > 0 && strings.HasPrefix(body.HTMLURL, "https://github.com/") {
		release = &Release{Version: strings.TrimPrefix(latest, "v"), URL: body.HTMLURL, PublishedAt: body.PublishedAt}
	}
	c.mu.Lock()
	c.latest = release
	c.mu.Unlock()

	return nil
}

// Latest は、動いている版より新しいリリースを返す。なければ (確かめていなければ) nil を返す。
func (c *Checker) Latest() *Release {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.latest
}

// canonical は、版に v を付けて、semver で比べられる形にする。
func canonical(version string) string {
	if strings.HasPrefix(version, "v") {
		return version
	}

	return "v" + version
}
