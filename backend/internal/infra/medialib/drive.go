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
	// KindGoogleDrive は、Google Drive のライブラリ (#1902)。管理者が接続したときだけ見せる。
	KindGoogleDrive Kind = "google_drive"
	// MyDriveName は、マイドライブの根元の名前。画面は、この名前を訳して出す。
	MyDriveName = "My Drive"
	// driveStatTTL は、ファイルの情報を覚えておく長さ。再生の画面は、索引を作り終えるまで数秒ごとに読み直すので、そのたびに問い合わせない。
	driveStatTTL = 30 * time.Second
	// maxDriveStatCache は、覚えておくファイルの情報の数の上限。
	maxDriveStatCache = 1024
	// driveSkipLimit は、読み出しの位置が少し先へ飛んだときに、新しく問い合わせずに読み捨てて進める長さの上限。
	driveSkipLimit = 4 << 20
	// driveReopenLimit は、読み出しが途中で切れたときに、読めた所から開き直す回数の上限。
	driveReopenLimit = 5
)

// ErrNotConnected は、Google Drive を接続していないことを表す。ErrNotFound でもある。
var ErrNotConnected = fmt.Errorf("%w: the Google Drive is not connected", ErrNotFound)

// DriveAPI は、Google Drive のライブラリが使う Drive API。*googledrive.Client が満たす。
type DriveAPI interface {
	Drives(ctx context.Context) ([]*googledrive.Drive, error)
	GetDrive(ctx context.Context, id string) (*googledrive.Drive, error)
	Children(ctx context.Context, parentID, driveID string) ([]*googledrive.File, error)
	Get(ctx context.Context, id string) (*googledrive.File, error)
	Download(ctx context.Context, id string, offset int64) (io.ReadCloser, error)
}

var _ DriveAPI = new(googledrive.Client)

// Drive は、管理者が接続した Google Drive のライブラリ (設計書 5.8、#1902)。
//
// Drive のファイルの名前は重なることがあり、/ を含むこともあるので、パスは名前でなく ID を / でつないで表す。最初の段は、
// マイドライブ (root) か共有ドライブの ID で、続く段はフォルダの ID、最後の段はファイルの ID にする。ファイルは最後の段の ID だけで引き、
// 途中の段は、フォルダをたどった道筋として、パンくずと「上へ」に使う。
type Drive struct {
	api DriveAPI
	// connected は、管理者が Google Drive を接続しているかどうかを返す。
	connected func(ctx context.Context) (bool, error)
	now       func() time.Time

	mu    sync.Mutex
	stats map[string]*driveStat
}

type driveStat struct {
	file      *googledrive.File
	fetchedAt time.Time
}

var (
	_ Library    = new(Drive)
	_ Identifier = new(Drive)
	_ Namer      = new(Drive)
	_ Connector  = new(Drive)
)

// NewDrive は、Google Drive のライブラリを返す。connected は、管理者が接続しているかどうかを返す。
func NewDrive(api DriveAPI, connected func(ctx context.Context) (bool, error), now func() time.Time) *Drive {
	return &Drive{api: api, connected: connected, now: now, mu: sync.Mutex{}, stats: map[string]*driveStat{}}
}

func (d *Drive) Name() string { return GoogleDriveName }

func (d *Drive) Kind() Kind { return KindGoogleDrive }

// Connected は、管理者が Google Drive を接続しているかどうかを返す。接続していなければ、ライブラリの一覧に出さない。
func (d *Drive) Connected(ctx context.Context) (bool, error) {
	return d.connected(ctx)
}

// Identity は、ファイル p を見分ける値として、最後の段 (ファイルの ID) を返す。フォルダをたどった道筋が違っても、
// 同じファイルなら索引の置き場と再生位置を分けない。ファイルを別のフォルダへ移しても変わらない。
func (d *Drive) Identity(p string) string {
	return p[strings.LastIndex(p, "/")+1:]
}

func (d *Drive) List(ctx context.Context, dir string) ([]*Entry, error) {
	dir, err := CleanPath(dir)
	if err != nil {
		return nil, err
	}
	if err := d.requireConnected(ctx); err != nil {
		return nil, err
	}
	if dir == "" {
		return d.roots(ctx)
	}
	parts := strings.Split(dir, "/")
	driveID := parts[0]
	if driveID == googledrive.MyDriveID {
		driveID = ""
	}
	children, err := d.api.Children(ctx, parts[len(parts)-1], driveID)
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
	if err := d.requireConnected(ctx); err != nil {
		return nil, err
	}
	parts := strings.Split(p, "/")
	names := make([]string, 0, len(parts))
	for i, part := range parts {
		switch {
		case i == 0 && part == googledrive.MyDriveID:
			names = append(names, MyDriveName)
		case i == 0:
			drive, err := d.api.GetDrive(ctx, part)
			if err != nil {
				return nil, driveError(err)
			}
			names = append(names, drive.Name)
		default:
			file, err := d.get(ctx, part)
			if err != nil {
				return nil, err
			}
			names = append(names, file.Name)
		}
	}

	return names, nil
}

func (d *Drive) Stat(ctx context.Context, p string) (*Entry, error) {
	p, err := CleanPath(p)
	if err != nil {
		return nil, err
	}
	if err := d.requireConnected(ctx); err != nil {
		return nil, err
	}
	if p == "" || !strings.Contains(p, "/") {
		// 根元と、マイドライブと共有ドライブの根元は、フォルダとして返す。
		return &Entry{Name: p, Path: p, Dir: true, Size: 0, ModifiedAt: time.Time{}}, nil
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

// roots は、根元に、マイドライブと共有ドライブを、フォルダとして並べる。
func (d *Drive) roots(ctx context.Context) ([]*Entry, error) {
	drives, err := d.api.Drives(ctx)
	if err != nil {
		return nil, driveError(err)
	}
	shared := make([]*Entry, 0, len(drives))
	for _, drive := range drives {
		shared = append(shared, &Entry{Name: drive.Name, Path: drive.ID, Dir: true, Size: 0, ModifiedAt: drive.CreatedTime})
	}
	slices.SortStableFunc(shared, func(a, b *Entry) int { return strings.Compare(a.Name, b.Name) })
	entries := make([]*Entry, 0, 1+len(shared))
	entries = append(entries, &Entry{Name: MyDriveName, Path: googledrive.MyDriveID, Dir: true, Size: 0, ModifiedAt: time.Time{}})

	return append(entries, shared...), nil
}

func (d *Drive) requireConnected(ctx context.Context) error {
	connected, err := d.connected(ctx)
	if err != nil {
		return err
	}
	if !connected {
		return ErrNotConnected
	}

	return nil
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

// driveError は、見えないファイルを ErrNotFound で包む。
func driveError(err error) error {
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
