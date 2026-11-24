package filelib

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/thumbnail"
)

// fakeExtractor は、FFmpeg の代わりに、小さな灰色の JPEG を返す。読んだ TS の大きさを数える。
type fakeExtractor struct {
	read int64
}

var _ thumbnail.Extractor = new(fakeExtractor)

func (e *fakeExtractor) Extract(_ context.Context, r io.Reader, _ thumbnail.Size) ([]byte, error) {
	n, err := io.Copy(io.Discard, r)
	if err != nil {
		return nil, err
	}
	e.read += n
	var out bytes.Buffer
	if err := jpeg.Encode(&out, image.NewGray(image.Rect(0, 0, 64, 36)), nil); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// constPreview は、プレビュー画像を作るかの設定を、決まった値で返す。
func constPreview(enabled bool) func(context.Context) (bool, error) {
	return func(context.Context) (bool, error) { return enabled, nil }
}

// fakeInserter は、入れたジョブの引数を保存する。
type fakeInserter struct {
	args []river.JobArgs
}

func (i *fakeInserter) Insert(_ context.Context, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	i.args = append(i.args, args)

	return &rivertype.JobInsertResult{Job: nil, UniqueSkippedAsDuplicate: false}, nil
}

// imagesFixture は、インデックスを作り終えたライブラリのファイルと、画像を作る Worker を持つ。
type imagesFixture struct {
	dir       string
	ref       Ref
	reader    *Source
	tenban    *Source
	worker    *ImagesWorker
	extractor *fakeExtractor
}

func newImagesFixture(t *testing.T) *imagesFixture {
	t.Helper()

	libraryDir, dataDir := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(libraryDir, internalShowTS), readTestTS(t), 0o600))
	libraries := medialib.NewSet(map[string]string{}, []string{}, map[string]string{internalLibrary: libraryDir})
	clk := clocktest.NewFake(time.Now().Add(time.Hour))
	builder := NewIndexer(dataDir, libraries, clk, slog.New(slog.DiscardHandler), true)
	ref := Ref{Library: internalLibrary, Path: internalShowTS}
	tenban := NewSource(libraries, builder)
	require.Eventually(t, func() bool {
		file, err := tenban.Open(t.Context(), ref)
		require.NoError(t, err)

		return file.Index.Complete
	}, 10*time.Second, 10*time.Millisecond)
	reader := NewIndexer(dataDir, libraries, clk, slog.New(slog.DiscardHandler), false)
	extractor := new(fakeExtractor)

	return &imagesFixture{
		dir: libraryDir, ref: ref, reader: NewSource(libraries, reader), tenban: tenban, extractor: extractor,
		worker: newImagesWorker(libraries, reader, constPreview(true), func() (thumbnail.Extractor, error) { return extractor, nil }),
	}
}

func (f *imagesFixture) open(t *testing.T) *File {
	t.Helper()

	file, err := f.reader.Open(t.Context(), f.ref)
	require.NoError(t, err)

	return file
}

func (f *imagesFixture) get(t *testing.T, name string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("GET /internal/media/library/{id}/{file}", NewImageHandler(f.reader, f.reader.Indexer(), slog.New(slog.DiscardHandler)))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/media/library/"+f.ref.Encode()+"/"+name, nil))

	return rec
}

// TestImagesWorker は、インデックスを作り終えたファイルの代表画像とプレビュー画像を、ライブラリの口でファイルを読んで作り、
// 今のファイルから作った画像だけを配信することを確認する (#1734)。
func TestImagesWorker(t *testing.T) {
	t.Parallel()

	f := newImagesFixture(t)
	file := f.open(t)
	inserter := new(fakeInserter)
	require.NoError(t, EnqueueImages(t.Context(), inserter, f.reader.Indexer(), file))
	require.Len(t, inserter.args, 1, "画像がなければ、作るジョブを入れる")
	args, ok := inserter.args[0].(ImagesArgs)
	require.True(t, ok)
	assert.Equal(t, ImagesArgs{Library: internalLibrary, Path: internalShowTS, FileID: file.ID()}, args)
	assert.Equal(t, http.StatusNotFound, f.get(t, PreviewFileName).Code, "作るまでは返さない")

	require.NoError(t, f.worker.Work(t.Context(), &river.Job[ImagesArgs]{JobRow: nil, Args: args}))
	assert.Positive(t, f.extractor.read, "ライブラリの口でファイルを読む")
	assert.True(t, f.reader.Indexer().HasImages(f.ref, file.ID()))
	for _, tt := range []struct {
		name        string
		contentType string
	}{
		{name: ThumbnailFileName, contentType: "image/jpeg"},
		{name: PreviewFileName, contentType: "image/jpeg"},
		{name: ManifestFileName, contentType: "application/json"},
	} {
		rec := f.get(t, tt.name)
		require.Equal(t, http.StatusOK, rec.Code, tt.name)
		assert.Equal(t, tt.contentType, rec.Header().Get("Content-Type"), tt.name)
	}
	assert.Equal(t, http.StatusNotFound, f.get(t, "index").Code, "画像でないファイルは返さない")
	inserter.args = nil
	require.NoError(t, EnqueueImages(t.Context(), inserter, f.reader.Indexer(), file))
	assert.Empty(t, inserter.args, "画像があれば、入れない")

	// ファイルを書き換えたら、前のファイルの画像は返さず、前のファイルのジョブは取りやめる。
	later := file.Entry.ModifiedAt.Add(time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(f.dir, internalShowTS), later, later))
	assert.Equal(t, http.StatusNotFound, f.get(t, PreviewFileName).Code)
	err := f.worker.Work(t.Context(), &river.Job[ImagesArgs]{JobRow: nil, Args: args})
	var cancel *rivertype.JobCancelError
	require.ErrorAs(t, err, &cancel)
	assert.ErrorIs(t, err, errFileChanged)
}

// TestImagesWorkerWithoutPreview は、プレビュー画像を作らない設定では代表画像だけを作り、前のファイルから作ったプレビュー画像を残さず、
// 配信もしないことを確認する。保存先はファイルのパスごとなので、残すと、書き換えた後のファイルに前のファイルのプレビュー画像を出す。
func TestImagesWorkerWithoutPreview(t *testing.T) {
	t.Parallel()

	f := newImagesFixture(t)
	file := f.open(t)
	args := ImagesArgs{Library: internalLibrary, Path: internalShowTS, FileID: file.ID()}
	require.NoError(t, f.worker.Work(t.Context(), &river.Job[ImagesArgs]{JobRow: nil, Args: args}))
	require.Equal(t, http.StatusOK, f.get(t, PreviewFileName).Code)

	// プレビュー画像を作らない設定にしてから、ファイルを書き換える。
	later := file.Entry.ModifiedAt.Add(time.Minute)
	require.NoError(t, os.Chtimes(filepath.Join(f.dir, internalShowTS), later, later))
	var changed *File
	require.Eventually(t, func() bool {
		var err error
		changed, err = f.tenban.Open(t.Context(), f.ref)
		require.NoError(t, err)

		return changed.Index.Complete
	}, 10*time.Second, 10*time.Millisecond)
	worker := newImagesWorker(f.worker.libraries, f.reader.Indexer(), constPreview(false), func() (thumbnail.Extractor, error) { return f.extractor, nil })
	args.FileID = changed.ID()
	require.NoError(t, worker.Work(t.Context(), &river.Job[ImagesArgs]{JobRow: nil, Args: args}))
	assert.Equal(t, http.StatusOK, f.get(t, ThumbnailFileName).Code)
	for _, name := range []string{PreviewFileName, ManifestFileName} {
		assert.Equal(t, http.StatusNotFound, f.get(t, name).Code, name)
		assert.NoFileExists(t, f.reader.Indexer().ImagePath(f.ref, name), name)
	}
}
