package kago_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/kago"
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
		{name: "file", poolID: poolMain, rel: "anime/番組.m2ts", want: "/mnt/main/anime/番組.m2ts"},
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

func TestCreateSidecarDir(t *testing.T) {
	t.Parallel()

	mount := t.TempDir()
	store := kago.New(map[string]string{poolMain: mount}, nil)
	f, rel, err := store.Create(poolMain, "", fileName)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	for range 2 {
		// 2 回目は、作ってあるディレクトリをそのまま使う。
		dir, err := store.CreateSidecarDir(poolMain, rel)
		require.NoError(t, err)
		assert.Equal(t, &kago.Location{PoolID: poolMain, Path: "番組.m2ts.kotatsu"}, dir)
		info, err := os.Stat(filepath.Join(mount, dir.Path))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	}
}

// TestSidecarDir は、付属ファイルをまとめて置くディレクトリがあれば、新しい録画の付属ファイルをそこに置き、
// 録画ファイルの横にすでにある付属ファイルは、そのまま使い続け、録画を消せばどちらも消すことを確かめる。
func TestSidecarDir(t *testing.T) {
	t.Parallel()

	mount, sidecars := t.TempDir(), t.TempDir()
	store := kago.New(map[string]string{poolMain: mount}, nil).WithSidecarDir(sidecars)
	create := func(dir, name string) string {
		f, rel, err := store.Create(poolMain, dir, name)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		return rel
	}

	fresh := create("GR", fileName)
	dir, err := store.CreateSidecarDir(poolMain, fresh)
	require.NoError(t, err)
	assert.Equal(t, &kago.Location{PoolID: kago.SidecarPoolID, Path: "main/GR/番組.m2ts.kotatsu"}, dir)
	_, err = os.Stat(filepath.Join(sidecars, dir.Path))
	require.NoError(t, err)
	assert.Equal(t, []*kago.Location{
		{PoolID: kago.SidecarPoolID, Path: "main/GR/番組.m2ts.kotatsu/index"}, {PoolID: poolMain, Path: "GR/番組.m2ts.kotatsu/index"},
	}, store.IndexLocations(poolMain, fresh))

	// 以前から録画ファイルの横にある付属ファイルは、移さずに使い続ける。
	legacy := create("", "古い録画.m2ts")
	require.NoError(t, os.Mkdir(filepath.Join(mount, legacy+".kotatsu"), 0o755))
	dir, err = store.CreateSidecarDir(poolMain, legacy)
	require.NoError(t, err)
	assert.Equal(t, &kago.Location{PoolID: poolMain, Path: "古い録画.m2ts.kotatsu"}, dir)

	require.NoError(t, store.Remove(poolMain, fresh))
	require.NoError(t, store.Remove(poolMain, legacy))
	for _, root := range []string{mount, sidecars} {
		entries, err := os.ReadDir(root)
		require.NoError(t, err)
		assert.Empty(t, entries, root)
	}
}

func TestCreateSidecarDirRejectsFile(t *testing.T) {
	t.Parallel()

	mount := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(mount, fileName+".kotatsu"), []byte{}, 0o600))
	store := kago.New(map[string]string{poolMain: mount}, nil)

	_, err := store.CreateSidecarDir(poolMain, fileName)
	require.ErrorIs(t, err, kago.ErrInvalidPath)
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

// TestRenameWithSidecarDir は、付属ファイルをまとめて置くディレクトリにある付属ファイルも、録画ファイルと一緒に移すことを確かめる。
func TestRenameWithSidecarDir(t *testing.T) {
	t.Parallel()

	mount, sidecars := t.TempDir(), t.TempDir()
	store := kago.New(map[string]string{poolMain: mount}, nil).WithSidecarDir(sidecars)
	f, rel, err := store.Create(poolMain, "old", fileName)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	dir, err := store.CreateSidecarDir(poolMain, rel)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(sidecars, dir.Path, "index"), []byte("i"), 0o600))

	got, err := store.Rename(poolMain, rel, "new", fileName, 1)
	require.NoError(t, err)
	assert.Equal(t, "new/番組.m2ts", got)
	_, err = os.Stat(filepath.Join(sidecars, "main/new/番組.m2ts.kotatsu/index"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(sidecars, "main/old"))
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Equal(t, []*kago.SidecarMove{
		{PoolID: kago.SidecarPoolID, From: "main/old/番組.m2ts.kotatsu", To: "main/new/番組.m2ts.kotatsu"},
		{PoolID: poolMain, From: "old/番組.m2ts.kotatsu", To: "new/番組.m2ts.kotatsu"},
	}, store.SidecarMoves(poolMain, rel, got))
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
