package filelib

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
)

// internalLibrary と internalShowTS は、テストのライブラリの名前と、その中のファイルの名前。
const (
	internalLibrary = "lib"
	internalShowTS  = "show.ts"
)

// indexFixture は、testdata/ts/test.ts を show.ts として置いたライブラリと、そのインデックスの保存先を持つ。
type indexFixture struct {
	indexer *Indexer
	clock   *clocktest.Fake
	ref     Ref
	entry   *medialib.Entry
	// index は、show.ts のインデックスのファイルの内容。
	index []byte
}

// readTestTS は、testdata/ts/test.ts を返す。
func readTestTS(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)

	return data
}

func newIndexFixture(t *testing.T) *indexFixture {
	t.Helper()

	libraryDir, dataDir := t.TempDir(), t.TempDir()
	data := readTestTS(t)
	require.NoError(t, os.WriteFile(filepath.Join(libraryDir, internalShowTS), data, 0o600))
	libraries := medialib.NewSet(map[string]string{}, []string{}, map[string]string{internalLibrary: libraryDir})
	// 置いたばかりのファイルは書いている途中とみなされるので、時計を進めておく。
	clk := clocktest.NewFake(time.Now().Add(time.Hour))
	indexer := NewIndexer(dataDir, libraries, clk, slog.New(slog.DiscardHandler), true)
	library, _ := libraries.Get(internalLibrary)
	ref := Ref{Library: internalLibrary, Path: internalShowTS}
	entry, err := library.Stat(t.Context(), ref.Path)
	require.NoError(t, err)
	var index bytes.Buffer
	writer, err := ts.NewIndexWriter(&index)
	require.NoError(t, err)
	_, err = ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer}).Write(data)
	require.NoError(t, err)

	return &indexFixture{indexer: indexer, clock: clk, ref: ref, entry: entry, index: index.Bytes()}
}

// place は、作っている途中と記録したインデックスの保存先に、name の名前でインデックスを置く。
func (f *indexFixture) place(t *testing.T, name string) {
	t.Helper()

	dir := f.indexer.dir(f.ref)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, f.indexer.writeSource(dir, &source{
		Version: sourceVersion, Library: f.ref.Library, Path: f.ref.Path, Size: f.entry.Size, ModifiedAt: f.entry.ModifiedAt.UnixNano(),
		Complete: false, Video: nil, Broadcast: nil, Audios: nil, Quality: nil,
	}))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), f.index, 0o600))
	// 書きかけの索引は、今 (時計の今) 伸びたものにする。
	require.NoError(t, os.Chtimes(filepath.Join(dir, name), f.clock.Now(), f.clock.Now()))
}

// TestEnsureLeavesOtherBuilds は、別の tenban が作っている途中のインデックス (書きかけが伸び続けているもの) は作り直さず、
// staleBuild より長く伸びていなければ、作っていた tenban が落ちたとみなして作り直すことを確認する。
func TestEnsureLeavesOtherBuilds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		age       time.Duration
		wantBuild bool
	}{
		{name: "growing", age: 0, wantBuild: false},
		{name: "stale", age: staleBuild + time.Second, wantBuild: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newIndexFixture(t)
			f.place(t, partialIndexName)
			f.clock.Advance(tt.age)
			// 作り始めても、作る枠が空くまで待たせ、作り始めたかを見分ける。
			for range maxBuilds {
				f.indexer.slots <- struct{}{}
			}
			f.indexer.Ensure(t.Context(), f.ref, f.entry)
			f.indexer.mu.Lock()
			building := f.indexer.building[indexKey(f.ref)] != nil
			f.indexer.mu.Unlock()
			assert.Equal(t, tt.wantBuild, building)
			for range maxBuilds {
				<-f.indexer.slots
			}
			if !tt.wantBuild {
				return
			}
			require.Eventually(t, func() bool {
				index, err := f.indexer.Read(f.ref, f.entry)
				require.NoError(t, err)

				return index.Complete
			}, 10*time.Second, 10*time.Millisecond)
		})
	}
}

// TestReadAfterRename は、書きかけのインデックスの名前を変えてから、作り終えたと記録するまでの間に読んでも、名前を変えたインデックスでシークできるが、
// ドロップなどの集計をまだ記録に書いていないので、作り終えていないものとして返すことを確認する (#1734)。
func TestReadAfterRename(t *testing.T) {
	t.Parallel()

	f := newIndexFixture(t)
	f.place(t, indexName)
	index, err := f.indexer.Read(f.ref, f.entry)
	require.NoError(t, err)
	assert.False(t, index.Complete, "集計を記録するまでは、作り終えていない")
	assert.Nil(t, index.Quality)
	assert.NotEmpty(t, index.Entries, "名前を変えた索引でシークできる")

	// 集計を記録する前に tenban が止まっても、書きかけの索引がないので、開けば作り直して集計を記録する。
	f.indexer.Ensure(t.Context(), f.ref, f.entry)
	require.Eventually(t, func() bool {
		index, err := f.indexer.Read(f.ref, f.entry)
		require.NoError(t, err)

		return index.Complete && index.Quality != nil
	}, 10*time.Second, 10*time.Millisecond)
}

// TestReadOldVersion は、前のバージョンの記録 (放送の情報のないもの) のインデックスは、シークに使えるが、作り終えていないものとして返し、
// 開くと作り直して、放送の情報を埋めることを確認する。画面は、作り終えるまで読み直すので、埋まった項目を受け取れる。
func TestReadOldVersion(t *testing.T) {
	t.Parallel()

	f := newIndexFixture(t)
	dir := f.indexer.dir(f.ref)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, f.indexer.writeSource(dir, &source{
		Version: 1, Library: f.ref.Library, Path: f.ref.Path, Size: f.entry.Size, ModifiedAt: f.entry.ModifiedAt.UnixNano(),
		Complete: true, Video: nil, Broadcast: nil, Audios: nil, Quality: nil,
	}))
	require.NoError(t, os.WriteFile(filepath.Join(dir, indexName), f.index, 0o600))
	index, err := f.indexer.Read(f.ref, f.entry)
	require.NoError(t, err)
	assert.False(t, index.Complete)
	assert.NotEmpty(t, index.Entries, "作り直すまでも、前の索引でシークできる")

	f.indexer.Ensure(t.Context(), f.ref, f.entry)
	require.Eventually(t, func() bool {
		index, err := f.indexer.Read(f.ref, f.entry)
		require.NoError(t, err)

		return index.Complete && index.Broadcast != nil && index.Quality != nil
	}, 10*time.Second, 10*time.Millisecond)
}
