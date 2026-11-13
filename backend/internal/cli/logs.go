package cli

import (
	"os"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/logring"
	"github.com/SlashNephy/kotatsu/internal/usecase/logreport"
)

// roleNames は、ロールの名前を返す。
func roleNames(roles []*Role) []string {
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		names = append(names, role.Name)
	}

	return names
}

// newLogReport は、管理の画面で直近のログを読む部品を作る (#1521、設計書 11.3)。
// ログは、このプロセスのメモリのバッファと、チューナーサーバーの /api/log から集め、設定と DB の秘密を伏せてから返す。
// tuner は、チューナーサーバーを設定していなければ nil。
func newLogReport(deps *Deps, sealed *sealedStores, location *time.Location, tuner logreport.TunerServer) *logreport.Service {
	lookupEnv := deps.LookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	buffer := deps.Logs
	if buffer == nil {
		buffer = logring.NewBuffer(1, 0)
	}
	stored := []logreport.StoredSecrets{sealed.webhooks, sealed.webPush}
	// nil の *animerecord.Service を interface に入れると nil と比べられなくなるので、あるときだけ足す。
	if sealed.animeRecords != nil {
		stored = append(stored, sealed.animeRecords)
	}

	return logreport.New(&logreport.Options{
		Buffer:     buffer,
		Tuner:      tuner,
		Hosts:      []string{deps.Config.Tuner.URL.Value(), deps.Config.Tuner.EDCB.Address, deps.Config.Database.URL.Value()},
		InstanceID: deps.InstanceID.String(),
		Roles:      deps.Roles,
		Secrets:    logreport.Secrets(deps.DB, logreport.ConfigSecrets(deps.Config, lookupEnv), stored...),
		Timeout:    0, Limit: 0, MaxLineLength: 0,
		Location: location,
	})
}
