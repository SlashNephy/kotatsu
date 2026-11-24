package medialib_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
)

const (
	// aTS と insideTS は、テストのライブラリのファイルの名前。
	aTS      = "a.ts"
	insideTS = "inside.ts"
	bM2TS    = "b.m2ts"
	showDir  = "show"
	notesTXT = "notes.txt"
	// episodeTS は、ディレクトリの中のファイルのパス。
	episodeTS = "show/ep1.ts"
	// mainPool と subPool は、テストの保存先プールの ID。
	mainPool = "main"
	subPool  = "sub"
)

// newLibrary は、次の内容のライブラリを作る。
//
//	a.ts, b.m2ts, notes.txt, .hidden.ts, lost+found/, show/ep1.ts
//	inside.ts -> a.ts (中を指すリンク)、escape.ts -> 外のファイル、escape-dir -> 外のディレクトリ、dangling.ts -> ないファイル
func newLibrary(t *testing.T) *medialib.Local {
	t.Helper()
	dir, outside := t.TempDir(), t.TempDir()
	for _, name := range []string{aTS, bM2TS, notesTXT, ".hidden.ts", episodeTS} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "lost+found"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.ts"), []byte("secret"), 0o600))
	require.NoError(t, os.Symlink(aTS, filepath.Join(dir, insideTS)))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.ts"), filepath.Join(dir, "escape.ts")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "escape-dir")))
	require.NoError(t, os.Symlink("missing.ts", filepath.Join(dir, "dangling.ts")))
	modified := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(dir, aTS), modified, modified))

	return medialib.NewLocal("lib", dir, medialib.KindFolder)
}

func TestLocalList(t *testing.T) {
	t.Parallel()
	library := newLibrary(t)

	tests := []struct {
		name      string
		dir       string
		wantNames []string
		wantErr   error
	}{
		// 隠しファイル、lost+found、外を指すリンク、指す先のないリンクは出さない。中を指すリンクは、指す先の種類で出す。
		{name: "root", dir: "", wantNames: []string{aTS, bM2TS, insideTS, notesTXT, showDir}},
		{name: "dot is the root", dir: ".", wantNames: []string{aTS, bM2TS, insideTS, notesTXT, showDir}},
		{name: "subdirectory", dir: showDir, wantNames: []string{"ep1.ts"}},
		{name: "parent", dir: "..", wantErr: medialib.ErrInvalidPath},
		{name: "traversal", dir: "show/../..", wantErr: medialib.ErrInvalidPath},
		{name: "absolute", dir: "/etc", wantErr: medialib.ErrInvalidPath},
		{name: "backslash", dir: `show\..`, wantErr: medialib.ErrInvalidPath},
		{name: "hidden", dir: ".hidden", wantErr: medialib.ErrInvalidPath},
		{name: "link to the outside", dir: "escape-dir", wantErr: medialib.ErrNotFound},
		{name: "missing", dir: "nothing", wantErr: medialib.ErrNotFound},
		{name: "file", dir: aTS, wantErr: medialib.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entries, err := library.List(t.Context(), tt.dir)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			names := []string{}
			for _, entry := range entries {
				names = append(names, entry.Name)
			}
			assert.Equal(t, tt.wantNames, names)
		})
	}
}

// TestLocalSearch は、名前の部分一致で、フォルダの下の再生できるファイルを探し、隠しファイル、再生できないファイル、
// シンボリックリンク (ライブラリの外を指すものを含む) を返さないことを確認する (#1902)。
func TestLocalSearch(t *testing.T) {
	t.Parallel()
	library := newLibrary(t)

	tests := []struct {
		name          string
		dir           string
		query         string
		limit         int
		wantPaths     []string
		wantTruncated bool
		wantErr       error
	}{
		{name: "everything with ts", dir: "", query: "TS", limit: 10, wantPaths: []string{aTS, bM2TS, episodeTS}},
		{name: "in a folder", dir: showDir, query: "ep", limit: 10, wantPaths: []string{episodeTS}},
		{name: "no match", dir: "", query: "secret", limit: 10, wantPaths: []string{}},
		{name: "limit", dir: "", query: ".ts", limit: 1, wantPaths: []string{aTS}, wantTruncated: true},
		{name: "above the root", dir: "..", query: "ts", limit: 10, wantErr: medialib.ErrInvalidPath},
		{name: "not a directory", dir: aTS, query: "ts", limit: 10, wantErr: medialib.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entries, truncated, err := library.Search(t.Context(), tt.dir, tt.query, tt.limit)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			paths := []string{}
			for _, entry := range entries {
				paths = append(paths, entry.Path)
			}
			assert.Equal(t, tt.wantPaths, paths)
			assert.Equal(t, tt.wantTruncated, truncated)
		})
	}
}

func TestLocalStatAndOpen(t *testing.T) {
	t.Parallel()
	library := newLibrary(t)

	tests := []struct {
		name     string
		path     string
		wantBody string
		wantErr  error
	}{
		{name: "file", path: aTS, wantBody: aTS},
		{name: "nested file", path: episodeTS, wantBody: episodeTS},
		{name: "link inside", path: insideTS, wantBody: aTS},
		{name: "link to an outside file", path: "escape.ts", wantErr: medialib.ErrNotFound},
		{name: "file through a link to the outside", path: "escape-dir/secret.ts", wantErr: medialib.ErrNotFound},
		{name: "traversal", path: "show/../../secret.ts", wantErr: medialib.ErrInvalidPath},
		{name: "absolute", path: "/etc/passwd", wantErr: medialib.ErrInvalidPath},
		{name: "hidden", path: ".hidden.ts", wantErr: medialib.ErrInvalidPath},
		{name: "missing", path: "missing.ts", wantErr: medialib.ErrNotFound},
		{name: "directory", path: showDir, wantErr: medialib.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, err := library.Open(t.Context(), tt.path)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			defer f.Close()
			body := make([]byte, len(tt.wantBody))
			_, err = f.ReadAt(body, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBody, string(body))
			entry, err := library.Stat(t.Context(), tt.path)
			require.NoError(t, err)
			assert.Equal(t, int64(len(tt.wantBody)), entry.Size)
			assert.False(t, entry.Dir)
		})
	}

	entry, err := library.Stat(t.Context(), aTS)
	require.NoError(t, err)
	assert.Equal(t, &medialib.Entry{Name: aTS, Path: aTS, Dir: false, Size: 4, ModifiedAt: entry.ModifiedAt}, entry)
	assert.True(t, entry.ModifiedAt.Equal(time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)))
}

func TestSet(t *testing.T) {
	t.Parallel()
	set := medialib.NewSet(map[string]string{subPool: "/mnt/sub", mainPool: "/mnt/main"}, []string{subPool, mainPool},
		map[string]string{"zeta": "/mnt/zeta", "anime": "/mnt/anime"})
	all := set.All()
	names := make([]string, 0, len(all))
	kinds := make([]medialib.Kind, 0, len(all))
	for _, library := range all {
		names = append(names, library.Name())
		kinds = append(kinds, library.Kind())
	}
	// プールを設定の順に、フォルダを名前の順に並べる。
	assert.Equal(t, []string{subPool, mainPool, "anime", "zeta"}, names)
	assert.Equal(t, []medialib.Kind{medialib.KindPool, medialib.KindPool, medialib.KindFolder, medialib.KindFolder}, kinds)
	_, ok := set.Get("anime")
	assert.True(t, ok)
	_, ok = set.Get("missing")
	assert.False(t, ok)
}

func TestPlayable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want bool
	}{
		{name: aTS, want: true},
		{name: "a.M2TS", want: true},
		{name: "a.mts", want: true},
		{name: "a.mp4", want: false},
		{name: "ts", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, medialib.Playable(tt.name))
		})
	}
}
