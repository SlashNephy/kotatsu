package kago_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
)

// programFile は、テストの録画ファイルのプール内のパス。
const programFile = "番組/録画.m2ts"

// indexBody は、テストの索引のファイルの中身。
const indexBody = "index"

// segmentDirID は、テストの生成物のディレクトリの持ち主の、セグメントのファイルの ID。
const segmentDirID = "0199f0a0-0000-7000-8000-000000000001"

// TestSegmentDirOwner は、データディレクトリの直下の、ID の名前のディレクトリから、持ち主の ID を求められることを確かめる。
func TestSegmentDirOwner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		poolID  string
		dir     string
		want    uuid.UUID
		wantErr bool
	}{
		{name: "ID の名前のディレクトリ", poolID: kago.DataPoolID, dir: segmentDirID, want: uuid.MustParse(segmentDirID)},
		{name: "プールの中は生成物のディレクトリでない", poolID: poolMain, dir: segmentDirID, wantErr: true},
		{name: "ID の下のディレクトリ", poolID: kago.DataPoolID, dir: segmentDirID + "/中", wantErr: true},
		{name: "ID でない名前", poolID: kago.DataPoolID, dir: "main", wantErr: true},
		{name: "正規の形でない ID", poolID: kago.DataPoolID, dir: "{" + segmentDirID + "}", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := kago.SegmentDirOwner(tt.poolID, tt.dir)
			if tt.wantErr {
				require.ErrorIs(t, err, kago.ErrInvalidPath)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestScan は、ファイルと生成物のディレクトリを返し、隠しファイル、lost+found、シンボリックリンク、
// 中にディレクトリのある生成物のディレクトリは返さないことを確かめる。ID の名前でないディレクトリの中は、ファイルとして返す。
func TestScan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		poolID string
		files  map[string]string
		want   map[string]int64
	}{
		{
			name:   "pool",
			poolID: poolMain,
			files: map[string]string{
				programFile: "ts", segmentDirID + "/" + kago.IndexFileName: indexBody, ".DS_Store": "x", "lost+found/#1": "x",
			},
			want: map[string]int64{programFile: 2, segmentDirID + "/" + kago.IndexFileName: 5},
		},
		{
			name:   "segment directory",
			poolID: kago.DataPoolID,
			files: map[string]string{
				segmentDirID + "/" + kago.IndexFileName: indexBody, segmentDirID + "/preview.jpg": "jpeg",
				"0199f0a0-0000-7000-8000-000000000002/中/index": indexBody, "main/番組.m2ts.kotatsu/" + kago.IndexFileName: indexBody,
			},
			want: map[string]int64{segmentDirID: 9, "main/番組.m2ts.kotatsu/" + kago.IndexFileName: 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			for rel, data := range tt.files {
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(data), 0o600))
			}
			require.NoError(t, os.Symlink("/etc/hostname", filepath.Join(dir, "リンク.m2ts")))
			store := kago.New(map[string]string{poolMain: dir}, nil).WithDataDir(dir)

			entries, err := store.Scan(tt.poolID)
			require.NoError(t, err)
			got := map[string]int64{}
			for _, entry := range entries {
				got[entry.Path] = entry.Size
				assert.Equal(t, kago.IsSegmentDir(tt.poolID, entry.Path), entry.SegmentDir, entry.Path)
			}
			assert.Equal(t, tt.want, got)
		})
	}
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
