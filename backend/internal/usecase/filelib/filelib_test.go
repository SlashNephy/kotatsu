package filelib_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/fileimport"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
)

const (
	// animeLibrary と testLibrary は、テストのライブラリの名前。
	animeLibrary = "anime"
	testLibrary  = "lib"
	// showTS と aTS は、テストのファイルの名前。
	showTS = "show.ts"
	aTS    = "a.ts"
)

// loadTestTS は、testdata/ts/test.ts (MPEG-2 の 480i) を返す。
func loadTestTS(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)

	return data
}

func TestRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoded string
		want    filelib.Ref
		wantOK  bool
	}{
		{name: "round trip", encoded: filelib.Ref{Library: animeLibrary, Path: "2026/秋/第1話.m2ts"}.Encode(), want: filelib.Ref{Library: animeLibrary, Path: "2026/秋/第1話.m2ts"}, wantOK: true},
		{name: "not base64", encoded: "!!", wantOK: false},
		{name: "no separator", encoded: "YW5pbWU", wantOK: false},
		{name: "traversal", encoded: filelib.Ref{Library: animeLibrary, Path: "../etc/passwd"}.Encode(), wantOK: false},
		{name: "absolute", encoded: filelib.Ref{Library: animeLibrary, Path: "/etc/passwd"}.Encode(), wantOK: false},
		{name: "root", encoded: filelib.Ref{Library: animeLibrary, Path: ""}.Encode(), wantOK: false},
		{name: "no library", encoded: filelib.Ref{Library: "", Path: aTS}.Encode(), wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := filelib.DecodeRef(tt.encoded)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestFileID(t *testing.T) {
	t.Parallel()

	ref := filelib.Ref{Library: animeLibrary, Path: aTS}
	at := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	base := filelib.FileID(ref, 100, at)
	tests := []struct {
		name string
		id   string
		same bool
	}{
		{name: "same file", id: filelib.FileID(ref, 100, at), same: true},
		{name: "same instant in another zone", id: filelib.FileID(ref, 100, at.In(time.FixedZone("JST", 9*3600))), same: true},
		{name: "grown", id: filelib.FileID(ref, 101, at), same: false},
		{name: "rewritten", id: filelib.FileID(ref, 100, at.Add(time.Second)), same: false},
		{name: "other path", id: filelib.FileID(filelib.Ref{Library: animeLibrary, Path: "b.ts"}, 100, at), same: false},
		{name: "other library", id: filelib.FileID(filelib.Ref{Library: "backup", Path: aTS}, 100, at), same: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.same, tt.id == base)
		})
	}
}

// newFixture は、testdata/ts/test.ts (MPEG-2 の 480i) を show.ts として置いたライブラリから、インデックスを作りながら開く Source と、
// ライブラリのディレクトリを返す。
func newFixture(t *testing.T) (*filelib.Source, string) {
	t.Helper()
	dir, dataDir := t.TempDir(), t.TempDir()
	data := loadTestTS(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, showTS), data, 0o600))
	// 192 バイトのパケットの Blu-ray の .m2ts。TS のパケットの前に 4 バイトの時刻を付ける。
	bdav := make([]byte, 0, 192*64)
	for range 64 {
		packet := make([]byte, 192)
		packet[4] = 0x47
		bdav = append(bdav, packet...)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "disc.m2ts"), bdav, 0o600))
	libraries := medialib.NewSet(map[string]string{}, []string{}, map[string]string{testLibrary: dir})
	// 置いたばかりのファイルは書いている途中とみなされるので、時計を進めておく。
	indexer := filelib.NewIndexer(dataDir, libraries, clocktest.NewFake(time.Now().Add(time.Hour)), slog.New(slog.DiscardHandler), true)
	// テストを終えるときは、作っている途中のインデックスを止めてから、一時ディレクトリを消す。
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		assert.NoError(t, indexer.Run(ctx))
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	return filelib.NewSource(libraries, indexer), dir
}

// waitComplete は、インデックスを作り終えるまで、開き直して待つ。
func waitComplete(t *testing.T, source *filelib.Source, ref filelib.Ref) *filelib.File {
	t.Helper()
	var file *filelib.File
	require.Eventually(t, func() bool {
		var err error
		file, err = source.Open(t.Context(), ref)
		require.NoError(t, err)

		return file.Index.Complete
	}, 10*time.Second, 10*time.Millisecond)

	return file
}

func TestSourceIndexesOnOpen(t *testing.T) {
	t.Parallel()

	source, dir := newFixture(t)
	ref := filelib.Ref{Library: testLibrary, Path: showTS}
	file := waitComplete(t, source, ref)
	require.NotEmpty(t, file.Index.Entries)
	require.NotNil(t, file.Index.Video)
	assert.Equal(t, "mpeg2", file.Index.Video.Codec)
	// 放送の情報は、録画の取り込みと同じく、TS の中の SDT、EIT[p/f]、TOT から読む。
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("JST", 9*3600))
	require.NotNil(t, file.Index.Broadcast)
	assert.Equal(t, new(uint16(0x7FE0)), file.Index.Broadcast.NetworkID)
	assert.Equal(t, uint16(0x0400), file.Index.Broadcast.ServiceID)
	assert.Equal(t, "こたつテスト１", file.Index.Broadcast.ServiceName)
	require.NotNil(t, file.Index.Broadcast.RecordedFrom)
	assert.True(t, file.Index.Broadcast.RecordedFrom.Equal(start))
	require.NotNil(t, file.Index.Broadcast.Program)
	assert.Equal(t, "テスト番組", file.Index.Broadcast.Program.Name)
	require.NotNil(t, file.Index.Quality)
	assert.Equal(t, &filelib.Quality{Drops: 0, Errors: 0, Scrambled: 0}, file.Index.Quality)
	assert.False(t, file.Recording.InProgress)
	seekable, duration, ok := file.Timeline()
	require.True(t, ok)
	assert.Positive(t, seekable)
	assert.Equal(t, seekable, duration)

	// ファイルを書き換えたら、前の索引は使わず、作り直す。
	path := filepath.Join(dir, showTS)
	later := file.Entry.ModifiedAt.Add(time.Minute)
	require.NoError(t, os.Chtimes(path, later, later))
	reopened, err := source.Open(t.Context(), ref)
	require.NoError(t, err)
	assert.NotEqual(t, file.ID(), reopened.ID())
	if !reopened.Index.Complete {
		assert.True(t, reopened.Recording.InProgress)
	}
	rebuilt := waitComplete(t, source, ref)
	assert.Equal(t, file.Index.Entries, rebuilt.Index.Entries)
}

func TestSourceOpenErrors(t *testing.T) {
	t.Parallel()

	source, _ := newFixture(t)
	tests := []struct {
		name    string
		ref     filelib.Ref
		wantErr error
	}{
		{name: "missing library", ref: filelib.Ref{Library: "missing", Path: showTS}, wantErr: medialib.ErrNotFound},
		{name: "missing file", ref: filelib.Ref{Library: testLibrary, Path: "missing.ts"}, wantErr: medialib.ErrNotFound},
		{name: "not a TS", ref: filelib.Ref{Library: testLibrary, Path: "notes.txt"}, wantErr: filelib.ErrNotAFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := source.Open(t.Context(), tt.ref)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestSourceBDAV は、192 バイトのパケットの .m2ts は、インデックスを作らず、映像の形式を読むときに見分けることを確認する。
func TestSourceBDAV(t *testing.T) {
	t.Parallel()

	source, dir := newFixture(t)
	file, err := source.Open(t.Context(), filelib.Ref{Library: testLibrary, Path: "disc.m2ts"})
	require.NoError(t, err)
	assert.Empty(t, file.Index.Entries)
	f, err := os.Open(filepath.Join(dir, "disc.m2ts"))
	require.NoError(t, err)
	defer f.Close()
	_, err = filelib.ProbeVideo(f, file.Entry.Size)
	require.ErrorIs(t, err, fileimport.ErrBDAV)
}

// TestSourceSkipsWritingFiles は、最後に書いてから 1 分たっていないファイルは、書いている途中とみなしてインデックスを作らず、
// 頭から流せることを確認する。録画中のプールのファイルで、作り終えるたびに作り直すのを繰り返さないためである。
func TestSourceSkipsWritingFiles(t *testing.T) {
	t.Parallel()

	dir, dataDir := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, showTS), loadTestTS(t), 0o600))
	libraries := medialib.NewSet(map[string]string{}, []string{}, map[string]string{testLibrary: dir})
	indexer := filelib.NewIndexer(dataDir, libraries, clocktest.NewFake(time.Now()), slog.New(slog.DiscardHandler), true)
	file, err := filelib.NewSource(libraries, indexer).Open(t.Context(), filelib.Ref{Library: testLibrary, Path: showTS})
	require.NoError(t, err)
	assert.True(t, file.Writing)
	assert.Empty(t, file.Index.Entries)
	assert.Equal(t, time.Duration(0), file.Recording.Seek(time.Minute).At)
	entries, err := os.ReadDir(dataDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "書いている途中のファイルの索引は作らない")
}

// TestIndexerReadOnly は、インデックスを作らない Indexer (kyusu) は、開いてもインデックスを作り始めないことを確認する。
func TestIndexerReadOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	data := loadTestTS(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, showTS), data, 0o600))
	libraries := medialib.NewSet(map[string]string{}, []string{}, map[string]string{testLibrary: dir})
	dataDir := t.TempDir()
	indexer := filelib.NewIndexer(dataDir, libraries, clocktest.NewFake(time.Now()), slog.New(slog.DiscardHandler), false)
	file, err := filelib.NewSource(libraries, indexer).Open(t.Context(), filelib.Ref{Library: testLibrary, Path: showTS})
	require.NoError(t, err)
	assert.Empty(t, file.Index.Entries)
	assert.True(t, file.Recording.InProgress)
	_, _, ok := file.Timeline()
	assert.False(t, ok)
	entries, err := os.ReadDir(dataDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
