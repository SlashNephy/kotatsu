package kago

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// lostAndFound は、ext4 などがファイルシステムの直下に作るディレクトリ。プールの中身ではないので調べない。
const lostAndFound = "lost+found"

// Entry は、プールを調べて見つけたファイルか、付属ファイルのディレクトリ。
type Entry struct {
	// Path は、プール内の相対パス (/ 区切り)。
	Path string
	// Sidecar は、付属ファイルのディレクトリかどうか。そうなら、Size と ModifiedAt は中のファイルから求める。
	Sidecar bool
	// Size は、ファイルの大きさ (バイト)。付属ファイルのディレクトリでは、中のファイルの合計。
	Size int64
	// ModifiedAt は、ファイルを最後に書いた日時。付属ファイルのディレクトリでは、中のファイルのうち最も新しいもの。
	ModifiedAt time.Time
	// ChangedAt は、書いたか、名前を変えたかで、ファイルを最後に変えた日時。付属ファイルのディレクトリでは、中のものも含めて最も新しいもの。
	// 名前を変えても更新日時は変わらないので、移したばかりのファイルを見分けるのに使う。
	ChangedAt time.Time
}

// newEntry は、ファイル rel の Entry を返す。
func newEntry(rel string, info fs.FileInfo) *Entry {
	return &Entry{Path: rel, Sidecar: false, Size: info.Size(), ModifiedAt: info.ModTime(), ChangedAt: latest(info.ModTime(), changeTime(info))}
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}

	return a
}

// ScanPoolIDs は、Scan で調べるプールの ID を返す。設定の順のプールの後に、付属ファイルをまとめて置くディレクトリを続ける。
func (s *Store) ScanPoolIDs() []string {
	ids := s.PoolIDs()
	if s.sidecarDir != "" {
		ids = append(ids, SidecarPoolID)
	}

	return ids
}

// Scan は、プールの中のファイルと、付属ファイルのディレクトリを返す。付属ファイルのディレクトリの中には入らず、1 つとして返す。
// 隠しファイル (. で始まる名前) と lost+found、シンボリックリンクのような普通のファイルでないものは返さない。
// プールがこのインスタンスにマウントされていなければ ErrPoolNotMounted を返す。
func (s *Store) Scan(poolID string) ([]*Entry, error) {
	mount, ok := s.mounts[poolID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPoolNotMounted, poolID)
	}
	root, err := os.OpenRoot(mount)
	if err != nil {
		return nil, fmt.Errorf("failed to open pool %s: %w", poolID, err)
	}
	defer root.Close()

	entries := []*Entry{}
	err = fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || rel == lostAndFound {
			if d.IsDir() {
				return fs.SkipDir
			}

			return nil
		}
		switch {
		case d.IsDir() && IsSidecarDir(rel):
			entry, err := sidecarEntry(root, rel)
			// 中にディレクトリのあるものは、付属ファイルのディレクトリとしては消せないので、返さない。
			if errors.Is(err, ErrInvalidPath) {
				return fs.SkipDir
			}
			if err != nil {
				return err
			}
			entries = append(entries, entry)

			return fs.SkipDir
		case d.Type().IsRegular():
			info, err := d.Info()
			if err != nil {
				return err
			}
			entries = append(entries, newEntry(rel, info))
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan pool %s: %w", poolID, err)
	}

	return entries, nil
}

// sidecarEntry は、付属ファイルのディレクトリ dir の中のファイルの大きさの合計と、最も新しい日時を返す。
// 中にディレクトリがあれば、付属ファイルのディレクトリではない (removeSidecarDir で消せない) ので、ErrInvalidPath を返す。
func sidecarEntry(root *os.Root, dir string) (*Entry, error) {
	entry := &Entry{Path: dir, Sidecar: true, Size: 0, ModifiedAt: time.Time{}, ChangedAt: time.Time{}}
	err := fs.WalkDir(root.FS(), dir, func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && rel != dir {
			return fmt.Errorf("%w: %q contains a directory", ErrInvalidPath, dir)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			entry.Size += info.Size()
		}
		entry.ModifiedAt = latest(entry.ModifiedAt, info.ModTime())
		entry.ChangedAt = latest(entry.ChangedAt, latest(info.ModTime(), changeTime(info)))

		return nil
	})
	if err != nil {
		return nil, err
	}

	return entry, nil
}

// IsSidecarDir は、プール内の相対パス rel が、付属ファイルのディレクトリの名前かどうかを返す。
func IsSidecarDir(rel string) bool {
	return strings.HasSuffix(rel, sidecarSuffix) && len(path.Base(rel)) > len(sidecarSuffix)
}

// SidecarOwner は、付属ファイルのディレクトリ (プール poolID の dir) を持つ録画ファイルの場所を返す。
// 録画ファイルの横に置いたものと、まとめて置くディレクトリ (SidecarPoolID) の中の「プール ID/録画ファイルのパス.kotatsu」の、どちらも扱う。
func SidecarOwner(poolID, dir string) (*Location, error) {
	if !IsSidecarDir(dir) {
		return nil, fmt.Errorf("%w: %q is not a sidecar directory", ErrInvalidPath, dir)
	}
	rel := strings.TrimSuffix(dir, sidecarSuffix)
	if poolID != SidecarPoolID {
		return &Location{PoolID: poolID, Path: rel}, nil
	}
	owner, rest, ok := strings.Cut(rel, "/")
	if !ok || owner == "" || rest == "" {
		return nil, fmt.Errorf("%w: %q has no pool in the sidecar directory", ErrInvalidPath, dir)
	}

	return &Location{PoolID: owner, Path: rest}, nil
}

// RemoveEntry は、Scan が返したファイルか付属ファイルのディレクトリを消し、空になったディレクトリをプールの直下まで遡って消す。
// ファイルなら、Remove と同じく、その付属ファイルのディレクトリも消す。
func (s *Store) RemoveEntry(poolID, rel string, sidecar bool) error {
	if _, err := s.Path(poolID, rel); err != nil {
		return err
	}
	if !sidecar {
		return s.Remove(poolID, rel)
	}
	if !IsSidecarDir(rel) {
		return fmt.Errorf("%w: %q is not a sidecar directory", ErrInvalidPath, rel)
	}
	if err := s.removeSidecarDir(poolID, rel); err != nil {
		return fmt.Errorf("failed to remove the sidecar directory in pool %s: %w", poolID, err)
	}
	for dir := filepath.Dir(rel); dir != "." && removedEmptyDir(filepath.Join(s.mounts[poolID], dir)); {
		dir = filepath.Dir(dir)
	}

	return nil
}

// Inspect は、Scan が返すものと同じ形で、プールのファイルか付属ファイルのディレクトリ rel を調べ直す。
// Scan が返さないもの (隠しファイル、lost+found、普通のファイルでないもの、sidecar と種類の合わないもの) なら ErrInvalidPath を返す。
// なければ os.ErrNotExist を包んで返す。
func (s *Store) Inspect(poolID, rel string, sidecar bool) (*Entry, error) {
	if _, err := s.Path(poolID, rel); err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	// Scan が返すものだけを受け付ける。隠しファイルと、付属ファイルのディレクトリの中は返さない。
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ".") {
			return nil, fmt.Errorf("%w: %q is hidden", ErrInvalidPath, rel)
		}
		if i < len(parts)-1 && IsSidecarDir(part) {
			return nil, fmt.Errorf("%w: %q is in a sidecar directory", ErrInvalidPath, rel)
		}
	}
	if rel == lostAndFound || strings.HasPrefix(rel, lostAndFound+"/") {
		return nil, fmt.Errorf("%w: %q is in %s", ErrInvalidPath, rel, lostAndFound)
	}
	root, err := os.OpenRoot(s.mounts[poolID])
	if err != nil {
		return nil, fmt.Errorf("failed to open pool %s: %w", poolID, err)
	}
	defer root.Close()
	info, err := root.Lstat(rel)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect a file in pool %s: %w", poolID, err)
	}
	switch {
	case sidecar && info.IsDir() && IsSidecarDir(rel):
		return sidecarEntry(root, rel)
	case !sidecar && info.Mode().IsRegular():
		return newEntry(rel, info), nil
	default:
		return nil, fmt.Errorf("%w: %q is not a file of the expected kind", ErrInvalidPath, rel)
	}
}
