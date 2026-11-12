package logreport

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/redact"
)

// ConfigSecrets は、設定の秘密 (docs/configuration.md で「秘密」とした項目) と、OTLP のヘッダーの環境変数から、伏せる値を返す。
func ConfigSecrets(cfg *config.Config, lookupEnv config.LookupEnv) []string {
	secrets := []string{cfg.Auth.SecretKey.Value(), cfg.Auth.OIDC.ClientSecret.Value()}
	secrets = append(secrets, redact.URLSecrets(cfg.Database.URL.Value(), false)...)
	secrets = append(secrets, redact.URLSecrets(cfg.Tuner.URL.Value(), false)...)
	// arm の取り込み元は、URL そのものに秘密を含めうるので、全体を伏せる。
	secrets = append(secrets, redact.URLSecrets(cfg.AnimeLinking.ArmURL.Value(), true)...)
	secrets = append(secrets, redact.HeaderSecrets(cfg.Telemetry.OTLPMetricsHeaders.Value())...)
	// OpenTelemetry の SDK が読む、OTLP の送り先に付けるヘッダーの環境変数。
	// トレースの Exporter は、設定 (telemetry.otlp_endpoint) のほかに、これらも読む。
	for _, name := range [...]string{"OTEL_EXPORTER_OTLP_HEADERS", "OTEL_EXPORTER_OTLP_TRACES_HEADERS", "OTEL_EXPORTER_OTLP_METRICS_HEADERS"} {
		if value, ok := lookupEnv(name); ok {
			secrets = append(secrets, redact.HeaderSecrets(value)...)
		}
	}

	return secrets
}

// StoredSecrets は、DB に保存した秘密 (Webhook の URL、Web Push の送り先、Annict と AniList の連携の秘密) を返すもの。
type StoredSecrets interface {
	LogSecrets(ctx context.Context, db sqlc.DBTX) ([]string, error)
}

// Secrets は、設定の秘密 static と、DB に保存した秘密 stored を合わせて返す SecretSource を作る。nil の stored は飛ばす。
func Secrets(db sqlc.DBTX, static []string, stored ...StoredSecrets) SecretSource {
	return func(ctx context.Context) ([]string, error) {
		secrets := append([]string{}, static...)
		for _, store := range stored {
			if store == nil {
				continue
			}
			values, err := store.LogSecrets(ctx, db)
			if err != nil {
				return nil, err
			}
			secrets = append(secrets, values...)
		}

		return secrets, nil
	}
}
