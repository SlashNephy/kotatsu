// Command fakegoogle は、Google Drive のライブラリ (#1902) を、本物の Google を使わずに確かめるための偽の Google を起動する。
//
// internal/testutil/googletest の偽の Google を、決まったアドレスで待ち受けさせる。認可のエンドポイントは、人の操作をまねずに、
// その場でコードを作って redirect_uri に戻す。Kotatsu は、設定の google.url (KOTATSU_GOOGLE_URL) をこのアドレスに向ける。
// -ts で渡した TS を、マイドライブの「Kotatsu」フォルダと、共有ドライブ「Backup」に置く。
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/SlashNephy/kotatsu/internal/testutil/googletest"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

func main() {
	os.Exit(run())
}

func run() int {
	listen := flag.String("listen", "127.0.0.1:5559", "address of the fake Google")
	clientID := flag.String("client-id", "kotatsu", "client ID")
	clientSecret := flag.String("client-secret", "kotatsu-secret", "client secret")
	redirectURIs := flag.String("redirect-uri", "", "comma-separated redirect URIs registered for the client")
	tsPaths := flag.String("ts", "", "comma-separated TS files placed in the fake Drive")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	server := googletest.New(*clientID, *clientSecret)
	for part := range strings.SplitSeq(*redirectURIs, ",") {
		if part = strings.TrimSpace(part); part != "" {
			server.RedirectURIs = append(server.RedirectURIs, part)
		}
	}
	if err := place(server, *tsPaths); err != nil {
		logger.ErrorContext(ctx, "failed to read the TS files", slog.Any("error", err))

		return 1
	}
	ln, err := httpserver.Listen(ctx, *listen)
	if err != nil {
		logger.ErrorContext(ctx, "failed to listen", slog.Any("error", err))

		return 1
	}
	logger.InfoContext(ctx, "fakegoogle started", slog.String("listen", *listen))
	if err := httpserver.Serve(ctx, logger, ln, server.Handler()); err != nil {
		logger.ErrorContext(ctx, "fakegoogle exited with an error", slog.Any("error", err))

		return 1
	}

	return 0
}

// place は、TS を、マイドライブの Kotatsu フォルダと、共有ドライブ Backup に置く。
func place(server *googletest.Server, paths string) error {
	// 書いている途中とみなされないよう、更新日時は少し前にする。
	modified := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	server.AddFile(&googletest.File{
		ID: "kotatsu-folder", Name: "Kotatsu", MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID,
		Content: nil, ModifiedTime: modified, Trashed: false,
	})
	server.AddDrive(&googletest.Drive{ID: "backup-drive", Name: "Backup"})
	for i, path := range strings.Split(paths, ",") {
		if path = strings.TrimSpace(path); path == "" {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := filepath.Base(path)
		server.AddFile(&googletest.File{
			ID: "file-" + string(rune('a'+i)), Name: name, MIMEType: "video/mp2t", Parent: "kotatsu-folder",
			Content: content, ModifiedTime: modified, Trashed: false,
		})
		server.AddFile(&googletest.File{
			ID: "backup-" + string(rune('a'+i)), Name: name, MIMEType: "video/mp2t", Parent: "backup-drive",
			Content: content, ModifiedTime: modified, Trashed: false,
		})
	}

	return nil
}
