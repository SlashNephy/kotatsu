package sharedfile_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
)

// TestOpenLetsOthersRemoveAndRename は、開いている間も、ほかの処理がファイルを消したり名前を変えたりできることを確かめる (#1096)。
// Windows の os.Open で開くと、どちらも共有違反で失敗する。
func TestOpenLetsOthersRemoveAndRename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		op   func(dir, path string) error
	}{
		{name: "remove", op: func(_, path string) error { return os.Remove(path) }},
		{name: "rename", op: func(dir, path string) error { return os.Rename(path, filepath.Join(dir, "renamed.ts")) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "segment.ts")
			require.NoError(t, os.WriteFile(path, []byte("ts"), 0o600))
			file, err := sharedfile.Open(path)
			require.NoError(t, err)
			t.Cleanup(func() { _ = file.Close() })

			require.NoError(t, tt.op(dir, path))
			data, err := io.ReadAll(file)
			require.NoError(t, err)
			assert.Equal(t, "ts", string(data), "the open file is still readable")
		})
	}
}

// TestReadFile は、ReadFile がファイルの中身を返し、なければ os.ErrNotExist を返すことを確かめる。
func TestReadFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "playlist.m3u8")
	require.NoError(t, os.WriteFile(path, []byte("#EXTM3U"), 0o600))

	data, err := sharedfile.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "#EXTM3U", string(data))
	_, err = sharedfile.ReadFile(filepath.Join(dir, "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestRenameReplaces は、Rename が、移す先にあるファイルを置き換えることを確かめる。
func TestRenameReplaces(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	from, to := filepath.Join(dir, "index.tmp"), filepath.Join(dir, "index")
	require.NoError(t, os.WriteFile(from, []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(to, []byte("old"), 0o600))

	require.NoError(t, sharedfile.Rename(from, to))
	data, err := os.ReadFile(to)
	require.NoError(t, err)
	assert.Equal(t, "new", string(data))
	_, err = os.Stat(from)
	require.ErrorIs(t, err, os.ErrNotExist)
}
