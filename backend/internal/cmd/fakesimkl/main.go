// Command fakesimkl は、画面で Simkl との連携を確かめるための、偽の Simkl を起動する (#1649)。
//
// internal/testutil/simkltest の偽の Simkl を、決まったアドレスで待ち受けさせる。認可のエンドポイントは、人の操作をまねず、
// その場でコードを作って redirect_uri に戻す。Kotatsu は、設定の simkl.url (KOTATSU_SIMKL_URL) をこのアドレスに向ける。
// 制御用のエンドポイント (本物の Simkl にはないもの) は、別のアドレスで待ち受け、ウォッチリストを変え、受け取った見た話を返す。
// 制御用のアドレスは、検証の一式の外から届かないところにする。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/SlashNephy/kotatsu/internal/testutil/simkltest"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

// maxControlBody は、制御用のエンドポイントで受け付ける本文の大きさの上限。
const maxControlBody = 1 << 16

func main() {
	os.Exit(run())
}

func run() int {
	listen := flag.String("listen", "127.0.0.1:5567", "address of the fake Simkl (both the web pages and the API)")
	control := flag.String("control", "127.0.0.1:5568", "address of the control endpoint that changes the watchlist")
	clientID := flag.String("client-id", "kotatsu", "client ID")
	clientSecret := flag.String("client-secret", "secret", "client secret")
	redirectURIs := flag.String("redirect-uri", "", "comma-separated redirect URIs registered for the app")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := serve(ctx, logger, *listen, *control, *clientID, *clientSecret, strings.Split(*redirectURIs, ",")); err != nil {
		logger.ErrorContext(ctx, "fakesimkl exited with an error", slog.Any("error", err))

		return 1
	}

	return 0
}

func serve(ctx context.Context, logger *slog.Logger, listen, control, clientID, clientSecret string, redirectURIs []string) error {
	listener, err := httpserver.Listen(ctx, listen)
	if err != nil {
		return err
	}
	fake := simkltest.NewOn(listener, clientID, clientSecret, redirectURIs...)
	defer fake.Close()
	logger.InfoContext(ctx, "fakesimkl started", slog.String("url", fake.URL()), slog.String("control", control))
	controlListener, err := httpserver.Listen(ctx, control)
	if err != nil {
		return err
	}

	return httpserver.Serve(ctx, logger, controlListener, controlHandler(fake))
}

// controlRequest は、ウォッチリストを変える制御の本文。
type controlRequest struct {
	Items []struct {
		SimklID int64  `json:"simklId"`
		TMDBID  int32  `json:"tmdbId"`
		Status  string `json:"status"`
	} `json:"items"`
	// Removed なら、作品を外したことにする (/sync/activities の removed_from_list を進める)。
	Removed bool `json:"removed"`
}

// controlHandler は、PUT /watchlist でウォッチリストを置き換え、GET /history で受け取った見た話を返す。
func controlHandler(fake *simkltest.Server) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /watchlist", func(w http.ResponseWriter, r *http.Request) {
		var body controlRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxControlBody)).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}
		now := time.Now().UTC()
		items := make([]*simkltest.Item, 0, len(body.Items))
		for _, item := range body.Items {
			items = append(items, &simkltest.Item{SimklID: item.SimklID, TMDBID: item.TMDBID, Status: item.Status, UpdatedAt: now})
		}
		fake.Lock()
		fake.Items = items
		fake.Activity = now.Format(time.RFC3339)
		if body.Removed {
			fake.RemovedActivity = fake.Activity
		}
		fake.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /history", func(w http.ResponseWriter, _ *http.Request) {
		fake.Lock()
		raw, err := json.Marshal(map[string]any{"history": fake.History, "requests": fake.Requests})
		fake.Unlock()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	})

	return mux
}
