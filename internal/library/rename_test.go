package library_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/library"
)

// oldName は、付け直す前の録画ファイルの名前。
const oldName = "古い名前.m2ts"

// TestRename は、今のテンプレートで付け直すと名前が変わる録画を計画し、ファイルと記録のパスを付け直し、
// 監査ログに残すことを確かめる。分かれた録画の 2 つ目以降のセグメントには連番を付ける。
func TestRename(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	// 既定のテンプレートは {{.StartAt.Format "2006-01-02 1504"}} {{.ServiceName}} {{.Name}}。録画は 11:00 に始めた。
	const want1 = "2026-10-10 1100 こたつテスト1 録画.m2ts"
	split := f.segmented(t, time.Hour, false,
		segment{pool: poolID, path: oldName, exists: true}, segment{pool: poolID, path: "古い名前 (2).ts", exists: true})
	before, err := os.ReadDir(f.dataRoot)
	require.NoError(t, err)

	plans, err := f.library.PlanRenames(t.Context(), f.pool, nil, time.UTC)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, split, plans[0].RecordingID)
	got := make([][2]string, 0, len(plans[0].Files))
	for _, file := range plans[0].Files {
		got = append(got, [2]string{file.From, file.To})
	}
	// 取り込んだ録画の .ts のような拡張子は残す。
	assert.Equal(t, [][2]string{{oldName, want1}, {"古い名前 (2).ts", "2026-10-10 1100 こたつテスト1 録画 (2).ts"}}, got)

	require.NoError(t, f.library.Rename(t.Context(), f.pool, audit.User(f.userID), plans[0]))
	// 生成物は、セグメントのファイルの ID で置くので、名前を変えても動かない。
	after, err := os.ReadDir(f.dataRoot)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	paths := []string{}
	rows, err := f.pool.Query(t.Context(), "SELECT path FROM recording_files WHERE recording_id = $1 ORDER BY started_at, kind", split)
	require.NoError(t, err)
	for rows.Next() {
		var p string
		require.NoError(t, rows.Scan(&p))
		paths = append(paths, p)
	}
	assert.Equal(t, []string{want1, "2026-10-10 1100 こたつテスト1 録画 (2).ts"}, paths)
	assert.Equal(t, []string{"recording.renamed"}, f.actions(t, split))

	// 付け直した後は、名前が変わらないので、計画に出てこない。
	plans, err = f.library.PlanRenames(t.Context(), f.pool, nil, time.UTC)
	require.NoError(t, err)
	assert.Empty(t, plans)
}

// TestRenameSkipsJoining は、セグメントをつないでいる途中の録画は付け直さないことを確かめる。
func TestRenameSkipsJoining(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.segmented(t, time.Hour, false, segment{pool: poolID, path: oldName, exists: true})
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, "古い名前.m2ts.joining"), []byte("x"), 0o600))
	plans, err := f.library.PlanRenames(t.Context(), f.pool, nil, time.UTC)
	require.NoError(t, err)
	require.Len(t, plans, 1)

	require.ErrorIs(t, f.library.Rename(t.Context(), f.pool, audit.User(f.userID), plans[0]), library.ErrBusy)
	_, err = os.Stat(filepath.Join(f.dir, oldName))
	assert.NoError(t, err)
}

// TestRenameAdvancesSequenceOnCollision は、移す先がふさがっているとき、分かれた録画の連番を (2) (2) のように重ねず、
// 次の番号に進めることを確かめる。
func TestRenameAdvancesSequenceOnCollision(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	const taken = "2026-10-10 1100 こたつテスト1 録画.m2ts"
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, taken), []byte("other"), 0o600))
	f.segmented(t, time.Hour, false,
		segment{pool: poolID, path: "a1.m2ts", exists: true}, segment{pool: poolID, path: "a2.m2ts", exists: true})
	plans, err := f.library.PlanRenames(t.Context(), f.pool, nil, time.UTC)
	require.NoError(t, err)
	require.Len(t, plans, 1)

	require.NoError(t, f.library.Rename(t.Context(), f.pool, audit.User(f.userID), plans[0]))
	for _, name := range []string{"2026-10-10 1100 こたつテスト1 録画 (2).m2ts", "2026-10-10 1100 こたつテスト1 録画 (3).m2ts"} {
		_, err := os.Stat(filepath.Join(f.dir, name))
		require.NoError(t, err, name)
	}
}
