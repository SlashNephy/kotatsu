// Package dramarecordapi は、Simkl から戻ってくる認可を受け付ける (GET /internal/auth/<記録先>/callback、#1649)。
package dramarecordapi

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/SlashNephy/kotatsu/internal/lib/logattr"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/dramarecord"
)

// SettingsPath は、連携を終えたときに戻す画面のパス (設計書 10.3)。アニメの記録先と同じ、アカウントの連携の画面に戻す。
const SettingsPath = "/settings/account"

// CallbackHandler は、記録先 name から戻ってきた認可を確かめ、ログイン中のユーザーと記録先を連携するハンドラーを返す
// (GET /internal/auth/<記録先>/callback)。記録先ごとに登録し、記録先をリクエストのパスからは選ばない。
//
// 記録先からのリダイレクトなので、Origin を確かめられない。代わりに、state で認可の途中の状態を引き、始めたセッションのまま
// 戻ってきたことと、応答の iss が記録先の認可サーバーであることを確かめる。結果は、設定の画面へのリダイレクトで伝える。
// 連携したら connected=<記録先> を、失敗したら error=<理由> を付けて戻す。理由には、トークンなどの中身を入れない。
func CallbackHandler(
	service *dramarecord.Service, db dramarecord.Database, client func(*http.Request) *auth.ClientInfo, name string, logger *slog.Logger,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		err := service.Complete(r.Context(), db, auth.PrincipalFrom(r.Context()), name, &dramarecord.Callback{
			State: query.Get("state"), Code: query.Get("code"), Error: query.Get("error"), Issuer: query.Get("iss"),
		}, client(r))
		if err != nil {
			reason := callbackErrorReason(err)
			if reason == "drama_record_failed" {
				logger.ErrorContext(r.Context(), "failed to connect the drama record destination",
					slog.String("destination", name), logattr.Error(err))
			} else {
				logger.InfoContext(r.Context(), "rejected a drama record connection",
					slog.String("destination", name), logattr.Error(err))
			}
			redirect(w, r, "error", reason)

			return
		}
		redirect(w, r, "connected", name)
	})
}

// callbackErrorReason は、連携のエラーを、画面に伝える理由にする。
func callbackErrorReason(err error) string {
	switch {
	case errors.Is(err, dramarecord.ErrAuthorizationExpired):
		return "drama_record_expired"
	case errors.Is(err, dramarecord.ErrAuthorizationMismatch), errors.Is(err, dramarecord.ErrUnknownDestination):
		return "drama_record_mismatch"
	case errors.Is(err, dramarecord.ErrAccessDenied):
		return "drama_record_denied"
	case errors.Is(err, dramarecord.ErrUnavailable):
		return "drama_record_unavailable"
	default:
		return "drama_record_failed"
	}
}

// redirect は、設定の画面に、key=value を付けて戻す。
func redirect(w http.ResponseWriter, r *http.Request, key, value string) {
	u := url.URL{Path: SettingsPath, RawQuery: url.Values{key: {value}}.Encode()}
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}
