// Package animerecordapi は、Annict と AniList から戻ってくる認可を受け付ける (GET /auth/anime-records/<記録先>/callback、#927)。
package animerecordapi

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// SettingsPath は、連携を終えたときに戻す画面のパス (設計書 10.3)。
const SettingsPath = "/settings/account"

// CallbackHandler は、記録先 name から戻ってきた認可を確かめ、ログイン中のユーザーと記録先を連携するハンドラーを返す
// (GET /auth/anime-records/<記録先>/callback)。記録先ごとに登録し、記録先をリクエストのパスからは選ばない。
//
// 記録先からのリダイレクトなので、Origin を確かめられない。代わりに、state で認可の途中の状態を引き、
// 始めたセッションのまま戻ってきたことを確かめる。結果は、設定の画面へのリダイレクトで伝える。
// 連携したら connected=<記録先> を、失敗したら error=<理由> を付けて戻す。理由には、トークンなどの中身を入れない。
func CallbackHandler(
	service *animerecord.Service, db animerecord.Database, client func(*http.Request) *auth.ClientInfo, name string, logger *slog.Logger,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		err := service.Complete(r.Context(), db, auth.PrincipalFrom(r.Context()), name, &animerecord.Callback{
			State: query.Get("state"), Code: query.Get("code"), Error: query.Get("error"),
		}, client(r))
		if err != nil {
			reason := callbackErrorReason(err)
			if reason == "anime_record_failed" {
				logger.ErrorContext(r.Context(), "failed to connect the anime record destination",
					slog.String("destination", name), slog.Any("error", err))
			} else {
				logger.InfoContext(r.Context(), "rejected an anime record connection",
					slog.String("destination", name), slog.Any("error", err))
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
	case errors.Is(err, animerecord.ErrAuthorizationExpired):
		return "anime_record_expired"
	case errors.Is(err, animerecord.ErrAuthorizationMismatch), errors.Is(err, animerecord.ErrUnknownDestination):
		return "anime_record_mismatch"
	case errors.Is(err, animerecord.ErrAccessDenied):
		return "anime_record_denied"
	case errors.Is(err, animerecord.ErrUnavailable):
		return "anime_record_unavailable"
	default:
		return "anime_record_failed"
	}
}

// redirect は、設定の画面に、key=value を付けて戻す。
func redirect(w http.ResponseWriter, r *http.Request, key, value string) {
	u := url.URL{Path: SettingsPath, RawQuery: url.Values{key: {value}}.Encode()}
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}
