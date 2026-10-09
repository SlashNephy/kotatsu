package kago_test

import (
	"errors"
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
			// fast は書いた後に 7 GiB しか残らない。big と spare は優先順位が同じなので ID の昇順で選ぶ。
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

func TestCreateSidecarDir(t *testing.T) {
	t.Parallel()

	mount := t.TempDir()
	store := kago.New(map[string]string{poolMain: mount}, nil)
	f, rel, err := store.Create(poolMain, "", fileName)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	for range 2 {
		// 2 回目は、作ってあるディレクトリをそのまま使う。
		dirRel, err := store.CreateSidecarDir(poolMain, rel)
		require.NoError(t, err)
		assert.Equal(t, "番組.m2ts.kotatsu", dirRel)
		info, err := os.Stat(filepath.Join(mount, dirRel))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	}
}

func TestFreeSpace(t *testing.T) {
	t.Parallel()

	free, err := kago.FreeSpace(t.TempDir())
	require.NoError(t, err)
	assert.Positive(t, free)

	_, err = kago.FreeSpace(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
}
