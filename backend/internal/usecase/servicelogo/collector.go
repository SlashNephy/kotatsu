// Package servicelogo は、録画しているTSのCDTから、サービスのロゴを集めて残す (#1308、設計書 3.1)。
// チューナーサーバーがロゴを返さないとき (mirakc は放送からロゴを集めない) に、集めたロゴを返す。
package servicelogo

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// key は、ロゴを指す、ネットワークとロゴの番号と種類の組。
type key struct {
	networkID uint16
	logoID    uint16
	logoType  uint8
}

// Collector は、TS を読む解析器から受け取ったロゴを溜め、Flush で DB に書く。
// ロゴは同じものが繰り返し送られるので、一度書いた版は、もう書かない。
type Collector struct {
	clock  clock.Clock
	logger *slog.Logger

	mu sync.Mutex
	// pending は、まだ書いていないロゴ。同じロゴは、新しく届いたほうで置き換える。
	pending map[key]*ts.Logo
	// saved は、書いたロゴの版。
	saved map[key]uint16
}

// NewCollector は Collector を返す。
func NewCollector(clk clock.Clock, logger *slog.Logger) *Collector {
	return &Collector{clock: clk, logger: logger, mu: sync.Mutex{}, pending: map[key]*ts.Logo{}, saved: map[key]uint16{}}
}

// Observe は、解析器が読んだロゴを受け取る。ts.AnalyzerOptions.OnLogo に渡す。解析器の中で呼ばれるので、DB には書かない。
func (c *Collector) Observe(logo *ts.Logo) {
	k := key{networkID: logo.NetworkID, logoID: logo.LogoID, logoType: logo.Type}
	c.mu.Lock()
	defer c.mu.Unlock()
	if version, ok := c.saved[k]; ok && version == logo.Version {
		return
	}
	c.pending[k] = logo
}

// Flush は、溜めたロゴを DB に書く。書けなかったロゴは、次の Flush で書き直す。
// 見るためだけのものなので、書けなくても録画は止めない。エラーはログに残すだけにする。
func (c *Collector) Flush(ctx context.Context, q *sqlc.Queries) {
	c.mu.Lock()
	pending := c.pending
	if len(pending) == 0 {
		c.mu.Unlock()

		return
	}
	c.pending = map[key]*ts.Logo{}
	c.mu.Unlock()

	for k, logo := range pending {
		// 読めないロゴは、同じ版が届き直しても読めないので、書き直さない。
		data, err := ts.LogoPNG(logo.PNG)
		if err == nil {
			err = save(ctx, q, logo, data, c.clock)
		}
		c.mu.Lock()
		if err == nil || data == nil {
			c.saved[k] = logo.Version
		} else if _, newer := c.pending[k]; !newer {
			c.pending[k] = logo
		}
		c.mu.Unlock()
		if err != nil && ctx.Err() == nil {
			c.logger.WarnContext(ctx, "failed to save the service logo", slog.Int("network_id", int(logo.NetworkID)),
				slog.Int("logo_id", int(logo.LogoID)), slog.Int("logo_type", int(logo.Type)), slog.Any("error", err))
		}
	}
}

// save は、パレットを補ったロゴ data を DB に書く。
func save(ctx context.Context, q *sqlc.Queries, logo *ts.Logo, data []byte, clk clock.Clock) error {
	if _, err := q.UpsertServiceLogo(ctx, sqlc.UpsertServiceLogoParams{
		NetworkID: int64(logo.NetworkID), LogoID: int64(logo.LogoID), LogoType: int16(logo.Type), Version: int64(logo.Version),
		Data: data, Now: clk.Now(),
	}); err != nil {
		return fmt.Errorf("failed to save the logo: %w", err)
	}

	return nil
}
