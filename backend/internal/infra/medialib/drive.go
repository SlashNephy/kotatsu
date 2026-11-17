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
	// GoogleDriveName は、Google Drive のライブラリの名前。同じ名前のプールかフォルダがあれば、そちらを出し、Google Drive は出さない。
	GoogleDriveName = "google-drive"
	// KindGoogleDrive は、Google Drive のライブラリ (#1902)。管理者が接続し、見せるフォルダを選んだときだけ見せる。
	KindGoogleDrive Kind = "google_drive"
	// driveStatTTL は、ファイルの情報を覚えておく長さ。再生の画面は、索引を作り終えるまで数秒ごとに読み直すので、そのたびに問い合わせない。
	driveStatTTL = 30 * time.Second
	// maxDriveStatCache は、覚えておくファイルの情報の数の上限。
	maxDriveStatCache = 1024
	// driveChainTTL は、ファイルが選んだフォルダの下にあるかを確かめた結果を覚えておく長さ。
	driveChainTTL = 5 * time.Minute
	// maxDriveChainCache は、覚えておく確かめた結果の数の上限。
	maxDriveChainCache = 4096
	// maxDriveDepth は、親をたどる深さの上限。
	maxDriveDepth = 64
	// driveSearchPages は、名前で探すときに読むページの数の上限 (1 ページ 1000 件)。
	driveSearchPages = 5
	// driveSkipLimit は、読み出しの位置が少し先へ飛んだときに、新しく問い合わせずに読み捨てて進める長さの上限。
	driveSkipLimit = 4 << 20
	// driveReopenLimit は、読み出しが途中で切れたときに、読めた所から開き直す回数の上限。
	driveReopenLimit = 5
)

var (
	// ErrNotConnected は、Google Drive を接続していないか、見せるフォルダを選んでいないことを表す。ErrNotFound でもある。
	ErrNotConnected = fmt.Errorf("%w: the Google Drive is not connected", ErrNotFound)
	// errOutsideFolders は、選んだフォルダの外のファイルであることを表す。ErrNotFound でもある。外にあるか、ないかは区別して見せない。
	errOutsideFolders = fmt.Errorf("%w: outside the selected folders", ErrNotFound)
)

// DriveAPI は、Google Drive のライブラリが使う Drive API。*googledrive.Client が満たす。
type DriveAPI interface {
	Children(ctx context.Context, parentID, driveID string) ([]*googledrive.File, error)
	SearchFiles(ctx context.Context, name, driveID string, maxPages int) ([]*googledrive.File, bool, error)
	Get(ctx context.Context, id string) (*googledrive.File, error)
	Download(ctx context.Context, id string, offset int64) (io.ReadCloser, error)
}

var _ DriveAPI = new(googledrive.Client)

// DriveFolder は、管理者が、ライブラリとして見せると選んだ Drive のフォルダ。
type DriveFolder struct {
	// ID は、フォルダの ID。
	ID string
	// DriveID は、フォルダのある共有ドライブの ID。マイドライブなら空。
	DriveID string
}

// DriveScope は、Google Drive の接続と、見せるフォルダ。
type DriveScope interface {
	// Connected は、管理者が Google Drive を接続していて、使えるかどうかを返す。
	Connected(ctx context.Context) (bool, error)
	// Folders は、管理者が選んだフォルダを、選んだ順に返す。
	Folders(ctx context.Context) ([]*DriveFolder, error)
}

// Drive は、管理者が接続した Google Drive のライブラリ (設計書 5.8、#1902)。
//
// 見せるのは、管理者が選んだフォルダ (共有ドライブの中のフォルダも可) の下だけにする。根元に選んだフォルダを並べる。
// Drive のファイルの名前は重なることがあり、/ を含むこともあるので、パスは名前でなく ID を / でつないで表す。最初の段は選んだフォルダの ID で、
// 続く段はフォルダの ID、最後の段はファイルの ID にする。パスは、選んだフォルダから親をたどった道筋と一致するものだけを受け付ける。
// ファイルの ID を直接渡されても、選んだフォルダの外のものは、一覧にも開く口にも出さない。
type Drive struct {
	api   DriveAPI
	scope DriveScope
	now   func() time.Time

	mu     sync.Mutex
	stats  map[string]*driveStat
	chains map[string]*driveChain
}

type driveStat struct {
	file      *googledrive.File
	fetchedAt time.Time
}

// driveChain は、あるファイルが、あるフォルダの下にあるかを確かめた結果。
type driveChain struct {
	// ids は、フォルダの直下からファイルまでの ID。下になければ nil。
	ids       []string
	checkedAt time.Time
}

var (
	_ Library    = new(Drive)
	_ Identifier = new(Drive)
	_ Namer      = new(Drive)
	_ Connector  = new(Drive)
	_ Searcher   = new(Drive)
)

// NewDrive は、Google Drive のライブラリを返す。scope は、接続と、管理者が選んだフォルダを返す。
func NewDrive(api DriveAPI, scope DriveScope, now func() time.Time) *Drive {
	return &Drive{api: api, scope: scope, now: now, mu: sync.Mutex{}, stats: map[string]*driveStat{}, chains: map[string]*driveChain{}}
}

func (d *Drive) Name() string { return GoogleDriveName }

func (d *Drive) Kind() Kind { return KindGoogleDrive }

// Connected は、管理者が Google Drive を接続し、見せるフォルダを選んだかどうかを返す。そうでなければ、ライブラリの一覧に出さない。
func (d *Drive) Connected(ctx context.Context) (bool, error) {
	connected, err := d.scope.Connected(ctx)
	if err != nil || !connected {
		return false, err
	}
	folders, err := d.scope.Folders(ctx)
	if err != nil {
		return false, err
	}

	return len(folders) > 0, nil
}

// Identity は、ファイル p を見分ける値として、最後の段 (ファイルの ID) を返す。ファイルを別のフォルダへ移しても、
// 索引の置き場と再生位置を分けない。
func (d *Drive) Identity(p string) string {
	return p[strings.LastIndex(p, "/")+1:]
}

func (d *Drive) List(ctx context.Context, dir string) ([]*Entry, error) {
	dir, err := CleanPath(dir)
	if err != nil {
		return nil, err
	}
	folders, err := d.folders(ctx)
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return d.roots(ctx, folders)
	}
	folder, parts, err := d.resolve(ctx, folders, dir)
	if err != nil {
		return nil, err
	}
	children, err := d.api.Children(ctx, parts[len(parts)-1], folder.DriveID)
	if err != nil {
		return nil, driveError(err)
	}
	entries := make([]*Entry, 0, len(children))
	for _, child := range children {
		// Google ドキュメントなどと、名前が . で始まるもの (ローカルのライブラリの隠しファイルにそろえる) は出さない。
		if (!child.Folder() && !child.Blob()) || strings.HasPrefix(child.Name, ".") {
			continue
		}
		d.remember(child)
		// 一覧に出したものは、選んだフォルダの下にあると分かっているので、確かめた結果として覚える。
		d.rememberChain(folder.ID, child.ID, append(slices.Clone(parts[1:]), child.ID))
		entries = append(entries, &Entry{
			Name: child.Name, Path: dir + "/" + child.ID, Dir: child.Folder(), Size: child.Size, ModifiedAt: child.ModifiedTime,
		})
	}
	slices.SortStableFunc(entries, func(a, b *Entry) int { return strings.Compare(a.Name, b.Name) })

	return entries, nil
}

// Names は、パス p の段ごとの表示名を返す。パンくずに使う。
func (d *Drive) Names(ctx context.Context, p string) ([]string, error) {
	p, err := CleanPath(p)
	if err != nil {
		return nil, err
	}
	if p == "" {
		return []string{}, nil
	}
	folders, err := d.folders(ctx)
	if err != nil {
		return nil, err
	}
	_, parts, err := d.resolve(ctx, folders, p)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		file, err := d.get(ctx, part)
		if err != nil {
			return nil, err
		}
		names = append(names, file.Name)
	}

	return names, nil
}

func (d *Drive) Stat(ctx context.Context, p string) (*Entry, error) {
	p, err := CleanPath(p)
	if err != nil {
		return nil, err
	}
	folders, err := d.folders(ctx)
	if err != nil {
		return nil, err
	}
	if p == "" {
		return &Entry{Name: "", Path: "", Dir: true, Size: 0, ModifiedAt: time.Time{}}, nil
	}
	if _, _, err := d.resolve(ctx, folders, p); err != nil {
		return nil, err
	}
	file, err := d.get(ctx, d.Identity(p))
	if err != nil {
		return nil, err
	}

	return &Entry{Name: file.Name, Path: p, Dir: file.Folder(), Size: file.Size, ModifiedAt: file.ModifiedTime}, nil
}

func (d *Drive) Open(ctx context.Context, p string) (File, error) {
	entry, err := d.Stat(ctx, p)
	if err != nil {
		return nil, err
	}
	if entry.Dir {
		return nil, fmt.Errorf("%w: %q is not a file", ErrNotFound, p)
	}

	id := d.Identity(p)

	return &driveFile{
		// Open に渡された ctx で、閉じるまでの読み出しを取りやめられるようにする。
		download: func(off int64) (io.ReadCloser, error) { return d.api.Download(ctx, id, off) },
		canceled: func() bool { return ctx.Err() != nil },
		id:       id, entry: entry, mu: sync.Mutex{}, pos: 0, body: nil, bodyPos: 0, closed: false,
	}, nil
}

// Search は、ディレクトリ dir (空なら選んだフォルダのすべて) の下から、名前に query を含む再生できるファイルを、limit 件まで返す。
// Drive の q では、あるフォルダの子孫だけには絞れないので、探す範囲を、選んだフォルダのある共有ドライブ (またはマイドライブ) に絞って
// 名前で探し、見つけたファイルの親をたどって、dir の下にあるものだけを残す。2 つ目の戻り値は、上限で打ち切ったかどうか。
func (d *Drive) Search(ctx context.Context, dir, query string, limit int) ([]*Entry, bool, error) {
	dir, err := CleanPath(dir)
	if err != nil {
		return nil, false, err
	}
	folders, err := d.folders(ctx)
	if err != nil {
		return nil, false, err
	}
	// anchors は、探す範囲の根元のフォルダの ID から、そのパスへの対応。
	anchors := map[string]string{}
	drives := []string{}
	if dir == "" {
		for _, folder := range folders {
			anchors[folder.ID] = folder.ID
			drives = append(drives, folder.DriveID)
		}
	} else {
		folder, parts, err := d.resolve(ctx, folders, dir)
		if err != nil {
			return nil, false, err
		}
		anchors[parts[len(parts)-1]] = dir
		drives = append(drives, folder.DriveID)
	}
	slices.Sort(drives)
	entries := []*Entry{}
	truncated := false
	for _, driveID := range slices.Compact(drives) {
		files, more, err := d.api.SearchFiles(ctx, query, driveID, driveSearchPages)
		if err != nil {
			return nil, false, driveError(err)
		}
		truncated = truncated || more
		for _, file := range files {
			if !file.Blob() || strings.HasPrefix(file.Name, ".") || !Playable(file.Name) {
				continue
			}
			d.remember(file)
			p, ok, err := d.locate(ctx, anchors, file.ID)
			if err != nil {
				return nil, false, err
			}
			if !ok {
				continue
			}
			if len(entries) >= limit {
				return entries, true, nil
			}
			entries = append(entries, &Entry{Name: file.Name, Path: p, Dir: false, Size: file.Size, ModifiedAt: file.ModifiedTime})
		}
	}
	slices.SortStableFunc(entries, func(a, b *Entry) int { return strings.Compare(a.Name, b.Name) })

	return entries, truncated, nil
}

// locate は、ファイル id が、anchors のどれかのフォルダの下にあれば、そのフォルダのパスからのパスを返す。
func (d *Drive) locate(ctx context.Context, anchors map[string]string, id string) (string, bool, error) {
	for anchor, base := range anchors {
		ids, err := d.chain(ctx, anchor, id)
		if errors.Is(err, errOutsideFolders) {
			continue
		}
		if err != nil {
			return "", false, err
		}

		return base + "/" + strings.Join(ids, "/"), true, nil
	}

	return "", false, nil
}

// roots は、根元に、選んだフォルダを、選んだ順に並べる。消されたか見えなくなったフォルダは出さない。
func (d *Drive) roots(ctx context.Context, folders []*DriveFolder) ([]*Entry, error) {
	entries := make([]*Entry, 0, len(folders))
	for _, folder := range folders {
		file, err := d.get(ctx, folder.ID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, &Entry{Name: file.Name, Path: folder.ID, Dir: true, Size: 0, ModifiedAt: file.ModifiedTime})
	}

	return entries, nil
}

// folders は、選んだフォルダを返す。接続していないか、フォルダを選んでいなければ ErrNotConnected を返す。
func (d *Drive) folders(ctx context.Context) ([]*DriveFolder, error) {
	connected, err := d.scope.Connected(ctx)
	if err != nil {
		return nil, err
	}
	if !connected {
		return nil, ErrNotConnected
	}
	folders, err := d.scope.Folders(ctx)
	if err != nil {
		return nil, err
	}
	if len(folders) == 0 {
		return nil, ErrNotConnected
	}

	return folders, nil
}

// resolve は、パス p の最初の段の選んだフォルダと、段を返す。最初の段が選んだフォルダでないか、続く段が、選んだフォルダから
// 親をたどった道筋と一致しなければ (選んだフォルダの外のファイルや、道筋を作り変えたパス)、ErrNotFound を返す。
func (d *Drive) resolve(ctx context.Context, folders []*DriveFolder, p string) (*DriveFolder, []string, error) {
	parts := strings.Split(p, "/")
	index := slices.IndexFunc(folders, func(f *DriveFolder) bool { return f.ID == parts[0] })
	if index < 0 {
		return nil, nil, errOutsideFolders
	}
	if len(parts) > 1 {
		ids, err := d.chain(ctx, parts[0], parts[len(parts)-1])
		if err != nil {
			return nil, nil, err
		}
		if !slices.Equal(ids, parts[1:]) {
			return nil, nil, errOutsideFolders
		}
	}

	return folders[index], parts, nil
}

// chain は、ファイル id がフォルダ anchor の下にあれば、anchor の直下から id までの ID を返す。下になければ errOutsideFolders を返す。
// 親をたどって確かめ、結果を driveChainTTL の間覚える。親が複数あるファイル (古い Drive のもの) は、最初の親だけをたどる。
func (d *Drive) chain(ctx context.Context, anchor, id string) ([]string, error) {
	key := anchor + "/" + id
	d.mu.Lock()
	cached, ok := d.chains[key]
	d.mu.Unlock()
	if ok && d.now().Sub(cached.checkedAt) < driveChainTTL {
		if cached.ids == nil {
			return nil, errOutsideFolders
		}

		return slices.Clone(cached.ids), nil
	}
	reversed := []string{}
	current := id
	for range maxDriveDepth {
		if current == anchor {
			slices.Reverse(reversed)
			d.rememberChain(anchor, id, reversed)

			return slices.Clone(reversed), nil
		}
		reversed = append(reversed, current)
		file, err := d.get(ctx, current)
		if errors.Is(err, ErrNotFound) && current != id {
			// 途中の親が見えなければ、選んだフォルダの下にない。
			break
		}
		if err != nil {
			return nil, err
		}
		if len(file.Parents) == 0 {
			break
		}
		current = file.Parents[0]
	}
	d.rememberChain(anchor, id, nil)

	return nil, errOutsideFolders
}

// rememberChain は、ファイル id がフォルダ anchor の下にあるかを確かめた結果を覚える。上限を超えたら、まとめて捨てる。
func (d *Drive) rememberChain(anchor, id string, ids []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.chains) >= maxDriveChainCache {
		clear(d.chains)
	}
	d.chains[anchor+"/"+id] = &driveChain{ids: ids, checkedAt: d.now()}
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

// remember は、引いたファイルの情報を覚える。上限を超えたら、古いものから捨てる。
func (d *Drive) remember(file *googledrive.File) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()
	if len(d.stats) >= maxDriveStatCache {
		for id, stat := range d.stats {
			if now.Sub(stat.fetchedAt) >= driveStatTTL {
				delete(d.stats, id)
			}
		}
		if len(d.stats) >= maxDriveStatCache {
			clear(d.stats)
		}
	}
	d.stats[file.ID] = &driveStat{file: file, fetchedAt: now}
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
