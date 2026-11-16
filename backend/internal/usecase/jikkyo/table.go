// Package jikkyo は、サービスとニコニコ実況のチャンネルの対応と、実況のコメントの取得を扱う (設計書 7.4)。
package jikkyo

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

//go:embed konomitv/jikkyo_channels.json
var konomiTVChannels []byte

const (
	// terrestrialNetworkID は、KonomiTV の対応表で、地上波のサービスに書かれているネットワーク ID。
	// 地上波のネットワーク ID は地域ごとに違うので、対応表では 15 にそろえ、サービス ID だけで引く。
	terrestrialNetworkID = 15
	// noChannel は、KonomiTV の対応表で、実況のチャンネルがないことを表す ID。
	noChannel = -1
	// overridesTTL は、管理者が画面で足した対応を、読み直さずに使う時間 (#1869)。
	// 番組表の一覧はサービスごとに引くので、引くたびには読まない。変えたインスタンスでは、Invalidate ですぐに読み直す。
	overridesTTL = 10 * time.Second
)

// OverridesFunc は、管理者が画面で足した、サービスと実況のチャンネルの対応を返す (#1869)。
// キーは「ネットワーク ID:サービス ID」、値は jk1 などの実況のチャンネルの ID。
type OverridesFunc func(ctx context.Context) (map[string]string, error)

// serviceKey は、ネットワーク ID とサービス ID の組。
type serviceKey struct {
	networkID int64
	serviceID int64
}

// Table は、サービスから実況のチャンネルの ID (jk1 など) を引く対応表。同梱した対応表に、管理者が画面で足した対応を重ねる。
type Table struct {
	// services は、同梱した対応表の、BS と CS のサービスの対応。
	// 空の値は、実況のチャンネルがないと対応表に書かれていること。
	services map[serviceKey]string
	// terrestrial は、地上波のサービス ID からの対応。空の値の意味は services と同じ。
	terrestrial map[int64]string

	overrides OverridesFunc
	clock     clock.Clock
	logger    *slog.Logger

	mu sync.Mutex
	// cached は、最後に読んだ、画面で足した対応。cachedAt から overridesTTL の間は読み直さない。
	cached   map[serviceKey]string
	cachedAt time.Time
}

// konomiTVChannel は、KonomiTV の対応表の 1 行。
type konomiTVChannel struct {
	JikkyoID  int    `json:"jikkyo_id"`
	NetworkID int64  `json:"network_id"`
	ServiceID string `json:"service_id"`
}

// NewTable は、同梱した KonomiTV の対応表に、overrides が返す対応を重ねた対応表を返す。overrides が nil なら、同梱した対応表だけを使う。
// logger には、画面で足した対応を読めなかったことを出す。
func NewTable(overrides OverridesFunc, clk clock.Clock, logger *slog.Logger) (*Table, error) {
	var channels []konomiTVChannel
	if err := json.Unmarshal(konomiTVChannels, &channels); err != nil {
		return nil, fmt.Errorf("failed to parse the jikkyo channel table: %w", err)
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	t := &Table{
		services: map[serviceKey]string{}, terrestrial: map[int64]string{},
		overrides: overrides, clock: clk, logger: logger, mu: sync.Mutex{}, cached: map[serviceKey]string{}, cachedAt: time.Time{},
	}
	for _, channel := range channels {
		serviceID, err := strconv.ParseInt(channel.ServiceID, 0, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid service ID %q in the jikkyo channel table: %w", channel.ServiceID, err)
		}
		// 実況のチャンネルがないサービスも、サブチャンネルとして前のサービスに寄せないよう、空の値で残す。
		id := ""
		if channel.JikkyoID != noChannel {
			id = "jk" + strconv.Itoa(channel.JikkyoID)
		}
		if channel.NetworkID == terrestrialNetworkID {
			t.terrestrial[serviceID] = id
		} else {
			t.services[serviceKey{networkID: channel.NetworkID, serviceID: serviceID}] = id
		}
	}

	return t, nil
}

// Invalidate は、画面で足した対応を、次に引くときに読み直させる。対応を変えたインスタンスで、すぐに効かせるのに使う。
func (t *Table) Invalidate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cachedAt = time.Time{}
}

// IsChannelID は、id が jk1 のような実況のチャンネルの ID の形かどうかを返す。
func IsChannelID(id string) bool {
	number, ok := strings.CutPrefix(id, "jk")
	if !ok || number == "" || len(number) > 6 {
		return false
	}
	_, err := strconv.ParseUint(number, 10, 32)

	return err == nil
}

// ParseServiceKey は、「ネットワーク ID:サービス ID」の形のキーを読む。読めなければ false を返す。
func ParseServiceKey(key string) (int64, int64, bool) {
	return parseServiceKey(key)
}

func parseServiceKey(key string) (int64, int64, bool) {
	network, service, ok := strings.Cut(key, ":")
	if !ok {
		return 0, 0, false
	}
	networkID, err := strconv.ParseInt(network, 0, 64)
	if err != nil {
		return 0, 0, false
	}
	serviceID, err := strconv.ParseInt(service, 0, 64)
	if err != nil {
		return 0, 0, false
	}

	return networkID, serviceID, true
}

// Channel は、対応表 (画面で足した対応を含む) にある実況のチャンネルの ID のうち、id と同じものを返す。なければ false を返す。
// 利用者が指定した文字列でなく、対応表の値で NX-Jikkyo へつなぐためである。
func (t *Table) Channel(ctx context.Context, id string) (string, bool) {
	if id == "" {
		return "", false
	}
	for _, channel := range t.overridesOf(ctx) {
		if channel == id {
			return channel, true
		}
	}
	for _, channel := range t.services {
		if channel == id {
			return channel, true
		}
	}
	for _, channel := range t.terrestrial {
		if channel == id {
			return channel, true
		}
	}

	return "", false
}

// Lookup は、サービスの実況のチャンネルの ID を返す。対応するチャンネルがなければ false を返す。
// 画面で足した対応は、ネットワーク ID とサービス ID の組で引き、同梱した対応表より優先する。
// 地上波は、サービス ID だけで引く。サブチャンネルは対応表にないので、サービス ID が 1 つ前か 2 つ前のメインチャンネルに寄せる。
func (t *Table) Lookup(ctx context.Context, networkID, serviceID int64) (string, bool) {
	key := serviceKey{networkID: networkID, serviceID: serviceID}
	if id, ok := t.overridesOf(ctx)[key]; ok {
		return id, true
	}
	if id, ok := t.services[key]; ok {
		return id, id != ""
	}
	if !domain.IsTerrestrialNetwork(networkID) {
		return "", false
	}
	for _, candidate := range []int64{serviceID, serviceID - 1, serviceID - 2} {
		if id, ok := t.terrestrial[candidate]; ok {
			return id, id != ""
		}
	}

	return "", false
}

// overridesOf は、画面で足した対応を返す。overridesTTL の間は、前に読んだものを使う。
// 読めなければ、前に読んだものを使い続け、overridesTTL の後に読み直す。番組表や視聴の画面を止めないためである。
func (t *Table) overridesOf(ctx context.Context) map[serviceKey]string {
	if t.overrides == nil {
		return t.cached
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.clock.Now()
	if !t.cachedAt.IsZero() && now.Sub(t.cachedAt) < overridesTTL {
		return t.cached
	}
	raw, err := t.overrides(ctx)
	// 要求が取り消されて読めなかったときは、ほかの要求のために、覚えた時刻を進めない。
	if err != nil && ctx.Err() != nil {
		return t.cached
	}
	// 読めなかったときも時刻を進め、DB が止まっている間に、引くたびに読みに行かないようにする。
	t.cachedAt = now
	if err != nil {
		t.logger.WarnContext(ctx, "failed to read the jikkyo channel overrides; using the last ones", slog.Any("error", err))

		return t.cached
	}
	parsed := make(map[serviceKey]string, len(raw))
	for key, id := range raw {
		networkID, serviceID, ok := parseServiceKey(key)
		// 保存するときに確かめているので、崩れた対応は、引くときに読み飛ばす。
		if !ok || !IsChannelID(id) {
			continue
		}
		parsed[serviceKey{networkID: networkID, serviceID: serviceID}] = id
	}
	t.cached = parsed

	return parsed
}
