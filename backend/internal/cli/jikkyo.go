package cli

import (
	"context"
	"log/slog"

	"github.com/SlashNephy/kotatsu/internal/infra/jikkyoclient"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
	"github.com/SlashNephy/kotatsu/internal/usecase/jikkyo"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// jikkyoParts は、tenban が実況のコメントを扱う部品 (設計書 7.4)。実況のチャンネルの対応表と、
// NX-Jikkyo と過去ログ API への問い合わせ。
type jikkyoParts struct {
	table  *jikkyo.Table
	client *jikkyoclient.Client
}

// newJikkyoTable は、実況のチャンネルの対応表を作る。同梱の表に、管理者が画面で足した対応 (#1869) を重ねる。
// 画面で足した対応は、全体の設定から読む。DB がなければ (テストで差し替えた構成)、同梱の表だけを使う。
func newJikkyoTable(deps *Deps, clk clock.Clock, logger *slog.Logger) (*jikkyo.Table, error) {
	var overrides jikkyo.OverridesFunc
	if deps.DB != nil {
		overrides = func(ctx context.Context) (map[string]string, error) {
			system, err := settings.Load(ctx, deps.DB)
			if err != nil {
				return nil, err
			}

			return system.JikkyoChannels, nil
		}
	}

	return jikkyo.NewTable(overrides, clk, logger.With(slog.String("component", "jikkyo")))
}

// newJikkyoClient は、NX-Jikkyo と過去ログ API に問い合わせる部品を作る。外部との通信は、記録する (#1369)。
func newJikkyoClient(clk clock.Clock, network *netactivity.Recorder) *jikkyoclient.Client {
	return jikkyoclient.NewClient(&jikkyoclient.ClientOptions{
		NXJikkyoURL: "", KakologURL: "", HTTPClient: jikkyoclient.NewHTTPClient(network), Clock: clk, LiveRetryDelays: nil,
	})
}
