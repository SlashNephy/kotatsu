package filelib

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/fileimport"
)

const (
	// indexName は、作り終えた索引のファイルの名前。
	indexName = "index"
	// partialIndexName は、作っている途中の索引のファイルの名前。I フレームを見つけるたびに追記するので、できた所まで読める。
	partialIndexName = "index.partial"
	// sourceName は、索引を作った元のファイルの大きさと更新日時を記録するファイルの名前。
	sourceName = "source.json"
	// staleBuild は、作っている途中の索引が、これより長く伸びなければ、作っていたプロセスが落ちたとみなして作り直す時間。
	// 別の tenban が作っている途中の索引は、作り直さずに使う。
	staleBuild = time.Minute
	// maxBuilds は、1 つの tenban で同時に作る索引の数。索引はファイルを読み通すので、ディスクを奪い合わないよう絞る。
	maxBuilds = 2
	// probeLimit は、映像の形式を読むときに、ファイルの頭から読む上限。
	probeLimit = 16 << 20
)

// source は、索引を作った元のファイルの記録 (source.json)。
type source struct {
	Library    string `json:"library"`
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	ModifiedAt int64  `json:"modifiedAt"`
	// Complete は、索引を作り終えたかどうか。
	Complete bool `json:"complete"`
	// Video は、映像の形式。読めなければ nil。
	Video *ts.VideoFormat `json:"video"`
}

// Index は、ライブラリのファイルの索引。
type Index struct {
	// Entries は、索引。まだなければ空。作っている途中なら、できた所まで。
	Entries []ts.IndexEntry
	// Complete は、索引を作り終えたかどうか。
	Complete bool
	// Video は、映像の形式。まだ読んでいなければ nil。
	Video *ts.VideoFormat
}

// Indexer は、ライブラリのファイルの索引を、データディレクトリの kago.LibraryIndexDir で読み書きする (設計書 5.8)。
// 中に「ライブラリの名前とパスのハッシュ/」を作り、索引 (index か、作っている途中の index.partial) と、元のファイルの記録 (source.json) を置く。
//
// 索引は、tenban が、ファイルを初めて開いたときに、裏で作る。nekonote のジョブにしないのは、次の理由による。
//   - ファイラーで開いてすぐにシークしたいので、ほかのジョブの後に並ばせたくない。I フレームを見つけるたびに追記するので、作りながら使える。
//   - ライブラリを読むのは、配信する tenban と kyusu で、nekonote にライブラリをマウントさせずに済む。
//   - 録画として登録しないので、ジョブの重複を防ぐ DB の行がない。同じ tenban の中では 1 つに束ね、別の tenban が作っている途中の索引は
//     (伸び続けていれば) 使う。落ちた tenban の書きかけは、staleBuild 伸びなければ作り直す。
//   - tenban を止めたら、作るのをやめる。次に開いたときに、始めから作り直す。
type Indexer struct {
	dataDir   string
	libraries *medialib.Set
	clock     clock.Clock
	logger    *slog.Logger
	// build は、索引を作るかどうか。kyusu は読むだけで作らない。
	build bool
	slots chan struct{}

	mu       sync.Mutex
	building map[string]bool
	// stopped は、Run が終わるときに閉じる。索引を作る処理は、リクエストより長く、Run の間だけ動かす。
	stopped  chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewIndexer は Indexer を返す。dataDir は、データディレクトリ。build が false なら、索引を読むだけで作らない。
func NewIndexer(dataDir string, libraries *medialib.Set, clk clock.Clock, logger *slog.Logger, build bool) *Indexer {
	return &Indexer{
		dataDir: dataDir, libraries: libraries, clock: clk, logger: logger, build: build, slots: make(chan struct{}, maxBuilds),
		mu: sync.Mutex{}, building: map[string]bool{}, stopped: make(chan struct{}), stopOnce: sync.Once{}, wg: sync.WaitGroup{},
	}
}

// Run は、ctx が終わるまで待ち、作っている途中の索引を止めて、止まるのを待つ。
func (x *Indexer) Run(ctx context.Context) error {
	<-ctx.Done()
	x.mu.Lock()
	x.stopOnce.Do(func() { close(x.stopped) })
	x.mu.Unlock()
	x.wg.Wait()

	return nil
}

// Read は、大きさと更新日時が entry のファイル ref の索引を読む。索引がないか、ファイルが変わっていれば、空の索引を返す。
func (x *Indexer) Read(ref Ref, entry *medialib.Entry) (*Index, error) {
	empty := &Index{Entries: []ts.IndexEntry{}, Complete: false, Video: nil}
	src, err := x.readSource(ref)
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return nil, err
	}
	if !src.matches(ref, entry) {
		return empty, nil
	}
	name := partialIndexName
	if src.Complete {
		name = indexName
	}
	complete := src.Complete
	file, err := sharedfile.Open(filepath.Join(x.dir(ref), name))
	if errors.Is(err, fs.ErrNotExist) && !complete {
		// 書きかけの索引の名前を変えてから、作り終えたと記録するまでの間なら、作り終えた索引がある。
		file, err = sharedfile.Open(filepath.Join(x.dir(ref), indexName))
		complete = err == nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return &Index{Entries: []ts.IndexEntry{}, Complete: false, Video: src.Video}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open the index: %w", err)
	}
	defer file.Close()
	entries, err := ts.ReadIndex(file)
	if err != nil {
		// 作り始めたばかりで、まだヘッダーを書いていないこともある。
		if !complete && errors.Is(err, ts.ErrInvalidIndex) {
			return &Index{Entries: []ts.IndexEntry{}, Complete: false, Video: src.Video}, nil
		}

		return nil, err
	}

	return &Index{Entries: entries, Complete: complete, Video: src.Video}, nil
}

// Ensure は、ファイル ref (大きさと更新日時は entry) の索引を作り終えていなければ、裏で作り始める。
// 同じ tenban で作っている途中か、別の tenban が作っている途中 (書きかけの索引が伸び続けている) なら、作り始めない。
// 作る処理は、ctx (リクエスト) が終わっても、Run が終わるまで続ける。
func (x *Indexer) Ensure(ctx context.Context, ref Ref, entry *medialib.Entry) {
	if !x.build {
		return
	}
	key := indexKey(ref)
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.building[key] || x.isStopped() {
		return
	}
	if src, err := x.readSource(ref); err == nil && src.matches(ref, entry) {
		if src.Complete {
			return
		}
		if info, err := os.Stat(filepath.Join(x.dir(ref), partialIndexName)); err == nil && x.clock.Now().Sub(info.ModTime()) < staleBuild {
			return
		}
	}
	x.building[key] = true
	x.wg.Go(func() {
		defer func() {
			x.mu.Lock()
			delete(x.building, key)
			x.mu.Unlock()
		}()
		ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		defer cancel()
		go func() {
			select {
			case <-x.stopped:
				cancel()
			case <-ctx.Done():
			}
		}()
		select {
		case x.slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-x.slots }()
		if err := x.buildIndex(ctx, ref, entry); err != nil && ctx.Err() == nil {
			x.logger.WarnContext(ctx, "failed to index the library file",
				slog.String("library", ref.Library), slog.String("path", ref.Path), slog.Any("error", err))
		}
	})
}

// isStopped は、Run が終わったかどうかを返す。
func (x *Indexer) isStopped() bool {
	select {
	case <-x.stopped:
		return true
	default:
		return false
	}
}

// dir は、ref の索引のディレクトリのパスを返す。
func (x *Indexer) dir(ref Ref) string {
	return filepath.Join(x.dataDir, kago.LibraryIndexDir, indexKey(ref))
}

// buildIndex は、ファイルを読み通して、索引を作る。I フレームを見つけるたびに、書きかけの索引に追記する。
func (x *Indexer) buildIndex(ctx context.Context, ref Ref, entry *medialib.Entry) error {
	library, ok := x.libraries.Get(ref.Library)
	if !ok {
		return fmt.Errorf("%w: library %s", medialib.ErrNotFound, ref.Library)
	}
	file, err := library.Open(ctx, ref.Path)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := fileimport.CheckPacketSize(file, entry.Size); err != nil {
		return err
	}
	video, err := probeVideo(file, entry.Size)
	if err != nil && !errors.Is(err, ErrNoVideo) {
		return err
	}
	dir := x.dir(ref)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create the index directory: %w", err)
	}
	src := &source{Library: ref.Library, Path: ref.Path, Size: entry.Size, ModifiedAt: entry.ModifiedAt.UnixNano(), Complete: false, Video: video}
	// 先に作り終えた索引を消してから、作っている途中だと記録する。読む側は、記録を見てから索引を選ぶ。
	if err := os.Remove(filepath.Join(dir, indexName)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to remove the old index: %w", err)
	}
	partialPath := filepath.Join(dir, partialIndexName)
	output, err := os.OpenFile(partialPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to create the index file: %w", err)
	}
	defer output.Close()
	if err := x.writeSource(dir, src); err != nil {
		return err
	}
	writer, err := ts.NewIndexWriter(output)
	if err != nil {
		return err
	}
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer, CutoffPTS: nil, OnTOT: nil, OnLogo: nil, OnNIT: nil, OnSDT: nil})
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek the file: %w", err)
	}
	// tenban を止めたら、読むのをやめる。
	reader := readerFunc(func(p []byte) (int, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}

		return file.Read(p)
	})
	if _, err := io.Copy(analyzer, reader); err != nil {
		return fmt.Errorf("failed to read the file: %w", err)
	}
	stats := analyzer.Stats()
	if stats.IndexError != nil {
		return fmt.Errorf("failed to write the index: %w", stats.IndexError)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("failed to close the index file: %w", err)
	}
	if err := sharedfile.Rename(partialPath, filepath.Join(dir, indexName)); err != nil {
		return fmt.Errorf("failed to finish the index file: %w", err)
	}
	src.Complete = true
	if src.Video == nil {
		src.Video = stats.Video
	}

	return x.writeSource(dir, src)
}

// readSource は、ref の索引の元のファイルの記録を読む。
func (x *Indexer) readSource(ref Ref) (*source, error) {
	data, err := sharedfile.ReadFile(filepath.Join(x.dir(ref), sourceName))
	if err != nil {
		return nil, err
	}
	var src source
	if err := json.Unmarshal(data, &src); err != nil {
		return nil, fmt.Errorf("failed to read the index source: %w", err)
	}

	return &src, nil
}

// writeSource は、元のファイルの記録を、一時ファイルに書いてから置き換える。読む側に書きかけの記録を見せない。
func (x *Indexer) writeSource(dir string, src *source) error {
	data, err := json.Marshal(src)
	if err != nil {
		return fmt.Errorf("failed to encode the index source: %w", err)
	}
	// 別の tenban が同じファイルの索引を作っていても、互いの一時ファイルを書き換えないよう、名前を重ねない。
	f, err := os.CreateTemp(dir, sourceName+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to write the index source: %w", err)
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	// 索引と同じく、kyusu (別のユーザーで動くこともある) からも読めるようにする。
	err = f.Chmod(0o644)
	if err == nil {
		_, err = f.Write(data)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("failed to write the index source: %w", err)
	}
	if err := sharedfile.Rename(tmp, filepath.Join(dir, sourceName)); err != nil {
		return fmt.Errorf("failed to replace the index source: %w", err)
	}

	return nil
}

// matches は、記録が、大きさと更新日時が entry のファイル ref のものかどうかを返す。
func (s *source) matches(ref Ref, entry *medialib.Entry) bool {
	return s.Library == ref.Library && s.Path == ref.Path && s.Size == entry.Size && s.ModifiedAt == entry.ModifiedAt.UnixNano()
}

// ErrNoVideo は、ファイルの頭から、映像の形式を読めなかったことを表す。
var ErrNoVideo = errors.New("no video format was found at the head of the file")

// ProbeVideo は、ファイルの頭を読んで、映像の形式を返す。読めなければ ErrNoVideo を返す。
// 192 バイトのパケットの Blu-ray の .m2ts なら fileimport.ErrBDAV を、TS でなければ fileimport.ErrNotTS を返す。
func ProbeVideo(file io.ReaderAt, size int64) (*ts.VideoFormat, error) {
	if err := fileimport.CheckPacketSize(file, size); err != nil {
		return nil, err
	}

	return probeVideo(file, size)
}

func probeVideo(file io.ReaderAt, size int64) (*ts.VideoFormat, error) {
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: nil, CutoffPTS: nil, OnTOT: nil, OnLogo: nil, OnNIT: nil, OnSDT: nil})
	buf := make([]byte, ts.PacketSize*1024)
	for offset := int64(0); offset < min(size, probeLimit); {
		n, err := file.ReadAt(buf, offset)
		if n > 0 {
			_, _ = analyzer.Write(buf[:n])
			offset += int64(n)
			if video := analyzer.Stats().Video; video != nil {
				return video, nil
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read the file: %w", err)
		}
	}

	return nil, ErrNoVideo
}

// readerFunc は、関数を io.Reader にする。
type readerFunc func(p []byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) {
	return f(p)
}
