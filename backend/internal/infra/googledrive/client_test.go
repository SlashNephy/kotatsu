package googledrive_test

import (
	"context"
	"io"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/googledrive"
	"github.com/SlashNephy/kotatsu/internal/testutil/googletest"
)

const (
	// folderID、fileID、driveID は、テストの Drive のフォルダ、ファイル、共有ドライブの ID。
	folderID = "anime"
	fileID   = "ep1"
	driveID  = "shared"
	// fileName は、テストのファイルの名前。folderName、ep2Name、notesName、backupName は、ほかのフォルダとファイルの名前。
	fileName   = "ep1.ts"
	folderName = "anime"
	ep2Name    = "ep2.ts"
	notesName  = "notes"
	backupName = "backup.ts"
	// myDriveCase と sharedDriveCase は、マイドライブと共有ドライブを読むテストの名前。
	myDriveCase     = "my drive"
	sharedDriveCase = "shared drive"
	// tsMime は、TS の MIME タイプ。
	tsMime = "video/mp2t"
)

// modified は、テストのファイルの更新日時を返す。
func modified() time.Time {
	return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
}

// tokens は、モックの Google が発行したアクセストークンを返し、破棄されたら発行し直す TokenSource。
type tokens struct {
	server *googletest.Server

	mu          sync.Mutex
	token       string
	invalidated int
}

func (t *tokens) Token(context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token == "" {
		t.token = t.server.IssueAccessToken()
	}

	return t.token, nil
}

func (t *tokens) Invalidate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.token = ""
	t.invalidated++
}

// newClient は、次の内容のモックの Drive と、そこへ向けたクライアントを返す。やり直す前に待った長さを waits に残す。
//
//	マイドライブ: anime/ (ep1.ts, ep2.ts), notes.gdoc, trashed.ts
//	共有ドライブ shared (Backup): backup.ts
func newClient(t *testing.T) *fixture {
	t.Helper()
	server := googletest.New("client", "secret")
	server.AddFile(&googletest.File{ID: folderID, Name: folderName, MIMEType: googletest.FolderMIMEType, Parent: googletest.MyDriveID, ModifiedTime: modified()})
	server.AddFile(&googletest.File{ID: fileID, Name: fileName, MIMEType: tsMime, Parent: folderID, Content: []byte("0123456789"), ModifiedTime: modified()})
	server.AddFile(&googletest.File{ID: "ep2", Name: ep2Name, MIMEType: tsMime, Parent: folderID, Content: []byte("ab"), ModifiedTime: modified()})
	server.AddFile(&googletest.File{ID: "doc", Name: notesName, MIMEType: "application/vnd.google-apps.document", Parent: googletest.MyDriveID, ModifiedTime: modified()})
	server.AddFile(&googletest.File{ID: "gone", Name: "trashed.ts", MIMEType: tsMime, Parent: googletest.MyDriveID, Content: []byte("x"), ModifiedTime: modified(), Trashed: true})
	server.AddDrive(&googletest.Drive{ID: driveID, Name: "Backup"})
	server.AddFile(&googletest.File{ID: "backup", Name: backupName, MIMEType: tsMime, Parent: driveID, Content: []byte("backup"), ModifiedTime: modified()})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	source := &tokens{server: server, mu: sync.Mutex{}, token: "", invalidated: 0}
	client := googledrive.NewClient(httpServer.Client(), httpServer.URL, source)
	waits := &[]time.Duration{}
	var mu sync.Mutex
	googledrive.SetSleep(client, func(_ context.Context, d time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		*waits = append(*waits, d)

		return nil
	})

	return &fixture{client: client, server: server, tokens: source, waits: waits}
}

// fixture は、モックの Drive と、そこへ向けたクライアント。waits は、やり直す前に待った長さ。
type fixture struct {
	client *googledrive.Client
	server *googletest.Server
	tokens *tokens
	waits  *[]time.Duration
}

func TestClientList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		parent    string
		driveID   string
		wantNames []string
	}{
		// ゴミ箱にあるものは返さない。モックの Drive は 1 件ずつのページで返すので、ページをたどって集める。
		{name: myDriveCase, parent: googledrive.MyDriveID, driveID: "", wantNames: []string{folderName, notesName}},
		{name: "folder", parent: folderID, driveID: "", wantNames: []string{fileName, ep2Name}},
		{name: sharedDriveCase, parent: driveID, driveID: driveID, wantNames: []string{backupName}},
		{name: "empty", parent: fileID, driveID: "", wantNames: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newClient(t).client
			files, err := client.Children(t.Context(), tt.parent, tt.driveID)
			require.NoError(t, err)
			names := []string{}
			for _, f := range files {
				names = append(names, f.Name)
			}
			assert.Equal(t, tt.wantNames, names)
		})
	}
}

// TestClientNamedChildren は、フォルダの中の、名前が決まったものだけを 1 つの問い合わせで取得することを確認する。
func TestClientNamedChildren(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		names   map[string][]string
		driveID string
		wantIDs []string
	}{
		// ゴミ箱にあるもの (trashed.ts) と、ほかのフォルダの同じ名前のものは返さない。
		{
			name: myDriveCase, names: map[string][]string{folderID: {fileName}, googledrive.MyDriveID: {notesName, "trashed.ts", fileName}}, driveID: "",
			wantIDs: []string{"doc", fileID},
		},
		{name: sharedDriveCase, names: map[string][]string{driveID: {backupName}}, driveID: driveID, wantIDs: []string{"backup"}},
		{name: "no names", names: map[string][]string{}, driveID: "", wantIDs: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files, err := newClient(t).client.NamedChildren(t.Context(), tt.names, tt.driveID)
			require.NoError(t, err)
			ids := []string{}
			for _, f := range files {
				ids = append(ids, f.ID)
			}
			assert.ElementsMatch(t, tt.wantIDs, ids)
		})
	}
}

// TestClientSearchFiles は、ページごとに渡し、止められたら残りのページを読まないことを確認する。
func TestClientSearchFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		maxPages      int
		stopAfter     int
		wantNames     []string
		wantTruncated bool
	}{
		// モックの Drive は 1 件ずつのページで返す。フォルダとゴミ箱にあるものは返さない。
		{name: "all pages", maxPages: 5, stopAfter: 0, wantNames: []string{fileName, ep2Name}, wantTruncated: false},
		{name: "stopped", maxPages: 5, stopAfter: 1, wantNames: []string{fileName}, wantTruncated: true},
		{name: "page limit", maxPages: 1, stopAfter: 0, wantNames: []string{fileName}, wantTruncated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			names := []string{}
			truncated, err := newClient(t).client.SearchFiles(t.Context(), "ep", "", tt.maxPages, func(files []*googledrive.File) bool {
				for _, f := range files {
					names = append(names, f.Name)
				}

				return tt.stopAfter == 0 || len(names) < tt.stopAfter
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantNames, names)
			assert.Equal(t, tt.wantTruncated, truncated)
		})
	}
}

// TestClientFolders は、ドライブのフォルダだけを返し、根元とファイルは返さないことを確認する。
func TestClientFolders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		driveID string
		want    []string
	}{
		{name: myDriveCase, driveID: "", want: []string{folderID}},
		{name: sharedDriveCase, driveID: driveID, want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			folders, err := newClient(t).client.Folders(t.Context(), tt.driveID)
			require.NoError(t, err)
			ids := []string{}
			for _, f := range folders {
				ids = append(ids, f.ID)
			}
			assert.Equal(t, tt.want, ids)
		})
	}
}

func TestClientMetadata(t *testing.T) {
	t.Parallel()
	client := newClient(t).client

	account, err := client.About(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "admin@example.com", account)

	drives, err := client.Drives(t.Context())
	require.NoError(t, err)
	require.Len(t, drives, 1)
	assert.Equal(t, "Backup", drives[0].Name)

	file, err := client.Get(t.Context(), fileID)
	require.NoError(t, err)
	assert.Equal(t, &googledrive.File{ID: fileID, Name: fileName, MIMEType: tsMime, Size: 10, ModifiedTime: modified(), Parents: []string{folderID}, DriveID: ""}, file)
	assert.False(t, file.Folder())
	assert.True(t, file.Blob())

	_, err = client.Get(t.Context(), "missing")
	require.ErrorIs(t, err, googledrive.ErrNotFound)
	_, err = client.Get(t.Context(), "gone")
	require.ErrorIs(t, err, googledrive.ErrNotFound)
}

func TestClientDownload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		offset    int64
		want      string
		wantRange string
	}{
		{name: "from the head", offset: 0, want: "0123456789", wantRange: "bytes=0-"},
		{name: "from the middle", offset: 4, want: "456789", wantRange: "bytes=4-"},
		{name: "the last byte", offset: 9, want: "9", wantRange: "bytes=9-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newClient(t)
			client, server := f.client, f.server
			body, err := client.Download(t.Context(), fileID, tt.offset)
			require.NoError(t, err)
			defer body.Close()
			got, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
			assert.Equal(t, []string{tt.wantRange}, server.Ranges())
		})
	}
}

// TestClientRetry は、上限に当たったとき (429、403 の userRateLimitExceeded) と、トークンの期限が切れたとき (401) に、
// やり直すことと、やり直してはならない失敗 (downloadQuotaExceeded) と、やり直しても当たり続けるときの扱いを確認する。
func TestClientRetry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		faults          []googletest.Fault
		expire          bool
		wantErr         error
		wantCalls       int
		wantWaits       int
		wantInvalidated int
	}{
		{name: "rate limited once", faults: []googletest.Fault{googletest.FaultRateLimit}, wantCalls: 2, wantWaits: 1},
		{
			name: "user rate limited twice", faults: []googletest.Fault{googletest.FaultUserRateLimit, googletest.FaultUserRateLimit},
			wantCalls: 3, wantWaits: 2,
		},
		{
			name: "rate limited every time",
			faults: []googletest.Fault{
				googletest.FaultRateLimit, googletest.FaultRateLimit, googletest.FaultRateLimit,
				googletest.FaultRateLimit, googletest.FaultRateLimit, googletest.FaultRateLimit,
			},
			wantErr: googledrive.ErrRateLimited, wantCalls: 6, wantWaits: 5,
		},
		{name: "download quota", faults: []googletest.Fault{googletest.FaultDownloadQuota}, wantErr: googledrive.ErrDownloadQuota, wantCalls: 1},
		{name: "expired token", expire: true, wantCalls: 2, wantWaits: 0, wantInvalidated: 1},
		{
			// 最後の 1 回で期限が切れても、作り直したトークンで 1 度はやり直す。
			name: "token expired on the last attempt",
			faults: []googletest.Fault{
				googletest.FaultRateLimit, googletest.FaultRateLimit, googletest.FaultRateLimit,
				googletest.FaultRateLimit, googletest.FaultRateLimit, googletest.FaultUnauthorized,
			},
			wantCalls: 7, wantWaits: 5, wantInvalidated: 1,
		},
		{
			name: "unauthorized twice", faults: []googletest.Fault{googletest.FaultUnauthorized, googletest.FaultUnauthorized},
			wantErr: googledrive.ErrUnauthorized, wantCalls: 2, wantInvalidated: 1,
		},
		{
			// 待つ長さの合計は、インデックスが作り直される時間より短く収める。Retry-After が長ければ、早めに諦める。
			name: "long Retry-After", faults: []googletest.Fault{googletest.FaultLongRateLimit, googletest.FaultLongRateLimit},
			wantErr: googledrive.ErrRateLimited, wantCalls: 2, wantWaits: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newClient(t)
			client, server, source, waits := f.client, f.server, f.tokens, f.waits
			// 先にトークンを作らせてから、期限を切る。
			_, err := source.Token(t.Context())
			require.NoError(t, err)
			if tt.expire {
				server.ExpireAccessTokens()
			}
			server.Inject(tt.faults...)
			body, err := client.Download(t.Context(), fileID, 0)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				got, err := io.ReadAll(body)
				require.NoError(t, err)
				require.NoError(t, body.Close())
				assert.Equal(t, "0123456789", string(got))
			}
			assert.Len(t, server.Requests(), tt.wantCalls)
			assert.Len(t, *waits, tt.wantWaits)
			assert.Equal(t, tt.wantInvalidated, source.invalidated)
		})
	}
}

// TestClientRangeIgnored は、範囲を無視して頭から返されたら、頭から読むときだけ使うことを確認する。
func TestClientRangeIgnored(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		offset  int64
		wantErr error
	}{
		{name: "from the head", offset: 0},
		{name: "from the middle", offset: 4, wantErr: googledrive.ErrRangeIgnored},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newClient(t)
			f.server.Inject(googletest.FaultIgnoreRange)
			body, err := f.client.Download(t.Context(), fileID, tt.offset)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			require.NoError(t, body.Close())
		})
	}
}

// TestClientRetryAfter は、429 の Retry-After に従って待つことを確認する。
func TestClientRetryAfter(t *testing.T) {
	t.Parallel()
	f := newClient(t)
	client, server, waits := f.client, f.server, f.waits
	server.Inject(googletest.FaultRateLimit)
	_, err := client.Get(t.Context(), fileID)
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{time.Second}, *waits)
}
