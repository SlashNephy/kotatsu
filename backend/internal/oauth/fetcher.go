package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/publicnet"
)

const (
	// fetchTimeout は、クライアントのメタデータを取ってくるのを待つ長さ。
	fetchTimeout = 5 * time.Second
	// maxMetadataBytes は、クライアントのメタデータの大きさの上限。
	maxMetadataBytes = 5 << 10
	// maxRedirects は、クライアントのメタデータを取るときに追うリダイレクトの回数の上限。
	maxRedirects = 3
	// defaultFreshness は、Cache-Control に max-age がないときに、取ってきたメタデータを使い回す長さ。
	defaultFreshness = 15 * time.Minute
	// minFreshness と maxFreshness は、max-age から決めた使い回す長さを収める範囲。
	// 短すぎると認可のたびに取りに行き、長すぎるとクライアントがリダイレクト URI を変えても反映されない。
	minFreshness = 5 * time.Minute
	maxFreshness = 24 * time.Hour
	// maxCacheEntries は、使い回すために覚えておくメタデータの数の上限。
	maxCacheEntries = 1024
)

var (
	// ErrClientMetadataUnavailable は、クライアントのメタデータを取ってこられなかったことを表す。
	ErrClientMetadataUnavailable = errors.New("client metadata is unavailable")

	errInsecureRedirect = errors.New("redirects must stay on https")
	errTooManyRedirects = errors.New("too many redirects")
)

// Fetcher は、CIMD のクライアントのメタデータを取ってきて確かめ、しばらく使い回す。
//
// client_id の URL は誰でも指定できるので、LAN の中のサーバーへ接続させないよう、接続するときのアドレスで
// グローバルでないものを断る。名前解決の結果を後から変えられたときや、リダイレクトの先にも効く。
// 使い回すのは tenban ごとのメモリの中で、エラーや正しくない文書は覚えない。
type Fetcher struct {
	client *http.Client
	clock  clock.Clock
	group  singleflight.Group

	mu    sync.Mutex
	cache map[string]*cachedMetadata
}

type cachedMetadata struct {
	metadata  *ClientMetadata
	fetchedAt time.Time
	expiresAt time.Time
}

// NewFetcher は、グローバルなアドレスにだけ接続する Fetcher を返す。
func NewFetcher(clk clock.Clock) *Fetcher {
	dialer := &net.Dialer{Timeout: fetchTimeout, Control: publicnet.Control}
	transport := &http.Transport{
		Proxy: nil, DialContext: dialer.DialContext, ForceAttemptHTTP2: true, TLSHandshakeTimeout: fetchTimeout,
		ResponseHeaderTimeout: fetchTimeout,
	}

	return newFetcher(clk, transport)
}

func newFetcher(clk clock.Clock, transport http.RoundTripper) *Fetcher {
	return &Fetcher{
		client: &http.Client{Transport: transport, Timeout: fetchTimeout, CheckRedirect: checkRedirect, Jar: nil},
		clock:  clk, group: singleflight.Group{}, mu: sync.Mutex{}, cache: make(map[string]*cachedMetadata),
	}
}

// checkRedirect は、https のまま maxRedirects 回までのリダイレクトだけを追う。
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return errTooManyRedirects
	}
	if req.URL.Scheme != schemeHTTPS {
		return errInsecureRedirect
	}

	return nil
}

// Fetch は、clientID のクライアントのメタデータを返す。覚えているものがまだ新しければ、それを返す。
// 返すメタデータは、ほかの呼び出しと共有するので、書き換えてはならない。
// client_id が正しくなければ ErrInvalidClientID を、取ってこられなければ ErrClientMetadataUnavailable を、
// 文書が正しくないか Kotatsu で使えなければ ErrInvalidClientMetadata を返す。
func (f *Fetcher) Fetch(ctx context.Context, clientID string) (*ClientMetadata, error) {
	if _, err := ParseClientID(clientID); err != nil {
		return nil, err
	}
	if metadata := f.cached(clientID); metadata != nil {
		return metadata, nil
	}
	// 同じクライアントの認可が重なっても、取りに行くのは 1 回にする。待っている側が諦めても、取りに行くのは続ける。
	ch := f.group.DoChan(clientID, func() (any, error) {
		return f.fetch(context.WithoutCancel(ctx), clientID)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-ch:
		if result.Err != nil {
			return nil, result.Err
		}
		metadata, _ := result.Val.(*ClientMetadata)

		return metadata, nil
	}
}

// cached は、覚えているメタデータがまだ新しければ返す。
func (f *Fetcher) cached(clientID string) *ClientMetadata {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.cache[clientID]
	if !ok || !f.clock.Now().Before(entry.expiresAt) {
		return nil
	}

	return entry.metadata
}

func (f *Fetcher) fetch(ctx context.Context, clientID string) (*ClientMetadata, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clientID, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrClientMetadataUnavailable, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrClientMetadataUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected status %d", ErrClientMetadataUnavailable, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrClientMetadataUnavailable, err)
	}
	if len(body) > maxMetadataBytes {
		return nil, fmt.Errorf("%w: the document exceeds %d bytes", ErrInvalidClientMetadata, maxMetadataBytes)
	}
	var metadata ClientMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidClientMetadata, err)
	}
	if err := metadata.validate(clientID); err != nil {
		return nil, err
	}
	f.store(clientID, &metadata, freshness(resp.Header.Get("Cache-Control")))

	return &metadata, nil
}

// store は、取ってきたメタデータを覚える。覚えている数が上限に達していれば、古くなったものを捨て、
// それでも空かなければ、最も前に取ってきたものを捨てる。
func (f *Fetcher) store(clientID string, metadata *ClientMetadata, ttl time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.clock.Now()
	if _, ok := f.cache[clientID]; !ok && len(f.cache) >= maxCacheEntries {
		oldestID, oldest := "", time.Time{}
		for id, entry := range f.cache {
			if !now.Before(entry.expiresAt) {
				delete(f.cache, id)

				continue
			}
			if oldestID == "" || entry.fetchedAt.Before(oldest) {
				oldestID, oldest = id, entry.fetchedAt
			}
		}
		if len(f.cache) >= maxCacheEntries {
			delete(f.cache, oldestID)
		}
	}
	f.cache[clientID] = &cachedMetadata{metadata: metadata, fetchedAt: now, expiresAt: now.Add(ttl)}
}

// freshness は、Cache-Control から、メタデータを使い回す長さを決める。max-age がなければ defaultFreshness にし、
// minFreshness から maxFreshness の範囲に収める。no-store や no-cache でも、認可のたびに取りに行かないよう、最短の長さは使い回す。
func freshness(cacheControl string) time.Duration {
	ttl := defaultFreshness
	for directive := range strings.SplitSeq(cacheControl, ",") {
		name, value, _ := strings.Cut(directive, "=")
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "max-age":
			if seconds, err := strconv.Atoi(strings.Trim(strings.TrimSpace(value), `"`)); err == nil {
				ttl = time.Duration(seconds) * time.Second
			}
		case "no-store", "no-cache":
			return minFreshness
		}
	}

	return min(max(ttl, minFreshness), maxFreshness)
}
