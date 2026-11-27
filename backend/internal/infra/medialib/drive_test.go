package medialib_test

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/googledrive"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/testutil/googletest"
)

// driveTokens は、モックの Google が発行したアクセストークンを返す。
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

const (
	// folderID は、見せると選んだマイドライブの中のフォルダの ID。videosLibrary は、そのライブラリの名前。
	folderID      = "folder-1"
	videosLibrary = "videos"
	// videosName は、見せると選んだマイドライブの中のフォルダの名前。
	videosName = "videos"
	// seasonID と seasonName は、選んだフォルダの中のフォルダ。
	seasonID   = "folder-2"
	seasonName = "season2"
	// secretID は、選んでいないマイドライブの中のフォルダの ID。
	secretID = "folder-3"
	// sharedID は、テストの共有ドライブの ID。subID は、その中の、見せると選んだフォルダの ID。archiveLibrary は、そのライブラリの名前。
	sharedID       = "drive-1"
	subID          = "folder-4"
	archiveLibrary = "archive"
	// tsMime は、TS の MIME タイプ。
	tsMime = "video/mp2t"
	// fromHead は、頭から読む Range。
	fromHead = "bytes=0-"
	// dupName は、選んだフォルダに 2 つ並ぶファイルの名前。slashName は、/ を含む名前。episode2Name は、season2 の中のファイルの名前。
	// archiveFile は、共有ドライブの、選んだフォルダの中のファイルの名前。
	dupName      = "dup.ts"
	slashName    = "a/b.ts"
	episode2Name = "episode2.ts"
	archiveFile  = "episode-archive.ts"
	// episodeName と bigName は、選んだフォルダの直下のファイルの名前。
	episodeName = "episode.ts"
	bigName     = "big.ts"
	// slashID と slashPath は、/ を含む名前のファイルの ID と、そのパス (/ を形の似た文字に置き換えたもの)。
	slashID   = "slash"
	slashPath = "a\u2215b.ts"
	// episodeQuery は、名前で探すテストの語。
	episodeQuery = "EPISODE"
)

// bigContent は、読み出しを確認する大きめのファイルの内容。
func bigContent() []byte {
	return []byte(strings.Repeat("0123456789", 1000))
}

// newDriveWith は、次の内容のモックの Drive の、scope で選んだフォルダのライブラリを作る部品を返す。
//
//	マイドライブ: videos/ (episode.ts, big.ts, memo (Google ドキュメント), .dot.ts, dup.ts ×2, a/b.ts, season2/ (episode2.ts),
//	             .trash/ (episode-dot.ts)),
//	             a.ts, secret/ (episode-secret.ts)
//	共有ドライブ Archive: episode-b.ts, sub/ (episode-archive.ts)
func newDriveWith(t *testing.T, scope medialib.DriveScope) (*medialib.Drive, *googletest.Server) {
	t.Helper()
	modified := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	server := googletest.New("client", "secret")
	server.AddDrive(&googletest.Drive{ID: sharedID, Name: "Archive"})
	for _, f := range []*googletest.File{
		{ID: folderID, Name: videosName, MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID},
		{ID: "ep1", Name: episodeName, MIMEType: tsMime, Parent: folderID, Content: []byte("episode1")},
		{ID: "big", Name: bigName, MIMEType: tsMime, Parent: folderID, Content: bigContent()},
		{ID: "doc", Name: "memo", MIMEType: "application/vnd.google-apps.document", Parent: folderID},
		{ID: "dot", Name: ".dot.ts", MIMEType: tsMime, Parent: folderID, Content: []byte("h")},
		{ID: "dupaaaa111", Name: dupName, MIMEType: tsMime, Parent: folderID, Content: []byte("d1")},
		{ID: "dupaaaa222", Name: dupName, MIMEType: tsMime, Parent: folderID, Content: []byte("d2")},
		{ID: slashID, Name: slashName, MIMEType: tsMime, Parent: folderID, Content: []byte("s")},
		{ID: seasonID, Name: seasonName, MIMEType: googletest.FolderMIMEType, Parent: folderID},
		{ID: "ep2", Name: episode2Name, MIMEType: tsMime, Parent: seasonID, Content: []byte("episode2")},
		{ID: "dotdir", Name: ".trash", MIMEType: googletest.FolderMIMEType, Parent: folderID},
		{ID: "ep-dot", Name: "episode-dot.ts", MIMEType: tsMime, Parent: "dotdir", Content: []byte("e")},
		{ID: "a", Name: "a.ts", MIMEType: tsMime, Parent: googletest.MyDriveID, Content: []byte("a")},
		{ID: secretID, Name: "private", MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID},
		{ID: "hidden-ep", Name: "episode-secret.ts", MIMEType: tsMime, Parent: secretID, Content: []byte("s")},
		{ID: "b", Name: "episode-b.ts", MIMEType: tsMime, Parent: sharedID, Content: []byte("b")},
		{ID: subID, Name: "sub", MIMEType: googletest.FolderMIMEType, Parent: sharedID},
		{ID: "c", Name: archiveFile, MIMEType: tsMime, Parent: subID, Content: []byte("c")},
	} {
		f.ModifiedTime = modified
		server.AddFile(f)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client := googledrive.NewClient(httpServer.Client(), httpServer.URL, &driveTokens{server: server, token: ""})

	return medialib.NewDrive(client, scope, time.Now), server
}

// defaultScope は、videos と、共有ドライブの sub (archive) を見せる scope を返す。
func defaultScope() *googletest.Scope {
	return googletest.NewScope(
		&medialib.DriveFolder{ID: folderID, DriveID: "", Library: videosLibrary},
		&medialib.DriveFolder{ID: subID, DriveID: sharedID, Library: archiveLibrary},
	)
}

// newDriveLibrary は、newDriveWith の、名前が name のライブラリを返す。
func newDriveLibrary(t *testing.T, name string) (medialib.Library, *googletest.Server) {
	t.Helper()
	drive, server := newDriveWith(t, defaultScope())
	library, ok := drive.Lookup(name)
	require.True(t, ok)

	return library, server
}

// TestDriveLibraries は、選んだフォルダごとにライブラリを作り、接続していなければ一覧に出さないことを確認する。
func TestDriveLibraries(t *testing.T) {
	t.Parallel()
	drive, _ := newDriveWith(t, defaultScope())
	set := medialib.NewSet(map[string]string{mainPool: "/mnt/main"}, []string{mainPool}, map[string]string{}).WithProvider(drive)
	available, err := set.Available(t.Context())
	require.NoError(t, err)
	names, kinds := make([]string, 0, len(available)), make([]medialib.Kind, 0, len(available))
	for _, l := range available {
		names = append(names, l.Name())
		kinds = append(kinds, l.Kind())
	}
	assert.Equal(t, []string{mainPool, videosLibrary, archiveLibrary}, names)
	assert.Equal(t, []medialib.Kind{medialib.KindPool, medialib.KindGoogleDrive, medialib.KindGoogleDrive}, kinds)
	_, ok := set.Get(archiveLibrary)
	assert.True(t, ok)
	_, ok = set.Get("missing")
	assert.False(t, ok)

	// 接続していなければ、一覧に出さず、読もうとしたら ErrNotConnected を返す。
	scope := defaultScope()
	scope.Disconnected = true
	drive, server := newDriveWith(t, scope)
	set = medialib.NewSet(map[string]string{}, []string{}, map[string]string{}).WithProvider(drive)
	available, err = set.Available(t.Context())
	require.NoError(t, err)
	assert.Empty(t, available)
	library, ok := set.Get(videosLibrary)
	require.True(t, ok)
	_, err = library.List(t.Context(), "")
	require.ErrorIs(t, err, medialib.ErrNotConnected)
	assert.Empty(t, server.Requests())

	// フォルダを選んでいなければ、ライブラリはない。
	drive, _ = newDriveWith(t, googletest.NewScope())
	libraries, err := drive.Libraries(t.Context())
	require.NoError(t, err)
	assert.Empty(t, libraries)

	// 設定のライブラリと同じ名前のものは出さない。
	drive, _ = newDriveWith(t, defaultScope())
	set = medialib.NewSet(map[string]string{}, []string{}, map[string]string{videosLibrary: "/mnt/videos"}).WithProvider(drive)
	l, ok := set.Get(videosLibrary)
	require.True(t, ok)
	assert.Equal(t, medialib.KindFolder, l.Kind())
	available, err = set.Available(t.Context())
	require.NoError(t, err)
	assert.Len(t, available, 2)
}

func TestDriveList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		library   string
		dir       string
		wantPaths []string
		wantNames []string
		wantErr   error
	}{
		{
			// パスは名前でつなぐ。同じ名前が並ぶときだけ、~ と ID の先頭を付ける。/ は形の似た文字にする。
			// Google ドキュメントと、. で始まる名前は出さない。
			name: "selected folder", library: videosLibrary, dir: "",
			wantPaths: []string{slashPath, bigName, dupName + "~dupaaaa1", dupName + "~dupaaaa2", episodeName, seasonName},
			wantNames: []string{slashName, bigName, dupName, dupName, episodeName, seasonName},
		},
		{name: "subfolder", library: videosLibrary, dir: seasonName, wantPaths: []string{seasonName + "/" + episode2Name}, wantNames: []string{episode2Name}},
		{name: "folder in a shared drive", library: archiveLibrary, dir: "", wantPaths: []string{archiveFile}, wantNames: []string{archiveFile}},
		// 選んだフォルダの外へは、名前ではたどれない。
		{name: "unknown folder", library: videosLibrary, dir: "private", wantErr: medialib.ErrNotFound},
		{name: "not a directory", library: videosLibrary, dir: episodeName, wantErr: medialib.ErrNotFound},
		{name: "parent", library: videosLibrary, dir: "..", wantErr: medialib.ErrInvalidPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			library, _ := newDriveLibrary(t, tt.library)
			entries, err := library.List(t.Context(), tt.dir)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			paths, names := []string{}, []string{}
			for _, entry := range entries {
				paths = append(paths, entry.Path)
				names = append(names, entry.Name)
			}
			assert.Equal(t, tt.wantPaths, paths)
			assert.Equal(t, tt.wantNames, names)
		})
	}
}

// TestDriveSharedDriveRoot は、共有ドライブの根元を選んでも、内容をたどれることを確認する。
func TestDriveSharedDriveRoot(t *testing.T) {
	t.Parallel()
	drive, _ := newDriveWith(t, googletest.NewScope(&medialib.DriveFolder{ID: sharedID, DriveID: sharedID, Library: "atmos"}))
	library, ok := drive.Lookup("atmos")
	require.True(t, ok)
	entries, err := library.List(t.Context(), "")
	require.NoError(t, err)
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	assert.Equal(t, []string{"episode-b.ts", "sub"}, paths)
}

func TestDriveStatAndIdentity(t *testing.T) {
	t.Parallel()
	library, server := newDriveLibrary(t, videosLibrary)

	entry, err := library.Stat(t.Context(), episodeName)
	require.NoError(t, err)
	assert.Equal(t, &medialib.Entry{Name: episodeName, Path: episodeName, Dir: false, Size: 8, ModifiedAt: entry.ModifiedAt}, entry)
	// 少し前に引いた結果は、問い合わせずに使う。
	requests := len(server.Requests())
	_, err = library.Stat(t.Context(), episodeName)
	require.NoError(t, err)
	assert.Len(t, server.Requests(), requests)

	tests := []struct {
		name     string
		path     string
		wantID   string
		wantSize int64
		wantErr  error
	}{
		{name: "nested", path: seasonName + "/" + episode2Name, wantID: "ep2", wantSize: 8},
		{name: "duplicated name", path: dupName + "~dupaaaa2", wantID: "dupaaaa222", wantSize: 2},
		{name: "duplicated name without the mark", path: dupName, wantErr: medialib.ErrNotFound},
		{name: "slash in the name", path: slashPath, wantID: slashID, wantSize: 1},
		{name: "absent", path: "absent.ts", wantErr: medialib.ErrNotFound},
		{name: "google document", path: "memo", wantErr: medialib.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entry, err := library.Stat(t.Context(), tt.path)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSize, entry.Size)
			// ファイルを見分ける値は、ライブラリの名前とパスによらず、Drive のファイルの ID にする。
			namespace, id := medialib.IdentityOf(library, tt.path)
			assert.Equal(t, medialib.GoogleDriveName, namespace)
			assert.Equal(t, tt.wantID, id)
		})
	}

	names, err := medialib.NamesOf(t.Context(), library, seasonName+"/episode2.ts")
	require.NoError(t, err)
	assert.Equal(t, []string{seasonName, episode2Name}, names)
	names, err = medialib.NamesOf(t.Context(), library, slashPath)
	require.NoError(t, err)
	assert.Equal(t, []string{slashName}, names, "the names keep the slash")
}

// TestDriveSearch は、名前で探し、選んだフォルダ (と、探すフォルダ) の下にあるものだけを、開けるパスで返すことを確認する。
func TestDriveSearch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		library       string
		dir           string
		query         string
		limit         int
		wantPaths     []string
		wantTruncated bool
	}{
		{
			// 選んでいないフォルダのもの、共有ドライブのもの、名前が . で始まるフォルダの下のものは返さない。
			name: "selected folder", library: videosLibrary, dir: "", query: episodeQuery, limit: 10,
			wantPaths: []string{episodeName, seasonName + "/" + episode2Name},
		},
		{name: "subfolder", library: videosLibrary, dir: seasonName, query: episodeQuery, limit: 10, wantPaths: []string{seasonName + "/" + episode2Name}},
		// 共有ドライブの、選んだフォルダの外のもの (episode-b.ts) は返さない。
		{name: "shared drive", library: archiveLibrary, dir: "", query: episodeQuery, limit: 10, wantPaths: []string{archiveFile}},
		{name: "limit", library: videosLibrary, dir: "", query: episodeQuery, limit: 1, wantPaths: []string{episodeName}, wantTruncated: true},
		// 同じ名前が並ぶものは、一覧と同じく ID の先頭を付けたパスにする。
		{name: "same names", library: videosLibrary, dir: "", query: "dup", limit: 10, wantPaths: []string{dupName + "~dupaaaa1", dupName + "~dupaaaa2"}},
		// 名前の / は、一覧と同じく形の似た文字に置き換える。
		{name: "slash", library: videosLibrary, dir: "", query: "b.ts", limit: 10, wantPaths: []string{slashPath}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			library, _ := newDriveLibrary(t, tt.library)
			searcher, ok := library.(medialib.Searcher)
			require.True(t, ok)
			entries, truncated, err := searcher.Search(t.Context(), tt.dir, tt.query, tt.limit)
			require.NoError(t, err)
			paths := []string{}
			for _, entry := range entries {
				paths = append(paths, entry.Path)
				// 返したパスは、一覧からたどるのと同じパスで、そのまま開ける。
				stat, err := library.Stat(t.Context(), entry.Path)
				require.NoError(t, err)
				assert.Equal(t, entry.Name, stat.Name)
			}
			assert.ElementsMatch(t, tt.wantPaths, paths)
			assert.Equal(t, tt.wantTruncated, truncated)
		})
	}
}

// TestDriveSearchLongNames は、名前の長いファイルが多く見つかっても、同じ名前を確認する問い合わせを、Drive API が拒否しない長さに
// 分けることを確認する。本物の Drive API は、q を URL に入れて 16 KB ほどになると 400 を返した。
func TestDriveSearchLongNames(t *testing.T) {
	t.Parallel()
	server := googletest.New("client", "secret")
	server.MaxQueryBytes = 12 << 10
	server.SearchPageSize = 100
	server.AddFile(&googletest.File{ID: folderID, Name: videosName, MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID})
	want := make([]string, 0, 40)
	for i := range 40 {
		name := fmt.Sprintf("%02d %s.ts", i, strings.Repeat("番組", 40))
		server.AddFile(&googletest.File{ID: fmt.Sprintf("ep-%02d", i), Name: name, MIMEType: tsMime, Parent: folderID, Content: []byte("e")})
		want = append(want, name)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client := googledrive.NewClient(httpServer.Client(), httpServer.URL, &driveTokens{server: server, token: ""})
	drive := medialib.NewDrive(client, googletest.NewScope(&medialib.DriveFolder{ID: folderID, DriveID: "", Library: videosLibrary}), time.Now)
	library, ok := drive.Lookup(videosLibrary)
	require.True(t, ok)
	searcher, ok := library.(medialib.Searcher)
	require.True(t, ok)

	entries, _, err := searcher.Search(t.Context(), "", "番組", 100)
	require.NoError(t, err)
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	assert.ElementsMatch(t, want, paths)
}

// countingDriveAPI は、Drive API の呼び出しを数える。
type countingDriveAPI struct {
	medialib.DriveAPI

	mu    sync.Mutex
	calls map[string]int
}

func (a *countingDriveAPI) Children(ctx context.Context, parentID, driveID string) ([]*googledrive.File, error) {
	a.count("Children")

	return a.DriveAPI.Children(ctx, parentID, driveID)
}

func (a *countingDriveAPI) NamedChildren(ctx context.Context, names map[string][]string, driveID string) ([]*googledrive.File, error) {
	a.count("NamedChildren")

	return a.DriveAPI.NamedChildren(ctx, names, driveID)
}

func (a *countingDriveAPI) Folders(ctx context.Context, driveID string) ([]*googledrive.File, error) {
	a.count("Folders")

	return a.DriveAPI.Folders(ctx, driveID)
}

func (a *countingDriveAPI) count(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls[name]++
}

// TestDriveSearchCalls は、名前で探すときに、見つけたファイルの親のフォルダの内容をすべて取得せず、同じ名前のものだけをまとめて
// 問い合わせることと、
// フォルダの一覧を取得した後に作ったフォルダの下のものも見つけることを確認する (#2150)。
func TestDriveSearchCalls(t *testing.T) {
	t.Parallel()
	server := googletest.New("client", "secret")
	server.AddFile(&googletest.File{ID: folderID, Name: videosName, MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID})
	for i := range 3 {
		season := fmt.Sprintf("season-%d", i)
		server.AddFile(&googletest.File{ID: season, Name: season, MIMEType: googletest.FolderMIMEType, Parent: folderID})
		server.AddFile(&googletest.File{ID: season + "-ep", Name: season + " episode.ts", MIMEType: tsMime, Parent: season, Content: []byte("e")})
	}
	const otherID, newSeason = "other", "season-new"
	server.AddFile(&googletest.File{ID: otherID, Name: otherID, MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID})
	server.AddFile(&googletest.File{ID: "other-ep", Name: "other episode.ts", MIMEType: tsMime, Parent: otherID, Content: []byte("o")})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client := googledrive.NewClient(httpServer.Client(), httpServer.URL, &driveTokens{server: server, token: ""})
	api := &countingDriveAPI{DriveAPI: client, mu: sync.Mutex{}, calls: map[string]int{}}
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	drive := medialib.NewDrive(api, googletest.NewScope(&medialib.DriveFolder{ID: folderID, DriveID: "", Library: videosLibrary}), func() time.Time { return now })
	library, ok := drive.Lookup(videosLibrary)
	require.True(t, ok)
	searcher, ok := library.(medialib.Searcher)
	require.True(t, ok)

	entries, _, err := searcher.Search(t.Context(), "", "episode", 10)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
	assert.Equal(t, map[string]int{"Folders": 1, "NamedChildren": 1}, api.calls)

	// フォルダの一覧はキャッシュしたまま (フォルダの内容のキャッシュは切れた後)、一覧にない新しいフォルダの下のものも見つける。
	now = now.Add(time.Minute)
	server.AddFile(&googletest.File{ID: newSeason, Name: newSeason, MIMEType: googletest.FolderMIMEType, Parent: folderID})
	server.AddFile(&googletest.File{ID: "season-new-ep", Name: "season-new episode.ts", MIMEType: tsMime, Parent: newSeason, Content: []byte("n")})
	entries, _, err = searcher.Search(t.Context(), "", "episode", 10)
	require.NoError(t, err)
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	assert.Contains(t, paths, "season-new/season-new episode.ts")
	assert.Equal(t, 1, api.calls["Folders"])

	// フォルダの一覧はキャッシュしたまま、選んだフォルダの外へ移したフォルダの下のものは返さない。
	now = now.Add(time.Minute)
	server.AddFile(&googletest.File{ID: "season-0", Name: "season-0", MIMEType: googletest.FolderMIMEType, Parent: otherID})
	entries, _, err = searcher.Search(t.Context(), "", "episode", 10)
	require.NoError(t, err)
	paths = make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	assert.NotContains(t, paths, "season-0/season-0 episode.ts")
	assert.Equal(t, 1, api.calls["Folders"])
}

// TestDriveSearchSameFolderNames は、同じ名前のフォルダが並ぶとき、見つけたファイルのパスで、フォルダの名前に一覧と同じく ID の先頭を
// 付けることを確認する。
func TestDriveSearchSameFolderNames(t *testing.T) {
	t.Parallel()
	server := googletest.New("client", "secret")
	server.AddFile(&googletest.File{ID: folderID, Name: videosName, MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID})
	for _, id := range []string{"seasonaaa111", "seasonaaa222"} {
		server.AddFile(&googletest.File{ID: id, Name: seasonName, MIMEType: googletest.FolderMIMEType, Parent: folderID})
		server.AddFile(&googletest.File{ID: id + "-ep", Name: episodeName, MIMEType: tsMime, Parent: id, Content: []byte("e")})
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client := googledrive.NewClient(httpServer.Client(), httpServer.URL, &driveTokens{server: server, token: ""})
	drive := medialib.NewDrive(client, googletest.NewScope(&medialib.DriveFolder{ID: folderID, DriveID: "", Library: videosLibrary}), time.Now)
	library, ok := drive.Lookup(videosLibrary)
	require.True(t, ok)
	searcher, ok := library.(medialib.Searcher)
	require.True(t, ok)

	entries, _, err := searcher.Search(t.Context(), "", "episode", 10)
	require.NoError(t, err)
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
		stat, err := library.Stat(t.Context(), entry.Path)
		require.NoError(t, err)
		assert.Equal(t, entry.Name, stat.Name)
	}
	assert.ElementsMatch(t, []string{seasonName + "~seasonaaa1/" + episodeName, seasonName + "~seasonaaa2/" + episodeName}, paths)
}

// TestDriveRead は、ファイルを途中から読めることと、続きを読むなら開いた本文を使い回すことを確認する。
func TestDriveRead(t *testing.T) {
	t.Parallel()

	type step struct {
		// readAt が負なら Read で、そうでなければ ReadAt で、n バイト読む。seek が負でなければ、先に Seek する。
		seek   int64
		readAt int64
		n      int
	}
	tests := []struct {
		name       string
		faults     []googletest.Fault
		steps      []step
		wantRanges []string
	}{
		{
			// 頭から続けて読むなら、1 回だけ問い合わせる。
			name:       "sequential reads",
			steps:      []step{{seek: -1, readAt: -1, n: 100}, {seek: -1, readAt: -1, n: 100}, {seek: -1, readAt: 200, n: 50}},
			wantRanges: []string{fromHead},
		},
		{
			// 少し先へ飛ぶなら読み飛ばして進め、戻るなら開き直す。
			name:       "skip and rewind",
			steps:      []step{{seek: -1, readAt: 0, n: 10}, {seek: -1, readAt: 5000, n: 10}, {seek: 100, readAt: -1, n: 10}},
			wantRanges: []string{fromHead, "bytes=100-"},
		},
		{
			// 途中で切れたら、読めた所から開き直す。
			name:       "cut in the middle",
			faults:     []googletest.Fault{googletest.FaultCut},
			steps:      []step{{seek: -1, readAt: 0, n: 3000}},
			wantRanges: []string{fromHead, "bytes=1024-"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			drive, server := newDriveLibrary(t, videosLibrary)
			f, err := drive.Open(t.Context(), bigName)
			content := bigContent()
			require.NoError(t, err)
			defer f.Close()
			info, err := f.Stat()
			require.NoError(t, err)
			assert.Equal(t, int64(len(content)), info.Size())
			server.Inject(tt.faults...)
			pos := int64(0)
			for _, s := range tt.steps {
				if s.seek >= 0 {
					pos, err = f.Seek(s.seek, io.SeekStart)
					require.NoError(t, err)
				}
				buf := make([]byte, s.n)
				if s.readAt >= 0 {
					_, err = f.ReadAt(buf, s.readAt)
					require.NoError(t, err)
					assert.Equal(t, content[s.readAt:s.readAt+int64(s.n)], buf)

					continue
				}
				_, err = io.ReadFull(f, buf)
				require.NoError(t, err)
				assert.Equal(t, content[pos:pos+int64(s.n)], buf)
				pos += int64(s.n)
			}
			assert.Equal(t, tt.wantRanges, server.Ranges())
		})
	}
}

func TestDriveReadEnd(t *testing.T) {
	t.Parallel()
	drive, server := newDriveLibrary(t, videosLibrary)
	f, err := drive.Open(t.Context(), episodeName)
	require.NoError(t, err)
	defer f.Close()

	// 終わりをまたぐ ReadAt は、読めた分と io.EOF を返す。
	buf := make([]byte, 5)
	n, err := f.ReadAt(buf, 5)
	require.ErrorIs(t, err, io.EOF)
	assert.Equal(t, "de1", string(buf[:n]))
	// 終わりより先は、問い合わせずに io.EOF を返す。
	_, err = f.ReadAt(buf, 8)
	require.ErrorIs(t, err, io.EOF)
	all, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "episode1", string(all))
	assert.Equal(t, []string{"bytes=5-", fromHead}, server.Ranges())

	_, err = drive.Open(t.Context(), seasonName)
	require.ErrorIs(t, err, medialib.ErrNotFound)
}

// TestDriveReadGivesUp は、途中で切れ続けたら、開き直す回数の上限で諦めることを確認する。
func TestDriveReadGivesUp(t *testing.T) {
	t.Parallel()
	drive, server := newDriveLibrary(t, videosLibrary)
	f, err := drive.Open(t.Context(), bigName)
	require.NoError(t, err)
	defer f.Close()
	faults := make([]googletest.Fault, 0, 6)
	for range 6 {
		faults = append(faults, googletest.FaultCut)
	}
	server.Inject(faults...)
	_, err = f.ReadAt(make([]byte, 9000), 0)
	require.Error(t, err)
	// 初めの 1 回と、開き直した 5 回。
	assert.Len(t, server.Ranges(), 6)
}

// revokedTokens は、接続が切れていて、アクセストークンを作れない TokenSource。
type revokedTokens struct{}

func (revokedTokens) Token(context.Context) (string, error) { return "", googledrive.ErrNotConnected }

func (revokedTokens) Invalidate() {}

// TestDriveRevoked は、接続しているとキャッシュしていても、トークンを作れなければ、接続していないものとして返すことを確認する。
// 配信や画像の口とジョブは、ErrNotFound で見分ける。
func TestDriveRevoked(t *testing.T) {
	t.Parallel()
	client := googledrive.NewClient(nil, "http://127.0.0.1:9", revokedTokens{})
	drive := medialib.NewDrive(client, defaultScope(), time.Now)
	library, ok := drive.Lookup(videosLibrary)
	require.True(t, ok)
	_, err := library.Stat(t.Context(), bigName)
	require.ErrorIs(t, err, medialib.ErrNotConnected)
	require.ErrorIs(t, err, medialib.ErrNotFound)
	_, err = library.List(t.Context(), "")
	require.ErrorIs(t, err, medialib.ErrNotFound)
}
