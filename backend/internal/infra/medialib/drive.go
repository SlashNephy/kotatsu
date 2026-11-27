package medialib

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/googledrive"
)

const (
	// GoogleDriveName は、Google Drive のファイルを見分ける値の名前空間。インデックスの保存先と再生位置の file_id は、ライブラリの名前でなく、
	// この名前と Drive のファイルの ID から作るので、ライブラリの名前を変えても、パスの形を変えても引き継がれる。
	GoogleDriveName = "google-drive"
	// KindGoogleDrive は、Google Drive のライブラリ (#1902)。管理者が選んだフォルダごとに 1 つある。
	KindGoogleDrive Kind = "google_drive"
	// driveStatTTL は、ファイルの情報とフォルダの内容をキャッシュしておく長さ。再生の画面は、インデックスを作り終えるまで数秒ごとに読み直すので、
	// そのたびに問い合わせない。
	driveStatTTL = 30 * time.Second
	// drivePathTTL は、パスからファイルの ID を引いた結果をキャッシュしておく長さ。
	drivePathTTL = 5 * time.Minute
	// maxDriveCache は、キャッシュしておくものの数の上限。超えたら、まとめて破棄する。
	maxDriveCache = 4096
	// maxDriveDepth は、親をたどる深さの上限。
	maxDriveDepth = 64
	// driveSearchPages は、名前で探すときに読むページの数の上限 (1 ページ 1000 件)。
	driveSearchPages = 5
	// driveTreeTTL は、名前で探すときに使う、ドライブのフォルダの一覧をキャッシュしておく長さ。一覧にないフォルダは 1 つずつ取得するので、
	// 新しく作ったフォルダは古い一覧でも見つかる。
	driveTreeTTL = 5 * time.Minute
	// driveNameBatch と driveNameQueryBytes は、同じ名前が並んでいるかを確認するときに、1 つの問い合わせに入れる名前の数と、
	// 名前を URL に入れたときの長さの合計の上限。q が長すぎると Drive API が 400 を返す (名前を URL に入れて 13 KB は通り、16 KB で拒否された)。
	driveNameBatch      = 30
	driveNameQueryBytes = 8 << 10
	// driveSkipLimit は、読み出しの位置が少し先へ飛んだときに、新しく問い合わせずに読み飛ばして進める長さの上限。
	driveSkipLimit = 4 << 20
	// driveReopenLimit は、読み出しが途中で切れたときに、読めた所から開き直す回数の上限。
	driveReopenLimit = 5
	// driveLookupTimeout は、ctx のない呼び出し (Set.Get) で、選んだフォルダを読み直すときの時間の上限。
	driveLookupTimeout = 10 * time.Second
	// driveIDSeparator は、同じフォルダに同じ名前が並ぶときに、名前の後ろに付ける、ID の先頭の前のマーク。
	driveIDSeparator = "~"
	// minDriveIDPrefix は、名前の後ろに付ける ID の先頭の、短い方の長さ。
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
	NamedChildren(ctx context.Context, names map[string][]string, driveID string) ([]*googledrive.File, error)
	Folders(ctx context.Context, driveID string) ([]*googledrive.File, error)
	SearchFiles(ctx context.Context, name, driveID string, maxPages int, page func([]*googledrive.File) bool) (bool, error)
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
// そのときだけ、名前の後ろに ~ と ID の先頭を付けて見分ける。名前の / と \ は、形の似た文字に置き換える。パスは、選んだフォルダから
// 名前をたどって引くので、選んだフォルダの外のファイルには届かない。ファイルの ID を見分ける値にし、インデックスと再生位置は ID で引き継ぐ。
type Drive struct {
	api   DriveAPI
	scope DriveScope
	now   func() time.Time

	mu    sync.Mutex
	stats map[string]*driveStat
	dirs  map[string]*driveDir
	paths map[string]*drivePath
	trees map[string]*driveTree
}

// driveTree は、ドライブ (共有ドライブか、マイドライブ) のフォルダの一覧。名前で探すときに、見つけたファイルの親をメモリの上でたどる。
type driveTree struct {
	folders   map[string]*googledrive.File
	fetchedAt time.Time
}

type driveStat struct {
	file      *googledrive.File
	fetchedAt time.Time
}

// driveDir は、フォルダの内容。セグメントの名前から子への対応も持つ。
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
		trees: map[string]*driveTree{},
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
// scope がキャッシュしたものを、時間を区切って読む。接続していなくても返し、読むときに ErrNotConnected を返す。
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

// Identity は、ファイル p を見分ける値として、Drive のファイルの ID を返す。パスから引いた結果をキャッシュしていなければ、
// 時間を区切って引く。引けなければ、パスをそのまま返す (インデックスも再生位置も、引けるようになるまで使えない)。
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

// Names は、パス p のセグメントごとの名前 (名前を置き換える前の、Drive の名前) を返す。パンくずに使う。
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
// 見つけたファイルの親を、ドライブのフォルダの一覧の上でたどって、dir の下にあるものだけを残す。返す分が見つかったら、残りのページは読まない。
// パスの階層の名前は、親のフォルダの内容をすべて読まず、同じ名前のものだけを問い合わせて決める。2 つ目の戻り値は、上限で打ち切ったかどうか。
func (l *DriveLibrary) Search(ctx context.Context, dir, query string, limit int) ([]*Entry, bool, error) {
	dir, err := CleanPath(dir)
	if err != nil {
		return nil, false, err
	}
	anchor, err := l.resolve(ctx, dir)
	if err != nil {
		return nil, false, err
	}
	tree, err := l.drive.tree(ctx, l.folder.DriveID)
	if err != nil {
		return nil, false, err
	}
	search := &driveSearch{
		library: l, anchor: anchor.ID, tree: tree, extra: map[string]*googledrive.File{}, segments: map[string]map[string]string{},
	}
	hits := []*driveHit{}
	more := false
	var chainErr error
	truncated, err := l.drive.api.SearchFiles(ctx, query, l.folder.DriveID, driveSearchPages, func(files []*googledrive.File) bool {
		for _, file := range files {
			if !visible(file) || !Playable(file.Name) {
				continue
			}
			chain, err := search.chain(ctx, file)
			if errors.Is(err, errOutsideFolder) {
				continue
			}
			if err != nil {
				chainErr = err

				return false
			}
			if len(hits) >= limit {
				more = true

				return false
			}
			hits = append(hits, &driveHit{file: file, chain: chain})
		}

		return true
	})
	if err != nil {
		return nil, false, driveError(err)
	}
	if chainErr != nil {
		return nil, false, chainErr
	}
	if err := search.name(ctx, hits); err != nil {
		return nil, false, err
	}
	entries := make([]*Entry, 0, len(hits))
	for _, hit := range hits {
		// 探す間に移したものなど、親のフォルダに見つからないものは返さない。
		rest, ok := search.path(hit)
		if !ok {
			continue
		}
		l.drive.remember(hit.file)
		entries = append(entries, &Entry{Name: hit.file.Name, Path: joinPath(dir, rest), Dir: false, Size: hit.file.Size, ModifiedAt: hit.file.ModifiedTime})
	}
	slices.SortStableFunc(entries, func(a, b *Entry) int {
		return cmpFirst(strings.Compare(a.Name, b.Name), strings.Compare(a.Path, b.Path))
	})

	return entries, truncated || more, nil
}

// driveSearch は、1 回の名前の検索で取得したフォルダと階層の名前。キャッシュの期限や上限に左右されないよう、検索の間はここに持つ。
type driveSearch struct {
	library *DriveLibrary
	anchor  string
	tree    *driveTree
	// extra は、フォルダの一覧になかったので 1 つずつ取得したフォルダ。見つからなかったものは nil にする。
	extra map[string]*googledrive.File
	// segments は、親のフォルダの ID から、子の ID と階層の名前への対応。
	segments map[string]map[string]string
}

// driveHit は、名前で見つけたファイルと、探したフォルダからファイルの親までのフォルダの ID。
type driveHit struct {
	file  *googledrive.File
	chain []string
}

// chain は、ファイル file の親を、探したフォルダまでたどり、探したフォルダからファイルの親までのフォルダの ID を返す。探したフォルダの
// 下になければ、errOutsideFolder を返す。途中に出さないフォルダ (名前が . で始まるもの) があっても返す。親が複数あるファイル
// (古い Drive のもの) は、最初の親だけをたどる。
func (s *driveSearch) chain(ctx context.Context, file *googledrive.File) ([]string, error) {
	chain := []string{}
	current := file
	for range maxDriveDepth {
		if len(current.Parents) == 0 {
			break
		}
		parentID := current.Parents[0]
		chain = append(chain, parentID)
		if parentID == s.anchor {
			slices.Reverse(chain)

			return chain, nil
		}
		parent, err := s.folder(ctx, parentID)
		if err != nil {
			return nil, err
		}
		if parent == nil || !visible(parent) {
			break
		}
		current = parent
	}

	return nil, errOutsideFolder
}

// folder は、フォルダ id を返す。フォルダの一覧になければ (ドライブの根元や、一覧を取得した後に作ったもの)、1 つずつ取得する。
// 見つからなければ nil を返す。
func (s *driveSearch) folder(ctx context.Context, id string) (*googledrive.File, error) {
	if folder, ok := s.tree.folders[id]; ok {
		return folder, nil
	}
	if folder, ok := s.extra[id]; ok {
		return folder, nil
	}
	folder, err := s.library.drive.get(ctx, id)
	if errors.Is(err, ErrNotFound) && !errors.Is(err, ErrNotConnected) {
		folder, err = nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.extra[id] = folder

	return folder, nil
}

// child は、ファイル hit の、探したフォルダから数えて i 番目の子 (フォルダかファイル) の ID と名前を返す。
func (s *driveSearch) child(hit *driveHit, i int) (string, string) {
	if i+1 < len(hit.chain) {
		id := hit.chain[i+1]
		if folder, ok := s.tree.folders[id]; ok {
			return id, folder.Name
		}
		if folder := s.extra[id]; folder != nil {
			return id, folder.Name
		}

		return id, ""
	}

	return hit.file.ID, hit.file.Name
}

// name は、ファイル hits の、探したフォルダから親までの階層の名前を決める。階層の名前は、一覧と同じく、親のフォルダの中に同じ名前が
// 並ぶときだけ ID の先頭を付けるので、親のフォルダの内容をすべては取得せず、親のフォルダの中の同じ名前のものだけをまとめて問い合わせる。
func (s *driveSearch) name(ctx context.Context, hits []*driveHit) error {
	wanted, err := s.wanted(ctx, hits)
	if err != nil {
		return err
	}
	for batch := range nameBatches(wanted) {
		found, err := s.library.drive.api.NamedChildren(ctx, batch, s.library.folder.DriveID)
		if err != nil {
			return driveError(err)
		}
		for parent, names := range batch {
			for _, name := range names {
				same := []*googledrive.File{}
				for _, f := range found {
					if visible(f) && slices.Contains(f.Parents, parent) && segmentName(f.Name) == segmentName(name) {
						same = append(same, f)
					}
				}
				s.remember(parent, driveSegments(same))
			}
		}
	}

	return nil
}

// wanted は、ファイル hits の、探したフォルダから親までの階層のうち、同じ名前のものを問い合わせて決めるものを、親のフォルダの ID から
// 名前への対応で返す。少し前に取得したフォルダの内容から決まるものと、名前に / か \ (と、置き換えた先の文字) を含むものは、
// ここで決める。
func (s *driveSearch) wanted(ctx context.Context, hits []*driveHit) (map[string][]string, error) {
	wanted := map[string][]string{}
	for _, hit := range hits {
		for i, parent := range hit.chain {
			id, name := s.child(hit, i)
			if _, ok := s.segments[parent][id]; ok {
				continue
			}
			if listing, ok := s.library.drive.cachedDir(parent); ok {
				s.remember(parent, listing.segments)

				continue
			}
			if strings.ContainsAny(name, "/\\\u2215\u29f5") {
				listing, err := s.library.drive.dir(ctx, parent, s.library.folder.DriveID)
				if err != nil {
					return nil, err
				}
				s.remember(parent, listing.segments)
				// 内容をすべて取得したので、同じフォルダの名前は問い合わせない。
				delete(wanted, parent)

				continue
			}
			if !slices.Contains(wanted[parent], name) {
				wanted[parent] = append(wanted[parent], name)
			}
		}
	}

	return wanted, nil
}

// remember は、親のフォルダ parent の中の、階層の名前から子への対応 segments を、子の ID から階層の名前への対応にして残す。
func (s *driveSearch) remember(parent string, segments map[string]*googledrive.File) {
	if s.segments[parent] == nil {
		s.segments[parent] = map[string]string{}
	}
	for segment, child := range segments {
		s.segments[parent][child.ID] = segment
	}
}

// path は、ファイル hit の、探したフォルダからのパスを返す。親のフォルダに見つからないもの (探す間に移したものなど) は、false を返す。
func (s *driveSearch) path(hit *driveHit) (string, bool) {
	segments := make([]string, 0, len(hit.chain))
	for i, parent := range hit.chain {
		id, _ := s.child(hit, i)
		segment, ok := s.segments[parent][id]
		if !ok {
			return "", false
		}
		segments = append(segments, segment)
	}

	return strings.Join(segments, "/"), true
}

// nameBatches は、親のフォルダの ID から名前への対応 names を、1 つの問い合わせに入る組に分けて返す。1 つの組には、名前を
// driveNameBatch 個まで、URL に入れたときの長さの合計で driveNameQueryBytes までを入れる。
func nameBatches(names map[string][]string) iter.Seq[map[string][]string] {
	return func(yield func(map[string][]string) bool) {
		parents := make([]string, 0, len(names))
		for parent := range names {
			parents = append(parents, parent)
		}
		slices.Sort(parents)
		batch, count, size := map[string][]string{}, 0, 0
		for _, parent := range parents {
			for _, name := range names[parent] {
				n := len(url.QueryEscape(name))
				if count > 0 && (count+1 > driveNameBatch || size+n > driveNameQueryBytes) {
					if !yield(batch) {
						return
					}
					batch, count, size = map[string][]string{}, 0, 0
				}
				batch[parent] = append(batch[parent], name)
				count++
				size += n
			}
		}
		if count > 0 {
			yield(batch)
		}
	}
}

// resolve は、ライブラリの中のパス p (空なら選んだフォルダ) のファイルかフォルダを返す。選んだフォルダから、セグメントの名前で子をたどる。
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

// driveSegments は、フォルダの内容から、セグメントの名前と子の対応を作る。ふだんは名前 (/ と \ を置き換えたもの) にし、同じ名前が並ぶときだけ、
// 名前の後ろに ~ と、並んだもの同士で重ならない長さの ID の先頭を付ける。
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

// segmentName は、Drive の名前を、パスのセグメントに置ける形にする。/ と \ はパスの区切りとして読まれるので、
// 形の似た文字 (∕ U+2215 と ⧵ U+29F5) に置き換える。
func segmentName(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, "/", "\u2215"), `\`, "\u29f5")
}

// visible は、ライブラリに出すものかどうかを返す。Google ドキュメントなどと、名前が . で始まるもの
// (ローカルのライブラリの隠しファイルにそろえる) は出さない。
func visible(file *googledrive.File) bool {
	return (file.Folder() || file.Blob()) && !strings.HasPrefix(file.Name, ".")
}

// dir は、フォルダ id の内容を返す。少し前に引いたものがあれば、問い合わせずに返す。
func (d *Drive) dir(ctx context.Context, id, driveID string) (*driveDir, error) {
	if cached, ok := d.cachedDir(id); ok {
		return cached, nil
	}
	children, err := d.api.Children(ctx, id, driveID)
	if err != nil {
		return nil, driveError(err)
	}

	return d.storeDir(id, children), nil
}

// cachedDir は、少し前に取得したフォルダ id の内容があれば返す。
func (d *Drive) cachedDir(id string) (*driveDir, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cached, ok := d.dirs[id]
	if !ok || d.now().Sub(cached.fetchedAt) >= driveStatTTL {
		return nil, false
	}

	return cached, true
}

// storeDir は、取得したフォルダ id の内容 children から、出すものだけを残した内容を作り、キャッシュして返す。
func (d *Drive) storeDir(id string, children []*googledrive.File) *driveDir {
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

	return listing
}

// tree は、ドライブ driveID (マイドライブなら空) のフォルダの一覧を返す。少し前に取得したものがあれば、問い合わせずに返す。
func (d *Drive) tree(ctx context.Context, driveID string) (*driveTree, error) {
	d.mu.Lock()
	cached, ok := d.trees[driveID]
	d.mu.Unlock()
	if ok && d.now().Sub(cached.fetchedAt) < driveTreeTTL {
		return cached, nil
	}
	folders, err := d.api.Folders(ctx, driveID)
	if err != nil {
		return nil, driveError(err)
	}
	tree := &driveTree{folders: make(map[string]*googledrive.File, len(folders)), fetchedAt: d.now()}
	for _, folder := range folders {
		tree.folders[folder.ID] = folder
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.trees[driveID] = tree

	return tree, nil
}

// cachedPath は、ライブラリ folder のパス p から引いたファイルの ID を、キャッシュしていれば返す。
func (d *Drive) cachedPath(folder *DriveFolder, p string) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cached, ok := d.paths[folder.ID+"\x00"+p]
	if !ok || d.now().Sub(cached.fetchedAt) >= drivePathTTL {
		return "", false
	}

	return cached.id, true
}

// rememberPath は、ライブラリ folder のパス p から引いたファイルの ID をキャッシュする。
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

// remember は、引いたファイルの情報をキャッシュする。上限を超えたら、まとめて破棄する。
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

// cmpFirst は、最初の 0 でない比較した結果を返す。
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
// 持ち続け、続きの位置を読むなら、その本文から読む。少し先へ飛んだときは、読み飛ばして進める。途中で切れたら、読めた所から開き直す。
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

// seekBody は、本文を off から読める状態にする。今の本文の少し先なら読み飛ばして進め、そうでなければ開き直す。
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
