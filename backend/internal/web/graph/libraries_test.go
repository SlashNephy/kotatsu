package graph_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	librariesQuery         = `{ libraries { name kind } }`
	libraryDirectoryQuery  = `query($library: String!, $path: String!) { libraryDirectory(library: $library, path: $path) { path library { name } entries { name path directory size playable } } }`
	libraryFileQuery       = `query($library: String!, $path: String!) { libraryFile(library: $library, path: $path) { id name size videoFormat watchPositionMs streamUrl hlsUrl index { complete seekableSeconds durationSeconds } } }`
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
)

type libraryFileResult struct {
	LibraryFile struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		Size            int64  `json:"size"`
		VideoFormat     string `json:"videoFormat"`
		WatchPositionMs *int64 `json:"watchPositionMs"`
		StreamURL       string `json:"streamUrl"`
		HlsURL          string `json:"hlsUrl"`
		Index           struct {
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

// newLibraryFixture は、保存先プール main と、フォルダのライブラリ anime を持つ GraphQL のサーバーを立てる。
// anime には、番組の TS (show/ep1.ts)、再生できないファイル、隠しファイル、Blu-ray の .m2ts (disc.m2ts) を置く。
func newLibraryFixture(t *testing.T) (*fixture, string) {
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
	libraries := medialib.NewSet(map[string]string{poolMain: poolDir}, []string{poolMain}, map[string]string{animeLibrary: libraryDir})
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

	return newFixture(t, &graph.Options{Libraries: libraries, LibrarySource: filelib.NewSource(libraries, indexer)}), libraryDir
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got struct {
				LibraryDirectory struct {
					Path    string  `json:"path"`
					Entries []entry `json:"entries"`
				} `json:"libraryDirectory"`
			}
			codes := f.query(t, libraryDirectoryQuery, map[string]any{varLibrary: tt.library, varPath: tt.path}, &got)
			assert.Equal(t, tt.wantCodes, nonNil(codes))
			if len(tt.wantCodes) == 0 {
				assert.Equal(t, tt.path, got.LibraryDirectory.Path)
				assert.Equal(t, tt.want, got.LibraryDirectory.Entries)
			}
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

	f, libraryDir := newLibraryFixture(t)
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
	assert.Equal(t, "ep1.ts", file.LibraryFile.Name)
	assert.Equal(t, "HD", file.LibraryFile.VideoFormat)
	assert.Equal(t, "/internal/streams/library/"+encoded, file.LibraryFile.StreamURL)
	assert.Positive(t, file.LibraryFile.Index.SeekableSeconds)
	require.NotNil(t, file.LibraryFile.Index.DurationSeconds)
	assert.GreaterOrEqual(t, *file.LibraryFile.Index.DurationSeconds, file.LibraryFile.Index.SeekableSeconds)
	assert.Nil(t, file.LibraryFile.WatchPositionMs)

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
