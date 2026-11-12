package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	// 表示のタイムゾーンのデータを、実行環境に頼らずバイナリに埋め込む (設計書 8.1)。
	_ "time/tzdata"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/lib/logring"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ログは標準出力に書き、管理の画面で読めるよう、最後の行をメモリにも写す (#1521、設計書 11.3)。
	logger := slog.New(logring.Install(slog.NewJSONHandler(os.Stdout, nil), logring.NewBuffer(logring.DefaultCapacity, logring.DefaultMaxLineLength)))
	if err := cli.NewCommand(cli.DefaultOptions(logger)).ExecuteContext(ctx); err != nil {
		logger.ErrorContext(ctx, "kotatsu exited with an error", slog.Any("error", err))

		return 1
	}

	return 0
}
