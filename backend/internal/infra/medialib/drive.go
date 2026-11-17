package medialib

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/googledrive"
)

const (
	// GoogleDriveName は、Google Drive のファイルを見分ける値の名前空間。索引の置き場と再生位置の file_id は、ライブラリの名前でなく、
	// この名前と Drive のファイルの ID から作るので、ライブラリの名前を変えても、パスの形を変えても引き継がれる。
	GoogleDriveName = "google-drive"
	// KindGoogleDrive は、Google Drive のライブラリ (#1902)。管理者が選んだフォルダごとに 1 つある。
	KindGoogleDrive Kind = "google_drive"
	// driveStatTTL は、ファイルの情報とフォルダの中身を覚えておく長さ。再生の画面は、索引を作り終えるまで数秒ごとに読み直すので、
	// そのたびに問い合わせない。
	driveStatTTL = 30 * time.Second
	// drivePathTTL は、パスからファイルの ID を引いた結果を覚えておく長さ。
	drivePathTTL = 5 * time.Minute
	// maxDriveCache は、覚えておくものの数の上限。超えたら、まとめて捨てる。
	maxDriveCache = 4096
	// maxDriveDepth は、親をたどる深さの上限。
	maxDriveDepth = 64
	// driveSearchPages は、名前で探すときに読むページの数の上限 (1 ページ 1000 件)。
	driveSearchPages = 5
	// driveSkipLimit は、読み出しの位置が少し先へ飛んだときに、新しく問い合わせずに読み捨てて進める長さの上限。
	driveSkipLimit = 4 << 20
	// driveReopenLimit は、読み出しが途中で切れたときに、読めた所から開き直す回数の上限。
	driveReopenLimit = 5
	// driveLookupTimeout は、ctx のない呼び出し (Set.Get) で、選んだフォルダを読み直すときの時間の上限。
	driveLookupTimeout = 10 * time.Second
	// driveIDSeparator は、同じフォルダに同じ名前が並ぶときに、名前の後ろに付ける、ID の頭の前の印。
	driveIDSeparator = "~"
	// minDriveIDPrefix は、名前の後ろに付ける ID の頭の、短い方の長さ。
	minDriveIDPrefix = 6
)

var (
	// ErrNotConnected は、Google Drive を接続していないことを表す。ErrNotFound でもある。
	ErrNotConnected = fmt.Errorf("%w: the Google Drive is not connected", ErrNotFound)
	// errOutsideFolder は、選んだフォルダの外のファイルであることを表す。ErrNotFound でもある。外にあるか、ないかは区別して見せない。
	errOutsideFolder = fmt.Errorf("%w: outside the selected folder", ErrNotFound)
)

// DriveAPI は、Google Drive のライブラリが使う Drive API。*googledrive.Client が満たす。
type DriveAPI interface {
	Children(ctx context.Context, parentID, driveID string) ([]*googledrive.File, error)
	SearchFiles(ctx context.Context, name, driveID string, maxPages int) ([]*googledrive.File, bool, error)
	Get(ctx context.Context, id string) (*googledrive.File, error)
	Download(ctx context.Context, id string, offset int64) (io.ReadCloser, error)
}

var _ DriveAPI = new(googledrive.Client)

// DriveFolder は、管理者が、ライブラリとして見せると選んだ Drive のフォルダ。選んだフォルダごとに、ライブラリが 1 つある。
type DriveFolder struct {
	// ID は、フォルダの ID。
	ID string
	// DriveID は、フォルダのある共有ドライブの ID。マイドライブなら空。
	DriveID string
	// Library は、ライブラリの名前。
	Library string
}

// DriveScope は、Google Drive の接続と、見せるフォルダ。
type DriveScope interface {
	// Connected は、管理者が Google Drive を接続していて、使えるかどうかを返す。
	Connected(ctx context.Context) (bool, error)
	// Folders は、管理者が選んだフォルダを、選んだ順に返す。
	Folders(ctx context.Context) ([]*DriveFolder, error)
}

// Drive は、管理者が接続した Google Drive の、選んだフォルダごとのライブラリを作る (設計書 5.8、#1902)。
//
// ライブラリの中のパスは、ふだんはフォルダとファイルの名前を / でつなぐ。Drive では同じフォルダに同じ名前が並ぶことがあるので、
// そのときだけ、名前の後ろに ~ と ID の頭を付けて見分ける。名前の / と \ は、形の似た文字に置き換える。パスは、選んだフォルダから
// 名前をたどって引くので、選んだフォルダの外のファイルには届かない。ファイルの ID を見分ける値にし、索引と再生位置は ID で引き継ぐ。
type Drive struct {
	api   DriveAPI
	scope DriveScope
	now   func() time.Time

	mu    sync.Mutex
	stats map[string]*driveStat
	dirs  map[string]*driveDir
	paths map[string]*drivePath
}

type driveStat struct {
	file      *googledrive.File
	fetchedAt time.Time
}

// driveDir は、フォルダの中身。段の名前から子への対応も持つ。
type driveDir struct {
	children  []*googledrive.File
	segments  map[string]*googledrive.File
	fetchedAt time.Time
}

// drivePath は、ライブラリの中のパスから引いたファイルの ID。
type drivePath struct {
	id        string
	fetchedAt time.Time
}

var _ LibraryProvider = new(Drive)

// NewDrive は、Google Drive のライブラリを作る部品を返す。scope は、接続と、管理者が選んだフォルダを返す。
func NewDrive(api DriveAPI, scope DriveScope, now func() time.Time) *Drive {
	return &Drive{
		api: api, scope: scope, now: now, mu: sync.Mutex{},
		stats: map[string]*driveStat{}, dirs: map[string]*driveDir{}, paths: map[string]*drivePath{},
	}
}

// Libraries は、選んだフォルダごとのライブラリを、選んだ順に返す。接続していなければ、何も返さない。
func (d *Drive) Libraries(ctx context.Context) ([]Library, error) {
	connected, err := d.scope.Connected(ctx)
	if err != nil || !connected {
		return []Library{}, err
	}
	folders, err := d.scope.Folders(ctx)
	if err != nil {
		return nil, err
	}
	libraries := make([]Library, 0, len(folders))
	for _, folder := range folders {
		libraries = append(libraries, &DriveLibrary{drive: d, folder: folder})
	}

	return libraries, nil
}

// Lookup は、名前が name のライブラリを返す。ctx を受け取らない呼び出し (Set.Get) から使うので、選んだフォルダは、
// scope が覚えたものを、時間を区切って読む。接続していなくても返し、読むときに ErrNotConnected を返す。
func (d *Drive) Lookup(name string) (Library, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), driveLookupTimeout)
	defer cancel()
	folders, err := d.scope.Folders(ctx)
	if err != nil {
		return nil, false
	}
	for _, folder := range folders {
		if folder.Library == name {
			return &DriveLibrary{drive: d, folder: folder}, true
		}
	}

	return nil, false
}

// DriveLibrary は、管理者が選んだ Drive のフォルダ 1 つのライブラリ。
type DriveLibrary struct {
	drive  *Drive
	folder *DriveFolder
}

var (
	_ Library    = new(DriveLibrary)
	_ Identifier = new(DriveLibrary)
	_ Namer      = new(DriveLibrary)
	_ Connector  = new(DriveLibrary)
	_ Searcher   = new(DriveLibrary)
)

func (l *DriveLibrary) Name() string { return l.folder.Library }

func (l *DriveLibrary) Kind() Kind { return KindGoogleDrive }

// Connected は、管理者が Google Drive を接続していて、使えるかどうかを返す。接続が切れていれば、ライブラリの一覧に出さない。
func (l *DriveLibrary) Connected(ctx context.Context) (bool, error) {
	return l.drive.scope.Connected(ctx)
}

// Identity は、ファイル p を見分ける値として、Drive のファイルの ID を返す。パスから引いた結果を覚えていなければ、
// 時間を区切って引く。引けなければ、パスをそのまま返す (索引も再生位置も、引けるようになるまで使えない)。
func (l *DriveLibrary) Identity(p string) (string, string) {
	if id, ok := l.drive.cachedPath(l.folder, p); ok {
		return GoogleDriveName, id
	}
	ctx, cancel := context.WithTimeout(context.Background(), driveLookupTimeout)
	defer cancel()
	file, err := l.resolve(ctx, p)
	if err != nil {
		return l.folder.Library, p
	}

	return GoogleDriveName, file.ID
}

func (l *DriveLibrary) List(ctx context.Context, dir string) ([]*Entry, error) {
	dir, err := CleanPath(dir)
	if err != nil {
		return nil, err
	}
	folder, err := l.resolve(ctx, dir)
	if err != nil {
		return nil, err
	}
	if !folder.Folder() {
		return nil, fmt.Errorf("%w: %q is not a directory", ErrNotFound, dir)
	}
	listing, err := l.drive.dir(ctx, folder.ID, l.folder.DriveID)
	if err != nil {
		return nil, err
	}
	entries := make([]*Entry, 0, len(listing.children))
	for segment, child := range listing.segments {
		entries = append(entries, &Entry{
			Name: child.Name, Path: joinPath(dir, segment), Dir: child.Folder(), Size: child.Size, ModifiedAt: child.ModifiedTime,
		})
	}
	slices.SortStableFunc(entries, func(a, b *Entry) int {
		return cmpFirst(strings.Compare(a.Name, b.Name), strings.Compare(a.Path, b.Path))
	})

	return entries, nil
}

// Names は、パス p の段ごとの名前 (名前を置き換える前の、Drive の名前) を返す。パンくずに使う。
func (l *DriveLibrary) Names(ctx context.Context, p string) ([]string, error) {
	p, err := CleanPath(p)
	if err != nil {
		return nil, err
	}
	if p == "" {
		return []string{}, nil
	}
	parts := strings.Split(p, "/")
	names := make([]string, 0, len(parts))
	for i := range parts {
		file, err := l.resolve(ctx, strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, err
		}
		names = append(names, file.Name)
	}

	return names, nil
}

func (l *DriveLibrary) Stat(ctx context.Context, p string) (*Entry, error) {
	p, err := CleanPath(p)
	if err != nil {
		return nil, err
	}
	file, err := l.resolve(ctx, p)
	if err != nil {
		return nil, err
	}
	if p == "" {
		return &Entry{Name: "", Path: "", Dir: true, Size: 0, ModifiedAt: file.ModifiedTime}, nil
	}

	return &Entry{Name: file.Name, Path: p, Dir: file.Folder(), Size: file.Size, ModifiedAt: file.ModifiedTime}, nil
}

func (l *DriveLibrary) Open(ctx context.Context, p string) (File, error) {
	p, err := CleanPath(p)
	if err != nil {
		return nil, err
	}
	file, err := l.resolve(ctx, p)
	if err != nil {
		return nil, err
	}
	if p == "" || file.Folder() {
		return nil, fmt.Errorf("%w: %q is not a file", ErrNotFound, p)
	}
	entry := &Entry{Name: file.Name, Path: p, Dir: false, Size: file.Size, ModifiedAt: file.ModifiedTime}
	id := file.ID
	api := l.drive.api

	return &driveFile{
		// Open に渡された ctx で、閉じるまでの読み出しを取りやめられるようにする。
		download: func(off int64) (io.ReadCloser, error) { return api.Download(ctx, id, off) },
		canceled: func() bool { return ctx.Err() != nil },
		id:       id, entry: entry, mu: sync.Mutex{}, pos: 0, body: nil, bodyPos: 0, closed: false,
	}, nil
}

// Search は、ディレクトリ dir の下から、名前に query を含む再生できるファイルを、limit 件まで返す。Drive の q では、
// あるフォルダの子孫だけには絞れないので、探す範囲を、選んだフォルダのある共有ドライブ (またはマイドライブ) に絞って名前で探し、
// 見つけたファイルの親をたどって、dir の下にあるものだけを残す。2 つ目の戻り値は、上限で打ち切ったかどうか。
func (l *DriveLibrary) Search(ctx context.Context, dir, query string, limit int) ([]*Entry, bool, error) {
	dir, err := CleanPath(dir)
	if err != nil {
		return nil, false, err
	}
	anchor, err := l.resolve(ctx, dir)
	if err != nil {
		return nil, false, err
	}
	files, truncated, err := l.drive.api.SearchFiles(ctx, query, l.folder.DriveID, driveSearchPages)
	if err != nil {
		return nil, false, driveError(err)
	}
	entries := []*Entry{}
	for _, file := range files {
		if !visible(file) || !Playable(file.Name) {
			continue
		}
		l.drive.remember(file)
		rest, err := l.pathUnder(ctx, anchor.ID, file)
		if errors.Is(err, errOutsideFolder) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		if len(entries) >= limit {
			return entries, true, nil
		}
		entries = append(entries, &Entry{Name: file.Name, Path: joinPath(dir, rest), Dir: false, Size: file.Size, ModifiedAt: file.ModifiedTime})
	}
	slices.SortStableFunc(entries, func(a, b *Entry) int {
		return cmpFirst(strings.Compare(a.Name, b.Name), strings.Compare(a.Path, b.Path))
	})

	return entries, truncated, nil
}

// pathUnder は、ファイル file がフォルダ anchor の下にあれば、anchor からのパスを返す。親をたどり、段ごとに、親のフォルダの中身から
// 段の名前を決める。下になければ errOutsideFolder を返す。親が複数あるファイル (古い Drive のもの) は、最初の親だけをたどる。
func (l *DriveLibrary) pathUnder(ctx context.Context, anchor string, file *googledrive.File) (string, error) {
	segments := []string{}
	current := file
	for range maxDriveDepth {
		if current.ID == anchor {
			slices.Reverse(segments)

			return strings.Join(segments, "/"), nil
		}
		if len(current.Parents) == 0 {
			break
		}
		listing, err := l.drive.dir(ctx, current.Parents[0], l.folder.DriveID)
		if err != nil {
			return "", err
		}
		segment, ok := listing.segmentOf(current.ID)
		if !ok {
			// 隠しファイルの下などで、ライブラリに出していないもの。
			break
		}
		segments = append(segments, segment)
		parent, err := l.drive.get(ctx, current.Parents[0])
		if errors.Is(err, ErrNotFound) && !errors.Is(err, ErrNotConnected) {
			break
		}
		if err != nil {
			return "", err
		}
		current = parent
	}

	return "", errOutsideFolder
}

// resolve は、ライブラリの中のパス p (空なら選んだフォルダ) のファイルかフォルダを返す。選んだフォルダから、段の名前で子をたどる。
func (l *DriveLibrary) resolve(ctx context.Context, p string) (*googledrive.File, error) {
	connected, err := l.drive.scope.Connected(ctx)
	if err != nil {
		return nil, err
	}
	if !connected {
		return nil, ErrNotConnected
	}
	if id, ok := l.drive.cachedPath(l.folder, p); ok {
		return l.drive.get(ctx, id)
	}
	current, err := l.drive.get(ctx, l.folder.ID)
	if err != nil {
		return nil, err
	}
	if p != "" {
		for segment := range strings.SplitSeq(p, "/") {
			if !current.Folder() {
				return nil, errOutsideFolder
			}
			listing, err := l.drive.dir(ctx, current.ID, l.folder.DriveID)
			if err != nil {
				return nil, err
			}
			child, ok := listing.segments[segment]
			if !ok {
				return nil, fmt.Errorf("%w: %q", ErrNotFound, p)
			}
			current = child
		}
	}
	l.drive.rememberPath(l.folder, p, current.ID)

	return current, nil
}

// segmentOf は、フォルダの中身のうち、ID が id の子の段の名前を返す。
func (d *driveDir) segmentOf(id string) (string, bool) {
	for segment, child := range d.segments {
		if child.ID == id {
			return segment, true
		}
	}

	return "", false
}

// driveSegments は、フォルダの中身から、段の名前と子の対応を作る。ふだんは名前 (/ と \ を置き換えたもの) にし、同じ名前が並ぶときだけ、
// 名前の後ろに ~ と、並んだもの同士で重ならない長さの ID の頭を付ける。
func driveSegments(children []*googledrive.File) map[string]*googledrive.File {
	groups := map[string][]*googledrive.File{}
	for _, child := range children {
		name := segmentName(child.Name)
		groups[name] = append(groups[name], child)
	}
	segments := make(map[string]*googledrive.File, len(children))
	for name, group := range groups {
		if len(group) == 1 {
			segments[name] = group[0]

			continue
		}
		size := minDriveIDPrefix
		for ; ; size++ {
			seen := map[string]bool{}
			unique := true
			for _, child := range group {
				prefix := child.ID[:min(size, len(child.ID))]
				unique = unique && !seen[prefix]
				seen[prefix] = true
			}
			if unique || slices.ContainsFunc(group, func(c *googledrive.File) bool { return size >= len(c.ID) }) {
				break
			}
		}
		for _, child := range group {
			segments[name+driveIDSeparator+child.ID[:min(size, len(child.ID))]] = child
		}
	}

	return segments
}

// segmentName は、Drive の名前を、パスの段に置ける形にする。/ と \ はパスの区切りとして読まれるので、
// 形の似た文字 (∕ U+2215 と ⧵ U+29F5) に置き換える。
func segmentName(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, "/", "\u2215"), `\`, "\u29f5")
}

// visible は、ライブラリに出すものかどうかを返す。Google ドキュメントなどと、名前が . で始まるもの
// (ローカルのライブラリの隠しファイルにそろえる) は出さない。
func visible(file *googledrive.File) bool {
	return (file.Folder() || file.Blob()) && !strings.HasPrefix(file.Name, ".")
}

// dir は、フォルダ id の中身を返す。少し前に引いたものがあれば、問い合わせずに返す。
func (d *Drive) dir(ctx context.Context, id, driveID string) (*driveDir, error) {
	d.mu.Lock()
	cached, ok := d.dirs[id]
	d.mu.Unlock()
	if ok && d.now().Sub(cached.fetchedAt) < driveStatTTL {
		return cached, nil
	}
	children, err := d.api.Children(ctx, id, driveID)
	if err != nil {
		return nil, driveError(err)
	}
	shown := make([]*googledrive.File, 0, len(children))
	for _, child := range children {
		if visible(child) {
			shown = append(shown, child)
			d.remember(child)
		}
	}
	listing := &driveDir{children: shown, segments: driveSegments(shown), fetchedAt: d.now()}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.dirs) >= maxDriveCache {
		clear(d.dirs)
	}
	d.dirs[id] = listing

	return listing, nil
}

// cachedPath は、ライブラリ folder のパス p から引いたファイルの ID を、覚えていれば返す。
func (d *Drive) cachedPath(folder *DriveFolder, p string) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cached, ok := d.paths[folder.ID+"\x00"+p]
	if !ok || d.now().Sub(cached.fetchedAt) >= drivePathTTL {
		return "", false
	}

	return cached.id, true
}

// rememberPath は、ライブラリ folder のパス p から引いたファイルの ID を覚える。
func (d *Drive) rememberPath(folder *DriveFolder, p, id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.paths) >= maxDriveCache {
		clear(d.paths)
	}
	d.paths[folder.ID+"\x00"+p] = &drivePath{id: id, fetchedAt: d.now()}
}

// get は、ファイル id の情報を返す。少し前に引いたものがあれば、問い合わせずに返す。
func (d *Drive) get(ctx context.Context, id string) (*googledrive.File, error) {
	d.mu.Lock()
	cached, ok := d.stats[id]
	d.mu.Unlock()
	if ok && d.now().Sub(cached.fetchedAt) < driveStatTTL {
		return cached.file, nil
	}
	file, err := d.api.Get(ctx, id)
	if err != nil {
		return nil, driveError(err)
	}
	d.remember(file)

	return file, nil
}

// remember は、引いたファイルの情報を覚える。上限を超えたら、まとめて捨てる。
func (d *Drive) remember(file *googledrive.File) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.stats) >= maxDriveCache {
		clear(d.stats)
	}
	d.stats[file.ID] = &driveStat{file: file, fetchedAt: d.now()}
}

// joinPath は、ディレクトリ dir と、その下のパス rest をつなぐ。
func joinPath(dir, rest string) string {
	switch {
	case dir == "":
		return rest
	case rest == "":
		return dir
	default:
		return dir + "/" + rest
	}
}

// cmpFirst は、最初の 0 でない比べた結果を返す。
func cmpFirst(results ...int) int {
	for _, r := range results {
		if r != 0 {
			return r
		}
	}

	return 0
}

// driveError は、見えないファイルを ErrNotFound で、接続が切れていてトークンを作れないことを ErrNotConnected で包む。
// 配信や画像の HTTP の口と、ジョブが、マウントしていないフォルダと同じく扱えるようにする。
func driveError(err error) error {
	if errors.Is(err, googledrive.ErrNotConnected) {
		return fmt.Errorf("%w: %w", ErrNotConnected, err)
	}
	if errors.Is(err, googledrive.ErrNotFound) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}

	return err
}

// driveFile は、Google Drive のファイルを、範囲を指定した読み出しで、途中から読めるようにしたもの。
//
// 1 回の読み出しごとに問い合わせると、細かく読む呼び出し (頭の解析など) で問い合わせが増えるので、終わりまでの範囲で開いた本文を
// 持ち続け、続きの位置を読むなら、その本文から読む。少し先へ飛んだときは、読み捨てて進める。途中で切れたら、読めた所から開き直す。
type driveFile struct {
	// download は、off から終わりまでを読む本文を開く。canceled は、Open に渡された ctx を取りやめたかどうかを返す。
	download func(off int64) (io.ReadCloser, error)
	canceled func() bool
	id       string
	entry    *Entry

	mu sync.Mutex
	// pos は、Read と Seek の位置。
	pos int64
	// body は、bodyPos から終わりまでを読む本文。開いていなければ nil。
	body    io.ReadCloser
	bodyPos int64
	closed  bool
}

var _ File = new(driveFile)

func (f *driveFile) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err := f.readAt(p, f.pos, false)
	f.pos += int64(n)

	return n, err
}

func (f *driveFile) ReadAt(p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.readAt(p, off, true)
}

func (f *driveFile) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += f.pos
	case io.SeekEnd:
		offset += f.entry.Size
	default:
		return 0, fmt.Errorf("invalid whence %d", whence)
	}
	if offset < 0 {
		return 0, fmt.Errorf("negative position %d", offset)
	}
	f.pos = offset

	return offset, nil
}

func (f *driveFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	f.closeBody()

	return nil
}

func (f *driveFile) Stat() (fs.FileInfo, error) {
	return &driveFileInfo{entry: f.entry}, nil
}

// readAt は、off から p を読む。full なら (ReadAt)、p を埋めるか、エラーになるまで読む。
func (f *driveFile) readAt(p []byte, off int64, full bool) (int, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	if off >= f.entry.Size {
		return 0, io.EOF
	}
	want := p[:min(int64(len(p)), f.entry.Size-off)]
	read, reopened := 0, 0
	for read < len(want) {
		if err := f.seekBody(off + int64(read)); err != nil {
			return read, err
		}
		n, err := f.body.Read(want[read:])
		read += n
		f.bodyPos += int64(n)
		if err != nil {
			// 終わりより手前で切れたら、読めた所から開き直す。
			f.closeBody()
			if reopened >= driveReopenLimit || f.canceled() {
				return read, fmt.Errorf("failed to read %s from Google Drive: %w", f.id, err)
			}
			reopened++

			continue
		}
		if !full && n > 0 {
			break
		}
	}
	if read < len(p) && off+int64(read) >= f.entry.Size {
		return read, io.EOF
	}

	return read, nil
}

// seekBody は、本文を off から読める状態にする。今の本文の少し先なら読み捨てて進め、そうでなければ開き直す。
func (f *driveFile) seekBody(off int64) error {
	if f.body != nil && off > f.bodyPos && off-f.bodyPos <= driveSkipLimit {
		skipped, err := io.CopyN(io.Discard, f.body, off-f.bodyPos)
		f.bodyPos += skipped
		if err != nil {
			f.closeBody()
		}
	}
	if f.body != nil && f.bodyPos == off {
		return nil
	}
	f.closeBody()
	body, err := f.download(off)
	if err != nil {
		return driveError(err)
	}
	f.body, f.bodyPos = body, off

	return nil
}

func (f *driveFile) closeBody() {
	if f.body != nil {
		_ = f.body.Close()
		f.body = nil
	}
}

// driveFileInfo は、Drive のファイルの fs.FileInfo。
type driveFileInfo struct {
	entry *Entry
}

func (i *driveFileInfo) Name() string       { return i.entry.Name }
func (i *driveFileInfo) Size() int64        { return i.entry.Size }
func (i *driveFileInfo) Mode() fs.FileMode  { return 0o444 }
func (i *driveFileInfo) ModTime() time.Time { return i.entry.ModifiedAt }
func (i *driveFileInfo) IsDir() bool        { return false }
func (i *driveFileInfo) Sys() any           { return nil }
