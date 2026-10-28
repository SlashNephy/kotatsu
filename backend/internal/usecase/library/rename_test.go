package library_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/library"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// oldName は、付け直す前の録画ファイルの名前。split1 と split2 は、分かれた録画の付け直す前のセグメントの名前。
const (
	oldName = "古い名前.m2ts"
	split1  = "a1.m2ts"
	split2  = "a2.m2ts"
)

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
		segment{pool: poolID, path: split1, exists: true}, segment{pool: poolID, path: split2, exists: true})
	plans, err := f.library.PlanRenames(t.Context(), f.pool, nil, time.UTC)
	require.NoError(t, err)
	require.Len(t, plans, 1)

	require.NoError(t, f.library.Rename(t.Context(), f.pool, audit.User(f.userID), plans[0]))
	for _, name := range []string{"2026-10-10 1100 こたつテスト1 録画 (2).m2ts", "2026-10-10 1100 こたつテスト1 録画 (3).m2ts"} {
		_, err := os.Stat(filepath.Join(f.dir, name))
		require.NoError(t, err, name)
	}
}

// TestRenameToTheSameFileWithAnotherName は、大文字と小文字や正規化だけが違う名前に付け直すとき、移す先が自分自身として見つかっても、
// ふさがっているとみなして連番を付けないことを確かめる (#1095)。正規化を区別するファイルシステムでは、HFS+ の代わりに、ハードリンクを置く。
func TestRenameToTheSameFileWithAnotherName(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	// 濁点を含む名前は、NFD にすると変わる。
	const want = "2026-10-10 1100 こたつテスト1 ガイド.m2ts"
	// 記録は NFD の名前で、同じファイルが NFC の名前でも見つかる。
	nfd := norm.NFD.String(want)
	require.NotEqual(t, want, nfd)
	recordingID := f.segmented(t, time.Hour, false, segment{pool: poolID, path: nfd, exists: true})
	_, err := f.pool.Exec(t.Context(), "UPDATE recordings SET name = 'ガイド' WHERE id = $1", recordingID)
	require.NoError(t, err)
	// 正規化を区別しないファイルシステムでは、NFC の名前で、すでに同じファイルが見つかる。
	if _, err := os.Stat(filepath.Join(f.dir, want)); err != nil {
		require.NoError(t, os.Link(filepath.Join(f.dir, nfd), filepath.Join(f.dir, want)))
	}

	plans, err := f.library.PlanRenames(t.Context(), f.pool, nil, time.UTC)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	require.Len(t, plans[0].Files, 1)
	assert.Equal(t, want, plans[0].Files[0].To, "no sequence number is added")

	require.NoError(t, f.library.Rename(t.Context(), f.pool, audit.User(f.userID), plans[0]))
	var path string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT path FROM recording_files WHERE recording_id = $1", recordingID).Scan(&path))
	assert.Equal(t, want, path)
}

// TestRenameSettles は、付け直しを最後まで実行した後は、計画に何も出てこないことを確かめる。
// ほかの録画が名前をふさいでいて連番が付いた録画は、付け直しても名前が変わらないので、計画に出さない。
// 付け直した後は、DB の記録のパスにファイルがあり、プールのファイルの数は変わらない。
func TestRenameSettles(t *testing.T) {
	t.Parallel()

	// 既定のテンプレートで、録画は 11:00 に始めた。
	const (
		base  = "2026-10-10 1100 こたつテスト1 録画.m2ts"
		base2 = "2026-10-10 1100 こたつテスト1 録画 (2).m2ts"
		base3 = "2026-10-10 1100 こたつテスト1 録画 (3).m2ts"
		// monthly は、年月ごとのディレクトリに分けるテンプレート。
		monthly  = `{{.StartAt.Format "2006-01"}}/{{.Name}}`
		october  = "2026-10/録画.m2ts"
		october2 = "2026-10/録画 (2).m2ts"
	)
	tests := []struct {
		name     string
		template string
		// others は、DB に記録のない、ほかの録画のファイル。
		others []string
		// recordings は、録画ごとのセグメントのパス。
		recordings [][]string
		// wantPlanned は、最初の計画に出てくるファイルと、その移る先。
		wantPlanned map[string]string
		// want は、付け直した後の、録画ごとのセグメントのパス。
		want [][]string
		// wantGone は、付け直した後に消えているディレクトリ。
		wantGone []string
	}{
		{
			name:        "テンプレートどおりの名前の録画は、計画に出さない",
			recordings:  [][]string{{base}},
			wantPlanned: map[string]string{},
			want:        [][]string{{base}},
		},
		{
			name:        "ほかの録画がふさいでいて連番が付いた録画は、計画に出さない",
			others:      []string{base},
			recordings:  [][]string{{base2}},
			wantPlanned: map[string]string{},
			want:        [][]string{{base2}},
		},
		{
			name:        "手前の連番がほかの録画で埋まっていれば、連番の付いた録画は計画に出さない",
			others:      []string{base, base2},
			recordings:  [][]string{{base3}},
			wantPlanned: map[string]string{},
			want:        [][]string{{base3}},
		},
		{
			name:        "手前の名前が空いていれば、連番の付いた録画を付け直す",
			recordings:  [][]string{{base2}},
			wantPlanned: map[string]string{base2: base},
			want:        [][]string{{base}},
		},
		{
			name:        "飛ばした連番は詰める",
			others:      []string{base},
			recordings:  [][]string{{base3}},
			wantPlanned: map[string]string{base3: base2},
			want:        [][]string{{base2}},
		},
		{
			name:       "同じ名前になる録画が 3 つあれば、連番を付けて付け直し、その後は計画に出さない",
			recordings: [][]string{{"a.m2ts"}, {"b.m2ts"}, {"c.m2ts"}},
			// 計画は録画ごとに求めるので、ほかの録画が移る先は計画に織り込まない。付け直すときに連番を進める。
			wantPlanned: map[string]string{"a.m2ts": base, "b.m2ts": base, "c.m2ts": base},
			want:        [][]string{{base}, {base2}, {base3}},
		},
		{
			name:        "ふさがれた名前に付け直した分かれた録画は、その後は計画に出さない",
			others:      []string{base},
			recordings:  [][]string{{split1, split2}},
			wantPlanned: map[string]string{split1: base2, split2: base3},
			want:        [][]string{{base2, base3}},
		},
		{
			name:        "今の整え方で使えない文字を含む名前は、付け直す",
			recordings:  [][]string{{"古い:名前.m2ts"}},
			wantPlanned: map[string]string{"古い:名前.m2ts": base},
			want:        [][]string{{base}},
		},
		{
			name:        "テンプレートの / でディレクトリを作り、空になった元のディレクトリを消す",
			template:    monthly,
			recordings:  [][]string{{"old/sub/a.m2ts"}},
			wantPlanned: map[string]string{"old/sub/a.m2ts": october},
			want:        [][]string{{october}},
			wantGone:    []string{"old"},
		},
		{
			name:        "連番が付いていても、ディレクトリが違えば付け直す",
			template:    monthly,
			others:      []string{october},
			recordings:  [][]string{{"2026-09/録画 (2).m2ts"}},
			wantPlanned: map[string]string{"2026-09/録画 (2).m2ts": october2},
			want:        [][]string{{october2}},
			wantGone:    []string{"2026-09"},
		},
		{
			name:        "年月ごとのディレクトリでふさがれて連番が付いた録画は、計画に出さない",
			template:    monthly,
			others:      []string{october},
			recordings:  [][]string{{october2}},
			wantPlanned: map[string]string{},
			want:        [][]string{{october2}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			if tt.template != "" {
				f.setTemplate(t, tt.template)
			}
			for _, other := range tt.others {
				require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(f.dir, other)), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(f.dir, other), []byte("other"), 0o600))
			}
			ids := make([]uuid.UUID, 0, len(tt.recordings))
			for _, paths := range tt.recordings {
				ids = append(ids, f.segmentedAt(t, paths...))
			}
			before := f.poolFiles(t)

			plans, err := f.library.PlanRenames(t.Context(), f.pool, nil, time.UTC)
			require.NoError(t, err)
			planned := map[string]string{}
			for _, plan := range plans {
				for _, file := range plan.Files {
					planned[file.From] = file.To
				}
			}
			assert.Equal(t, tt.wantPlanned, planned)

			for _, plan := range plans {
				require.NoError(t, f.library.Rename(t.Context(), f.pool, audit.User(f.userID), plan))
			}
			got := make([][]string, 0, len(ids))
			for _, recordingID := range ids {
				got = append(got, f.segmentPaths(t, recordingID))
			}
			assert.ElementsMatch(t, tt.want, got)
			assert.Len(t, f.poolFiles(t), len(before), "no file is lost")
			for _, dir := range tt.wantGone {
				_, err := os.Stat(filepath.Join(f.dir, dir))
				require.ErrorIs(t, err, fs.ErrNotExist, dir)
			}

			// 付け直した後は、もう一度探しても何も出てこない。
			plans, err = f.library.PlanRenames(t.Context(), f.pool, nil, time.UTC)
			require.NoError(t, err)
			assert.Empty(t, plans)
		})
	}
}

// setTemplate は、全体の録画設定のファイル名のテンプレートを template にする。
func (f *fixture) setTemplate(t *testing.T, template string) {
	t.Helper()

	_, err := settings.NewStore(f.clock, audit.NewRecorder(f.clock)).Update(t.Context(), f.pool, audit.User(f.userID),
		&settings.Changes{Recording: &domain.RecordingOverrides{FileNameTemplate: &template}})
	require.NoError(t, err)
}

// segmentedAt は、1 時間前に始めた、セグメントのファイルが paths にある録画を作る。
func (f *fixture) segmentedAt(t *testing.T, paths ...string) uuid.UUID {
	t.Helper()

	segments := make([]segment, 0, len(paths))
	for _, p := range paths {
		segments = append(segments, segment{pool: poolID, path: p, exists: true})
	}

	return f.segmented(t, time.Hour, false, segments...)
}

// segmentPaths は、録画 recordingID のセグメントのパスを、始まりの順に返す。記録のパスにファイルがあることも確かめる。
func (f *fixture) segmentPaths(t *testing.T, recordingID uuid.UUID) []string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT path FROM recording_files WHERE recording_id = $1 AND kind = 'segment' ORDER BY started_at",
		recordingID)
	require.NoError(t, err)
	paths, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	for _, p := range paths {
		_, err := os.Stat(filepath.Join(f.dir, p))
		require.NoError(t, err, "the recorded path %s exists", p)
	}

	return paths
}

// poolFiles は、プールにあるファイルの相対パスを返す。
func (f *fixture) poolFiles(t *testing.T) []string {
	t.Helper()

	files := []string{}
	require.NoError(t, filepath.WalkDir(f.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(f.dir, p)
		files = append(files, rel)

		return err
	}))

	return files
}

// TestRenameUndoesToUnsanitizedName は、付け直しの途中で失敗したとき、先に移したファイルを、
// 今の整え方で使えない文字を含む元の名前にも戻し、記録と合わせることを確かめる (#917)。
func TestRenameUndoesToUnsanitizedName(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	const unsanitized = "古い:名前.m2ts"
	// 2 つ目のセグメントのファイルがないので、1 つ目を移した後に失敗する。
	recordingID := f.segmented(t, time.Hour, false,
		segment{pool: poolID, path: unsanitized, exists: true}, segment{pool: poolID, path: "missing.m2ts", exists: false})
	plans, err := f.library.PlanRenames(t.Context(), f.pool, nil, time.UTC)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	require.Len(t, plans[0].Files, 2)

	require.Error(t, f.library.Rename(t.Context(), f.pool, audit.User(f.userID), plans[0]))

	assert.Equal(t, []string{unsanitized, "missing.m2ts"}, f.segmentPathsUnchecked(t, recordingID))
	_, err = os.Stat(filepath.Join(f.dir, unsanitized))
	require.NoError(t, err, "the moved file is restored to the unsanitized name")
	assert.Equal(t, []string{unsanitized}, f.poolFiles(t))
	assert.Empty(t, f.actions(t, recordingID))
}

// segmentPathsUnchecked は、segmentPaths と同じく記録のパスを返すが、ファイルがあるかは確かめない。
func (f *fixture) segmentPathsUnchecked(t *testing.T, recordingID uuid.UUID) []string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT path FROM recording_files WHERE recording_id = $1 AND kind = 'segment' ORDER BY started_at",
		recordingID)
	require.NoError(t, err)
	paths, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)

	return paths
}
