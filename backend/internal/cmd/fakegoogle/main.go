// Command fakegoogle は、Google Drive のライブラリ (#1902) を、本物の Google を使わずに確認するためのモックの Google を起動する。
//
// internal/testutil/googletest のモックの Google を、決まったアドレスで待ち受けさせる。認可のエンドポイントは、人の操作をまねずに、
// その場でコードを作って redirect_uri に戻す。Kotatsu は、設定の google.url (KOTATSU_GOOGLE_URL) をこのアドレスに向ける。
// -ts で渡した TS を、マイドライブの「Kotatsu」(と、その中の「Season 2」)、「Private」と、共有ドライブ「Backup」に置く。
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

// kotatsuFolder は、マイドライブの Kotatsu フォルダの ID。
const kotatsuFolder = "kotatsu-folder"

func main() {
	os.Exit(run())
}

func run() int {
	listen := flag.String("listen", "127.0.0.1:5559", "address of the fake Google")
	clientID := flag.String("client-id", "kotatsu", "client ID")
	clientSecret := flag.String("client-secret", "kotatsu-secret", "client secret")
	redirectURIs := flag.String(
		"redirect-uri", "https://localhost:9090/internal/auth/google/callback", "comma-separated redirect URIs registered for the client",
	)
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

// place は、TS を次のように置く。見せるフォルダで絞ることと、名前で探すことを確認できるようにする。
//
//	マイドライブ: Kotatsu/ (<名前> ×2, 2026/11 <名前>, Season 2/ (s2-<名前>)), Private/ (private-<名前>)
//	共有ドライブ Backup: <名前>
func place(server *googletest.Server, paths string) error {
	// 書いている途中とみなされないよう、更新日時は少し前にする。
	modified := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	folder := func(id, name, parent string) {
		server.AddFile(&googletest.File{
			ID: id, Name: name, MIMEType: googletest.FolderMIMEType, Parent: parent, Content: nil, ModifiedTime: modified, Trashed: false,
		})
	}
	folder(kotatsuFolder, "Kotatsu", googletest.MyDriveID)
	folder("season2-folder", "Season 2", kotatsuFolder)
	folder("private-folder", "Private", googletest.MyDriveID)
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
		suffix := string(rune('a' + i))
		for _, f := range []struct{ id, name, parent string }{
			{id: "file-" + suffix, name: name, parent: kotatsuFolder},
			// 同じフォルダに同じ名前が並ぶものと、名前に / を含むもの。
			{id: "dupfile-" + suffix, name: name, parent: kotatsuFolder},
			{id: "slash-" + suffix, name: "2026/11 " + name, parent: kotatsuFolder},
			{id: "season2-" + suffix, name: "s2-" + name, parent: "season2-folder"},
			{id: "private-" + suffix, name: "private-" + name, parent: "private-folder"},
			{id: "backup-" + suffix, name: name, parent: "backup-drive"},
		} {
			server.AddFile(&googletest.File{
				ID: f.id, Name: f.name, MIMEType: "video/mp2t", Parent: f.parent, Content: content, ModifiedTime: modified, Trashed: false,
			})
		}
	}

	return nil
}
