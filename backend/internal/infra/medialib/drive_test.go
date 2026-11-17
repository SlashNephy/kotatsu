package medialib_test

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/googledrive"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/testutil/googletest"
)

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

const (
	// folderID と folderPath は、見せると選んだマイドライブの中のフォルダの ID と、そのパス (根元に並ぶ)。
	folderID   = "folder-1"
	folderPath = folderID
	// seasonID は、選んだフォルダの中のフォルダの ID。
	seasonID = "folder-2"
	// seasonName は、選んだフォルダの中のフォルダの名前。
	seasonName = "season2"
	// secretID は、選んでいないマイドライブの中のフォルダの ID。
	secretID = "folder-3"
	// sharedID と sharedName は、テストの共有ドライブ。subID は、その中の、見せると選んだフォルダの ID。
	sharedID   = "drive-1"
	sharedName = "Archive"
	subID      = "folder-4"
	// folderName は、テストの Drive のフォルダの名前。tsMime は、TS の MIME タイプ。
	folderName = "videos"
	tsMime     = "video/mp2t"
	// fromHead は、頭から読む Range。
	fromHead = "bytes=0-"
	// episodeName は、選んだフォルダの直下のファイルの名前。
	episodeName = "episode.ts"
)

// bigContent は、読み出しを確かめる大きめのファイルの中身。
func bigContent() []byte {
	return []byte(strings.Repeat("0123456789", 1000))
}

// newDriveWith は、次の中身の偽の Drive の、scope で絞ったライブラリを返す。
//
//	マイドライブ: videos/ (episode.ts, big.ts, memo (Google ドキュメント), .dot.ts, season2/ (episode2.ts)), a.ts, secret/ (episode-secret.ts)
//	共有ドライブ Archive: episode-b.ts, sub/ (episode-archive.ts)
func newDriveWith(t *testing.T, scope medialib.DriveScope) (*medialib.Drive, *googletest.Server) {
	t.Helper()
	modified := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	server := googletest.New("client", "secret")
	server.AddDrive(&googletest.Drive{ID: sharedID, Name: sharedName})
	for _, f := range []*googletest.File{
		{ID: folderID, Name: folderName, MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID},
		{ID: "ep1", Name: episodeName, MIMEType: tsMime, Parent: folderID, Content: []byte("episode1")},
		{ID: "big", Name: "big.ts", MIMEType: tsMime, Parent: folderID, Content: bigContent()},
		{ID: "doc", Name: "memo", MIMEType: "application/vnd.google-apps.document", Parent: folderID},
		{ID: "dot", Name: ".dot.ts", MIMEType: tsMime, Parent: folderID, Content: []byte("h")},
		{ID: seasonID, Name: seasonName, MIMEType: googletest.FolderMIMEType, Parent: folderID},
		{ID: "ep2", Name: "episode2.ts", MIMEType: tsMime, Parent: seasonID, Content: []byte("episode2")},
		{ID: "a", Name: "a.ts", MIMEType: tsMime, Parent: googletest.MyDriveID, Content: []byte("a")},
		{ID: secretID, Name: "secret", MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID},
		{ID: "hidden-ep", Name: "episode-secret.ts", MIMEType: tsMime, Parent: secretID, Content: []byte("s")},
		{ID: "b", Name: "episode-b.ts", MIMEType: tsMime, Parent: sharedID, Content: []byte("b")},
		{ID: subID, Name: "sub", MIMEType: googletest.FolderMIMEType, Parent: sharedID},
		{ID: "c", Name: "episode-archive.ts", MIMEType: tsMime, Parent: subID, Content: []byte("c")},
	} {
		f.ModifiedTime = modified
		server.AddFile(f)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client := googledrive.NewClient(httpServer.Client(), httpServer.URL, &driveTokens{server: server, token: ""})

	return medialib.NewDrive(client, scope, time.Now), server
}

// newDrive は、videos と、共有ドライブの sub を見せる、newDriveWith のライブラリを返す。
func newDrive(t *testing.T) (*medialib.Drive, *googletest.Server) {
	t.Helper()

	return newDriveWith(t, googletest.NewScope(&medialib.DriveFolder{ID: folderID, DriveID: ""}, &medialib.DriveFolder{ID: subID, DriveID: sharedID}))
}

func TestDriveList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		dir       string
		wantPaths []string
		wantNames []string
		wantErr   error
	}{
		// 根元には、選んだフォルダを、選んだ順に並べる。
		{name: "selected folders", dir: "", wantPaths: []string{folderID, subID}, wantNames: []string{folderName, "sub"}},
		// パスは ID をつなぐ。Google ドキュメントと、. で始まる名前は出さない。
		{
			name: "folder", dir: folderPath,
			wantPaths: []string{folderPath + "/big", folderPath + "/ep1", folderPath + "/" + seasonID},
			wantNames: []string{"big.ts", episodeName, seasonName},
		},
		{name: "subfolder", dir: folderPath + "/" + seasonID, wantPaths: []string{folderPath + "/" + seasonID + "/ep2"}, wantNames: []string{"episode2.ts"}},
		{name: "folder in a shared drive", dir: subID, wantPaths: []string{subID + "/c"}, wantNames: []string{"episode-archive.ts"}},
		// 選んでいないフォルダと、ドライブの根元は見せない。
		{name: "unselected folder", dir: secretID, wantErr: medialib.ErrNotFound},
		{name: "my drive", dir: googletest.MyDriveID, wantErr: medialib.ErrNotFound},
		{name: "shared drive", dir: sharedID, wantErr: medialib.ErrNotFound},
		// 選んだフォルダから始めても、外のフォルダの ID をつないだパスは受け付けない。
		{name: "forged path", dir: folderPath + "/" + secretID, wantErr: medialib.ErrNotFound},
		{name: "parent", dir: folderPath + "/..", wantErr: medialib.ErrInvalidPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			drive, _ := newDrive(t)
			entries, err := drive.List(t.Context(), tt.dir)
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

func TestDriveNotConnected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		scope *googletest.Scope
	}{
		{name: "not connected", scope: &googletest.Scope{Disconnected: true, Selected: []*medialib.DriveFolder{{ID: folderID, DriveID: ""}}}},
		// 接続していても、見せるフォルダを選ぶまでは出さない。
		{name: "no folders", scope: googletest.NewScope()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			drive, server := newDriveWith(t, tt.scope)
			_, err := drive.List(t.Context(), "")
			require.ErrorIs(t, err, medialib.ErrNotConnected)
			_, err = drive.Stat(t.Context(), folderPath+"/ep1")
			require.ErrorIs(t, err, medialib.ErrNotFound)
			_, err = drive.Open(t.Context(), folderPath+"/ep1")
			require.ErrorIs(t, err, medialib.ErrNotConnected)
			_, _, err = drive.Search(t.Context(), "", "episode", 10)
			require.ErrorIs(t, err, medialib.ErrNotConnected)
			assert.Empty(t, server.Requests())

			// ライブラリの一覧に出さない。
			set := medialib.NewSet(map[string]string{}, []string{}, map[string]string{}).With(drive)
			available, err := set.Available(t.Context())
			require.NoError(t, err)
			assert.Empty(t, available)
			_, ok := set.Get(medialib.GoogleDriveName)
			assert.True(t, ok)
		})
	}
}

func TestDriveSetName(t *testing.T) {
	t.Parallel()
	drive, _ := newDrive(t)
	// 同じ名前のフォルダがあれば、フォルダを出し、Google Drive は足さない。
	set := medialib.NewSet(map[string]string{}, []string{}, map[string]string{medialib.GoogleDriveName: "/mnt/drive"}).With(drive)
	l, ok := set.Get(medialib.GoogleDriveName)
	require.True(t, ok)
	assert.Equal(t, medialib.KindFolder, l.Kind())

	set = medialib.NewSet(map[string]string{}, []string{}, map[string]string{}).With(drive)
	available, err := set.Available(t.Context())
	require.NoError(t, err)
	require.Len(t, available, 1)
	assert.Equal(t, medialib.KindGoogleDrive, available[0].Kind())
}

func TestDriveStatAndNames(t *testing.T) {
	t.Parallel()
	drive, server := newDrive(t)

	entry, err := drive.Stat(t.Context(), folderPath+"/ep1")
	require.NoError(t, err)
	assert.Equal(t, episodeName, entry.Name)
	assert.Equal(t, folderPath+"/ep1", entry.Path)
	assert.Equal(t, int64(8), entry.Size)
	assert.False(t, entry.Dir)
	// 少し前に引いた情報と、選んだフォルダの下にあると確かめた結果は、問い合わせずに使う。
	requests := len(server.Requests())
	_, err = drive.Stat(t.Context(), folderPath+"/ep1")
	require.NoError(t, err)
	assert.Len(t, server.Requests(), requests)

	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{name: "absent", path: folderPath + "/absent", wantErr: medialib.ErrNotFound},
		// ファイルの ID を直接渡されても、選んだフォルダの外のものは見せない。
		{name: "outside file", path: folderPath + "/a", wantErr: medialib.ErrNotFound},
		{name: "file in an unselected folder", path: folderPath + "/hidden-ep", wantErr: medialib.ErrNotFound},
		{name: "unselected root", path: secretID + "/hidden-ep", wantErr: medialib.ErrNotFound},
		// 選んだフォルダの下でも、親をたどった道筋と違うパスは受け付けない。
		{name: "wrong chain", path: folderPath + "/ep2", wantErr: medialib.ErrNotFound},
		{name: "nested", path: folderPath + "/" + seasonID + "/ep2"},
		{name: "shared drive", path: subID + "/c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := drive.Stat(t.Context(), tt.path)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
		})
	}

	// ファイルを見分ける値は、ファイルの ID にする。
	assert.Equal(t, "ep1", medialib.IdentityOf(drive, folderPath+"/ep1"))

	names, err := medialib.NamesOf(t.Context(), drive, folderPath+"/"+seasonID)
	require.NoError(t, err)
	assert.Equal(t, []string{folderName, seasonName}, names)
	_, err = medialib.NamesOf(t.Context(), drive, folderPath+"/"+secretID)
	require.ErrorIs(t, err, medialib.ErrNotFound, "the names outside the folders are not shown")
}

// TestDriveSearch は、名前で探し、選んだフォルダ (と、探すフォルダ) の下にあるものだけを返すことを確かめる。
func TestDriveSearch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		dir           string
		limit         int
		wantPaths     []string
		wantTruncated bool
	}{
		{
			// 選んでいないフォルダのものと、共有ドライブの、選んだフォルダの外のものは返さない。
			name: "all folders", dir: "", limit: 10,
			wantPaths: []string{subID + "/c", folderPath + "/ep1", folderPath + "/" + seasonID + "/ep2"},
		},
		{name: "subfolder", dir: folderPath + "/" + seasonID, limit: 10, wantPaths: []string{folderPath + "/" + seasonID + "/ep2"}},
		{name: "limit", dir: folderPath, limit: 1, wantPaths: []string{folderPath + "/ep1"}, wantTruncated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			drive, _ := newDrive(t)
			entries, truncated, err := drive.Search(t.Context(), tt.dir, "EPISODE", tt.limit)
			require.NoError(t, err)
			paths := []string{}
			for _, entry := range entries {
				paths = append(paths, entry.Path)
			}
			assert.ElementsMatch(t, tt.wantPaths, paths)
			assert.Equal(t, tt.wantTruncated, truncated)
		})
	}

	drive, _ := newDrive(t)
	_, _, err := drive.Search(t.Context(), secretID, "episode", 10)
	require.ErrorIs(t, err, medialib.ErrNotFound)
}

// TestDriveRead は、ファイルを途中から読めることと、続きを読むなら開いた本文を使い回すことを確かめる。
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
			// 少し先へ飛ぶなら読み捨てて進め、戻るなら開き直す。
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
			drive, server := newDrive(t)
			f, err := drive.Open(t.Context(), folderPath+"/big")
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
	drive, server := newDrive(t)
	f, err := drive.Open(t.Context(), folderPath+"/ep1")
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

	_, err = drive.Open(t.Context(), folderPath)
	require.ErrorIs(t, err, medialib.ErrNotFound)
}

// TestDriveReadGivesUp は、途中で切れ続けたら、開き直す回数の上限で諦めることを確かめる。
func TestDriveReadGivesUp(t *testing.T) {
	t.Parallel()
	drive, server := newDrive(t)
	f, err := drive.Open(t.Context(), folderPath+"/big")
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

// TestDriveRevoked は、接続していると覚えていても、トークンを作れなければ、接続していないものとして返すことを確かめる。
// 配信や画像の口とジョブは、ErrNotFound で見分ける。
func TestDriveRevoked(t *testing.T) {
	t.Parallel()
	client := googledrive.NewClient(nil, "http://127.0.0.1:9", revokedTokens{})
	drive := medialib.NewDrive(client, googletest.NewScope(&medialib.DriveFolder{ID: folderID, DriveID: ""}), time.Now)
	_, err := drive.Stat(t.Context(), folderPath+"/big")
	require.ErrorIs(t, err, medialib.ErrNotConnected)
	require.ErrorIs(t, err, medialib.ErrNotFound)
	_, err = drive.List(t.Context(), "")
	require.ErrorIs(t, err, medialib.ErrNotFound)
}
