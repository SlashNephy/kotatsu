package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/SlashNephy/kotatsu/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := cli.NewCommand(logger, cli.DefaultRoles()).ExecuteContext(ctx); err != nil {
		logger.ErrorContext(ctx, "kotatsu exited with an error", slog.Any("error", err))

		return 1
	}

	return 0
}
