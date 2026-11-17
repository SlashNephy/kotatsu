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
	// folderID と folderPath は、テストの Drive のフォルダの ID と、マイドライブからのパス。
	folderID   = "folder-1"
	folderPath = "root/" + folderID
	// sharedID と sharedName は、テストの共有ドライブ。
	sharedID   = "drive-1"
	sharedName = "Archive"
	// folderName は、テストの Drive のフォルダの名前。tsMime は、TS の MIME タイプ。
	folderName = "videos"
	tsMime     = "video/mp2t"
	// fromHead は、頭から読む Range。
	fromHead = "bytes=0-"
)

// bigContent は、読み出しを確かめる大きめのファイルの中身。
func bigContent() []byte {
	return []byte(strings.Repeat("0123456789", 1000))
}

// newDrive は、次の中身の偽の Drive のライブラリを返す。connected が false なら、接続していないものとする。
//
//	マイドライブ: videos/ (episode.ts, big.ts, memo (Google ドキュメント), .dot.ts), a.ts
//	共有ドライブ Archive: b.ts
func newDrive(t *testing.T, connected bool) (*medialib.Drive, *googletest.Server) {
	t.Helper()
	modified := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	server := googletest.New("client", "secret")
	for _, f := range []*googletest.File{
		{ID: folderID, Name: folderName, MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID},
		{ID: "ep1", Name: "episode.ts", MIMEType: tsMime, Parent: folderID, Content: []byte("episode1")},
		{ID: "big", Name: "big.ts", MIMEType: tsMime, Parent: folderID, Content: bigContent()},
		{ID: "doc", Name: "memo", MIMEType: "application/vnd.google-apps.document", Parent: folderID},
		{ID: "dot", Name: ".dot.ts", MIMEType: tsMime, Parent: folderID, Content: []byte("h")},
		{ID: "a", Name: "a.ts", MIMEType: tsMime, Parent: googletest.MyDriveID, Content: []byte("a")},
		{ID: "b", Name: "b.ts", MIMEType: tsMime, Parent: sharedID, Content: []byte("b")},
	} {
		f.ModifiedTime = modified
		server.AddFile(f)
	}
	server.AddDrive(&googletest.Drive{ID: sharedID, Name: sharedName})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client := googledrive.NewClient(httpServer.Client(), httpServer.URL, &driveTokens{server: server, token: ""})

	return medialib.NewDrive(client, func(context.Context) (bool, error) { return connected, nil }, time.Now), server
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
		// 根元には、マイドライブと共有ドライブを並べる。
		{name: "drives", dir: "", wantPaths: []string{googledrive.MyDriveID, sharedID}, wantNames: []string{medialib.MyDriveName, sharedName}},
		// パスは ID をつなぐ。Google ドキュメントと、. で始まる名前は出さない。
		{name: "my drive", dir: googledrive.MyDriveID, wantPaths: []string{"root/a", folderPath}, wantNames: []string{"a.ts", folderName}},
		{name: "folder", dir: folderPath, wantPaths: []string{folderPath + "/big", folderPath + "/ep1"}, wantNames: []string{"big.ts", "episode.ts"}},
		{name: "shared drive", dir: sharedID, wantPaths: []string{sharedID + "/b"}, wantNames: []string{"b.ts"}},
		{name: "parent", dir: "root/..", wantErr: medialib.ErrInvalidPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			drive, _ := newDrive(t, true)
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
	drive, server := newDrive(t, false)
	_, err := drive.List(t.Context(), "")
	require.ErrorIs(t, err, medialib.ErrNotFound)
	_, err = drive.Stat(t.Context(), "root/a")
	require.ErrorIs(t, err, medialib.ErrNotConnected)
	_, err = drive.Open(t.Context(), "root/a")
	require.ErrorIs(t, err, medialib.ErrNotConnected)
	assert.Empty(t, server.Requests())

	// 接続していなければ、ライブラリの一覧に出さない。
	set := medialib.NewSet(map[string]string{}, []string{}, map[string]string{}).With(drive)
	available, err := set.Available(t.Context())
	require.NoError(t, err)
	assert.Empty(t, available)
	_, ok := set.Get(medialib.GoogleDriveName)
	assert.True(t, ok)
}

func TestDriveSetName(t *testing.T) {
	t.Parallel()
	drive, _ := newDrive(t, true)
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
	drive, server := newDrive(t, true)

	entry, err := drive.Stat(t.Context(), folderPath+"/ep1")
	require.NoError(t, err)
	assert.Equal(t, "episode.ts", entry.Name)
	assert.Equal(t, folderPath+"/ep1", entry.Path)
	assert.Equal(t, int64(8), entry.Size)
	assert.False(t, entry.Dir)
	// 少し前に引いた情報は、問い合わせずに使う。
	requests := len(server.Requests())
	_, err = drive.Stat(t.Context(), folderPath+"/ep1")
	require.NoError(t, err)
	assert.Len(t, server.Requests(), requests)

	_, err = drive.Stat(t.Context(), "root/missing")
	require.ErrorIs(t, err, medialib.ErrNotFound)

	// ファイルを見分ける値は、フォルダをたどった道筋によらず、ファイルの ID にする。
	assert.Equal(t, "ep1", medialib.IdentityOf(drive, folderPath+"/ep1"))
	assert.Equal(t, "ep1", medialib.IdentityOf(drive, sharedID+"/ep1"))

	tests := []struct {
		name string
		path string
		want []string
	}{
		{name: "top", path: "", want: []string{}},
		{name: "my drive", path: folderPath, want: []string{medialib.MyDriveName, folderName}},
		{name: "shared drive", path: sharedID, want: []string{sharedName}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			names, err := medialib.NamesOf(t.Context(), drive, tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, names)
		})
	}
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
			drive, server := newDrive(t, true)
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
	drive, server := newDrive(t, true)
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
	drive, server := newDrive(t, true)
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
	drive := medialib.NewDrive(client, func(context.Context) (bool, error) { return true, nil }, time.Now)
	_, err := drive.Stat(t.Context(), folderPath+"/big")
	require.ErrorIs(t, err, medialib.ErrNotConnected)
	require.ErrorIs(t, err, medialib.ErrNotFound)
	_, err = drive.List(t.Context(), "")
	require.ErrorIs(t, err, medialib.ErrNotFound)
}
