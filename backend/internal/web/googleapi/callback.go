// Package googleapi は、Google から戻ってくる認可を受け付ける (GET /internal/auth/google/callback、#1902)。
package googleapi

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/SlashNephy/kotatsu/internal/lib/logattr"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/google"
)

// IntegrationsPath は、接続を終えたときに戻す、管理者の連携の画面のパス。
const IntegrationsPath = "/admin/integrations"

// CallbackHandler は、Google から戻ってきた認可を確認し、始めたときの目的 (Google Drive の接続など) に応じて接続するハンドラーを返す
// (GET /internal/auth/google/callback)。リダイレクト URI は Google の連携で共有し、目的は認可の途中の状態から決める。
//
// Google からのリダイレクトなので、Origin を確認できない。代わりに、state で認可の途中の状態を引き、始めたセッションのまま
// 戻ってきたことを確認する。結果は、連携の画面へのリダイレクトで伝える。接続したら connected=google-drive を、失敗したら
// error=<理由> を付けて戻す。理由には、トークンなどの内容を入れない。
func CallbackHandler(service *google.Service, db google.Database, client func(*http.Request) *auth.ClientInfo, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		purpose, err := service.Complete(r.Context(), db, auth.PrincipalFrom(r.Context()), &google.Callback{
			State: query.Get("state"), Code: query.Get("code"), Error: query.Get("error"), Scope: query.Get("scope"),
		}, client(r))
		if err != nil {
			reason := callbackErrorReason(err)
			if reason == "google_failed" {
				logger.ErrorContext(r.Context(), "failed to complete the Google authorization", logattr.String("purpose", purpose), logattr.Error(err))
			} else {
				logger.InfoContext(r.Context(), "rejected a Google authorization", logattr.String("purpose", purpose), logattr.Error(err))
			}
			redirect(w, r, "error", reason)

			return
		}
		redirect(w, r, "connected", connectedName(purpose))
	})
}

// connectedName は、接続した目的を、画面に伝える名前にする。
func connectedName(purpose string) string {
	switch purpose {
	case google.PurposeDrive:
		return "google-drive"
	default:
		return "google"
	}
}

// callbackErrorReason は、接続のエラーを、画面に伝える理由にする。
func callbackErrorReason(err error) string {
	switch {
	case errors.Is(err, google.ErrAuthorizationExpired):
		return "google_expired"
	case errors.Is(err, google.ErrAuthorizationMismatch):
		return "google_mismatch"
	case errors.Is(err, google.ErrAccessDenied):
		return "google_denied"
	case errors.Is(err, google.ErrNoRefreshToken):
		return "google_no_refresh_token"
	case errors.Is(err, google.ErrUnavailable):
		return "google_unavailable"
	default:
		return "google_failed"
	}
}

// redirect は、連携の画面に、key=value を付けて戻す。
func redirect(w http.ResponseWriter, r *http.Request, key, value string) {
	u := url.URL{Path: IntegrationsPath, RawQuery: url.Values{key: {value}}.Encode()}
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}
