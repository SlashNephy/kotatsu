package kago_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/kago"
)

// programFile は、テストの録画ファイルのプール内のパス。
const programFile = "番組/録画.m2ts"

// TestSidecarOwner は、付属ファイルのディレクトリから、持ち主の録画ファイルの場所を求められることを確かめる。
func TestSidecarOwner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		poolID  string
		dir     string
		want    *kago.Location
		wantErr bool
	}{
		{name: "録画ファイルの横", poolID: poolMain, dir: programFile + ".kotatsu", want: &kago.Location{PoolID: poolMain, Path: programFile}},
		{name: "まとめて置くディレクトリ", poolID: kago.SidecarPoolID, dir: "main/番組/録画.m2ts.kotatsu", want: &kago.Location{PoolID: poolMain, Path: programFile}},
		{name: "まとめて置くディレクトリの直下はプールが分からない", poolID: kago.SidecarPoolID, dir: "録画.m2ts.kotatsu", wantErr: true},
		{name: "付属ファイルのディレクトリでない", poolID: poolMain, dir: "録画.m2ts", wantErr: true},
		{name: "名前が接尾辞だけ", poolID: poolMain, dir: "番組/.kotatsu", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := kago.SidecarOwner(tt.poolID, tt.dir)
			if tt.wantErr {
				require.ErrorIs(t, err, kago.ErrInvalidPath)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestScan は、ファイルと付属ファイルのディレクトリを返し、隠しファイル、lost+found、シンボリックリンク、
// 中にディレクトリのある付属ファイルのディレクトリは返さないことを確かめる。
func TestScan(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for rel, data := range map[string]string{
		programFile:                      "ts",
		programFile + ".kotatsu/index":   "index",
		programFile + ".kotatsu/preview": "jpeg",
		"入れ子.m2ts.kotatsu/中/index":       "index",
		".DS_Store":                      "x",
		"lost+found/#1":                  "x",
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(data), 0o600))
	}
	require.NoError(t, os.Symlink("/etc/hostname", filepath.Join(dir, "リンク.m2ts")))
	store := kago.New(map[string]string{poolMain: dir}, nil)

	entries, err := store.Scan(poolMain)
	require.NoError(t, err)
	got := map[string]int64{}
	for _, entry := range entries {
		got[entry.Path] = entry.Size
		assert.Equal(t, kago.IsSidecarDir(entry.Path), entry.Sidecar, entry.Path)
	}
	assert.Equal(t, map[string]int64{programFile: 2, programFile + ".kotatsu": 9}, got)
}

// TestInspectChangedAt は、名前を変えたファイルは、更新日時が古くても、変えた日時が新しくなることを確かめる。
// 付け直している途中の録画ファイルを、DB にないとみなさないために使う。
func TestInspectChangedAt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	old := time.Now().Add(-24 * time.Hour)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "前.m2ts"), []byte("ts"), 0o600))
	require.NoError(t, os.Chtimes(filepath.Join(dir, "前.m2ts"), old, old))
	before := time.Now().Add(-time.Second)
	require.NoError(t, os.Rename(filepath.Join(dir, "前.m2ts"), filepath.Join(dir, "後.m2ts")))
	store := kago.New(map[string]string{poolMain: dir}, nil)

	entry, err := store.Inspect(poolMain, "後.m2ts", false)
	require.NoError(t, err)
	assert.WithinDuration(t, old, entry.ModifiedAt, time.Second)
	assert.True(t, entry.ChangedAt.After(before), "changed at %v", entry.ChangedAt)
}
