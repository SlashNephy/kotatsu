package cli

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/lib/clientinfo"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/password"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// accountParts は、tenban がユーザーのアカウントとログインを扱う部品 (設計書 8.1、8.4)。
type accountParts struct {
	sessions       *auth.Sessions
	factors        *auth.SecondFactors
	passkeys       *auth.Passkeys
	passwordPolicy *password.Policy
	userAdmin      *auth.UserAdmin
	passwords      *auth.Passwords
	apiTokens      *auth.APITokens
}

func poolOf(deps *Deps) *pgxpool.Pool {
	return deps.DB
}

// newSessions は、ログインのセッションを扱う部品を作る。
func newSessions(clk clock.Clock, logger *slog.Logger) *auth.Sessions {
	return auth.NewSessions(&auth.SessionsOptions{
		IdleTimeout:     0,
		AbsoluteTimeout: 0,
		// リバースプロキシが HTTPS を終端するときも、信頼済みのプロキシの X-Forwarded-Proto で Secure を付ける。
		IsSecure: func(r *http.Request) bool {
			client := clientinfo.From(r.Context())

			return client != nil && client.Secure
		},
		Clock:  clk,
		Logger: logger.With(slog.String("component", "session")),
	})
}

// newSecondFactors は、二要素認証を扱う部品を作る。TOTP の秘密キーは、アプリの秘密キーで暗号化して保存する (設計書 8.4)。
func newSecondFactors(clk clock.Clock, recorder audit.Recorder, secret appSecret) (*auth.SecondFactors, error) {
	return auth.NewSecondFactors(clk, recorder, string(secret))
}

// newPasskeys は、パスキーを扱う部品を作る。
func newPasskeys(clk clock.Clock, recorder audit.Recorder, logger *slog.Logger, sessions *auth.Sessions) *auth.Passkeys {
	return auth.NewPasskeys(&auth.PasskeysOptions{
		Clock:    clk,
		Recorder: recorder,
		Logger:   logger.With(slog.String("component", "passkey")),
		Sessions: sessions,
	})
}

// newAccountPasswordPolicy は、画面から設定するパスワードの規定を作る。強さの見積もりの時間は、既定のままにする。
func newAccountPasswordPolicy(deps *Deps, logger *slog.Logger) (*password.Policy, error) {
	return password.NewPolicy(&password.PolicyOptions{HIBP: hibpOf(deps.Config, deps.Network), StrengthTimeout: 0, Estimator: nil, Logger: logger})
}
