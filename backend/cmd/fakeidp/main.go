// Command fakeidp は、E2E テストのための偽の OIDC の IdP を起動する (#1525、設計書 11.6)。
//
// internal/testutil/oidctest の IdP を、決まったアドレスで待ち受けさせる。認可のエンドポイントは、人の操作をまねずに、
// その場でコードを作って redirect_uri に戻す。制御用のエンドポイント (本物の IdP にはないもの) は、別のアドレスで待ち受け、
// 次に発行する ID トークンの sub、ユーザー名、グループを変える。制御用のアドレスは、テストの一式の外から届かないところにする。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/SlashNephy/kotatsu/internal/testutil/oidctest"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

// maxControlBody は、制御用のエンドポイントで受け付ける本文の大きさの上限。
const maxControlBody = 1 << 16

func main() {
	os.Exit(run())
}

func run() int {
	listen := flag.String("listen", "127.0.0.1:5557", "address of the identity provider")
	control := flag.String("control", "127.0.0.1:5558", "address of the control endpoint that changes the next ID token")
	clientID := flag.String("client-id", "kotatsu", "client ID")
	clientSecret := flag.String("client-secret", "", "client secret")
	redirectURIs := flag.String("redirect-uri", "", "comma-separated redirect URIs registered for the client")
	groups := flag.String("groups", "", "comma-separated groups of the default user")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := serve(ctx, logger, &options{
		listen: *listen, control: *control, clientID: *clientID, clientSecret: *clientSecret,
		redirectURIs: split(*redirectURIs), groups: split(*groups),
	}); err != nil {
		logger.ErrorContext(ctx, "fakeidp exited with an error", slog.Any("error", err))

		return 1
	}

	return 0
}

type options struct {
	listen, control        string
	clientID, clientSecret string
	redirectURIs, groups   []string
}

func split(s string) []string {
	parts := []string{}
	for part := range strings.SplitSeq(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}

	return parts
}

func serve(ctx context.Context, logger *slog.Logger, opts *options) error {
	ln, err := httpserver.Listen(ctx, opts.listen)
	if err != nil {
		return err
	}
	provider, err := oidctest.NewOn(ln, opts.clientID, opts.clientSecret, opts.redirectURIs...)
	if err != nil {
		return err
	}
	defer provider.Close()
	// 認可のエンドポイントで、その場でログインしたことにするので、認証した時刻は発行した時刻にする。
	// 載せないと、Kotatsu はいつ認証したか分からず、直近に認証したものとして扱わない (#1013)。
	provider.AuthTimeNow = true
	defaults := &identity{Subject: "e2e-oidc", Username: "e2e-oidc", Name: "E2E の OIDC ユーザー", Groups: opts.groups, OmitGroups: false}
	defaults.apply(provider)
	logger.InfoContext(ctx, "fakeidp started", slog.String("issuer", provider.Issuer()), slog.String("control", opts.control))

	controlListener, err := httpserver.Listen(ctx, opts.control)
	if err != nil {
		return err
	}

	return httpserver.Serve(ctx, logger, controlListener, controlHandler(provider, defaults))
}

// identity は、次に発行する ID トークンのユーザー。
type identity struct {
	Subject  string   `json:"subject"`
	Username string   `json:"username"`
	Name     string   `json:"name"`
	Groups   []string `json:"groups"`
	// OmitGroups なら、グループの claim を ID トークンに載せない。IdP の設定漏れをまねる。
	OmitGroups bool `json:"omitGroups"`
}

func (i *identity) apply(provider *oidctest.Provider) {
	claims := map[string]any{"preferred_username": i.Username, "name": i.Name}
	if !i.OmitGroups {
		groups := i.Groups
		if groups == nil {
			groups = []string{}
		}
		claims["groups"] = groups
	}
	provider.SetNext(i.Subject, claims)
}

// controlHandler は、制御用のエンドポイントを返す。
//   - POST /next: 本文の identity で、次に発行する ID トークンの中身を変える。
//   - POST /reset: 起動したときのユーザーに戻す。振る舞いを変えたテストは、終わりに呼ぶ。
func controlHandler(provider *oidctest.Provider, defaults *identity) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /next", func(w http.ResponseWriter, r *http.Request) {
		var next identity
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxControlBody))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&next); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}
		if next.Subject == "" {
			http.Error(w, errors.New("subject is required").Error(), http.StatusBadRequest)

			return
		}
		next.apply(provider)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /reset", func(w http.ResponseWriter, _ *http.Request) {
		defaults.apply(provider)
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}
