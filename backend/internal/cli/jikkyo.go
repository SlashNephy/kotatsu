package cli

import (
	"github.com/SlashNephy/kotatsu/internal/infra/jikkyoclient"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
	"github.com/SlashNephy/kotatsu/internal/usecase/jikkyo"
)

// jikkyoParts は、tenban が実況のコメントを扱う部品 (設計書 7.4)。実況のチャンネルの対応表と、
// NX-Jikkyo と過去ログ API への問い合わせ。
type jikkyoParts struct {
	table  *jikkyo.Table
	client *jikkyoclient.Client
}

// newJikkyoTable は、実況のチャンネルの対応表を作る。同梱の表に、設定で上書きした対応を足す。
func newJikkyoTable(deps *Deps) (*jikkyo.Table, error) {
	return jikkyo.NewTable(deps.Config.Jikkyo.Channels)
}

// newJikkyoClient は、NX-Jikkyo と過去ログ API に問い合わせる部品を作る。外部との通信は、記録する (#1369)。
func newJikkyoClient(clk clock.Clock, network *netactivity.Recorder) *jikkyoclient.Client {
	return jikkyoclient.NewClient(&jikkyoclient.ClientOptions{
		NXJikkyoURL: "", KakologURL: "", HTTPClient: jikkyoclient.NewHTTPClient(network), Clock: clk, LiveRetryDelays: nil,
	})
}
