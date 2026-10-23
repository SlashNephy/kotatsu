// Command faketuner は、開発とテストのための偽のチューナーサーバーを起動する。
//
// Mirakurun 互換の API を持ち、合成した EPG とイベントを返し、testdata/ts/test.ts を繰り返し流す。
// --demo-interval を指定すると、その間隔で放送中の番組を延長し、番組の update イベントを発生させる。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SlashNephy/kotatsu/internal/httpserver"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
)

func main() {
	os.Exit(run())
}

func run() int {
	listen := flag.String("listen", "127.0.0.1:40772", "address to listen on")
	tsPath := flag.String("ts", "testdata/ts/test.ts", "TS file to stream; null packets are streamed when the file does not exist")
	demoInterval := flag.Duration("demo-interval", 0, "extend the airing program at this interval to emit events (0 disables)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := serve(ctx, logger, *listen, *tsPath, *demoInterval); err != nil {
		logger.ErrorContext(ctx, "faketuner exited with an error", slog.Any("error", err))

		return 1
	}

	return 0
}

func serve(ctx context.Context, logger *slog.Logger, listen, tsPath string, demoInterval time.Duration) error {
	ts, err := os.ReadFile(tsPath)
	if errors.Is(err, fs.ErrNotExist) {
		logger.WarnContext(ctx, "TS file not found; streaming null packets", slog.String("path", tsPath))
	} else if err != nil {
		return fmt.Errorf("failed to read the TS file: %w", err)
	}

	opts := faketuner.DefaultOptions(time.Now(), ts)
	server := faketuner.New(opts)

	if demoInterval > 0 {
		go extendAiringProgram(ctx, logger, server, opts.Programs, demoInterval)
	}

	ln, err := httpserver.Listen(ctx, listen)
	if err != nil {
		return err
	}

	return httpserver.Serve(ctx, logger, ln, server)
}

// extendAiringProgram は、interval ごとに放送中の番組の 1 つを 5 分延長する。
func extendAiringProgram(ctx context.Context, logger *slog.Logger, server *faketuner.Server, programs []*faketuner.Program, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		now := time.Now().UnixMilli()
		for _, p := range programs {
			if p.StartAt <= now && now < p.StartAt+p.Duration {
				if err := server.ExtendProgram(p.ID, 5*time.Minute); err != nil {
					logger.WarnContext(ctx, "failed to extend the program", slog.Any("error", err))
				}

				break
			}
		}
	}
}
