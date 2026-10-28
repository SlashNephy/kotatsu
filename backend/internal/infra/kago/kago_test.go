package kago_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
)

const (
	gib = uint64(1) << 30

	poolMain   = "main"
	poolFast   = "fast"
	poolBig    = "big"
	poolSpare  = "spare"
	poolBroken = "broken"
	mountBig   = "/mnt/big"
	mountFast  = "/mnt/fast"
	mountSpare = "/mnt/spare"
	fileName   = "番組.m2ts"
)

var errStatfs = errors.New("statfs failed")

// fakeFreeSpace は、マウント先ごとに決めた空き容量を返す。値がなければ errStatfs を返す。
func fakeFreeSpace(free map[string]uint64) kago.FreeSpaceFunc {
	return func(path string) (uint64, error) {
		v, ok := free[path]
		if !ok {
			return 0, errStatfs
		}

		return v, nil
	}
}

func TestSelect(t *testing.T) {
	t.Parallel()

	mounts := map[string]string{poolFast: mountFast, poolBig: mountBig, poolSpare: mountSpare, poolBroken: "/mnt/broken"}
	pools := []*kago.Pool{
		{ID: poolBig, Priority: 10, MinFreeBytes: 50 * gib},
		{ID: poolFast, Priority: 20, MinFreeBytes: 10 * gib},
		{ID: poolSpare, Priority: 10, MinFreeBytes: 0},
		{ID: poolBroken, Priority: 30, MinFreeBytes: 0},
		// このロールにはマウントされていない。
		{ID: "remote", Priority: 100, MinFreeBytes: 0},
	}

	tests := []struct {
		name      string
		free      map[string]uint64
		estimated uint64
		wantID    string
		wantErr   error
	}{
		{
			name:      "the highest priority pool that fits",
			free:      map[string]uint64{mountFast: 100 * gib, mountBig: 1000 * gib, mountSpare: 1000 * gib},
			estimated: 8 * gib,
			wantID:    poolFast,
		},
		{
			name:      "a pool whose free space falls below its threshold is skipped",
			free:      map[string]uint64{mountFast: 15 * gib, mountBig: 1000 * gib, mountSpare: 1000 * gib},
			estimated: 8 * gib,
			// fast は書いた後に 7 GiB しか残らない。big と spare は優先順位が同じなので、プールの順で先の big を選ぶ。
			wantID: poolBig,
		},
		{
			name:      "exactly at the threshold fits",
			free:      map[string]uint64{mountFast: 18 * gib},
			estimated: 8 * gib,
			wantID:    poolFast,
		},
		{
			name:      "a pool smaller than the estimate is skipped",
			free:      map[string]uint64{mountFast: 5 * gib, mountBig: 10 * gib, mountSpare: 9 * gib},
			estimated: 8 * gib,
			wantID:    poolSpare,
		},
		{
			name:      "every pool falls below its threshold",
			free:      map[string]uint64{mountFast: 15 * gib, mountBig: 55 * gib, mountSpare: 5 * gib},
			estimated: 8 * gib,
			// どのプールも書いた後に閾値を下回る。録画を失敗させず、推定サイズが入るうちで優先順位が最も高い fast を選ぶ (#511)。
			// 閾値まで空けるのは、容量の管理が録画の間に古い録画を消して行う。
			wantID: poolFast,
		},
		{
			name:      "below the threshold, a pool smaller than the estimate is skipped",
			free:      map[string]uint64{mountFast: 5 * gib, mountBig: 55 * gib, mountSpare: 5 * gib},
			estimated: 8 * gib,
			wantID:    poolBig,
		},
		{
			name:      "no pool fits",
			free:      map[string]uint64{mountFast: 1 * gib, mountBig: 1 * gib, mountSpare: 1 * gib},
			estimated: 8 * gib,
			wantErr:   kago.ErrNoPoolFits,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := kago.New(mounts, fakeFreeSpace(tt.free))
			got, err := store.Select(pools, tt.estimated)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				// 空き容量を調べられなかったプールの理由も含める。
				require.ErrorIs(t, err, errStatfs)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}

func TestPath(t *testing.T) {
	t.Parallel()

	store := kago.New(map[string]string{poolMain: "/mnt/main"}, nil)

	tests := []struct {
		name    string
		poolID  string
		rel     string
		want    string
		wantErr error
	}{
		{name: "file", poolID: poolMain, rel: "anime/番組.m2ts", want: filepath.FromSlash("/mnt/main/anime/番組.m2ts")},
		{name: "parent directory", poolID: poolMain, rel: "../etc/passwd", wantErr: kago.ErrInvalidPath},
		{name: "parent directory in the middle", poolID: poolMain, rel: "a/../../b", wantErr: kago.ErrInvalidPath},
		{name: "absolute path", poolID: poolMain, rel: "/etc/passwd", wantErr: kago.ErrInvalidPath},
		{name: "empty", poolID: poolMain, rel: "", wantErr: kago.ErrInvalidPath},
		{name: "unknown pool", poolID: "sub", rel: "a.m2ts", wantErr: kago.ErrPoolNotMounted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := store.Path(tt.poolID, tt.rel)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// segmentFile は、TestOpen でプールに置くファイルの名前。
const segmentFile = "a.m2ts"

// TestOpen は、プールの中のファイルだけを開き、シンボリックリンクでもプールの外を開かないことを確かめる。
func TestOpen(t *testing.T) {
	t.Parallel()

	dir, outside := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, segmentFile), []byte("ts"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(dir, "link.m2ts")))
	store := kago.New(map[string]string{poolMain: dir}, nil)

	tests := []struct {
		name    string
		poolID  string
		rel     string
		want    string
		wantErr bool
	}{
		{name: "file", poolID: poolMain, rel: segmentFile, want: "ts", wantErr: false},
		{name: "symbolic link to the outside", poolID: poolMain, rel: "link.m2ts", want: "", wantErr: true},
		{name: "outside the pool", poolID: poolMain, rel: "../secret", want: "", wantErr: true},
		{name: "unknown pool", poolID: "sub", rel: segmentFile, want: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, err := store.Open(tt.poolID, tt.rel)
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			defer file.Close()
			got, err := io.ReadAll(file)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

// TestCreateSegmentDir は、生成物のディレクトリを、データディレクトリの中に、セグメントのファイルの ID で作ることを確かめる。
func TestCreateSegmentDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dataDir bool
		wantErr error
	}{
		{name: "created by the file ID", dataDir: true, wantErr: nil},
		{name: "no segment directory", dataDir: false, wantErr: kago.ErrPoolNotMounted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dataRoot := t.TempDir()
			store := kago.New(map[string]string{poolMain: t.TempDir()}, nil)
			if tt.dataDir {
				store = store.WithDataDir(dataRoot)
			}
			fileID := uuid.New()
			for range 2 {
				// 2 回目は、作ってあるディレクトリをそのまま使う。
				dir, err := store.CreateSegmentDir(fileID)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)

					return
				}
				require.NoError(t, err)
				assert.Equal(t, &kago.Location{PoolID: kago.DataPoolID, Path: fileID.String()}, dir)
				info, err := os.Stat(filepath.Join(dataRoot, dir.Path))
				require.NoError(t, err)
				assert.True(t, info.IsDir())
			}
		})
	}
}

func TestCreateSegmentDirRejectsFile(t *testing.T) {
	t.Parallel()

	dataRoot := t.TempDir()
	fileID := uuid.New()
	require.NoError(t, os.WriteFile(filepath.Join(dataRoot, fileID.String()), []byte{}, 0o600))
	store := kago.New(map[string]string{poolMain: t.TempDir()}, nil).WithDataDir(dataRoot)

	_, err := store.CreateSegmentDir(fileID)
	require.ErrorIs(t, err, kago.ErrInvalidPath)
}

// TestRemoveWithDataDir は、録画ファイルを消すと、そのファイルの ID の生成物のディレクトリも消し、
// ほかのファイルの生成物のディレクトリは残すことを確かめる。
func TestRemoveWithDataDir(t *testing.T) {
	t.Parallel()

	mount, dataRoot := t.TempDir(), t.TempDir()
	store := kago.New(map[string]string{poolMain: mount}, nil).WithDataDir(dataRoot)
	f, rel, err := store.Create(poolMain, "GR", fileName)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	removed, kept := uuid.New(), uuid.New()
	for _, fileID := range []uuid.UUID{removed, kept} {
		dir, err := store.CreateSegmentDir(fileID)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dataRoot, dir.Path, kago.IndexFileName), []byte("i"), 0o600))
	}

	require.NoError(t, store.Remove(poolMain, rel, removed))
	entries, err := os.ReadDir(mount)
	require.NoError(t, err)
	assert.Empty(t, entries)
	entries, err = os.ReadDir(dataRoot)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, kept.String(), entries[0].Name())
}

func TestFreeSpace(t *testing.T) {
	t.Parallel()

	free, err := kago.FreeSpace(t.TempDir())
	require.NoError(t, err)
	assert.Positive(t, free)

	_, err = kago.FreeSpace(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

func TestTotalSpace(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	total, err := kago.TotalSpace(dir)
	require.NoError(t, err)
	free, err := kago.FreeSpace(dir)
	require.NoError(t, err)
	// 容量は、root のために取っておく分を含むので、使える空き容量より小さくならない。
	assert.GreaterOrEqual(t, total, free)

	_, err = kago.TotalSpace(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}

// TestSelectPrefersConfiguredOrder は、優先順位が同じプールを、設定に書いた順で選ぶことを確かめる。
func TestSelectPrefersConfiguredOrder(t *testing.T) {
	t.Parallel()

	const (
		poolMain  = "main"
		poolLocal = "local"
	)

	mounts := map[string]string{poolMain: mountBig, poolLocal: mountSpare}
	free := fakeFreeSpace(map[string]uint64{mountBig: 1000 * gib, mountSpare: 1000 * gib})
	// どちらも優先順位の設定がない (0 の) プール。
	pools := []*kago.Pool{{ID: poolLocal, Priority: 0, MinFreeBytes: 0}, {ID: poolMain, Priority: 0, MinFreeBytes: 0}}

	tests := []struct {
		name      string
		order     []string
		wantID    string
		wantOrder []string
	}{
		{name: "設定に書いた順で先のプール", order: []string{poolMain, poolLocal}, wantID: poolMain, wantOrder: []string{poolMain, poolLocal}},
		{name: "順を逆にすると逆のプール", order: []string{poolLocal, poolMain}, wantID: poolLocal, wantOrder: []string{poolLocal, poolMain}},
		{name: "順がなければ ID の昇順", order: nil, wantID: poolLocal, wantOrder: []string{poolLocal, poolMain}},
		{name: "順にないプールは後ろに ID の昇順で並べる", order: []string{poolMain, "unknown"}, wantID: poolMain, wantOrder: []string{poolMain, poolLocal}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := kago.NewOrdered(mounts, tt.order, free)
			got, err := store.Select(pools, 8*gib)
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
			assert.Equal(t, tt.wantOrder, store.PoolIDs())
		})
	}
}

// TestRenameKeepsSegmentDir は、録画ファイルの名前を変えても、ID で置いた生成物のディレクトリは動かさないことを確かめる。
func TestRenameKeepsSegmentDir(t *testing.T) {
	t.Parallel()

	mount, dataRoot := t.TempDir(), t.TempDir()
	store := kago.New(map[string]string{poolMain: mount}, nil).WithDataDir(dataRoot)
	f, rel, err := store.Create(poolMain, "old", fileName)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	dir, err := store.CreateSegmentDir(uuid.New())
	require.NoError(t, err)
	index := filepath.Join(dataRoot, dir.Path, kago.IndexFileName)
	require.NoError(t, os.WriteFile(index, []byte("i"), 0o600))

	got, err := store.Rename(poolMain, rel, "new", fileName, 1)
	require.NoError(t, err)
	assert.Equal(t, "new/番組.m2ts", got)
	_, err = os.Stat(index)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(mount, "old"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestSelectWithoutMeasurablePools は、空き容量を調べられたプールが 1 つもなければ、容量が足りない (ErrNoPoolFits) とせず、
// ErrPoolUnavailable を返すことを確かめる (#526)。容量が足りないと記録すると、調べる方向を誤る。
func TestSelectWithoutMeasurablePools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mounts map[string]string
		// wantStatfs は、調べられなかった理由 (statfs の失敗) を返すかどうか。
		wantStatfs bool
	}{
		{name: "statfs fails", mounts: map[string]string{poolBroken: "/mnt/broken"}, wantStatfs: true},
		{name: "not mounted", mounts: map[string]string{}, wantStatfs: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := kago.New(tt.mounts, fakeFreeSpace(map[string]uint64{}))
			_, err := store.Select([]*kago.Pool{{ID: poolBroken, Priority: 0, MinFreeBytes: 0}}, 8*gib)
			require.ErrorIs(t, err, kago.ErrPoolUnavailable)
			require.NotErrorIs(t, err, kago.ErrNoPoolFits)
			if tt.wantStatfs {
				require.ErrorIs(t, err, errStatfs)
			}
		})
	}
}

// TestWithTimeshiftDir は、タイムシフトのディレクトリが、プールの一覧にも走査にも出ず、Path では引けることを確かめる。
func TestWithTimeshiftDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		timeshiftDir bool
		// noPools は、プールを設定していないかどうか。tenban は、プールがなくてもタイムシフトのディレクトリを持てる。
		noPools     bool
		wantHas     bool
		wantPathErr error
	}{
		{name: "mounted", timeshiftDir: true, noPools: false, wantHas: true, wantPathErr: nil},
		{name: "mounted without pools", timeshiftDir: true, noPools: true, wantHas: true, wantPathErr: nil},
		{name: "not mounted", timeshiftDir: false, noPools: false, wantHas: false, wantPathErr: kago.ErrPoolNotMounted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			var pools map[string]string
			if !tt.noPools {
				pools = map[string]string{poolMain: t.TempDir()}
			}
			store := kago.New(pools, nil)
			if !tt.noPools {
				store = store.WithDataDir(t.TempDir())
			}
			if tt.timeshiftDir {
				store = store.WithTimeshiftDir(root)
			}

			assert.Equal(t, tt.wantHas, store.HasTimeshiftDir())
			assert.NotContains(t, store.PoolIDs(), kago.TimeshiftPoolID)
			assert.NotContains(t, store.ScanPoolIDs(), kago.TimeshiftPoolID)
			path, err := store.Path(kago.TimeshiftPoolID, "1/a.m2ts")
			if tt.wantPathErr != nil {
				require.ErrorIs(t, err, tt.wantPathErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(root, "1", "a.m2ts"), path)
		})
	}
}

// TestWithTimeshiftDirKeepsOtherDirs は、タイムシフトのディレクトリを足しても、データディレクトリが残ることを確かめる。
func TestWithTimeshiftDirKeepsOtherDirs(t *testing.T) {
	t.Parallel()

	dataRoot := t.TempDir()
	store := kago.New(map[string]string{poolMain: t.TempDir()}, nil).WithTimeshiftDir(t.TempDir()).WithDataDir(dataRoot).WithTotalSpace(kago.TotalSpace)

	assert.True(t, store.HasTimeshiftDir())
	assert.Contains(t, store.ScanPoolIDs(), kago.DataPoolID)
}
