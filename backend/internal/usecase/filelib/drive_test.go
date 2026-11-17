package filelib_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/googledrive"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/googletest"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
)

// driveFolder は、テストの Drive のフォルダの ID。
const driveFolder = "folder-1"

// driveTokens は、偽の Google が発行したアクセストークンを返す。
type driveTokens struct {
	server *googletest.Server
	token  string
}

func (t *driveTokens) Token(context.Context) (string, error) {
	if t.token == "" {
		t.token = t.server.IssueAccessToken()
	}

	return t.token, nil
}

func (t *driveTokens) Invalidate() { t.token = "" }

// TestSourceGoogleDrive は、Google Drive のファイルを、ローカルのファイルと同じく開いて索引を作れることと、
// ライブラリやパスが違っても、同じファイルなら索引とファイルの ID を分けないことを確かめる (#1902)。
func TestSourceGoogleDrive(t *testing.T) {
	t.Parallel()

	server := googletest.New("client", "secret")
	modified := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	server.AddFile(&googletest.File{ID: driveFolder, Name: animeLibrary, MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID, ModifiedTime: modified})
	server.AddFile(&googletest.File{ID: "show", Name: showTS, MIMEType: "video/mp2t", Parent: driveFolder, Content: loadTestTS(t), ModifiedTime: modified})
	server.AddFile(&googletest.File{ID: "notes", Name: "memo.txt", MIMEType: "text/plain", Parent: driveFolder, Content: []byte("x"), ModifiedTime: modified})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client := googledrive.NewClient(httpServer.Client(), httpServer.URL, &driveTokens{server: server, token: ""})
	// マイドライブの根元と、その中のフォルダの両方を、見せるフォルダに選ぶ。同じファイルを 2 つのライブラリから開ける。
	scope := googletest.NewScope(
		&medialib.DriveFolder{ID: googletest.MyDriveID, DriveID: "", Library: "mydrive"},
		&medialib.DriveFolder{ID: driveFolder, DriveID: "", Library: animeLibrary},
	)
	drive := medialib.NewDrive(client, scope, time.Now)
	libraries := medialib.NewSet(map[string]string{}, []string{}, map[string]string{}).WithProvider(drive)
	indexer := filelib.NewIndexer(t.TempDir(), libraries, clocktest.NewFake(time.Now()), slog.New(slog.DiscardHandler), true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		assert.NoError(t, indexer.Run(ctx))
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	source := filelib.NewSource(libraries, indexer)

	ref := filelib.Ref{Library: "mydrive", Path: "anime/" + showTS}
	file := waitComplete(t, source, ref)
	require.NotEmpty(t, file.Index.Entries)
	require.NotNil(t, file.Index.Video)
	assert.Equal(t, "mpeg2", file.Index.Video.Codec)
	assert.Equal(t, showTS, file.Entry.Name)

	// 別のライブラリから開いても、索引を使い回し、同じ ID にする。ライブラリの名前を変えたときと同じ。
	other, err := source.Open(t.Context(), filelib.Ref{Library: animeLibrary, Path: showTS})
	require.NoError(t, err)
	assert.True(t, other.Index.Complete)
	assert.Equal(t, file.ID(), other.ID())

	// 拡張子で、再生できるファイルかを見る。
	_, err = source.Open(t.Context(), filelib.Ref{Library: animeLibrary, Path: "memo.txt"})
	require.ErrorIs(t, err, filelib.ErrNotAFile)
}
