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
	"github.com/SlashNephy/kotatsu/internal/lib/logattr"
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
	// settleTime は、ファイルを最後に書いてから、索引を作り始めるまでに待つ時間。書いている途中のファイル (mikan が録っている
	// プールの録画や、写している途中のファイル) は、作り終えるたびに大きさが変わり、作り直しを繰り返すので、索引を作らない。
	settleTime = time.Minute
	// probeLimit は、映像の形式を読むときに、ファイルの頭から読む上限。
	probeLimit = 16 << 20
)

// sourceVersion は、source.json の版。読み取る項目を足したら上げ、前の版の記録のファイルは、索引を作り直して埋める。
// 2 で、放送の情報 (Broadcast)、音声の構成、ドロップの集計を足した (#1734 の 2 段目)。
const sourceVersion = 2

// source は、索引を作った元のファイルの記録 (source.json)。
type source struct {
	Version    int    `json:"version"`
	Library    string `json:"library"`
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	ModifiedAt int64  `json:"modifiedAt"`
	// Complete は、索引を作り終えたかどうか。
	Complete bool `json:"complete"`
	// Video は、映像の形式。読めなければ nil。
	Video *ts.VideoFormat `json:"video"`
	// Broadcast は、TS の中の SDT、EIT[p/f]、TOT から読んだ放送の情報。読めなければ nil。索引を作り始める前に書く。
	Broadcast *Broadcast `json:"broadcast"`
	// Audios は、放送中だった番組の音声の構成 (EPG と同じ形の JSON)。索引を作り終えたときに書く。読めなければ nil。
	Audios json.RawMessage `json:"audios,omitempty"`
	// Quality は、ドロップなどの集計。索引を作り終えたときに書く。
	Quality *Quality `json:"quality"`
}

// Broadcast は、ライブラリのファイルの、TS の中から読んだ放送の情報 (#1734)。録画の取り込み (設計書 6.5) と同じく読む。
type Broadcast struct {
	// NetworkID は、SDT[actual] の original_network_id。読めなければ nil。
	NetworkID *uint16 `json:"networkId"`
	// ServiceID は、映像を持つ最初のサービスのサービス ID。索引と配信 (tsreadex の -n -1) も、このサービスを使う。
	ServiceID uint16 `json:"serviceId"`
	// ServiceName は、SDT[actual] のサービス名。読めなければ空。
	ServiceName string `json:"serviceName"`
	// RecordedFrom と RecordedUntil は、頭で最初に読んだ TOT と、終わりで最後に読んだ TOT。読めなければ nil。
	// 再生の 0 秒 (最初の I フレーム) を RecordedFrom とみなす。取り込んだ録画と同じく、TOT の間隔 (数秒) だけずれることがある。
	RecordedFrom  *time.Time `json:"recordedFrom"`
	RecordedUntil *time.Time `json:"recordedUntil"`
	// Program は、録った範囲に最も長く重なる番組。読めなければ nil。
	Program *Program `json:"program"`
}

// Program は、EIT[p/f] で読んだ番組 (fileimport.Event を、記録に置ける形にしたもの)。
type Program struct {
	EventID     uint16          `json:"eventId"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Extended    []*ExtendedItem `json:"extended"`
	// Start と Duration は、番組の開始と長さ。未定なら nil。
	Start    *time.Time     `json:"start"`
	Duration *time.Duration `json:"duration"`
}

// ExtendedItem は、番組の詳細の 1 項目。
type ExtendedItem struct {
	Heading string `json:"heading"`
	Text    string `json:"text"`
}

// newProgram は、fileimport.Event を Program にする。event が nil なら nil を返す。
func newProgram(event *fileimport.Event) *Program {
	if event == nil {
		return nil
	}
	extended := make([]*ExtendedItem, 0, len(event.Extended))
	for _, item := range event.Extended {
		extended = append(extended, &ExtendedItem{Heading: item.Heading, Text: item.Text})
	}

	return &Program{
		EventID: event.EventID, Name: event.Name, Description: event.Description, Extended: extended, Start: event.Start, Duration: event.Duration,
	}
}

// Quality は、ライブラリのファイルのドロップなどの集計。録画の解析 (設計書 6.1) と同じく数える。
type Quality struct {
	Drops     int64 `json:"drops"`
	Errors    int64 `json:"errors"`
	Scrambled int64 `json:"scrambled"`
}

// Index は、ライブラリのファイルの索引。
type Index struct {
	// Entries は、索引。まだなければ空。作っている途中なら、できた所まで。
	Entries []ts.IndexEntry
	// Complete は、索引を作り終えたかどうか。
	Complete bool
	// Video は、映像の形式。まだ読んでいなければ nil。
	Video *ts.VideoFormat
	// Broadcast は、放送の情報。まだ読んでいないか、読めなければ nil。
	Broadcast *Broadcast
	// Audios は、音声の構成 (EPG と同じ形の JSON)。まだ読んでいないか、読めなければ nil。
	Audios json.RawMessage
	// Quality は、ドロップなどの集計。索引を作り終えるまでは nil。
	Quality *Quality
}

// Indexer は、ライブラリのファイルの索引を、データディレクトリの kago.LibraryIndexDir で読み書きする (設計書 5.8)。
// 中に「ライブラリの名前とパスのハッシュ/」を作り、索引 (index か、作っている途中の index.partial) と、元のファイルの記録 (source.json) を置く。
//
// 索引は、tenban が、ファイルを初めて開いたときに、裏で作る。nekonote のジョブにしないのは、次の理由による。
//   - ファイラーで開いてすぐにシークしたいので、ほかのジョブの後に並ばせたくない。I フレームを見つけるたびに追記するので、作りながら使える。
//   - 索引は配信の前に要るので、配信する tenban で作る。kyusu と nekonote (画像) は索引を読むだけにする。
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

	mu sync.Mutex
	// building は、作っている途中の索引。片付ける (ClearArtifacts) ときに取りやめて、止まるのを待つ。
	building map[string]*indexBuild
	// clearing は、片付けている途中かどうか。その間は、索引を作り始めない。
	clearing bool
	// stopped は、Run が終わるときに閉じる。索引を作る処理は、リクエストより長く、Run の間だけ動かす。
	stopped  chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewIndexer は Indexer を返す。dataDir は、データディレクトリ。build が false なら、索引を読むだけで作らない。
func NewIndexer(dataDir string, libraries *medialib.Set, clk clock.Clock, logger *slog.Logger, build bool) *Indexer {
	return &Indexer{
		dataDir: dataDir, libraries: libraries, clock: clk, logger: logger, build: build, slots: make(chan struct{}, maxBuilds),
		mu: sync.Mutex{}, building: map[string]*indexBuild{}, clearing: false, stopped: make(chan struct{}), stopOnce: sync.Once{}, wg: sync.WaitGroup{},
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
	empty := &Index{Entries: []ts.IndexEntry{}, Complete: false, Video: nil, Broadcast: nil, Audios: nil, Quality: nil}
	src, err := x.readSource(ref)
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return nil, err
	}
	if !x.matches(src, ref, entry) {
		return empty, nil
	}
	name := partialIndexName
	if src.Complete {
		name = indexName
	}
	// 前の版の記録は、作り直して項目を埋めるまで、作り終えていないものとして返す。画面が読み直して、埋まった項目を受け取れるようにする。
	complete := src.Complete && src.Version == sourceVersion
	file, err := sharedfile.Open(filepath.Join(x.dir(ref), name))
	if errors.Is(err, fs.ErrNotExist) && !complete {
		// 書きかけの索引の名前を変えてから、作り終えたと記録するまでの間なら、作り終えた索引がある。索引ではシークできるが、
		// ドロップなどの集計はまだ記録に書いていないので、作り終えていないものとして返す。画面が読み直して、集計を受け取る。
		file, err = sharedfile.Open(filepath.Join(x.dir(ref), indexName))
	}
	if errors.Is(err, fs.ErrNotExist) {
		return src.index([]ts.IndexEntry{}, false), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open the index: %w", err)
	}
	defer file.Close()
	entries, err := ts.ReadIndex(file)
	if err != nil {
		// 作り始めたばかりで、まだヘッダーを書いていないこともある。
		if !complete && errors.Is(err, ts.ErrInvalidIndex) {
			return src.index([]ts.IndexEntry{}, false), nil
		}

		return nil, err
	}

	return src.index(entries, complete), nil
}

// Ensure は、ファイル ref (大きさと更新日時は entry) の索引を作り終えていなければ、裏で作り始める。
// 同じ tenban で作っている途中か、別の tenban が作っている途中 (書きかけの索引が伸び続けている) なら、作り始めない。
// 作る処理は、ctx (リクエスト) が終わっても、Run が終わるまで続ける。
func (x *Indexer) Ensure(ctx context.Context, ref Ref, entry *medialib.Entry) {
	if !x.build {
		return
	}
	if x.Writing(entry) {
		return
	}
	key := indexKey(x.identify(ref))
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.building[key] != nil || x.clearing || x.isStopped() {
		return
	}
	if src, err := x.readSource(ref); err == nil && x.matches(src, ref, entry) {
		if src.Complete && src.Version == sourceVersion {
			return
		}
		if info, err := os.Stat(filepath.Join(x.dir(ref), partialIndexName)); err == nil && x.clock.Now().Sub(info.ModTime()) < staleBuild {
			return
		}
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	b := &indexBuild{cancel: cancel, done: make(chan struct{})}
	x.building[key] = b
	x.wg.Go(func() {
		defer func() {
			cancel()
			x.mu.Lock()
			delete(x.building, key)
			x.mu.Unlock()
			close(b.done)
		}()
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
				logattr.String("library", ref.Library), logattr.String("path", ref.Path), logattr.Error(err))
		}
	})
}

// indexBuild は、作っている途中の索引。
type indexBuild struct {
	// cancel は、作るのを取りやめる。
	cancel context.CancelFunc
	// done は、作るのをやめたら閉じる。
	done chan struct{}
}

// Writing は、ファイル (大きさと更新日時は entry) を、書いている途中とみなすかを返す。最後に書いてから settleTime たっていなければ、
// 書いている途中とみなし、索引を作らない。頭から再生することはできる。
func (x *Indexer) Writing(entry *medialib.Entry) bool {
	return x.clock.Now().Sub(entry.ModifiedAt) < settleTime
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
	return filepath.Join(x.dataDir, kago.LibraryIndexDir, indexKey(x.identify(ref)))
}

// identify は、ref のパスを、ライブラリがファイルを見分ける値 (medialib.IdentityOf) に置き換える。Google Drive では、
// フォルダをたどった道筋が違っても、同じファイルの索引を使い回す。
func (x *Indexer) identify(ref Ref) Ref {
	library, ok := x.libraries.Get(ref.Library)
	if !ok {
		return ref
	}

	return Ref{Library: ref.Library, Path: medialib.IdentityOf(library, ref.Path)}
}

// matches は、記録 src が、ファイル ref (大きさと更新日時は entry) のものかどうかを返す。
func (x *Indexer) matches(src *source, ref Ref, entry *medialib.Entry) bool {
	return src.matches(x.identify(Ref{Library: src.Library, Path: src.Path}), x.identify(ref), entry)
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
	src := &source{
		Version: sourceVersion, Library: ref.Library, Path: ref.Path, Size: entry.Size, ModifiedAt: entry.ModifiedAt.UnixNano(), Complete: false,
		Video: video, Broadcast: x.probeBroadcast(ctx, ref, file, entry.Size), Audios: nil, Quality: nil,
	}
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
	if err := src.count(stats); err != nil {
		return err
	}

	return x.writeSource(dir, src)
}

// count は、索引を作り終えたときの統計から、ドロップなどの集計と、放送中だった番組の音声の構成を記録に置く。
func (s *source) count(stats *ts.Stats) error {
	s.Quality = &Quality{Drops: 0, Errors: 0, Scrambled: 0}
	for _, pid := range stats.PIDs {
		s.Quality.Drops += pid.Drops
		s.Quality.Errors += pid.Errors
		s.Quality.Scrambled += pid.Scrambled
	}
	if s.Broadcast == nil {
		return nil
	}
	var eventID *uint16
	if s.Broadcast.Program != nil {
		eventID = &s.Broadcast.Program.EventID
	}
	audios := ts.PresentAudios(ts.MergeEvents(stats.Events), s.Broadcast.ServiceID, eventID)
	if audios == nil {
		return nil
	}
	encoded, err := ts.EncodeAudios(audios)
	if err != nil {
		return fmt.Errorf("failed to encode the audio components: %w", err)
	}
	s.Audios = encoded

	return nil
}

// probeBroadcast は、ファイルの頭と終わりを読んで、放送の情報を返す。録画の取り込み (fileimport.ReadProbe) と同じく読み、
// 映像を持つ最初のサービスと、録った範囲に最も長く重なる番組を選ぶ。読めなければ nil を返す。
func (x *Indexer) probeBroadcast(ctx context.Context, ref Ref, file io.ReaderAt, size int64) *Broadcast {
	probe, err := fileimport.ReadProbe(file, size)
	if err != nil {
		x.logger.WarnContext(ctx, "failed to read the broadcast of the library file",
			logattr.String("library", ref.Library), logattr.String("path", ref.Path), slog.Any("error", err))

		return nil
	}
	if len(probe.Services) == 0 {
		return nil
	}
	service := probe.Services[0]

	return &Broadcast{
		NetworkID: probe.NetworkID, ServiceID: service.ServiceID, ServiceName: service.Name,
		RecordedFrom: probe.RecordedFrom, RecordedUntil: probe.RecordedUntil, Program: newProgram(service.Program(probe.RecordedFrom, probe.RecordedUntil)),
	}
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

	return WriteFile(dir, sourceName, data)
}

// WriteFile は、索引の置き場 dir に、name のファイルを、一時ファイルに書いてから置き換えて書く。読む側に書きかけのファイルを見せない。
// 索引のほか、ライブラリのファイルの画像 (nekonote のジョブが作る) も、これで置く。
func WriteFile(dir, name string, data []byte) error {
	// 別の tenban が同じファイルの索引を作っていても、互いの一時ファイルを書き換えないよう、名前を重ねない。
	f, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", name, err)
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
		return fmt.Errorf("failed to write %s: %w", name, err)
	}
	if err := sharedfile.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("failed to replace %s: %w", name, err)
	}

	return nil
}

// index は、記録と索引から Index を作る。
func (s *source) index(entries []ts.IndexEntry, complete bool) *Index {
	return &Index{Entries: entries, Complete: complete, Video: s.Video, Broadcast: s.Broadcast, Audios: s.Audios, Quality: s.Quality}
}

// matches は、記録が、大きさと更新日時が entry のファイル ref のものかどうかを返す。
func (s *source) matches(stored, ref Ref, entry *medialib.Entry) bool {
	return stored == ref && s.Size == entry.Size && s.ModifiedAt == entry.ModifiedAt.UnixNano()
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
