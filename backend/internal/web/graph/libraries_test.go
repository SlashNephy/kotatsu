package graph_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/googledrive"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/googletest"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	librariesQuery         = `{ libraries { name kind } }`
	libraryDirectoryQuery  = `query($library: String!, $path: String!) { libraryDirectory(library: $library, path: $path) { path names library { name } entries { name path directory size playable } } }`
	libraryFileQuery       = `query($library: String!, $path: String!) { libraryFile(library: $library, path: $path) { id fileId name size videoFormat watchPositionMs streamUrl hlsUrl jikkyoCommentsUrl thumbnailUrl previewUrl cmSections { startSeconds endSeconds } autoCaptions { startSeconds endSeconds text } autoCaptionsStatus { state model count } broadcast { networkId serviceId serviceName service { name } jikkyoChannelId recordedFrom program { eventId name endAt } } quality { drops } index { complete seekableSeconds durationSeconds } } }`
	generateLibraryCaps    = `mutation($id: ID!) { generateLibraryCaptions(id: $id) { id autoCaptionsStatus { state } } }`
	saveLibraryPosition    = `mutation($id: ID!, $positionMs: Int64!) { saveLibraryWatchPosition(id: $id, positionMs: $positionMs) { id watchPositionMs } }`
	libraryTestFileContent = "../../../testdata/ts/test.ts"
	// animeLibrary は、テストのフォルダのライブラリの名前。discM2TS と episodePath は、その中のファイルのパス。
	animeLibrary = "anime"
	discM2TS     = "disc.m2ts"
	episodePath  = "show/ep1.ts"
	showDir      = "show"
	// varLibrary と varPath は、ライブラリとパスを渡す変数の名前。
	varLibrary = "library"
	varPath    = "path"
	// varQuery は、検索の語を渡す変数の名前。
	varQuery = "query"
)

type libraryFileResult struct {
	LibraryFile struct {
		ID                string  `json:"id"`
		FileID            string  `json:"fileId"`
		Name              string  `json:"name"`
		Size              int64   `json:"size"`
		VideoFormat       string  `json:"videoFormat"`
		WatchPositionMs   *int64  `json:"watchPositionMs"`
		StreamURL         string  `json:"streamUrl"`
		HlsURL            string  `json:"hlsUrl"`
		JikkyoCommentsURL string  `json:"jikkyoCommentsUrl"`
		ThumbnailURL      *string `json:"thumbnailUrl"`
		PreviewURL        *string `json:"previewUrl"`
		AutoCaptions      []struct {
			StartSeconds float64 `json:"startSeconds"`
			EndSeconds   float64 `json:"endSeconds"`
			Text         string  `json:"text"`
		} `json:"autoCaptions"`
		AutoCaptionsStatus struct {
			State string  `json:"state"`
			Model *string `json:"model"`
			Count int     `json:"count"`
		} `json:"autoCaptionsStatus"`
		CMSections []struct {
			StartSeconds float64 `json:"startSeconds"`
			EndSeconds   float64 `json:"endSeconds"`
		} `json:"cmSections"`
		Broadcast *struct {
			NetworkID   *int                   `json:"networkId"`
			ServiceID   int                    `json:"serviceId"`
			ServiceName string                 `json:"serviceName"`
			Service     *struct{ Name string } `json:"service"`
			Program     *struct {
				EventID int        `json:"eventId"`
				Name    string     `json:"name"`
				EndAt   *time.Time `json:"endAt"`
			} `json:"program"`
		} `json:"broadcast"`
		Quality *struct {
			Drops int64 `json:"drops"`
		} `json:"quality"`
		Index struct {
			Complete        bool     `json:"complete"`
			SeekableSeconds float64  `json:"seekableSeconds"`
			DurationSeconds *float64 `json:"durationSeconds"`
		} `json:"index"`
	} `json:"libraryFile"`
}

// loadLibraryTS は、testdata/ts/test.ts を返す。
func loadLibraryTS(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.FromSlash(libraryTestFileContent))
	require.NoError(t, err)

	return data
}

// libraryParts は、ライブラリのテストで、サーバーの外から触る部品。
type libraryParts struct {
	// dir は、ライブラリ anime のディレクトリ。
	dir string
	// images は、画像を作るジョブを入れる代わりに、入れたジョブを覚える。
	images *fakeImages
	// indexer は、索引と画像の置き場を読み書きする。
	indexer *filelib.Indexer
	// dataDir は、データディレクトリ。
	dataDir string
}

// newLibraryFixture は、保存先プール main と、フォルダのライブラリ anime を持つ GraphQL のサーバーを立てる。
// anime には、番組の TS (show/ep1.ts)、再生できないファイル、隠しファイル、Blu-ray の .m2ts (disc.m2ts) を置く。
func newLibraryFixture(t *testing.T) (*fixture, *libraryParts) {
	t.Helper()

	poolDir, libraryDir, dataDir := t.TempDir(), t.TempDir(), t.TempDir()
	data := loadLibraryTS(t)
	require.NoError(t, os.MkdirAll(filepath.Join(libraryDir, showDir), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(libraryDir, showDir, "ep1.ts"), data, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(libraryDir, "notes.txt"), []byte("notes"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(libraryDir, ".hidden.ts"), data, 0o600))
	bdav := make([]byte, 192*64)
	for k := range 64 {
		bdav[k*192+4] = 0x47
	}
	require.NoError(t, os.WriteFile(filepath.Join(libraryDir, discM2TS), bdav, 0o600))
	// 接続していない Google Drive は、一覧に出さず、開けない (#1902)。
	disconnected := medialib.NewDrive(googledrive.NewClient(nil, "http://127.0.0.1:9", nil),
		&googletest.Scope{Disconnected: true, Selected: []*medialib.DriveFolder{}}, time.Now)
	libraries := medialib.NewSet(map[string]string{poolMain: poolDir}, []string{poolMain}, map[string]string{animeLibrary: libraryDir}).
		With(disconnected)
	// 置いたばかりのファイルは書いている途中とみなされるので、時計を進めておく。
	indexer := filelib.NewIndexer(dataDir, libraries, clocktest.NewFake(time.Now().Add(time.Hour)), slog.New(slog.DiscardHandler), true)
	// テストを終えるときに、索引を作る処理を止めてから一時ディレクトリを消させる。
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

	images := new(fakeImages)

	f := newFixture(t, &graph.Options{Libraries: libraries, LibrarySource: filelib.NewSource(libraries, indexer), LibraryJobs: images})
	// 字幕の自動生成の状態は、River のジョブの表から読む。
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))

	return f, &libraryParts{dir: libraryDir, images: images, indexer: indexer, dataDir: dataDir}
}

// TestLibraries は、保存先プールとフォルダをライブラリとして返し、ディレクトリの中身は、ディレクトリと再生できるファイルだけを返すことを確かめる (#1734)。
func TestLibraries(t *testing.T) {
	t.Parallel()

	f, _ := newLibraryFixture(t)
	viewer := f.as(t, "viewer", domain.RoleViewer)
	var libraries struct {
		Libraries []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"libraries"`
	}
	require.Empty(t, viewer.query(t, librariesQuery, map[string]any{}, &libraries), "視聴者も使える")
	require.Len(t, libraries.Libraries, 2)
	assert.Equal(t, poolMain, libraries.Libraries[0].Name)
	assert.Equal(t, "POOL", libraries.Libraries[0].Kind)
	assert.Equal(t, animeLibrary, libraries.Libraries[1].Name)
	assert.Equal(t, "FOLDER", libraries.Libraries[1].Kind)

	type entry struct {
		Name      string `json:"name"`
		Path      string `json:"path"`
		Directory bool   `json:"directory"`
		Size      int64  `json:"size"`
		Playable  bool   `json:"playable"`
	}
	tests := []struct {
		name      string
		library   string
		path      string
		wantCodes []string
		want      []entry
	}{
		{
			name: "root", library: animeLibrary, path: "",
			// ディレクトリを先に、再生できるファイルを後に並べる。再生できないファイルと隠しファイルは返さない。
			want: []entry{{Name: showDir, Path: showDir, Directory: true}, {Name: discM2TS, Path: discM2TS, Size: 192 * 64, Playable: true}},
		},
		{name: "subdirectory", library: animeLibrary, path: showDir, want: []entry{{Name: "ep1.ts", Path: episodePath, Size: 1125744, Playable: true}}},
		{name: "outside", library: animeLibrary, path: "..", wantCodes: []string{codeBadUserInput}},
		{name: "missing directory", library: animeLibrary, path: "missing", wantCodes: []string{codeNotFound}},
		{name: "missing library", library: "missing", path: "", wantCodes: []string{codeNotFound}},
		{name: "disconnected Google Drive", library: medialib.GoogleDriveName, path: "", wantCodes: []string{codeNotFound}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got struct {
				LibraryDirectory struct {
					Path    string   `json:"path"`
					Names   []string `json:"names"`
					Entries []entry  `json:"entries"`
				} `json:"libraryDirectory"`
			}
			codes := f.query(t, libraryDirectoryQuery, map[string]any{varLibrary: tt.library, varPath: tt.path}, &got)
			assert.Equal(t, tt.wantCodes, nonNil(codes))
			if len(tt.wantCodes) == 0 {
				assert.Equal(t, tt.path, got.LibraryDirectory.Path)
				assert.Equal(t, tt.want, got.LibraryDirectory.Entries)
				// フォルダのライブラリでは、パンくずの名前は、パスの段そのもの。
				wantNames := []string{}
				if tt.path != "" {
					wantNames = strings.Split(tt.path, "/")
				}
				assert.Equal(t, wantNames, got.LibraryDirectory.Names)
			}
		})
	}
}

// TestLibrarySearch は、フォルダのライブラリで、名前の部分一致でディレクトリの下を探せることを確かめる (#1902)。
func TestLibrarySearch(t *testing.T) {
	t.Parallel()

	f, _ := newLibraryFixture(t)
	viewer := f.as(t, "viewer", domain.RoleViewer)
	const search = `query($library: String!, $path: String!, $query: String!) {
		librarySearch(library: $library, path: $path, query: $query) { truncated hits { folderNames entry { name path playable } } } }`
	type result struct {
		LibrarySearch struct {
			Truncated bool `json:"truncated"`
			Hits      []struct {
				FolderNames []string `json:"folderNames"`
				Entry       struct {
					Name     string `json:"name"`
					Path     string `json:"path"`
					Playable bool   `json:"playable"`
				} `json:"entry"`
			} `json:"hits"`
		} `json:"librarySearch"`
	}
	tests := []struct {
		name      string
		library   string
		path      string
		query     string
		wantPaths []string
		wantCodes []string
	}{
		{name: "root", library: animeLibrary, path: "", query: "EP1", wantPaths: []string{episodePath}},
		{name: "folder", library: animeLibrary, path: showDir, query: "ts", wantPaths: []string{episodePath}},
		{name: "no match", library: animeLibrary, path: "", query: "notes", wantPaths: []string{}},
		{name: "empty query", library: animeLibrary, path: "", query: "  ", wantCodes: []string{codeBadUserInput}},
		{name: "parent", library: animeLibrary, path: "..", query: "ts", wantCodes: []string{codeBadUserInput}},
		{name: "disconnected Google Drive", library: medialib.GoogleDriveName, path: "", query: "ts", wantCodes: []string{codeNotFound}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got result
			codes := viewer.query(t, search, map[string]any{varLibrary: tt.library, varPath: tt.path, varQuery: tt.query}, &got)
			assert.Equal(t, tt.wantCodes, nonNil(codes))
			if len(tt.wantCodes) > 0 {
				return
			}
			paths := []string{}
			for _, hit := range got.LibrarySearch.Hits {
				paths = append(paths, hit.Entry.Path)
				assert.True(t, hit.Entry.Playable)
				// フォルダの名前は、パスの段そのもの。
				assert.Equal(t, []string{showDir}, hit.FolderNames)
			}
			assert.Equal(t, tt.wantPaths, paths)
			assert.False(t, got.LibrarySearch.Truncated)
		})
	}
}

// nonNil は、空のエラーコードの一覧を nil にそろえる。
func nonNil(codes []string) []string {
	if len(codes) == 0 {
		return nil
	}

	return codes
}

// TestLibraryFile は、ファイルを開くと索引を作り始め、作り終えたらシークできる所と長さを返し、再生位置をユーザーごとに保存することを確かめる (#1734)。
func TestLibraryFile(t *testing.T) {
	t.Parallel()

	f, parts := newLibraryFixture(t)
	libraryDir, images, indexer := parts.dir, parts.images, parts.indexer
	variables := map[string]any{varLibrary: animeLibrary, varPath: episodePath}
	var file libraryFileResult
	require.Eventually(t, func() bool {
		require.Empty(t, f.query(t, libraryFileQuery, variables, &file))

		return file.LibraryFile.Index.Complete
	}, 10*time.Second, 20*time.Millisecond)
	encoded := filelib.Ref{Library: animeLibrary, Path: episodePath}.Encode()
	info, err := os.Stat(filepath.Join(libraryDir, filepath.FromSlash(episodePath)))
	require.NoError(t, err)
	fileID := filelib.FileID(filelib.Ref{Library: animeLibrary, Path: episodePath}, info.Size(), info.ModTime())
	assert.Equal(t, graph.GlobalID("LibraryFile", fileID+"."+encoded), file.LibraryFile.ID)
	assert.Equal(t, fileID, file.LibraryFile.FileID, "端末に保存するときの名前に使う、パスを含まない ID")
	assert.Equal(t, "ep1.ts", file.LibraryFile.Name)
	assert.Equal(t, "HD", file.LibraryFile.VideoFormat)
	assert.Equal(t, "/internal/streams/library/"+encoded, file.LibraryFile.StreamURL)
	assert.Positive(t, file.LibraryFile.Index.SeekableSeconds)
	require.NotNil(t, file.LibraryFile.Index.DurationSeconds)
	assert.GreaterOrEqual(t, *file.LibraryFile.Index.DurationSeconds, file.LibraryFile.Index.SeekableSeconds)
	assert.Nil(t, file.LibraryFile.WatchPositionMs)
	// 放送の情報と実況の過去ログの URL (#1734 の 2 段目)。局は Kotatsu にないので、service は null で、SDT の名前を返す。
	assert.Equal(t, "/internal/media/library/"+encoded+"/jikkyo.json", file.LibraryFile.JikkyoCommentsURL)
	// 画像はまだないので null で、索引を作り終えたファイルを引くと、作るジョブを入れる。
	assert.Nil(t, file.LibraryFile.ThumbnailURL)
	assert.Nil(t, file.LibraryFile.PreviewURL)
	assert.Contains(t, images.kinds(), "library_images_generate")
	// CM の区間も、まだ見分けていないので null で、見分けるジョブを入れる。
	assert.Nil(t, file.LibraryFile.CMSections)
	assert.Contains(t, images.kinds(), "library_cm_detect")
	// 画像を作り終えたら、ファイルの ID を付けた URL を返す。プレビュー画像を作っていなければ、代表画像だけを返す。
	ref := filelib.Ref{Library: animeLibrary, Path: episodePath}
	dir, err := indexer.ImageDir(ref)
	require.NoError(t, err)
	require.NoError(t, filelib.WriteFile(dir, filelib.ThumbnailFileName, []byte("jpeg")))
	require.NoError(t, indexer.MarkImages(ref, fileID))
	var made libraryFileResult
	require.Empty(t, f.query(t, libraryFileQuery, variables, &made))
	assert.Equal(t, new("/internal/media/library/"+encoded+"/thumbnail.jpg?v="+fileID), made.LibraryFile.ThumbnailURL)
	assert.Nil(t, made.LibraryFile.PreviewURL)
	require.NoError(t, filelib.WriteFile(dir, filelib.PreviewFileName, []byte("jpeg")))
	require.Empty(t, f.query(t, libraryFileQuery, variables, &made))
	assert.Equal(t, new("/internal/media/library/"+encoded+"/preview.jpg?v="+fileID), made.LibraryFile.PreviewURL)
	// 見分けたら、区間を再生の 0 秒からの秒で返す。
	require.NoError(t, indexer.WriteCM(ref, &filelib.CMResult{
		FileID: fileID, Detector: "ffmpeg", Version: 1, Note: "",
		Sections: []*filelib.CMSection{{Start: 1500 * time.Millisecond, End: 16500 * time.Millisecond}},
	}))
	require.Empty(t, f.query(t, libraryFileQuery, variables, &made))
	require.Len(t, made.LibraryFile.CMSections, 1)
	assert.InDelta(t, 1.5, made.LibraryFile.CMSections[0].StartSeconds, 1e-9)
	assert.InDelta(t, 16.5, made.LibraryFile.CMSections[0].EndSeconds, 1e-9)

	// 字幕の自動生成は、頼まれたときだけ。視聴者は頼めない。
	assert.Empty(t, made.LibraryFile.AutoCaptions)
	assert.Equal(t, "NOT_GENERATED", made.LibraryFile.AutoCaptionsStatus.State)
	assert.Equal(t, []string{codeForbidden},
		f.as(t, "captions-viewer", domain.RoleViewer).query(t, generateLibraryCaps, map[string]any{"id": made.LibraryFile.ID}, nil))
	require.Empty(t, f.query(t, generateLibraryCaps, map[string]any{"id": made.LibraryFile.ID}, nil))
	assert.Contains(t, images.kinds(), "library_auto_captions")
	require.NoError(t, indexer.WriteCaptions(ref, &filelib.CaptionsResult{
		FileID: fileID, Model: "ggml-model.bin", GeneratedAt: time.Now(),
		Cues: []*filelib.CaptionCue{{Start: 2 * time.Second, End: 3500 * time.Millisecond, Text: "こんにちは"}},
	}))
	require.Empty(t, f.query(t, libraryFileQuery, variables, &made))
	require.Len(t, made.LibraryFile.AutoCaptions, 1)
	assert.InDelta(t, 2, made.LibraryFile.AutoCaptions[0].StartSeconds, 1e-9)
	assert.InDelta(t, 3.5, made.LibraryFile.AutoCaptions[0].EndSeconds, 1e-9)
	assert.Equal(t, "こんにちは", made.LibraryFile.AutoCaptions[0].Text)
	assert.Equal(t, "GENERATED", made.LibraryFile.AutoCaptionsStatus.State)
	assert.Equal(t, 1, made.LibraryFile.AutoCaptionsStatus.Count)
	require.NotNil(t, file.LibraryFile.Broadcast)
	assert.Equal(t, new(0x7FE0), file.LibraryFile.Broadcast.NetworkID)
	assert.Equal(t, 0x0400, file.LibraryFile.Broadcast.ServiceID)
	assert.Equal(t, "こたつテスト１", file.LibraryFile.Broadcast.ServiceName)
	assert.Nil(t, file.LibraryFile.Broadcast.Service)
	require.NotNil(t, file.LibraryFile.Broadcast.Program)
	assert.Equal(t, "テスト番組", file.LibraryFile.Broadcast.Program.Name)
	require.NotNil(t, file.LibraryFile.Broadcast.Program.EndAt)
	require.NotNil(t, file.LibraryFile.Quality)
	assert.Equal(t, int64(0), file.LibraryFile.Quality.Drops)

	var saved struct {
		SaveLibraryWatchPosition struct {
			WatchPositionMs *int64 `json:"watchPositionMs"`
		} `json:"saveLibraryWatchPosition"`
	}
	require.Empty(t, f.query(t, saveLibraryPosition, map[string]any{"id": file.LibraryFile.ID, varPositionMs: 4500}, &saved))
	require.NotNil(t, saved.SaveLibraryWatchPosition.WatchPositionMs)
	assert.Equal(t, int64(4500), *saved.SaveLibraryWatchPosition.WatchPositionMs)
	// 見ている間にファイルを書き換えたら、前のファイルの ID での保存は、書き換えた後のファイルの位置にしない。
	later := info.ModTime().Add(time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(libraryDir, filepath.FromSlash(episodePath)), later, later))
	var stale struct {
		SaveLibraryWatchPosition struct {
			ID              string `json:"id"`
			WatchPositionMs *int64 `json:"watchPositionMs"`
		} `json:"saveLibraryWatchPosition"`
	}
	require.Empty(t, f.query(t, saveLibraryPosition, map[string]any{"id": file.LibraryFile.ID, varPositionMs: 9000}, &stale))
	assert.NotEqual(t, file.LibraryFile.ID, stale.SaveLibraryWatchPosition.ID)
	assert.Nil(t, stale.SaveLibraryWatchPosition.WatchPositionMs)
	// 書き換えた後のファイルに、前のファイルの ID で字幕の生成は頼めない。
	assert.Equal(t, []string{codeConflict}, f.query(t, generateLibraryCaps, map[string]any{"id": file.LibraryFile.ID}, nil))
	// ほかのユーザーには、その再生位置を返さない。
	var other libraryFileResult
	require.Empty(t, f.as(t, "other", domain.RoleMember).query(t, libraryFileQuery, variables, &other))
	assert.Nil(t, other.LibraryFile.WatchPositionMs)

	tests := []struct {
		name      string
		query     string
		variables map[string]any
		wantCodes []string
	}{
		{name: "Blu-ray", query: libraryFileQuery, variables: map[string]any{varLibrary: animeLibrary, varPath: discM2TS}, wantCodes: []string{codeBadUserInput}},
		{name: "not playable", query: libraryFileQuery, variables: map[string]any{varLibrary: animeLibrary, varPath: "notes.txt"}, wantCodes: []string{codeNotFound}},
		{name: "outside", query: libraryFileQuery, variables: map[string]any{varLibrary: animeLibrary, varPath: "../ep1.ts"}, wantCodes: []string{codeBadUserInput}},
		{name: "directory", query: libraryFileQuery, variables: map[string]any{varLibrary: animeLibrary, varPath: showDir}, wantCodes: []string{codeNotFound}},
		{name: "invalid ID", query: saveLibraryPosition, variables: map[string]any{"id": graph.GlobalID("Recording", encoded), varPositionMs: 1}, wantCodes: []string{codeBadUserInput}},
		{name: "negative position", query: saveLibraryPosition, variables: map[string]any{"id": file.LibraryFile.ID, varPositionMs: -1}, wantCodes: []string{codeBadUserInput}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantCodes, f.query(t, tt.query, tt.variables, nil))
		})
	}
}

// fakeImages は、ライブラリのファイルの画像や CM の区間のジョブを入れる代わりに、入れたジョブの種類を覚える。
type fakeImages struct {
	mu   sync.Mutex
	args []river.JobArgs
}

func (f *fakeImages) Insert(_ context.Context, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.args = append(f.args, args)

	return &rivertype.JobInsertResult{Job: nil, UniqueSkippedAsDuplicate: false}, nil
}

func (f *fakeImages) kinds() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	kinds := make([]string, 0, len(f.args))
	for _, args := range f.args {
		kinds = append(kinds, args.Kind())
	}

	return kinds
}
