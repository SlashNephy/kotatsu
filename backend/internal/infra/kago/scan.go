package kago

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// lostAndFound は、ext4 などがファイルシステムの直下に作るディレクトリ。プールの中身ではないので調べない。
const lostAndFound = "lost+found"

// Entry は、プールを調べて見つけたファイルか、生成物のディレクトリ。
type Entry struct {
	// Path は、プール内の相対パス (/ 区切り)。
	Path string
	// SegmentDir は、データディレクトリの、セグメントのファイルの ID の生成物のディレクトリかどうか。そうなら、Size と ModifiedAt は中のファイルから求める。
	SegmentDir bool
	// Size は、ファイルの大きさ (バイト)。生成物のディレクトリでは、中のファイルの合計。
	Size int64
	// ModifiedAt は、ファイルを最後に書いた日時。生成物のディレクトリでは、中のファイルのうち最も新しいもの。
	ModifiedAt time.Time
	// ChangedAt は、書いたか、名前を変えたかで、ファイルを最後に変えた日時。生成物のディレクトリでは、中のものも含めて最も新しいもの。
	// 名前を変えても更新日時は変わらないので、移したばかりのファイルを見分けるのに使う。
	ChangedAt time.Time
}

// newEntry は、root の中のファイル rel の Entry を返す。
func newEntry(root *os.Root, rel string, info fs.FileInfo) *Entry {
	return &Entry{Path: rel, SegmentDir: false, Size: info.Size(), ModifiedAt: info.ModTime(), ChangedAt: latest(info.ModTime(), changeTime(root, rel, info))}
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}

	return a
}

// ScanPoolIDs は、Scan で調べるプールの ID を返す。設定の順のプールの後に、データディレクトリを続ける。
func (s *Store) ScanPoolIDs() []string {
	ids := s.PoolIDs()
	if s.dataDir != "" {
		ids = append(ids, DataPoolID)
	}

	return ids
}

// Scan は、プールの中のファイルと、生成物のディレクトリを返す。生成物のディレクトリの中には入らず、1 つとして返す。
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
		case d.IsDir() && IsSegmentDir(poolID, rel):
			entry, err := segmentDirEntry(root, rel)
			// 中にディレクトリのあるものは、生成物のディレクトリとしては消せないので、返さない。
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
			entries = append(entries, newEntry(root, rel, info))
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan pool %s: %w", poolID, err)
	}

	return entries, nil
}

// segmentDirEntry は、生成物のディレクトリ dir の中のファイルの大きさの合計と、最も新しい日時を返す。
// 中にディレクトリがあれば、生成物のディレクトリではない (removeSegmentDir で消せない) ので、ErrInvalidPath を返す。
func segmentDirEntry(root *os.Root, dir string) (*Entry, error) {
	entry := &Entry{Path: dir, SegmentDir: true, Size: 0, ModifiedAt: time.Time{}, ChangedAt: time.Time{}}
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
		entry.ChangedAt = latest(entry.ChangedAt, latest(info.ModTime(), changeTime(root, rel, info)))

		return nil
	})
	if err != nil {
		return nil, err
	}

	return entry, nil
}

// IsSegmentDir は、プール poolID の相対パス rel が、生成物のディレクトリかどうかを返す。
// 生成物のディレクトリは、データディレクトリ (DataPoolID) の直下の、セグメントのファイルの ID の名前のもの。
func IsSegmentDir(poolID, rel string) bool {
	_, err := SegmentDirOwner(poolID, rel)

	return err == nil
}

// SegmentDirOwner は、生成物のディレクトリ (プール poolID の dir) を持つセグメントのファイルの ID を返す。
func SegmentDirOwner(poolID, dir string) (uuid.UUID, error) {
	id, err := uuid.Parse(dir)
	if poolID != DataPoolID || err != nil || id.String() != dir {
		return uuid.Nil, fmt.Errorf("%w: %q is not a segment directory", ErrInvalidPath, dir)
	}

	return id, nil
}

// RemoveEntry は、Scan が返したファイルか生成物のディレクトリを消し、空になったディレクトリをプールの直下まで遡って消す。
// DB にないファイルの ID は分からないので、ファイルを消すときに、生成物のディレクトリは消さない。
func (s *Store) RemoveEntry(poolID, rel string, segmentDir bool) error {
	if _, err := s.Path(poolID, rel); err != nil {
		return err
	}
	if !segmentDir {
		return s.Remove(poolID, rel, uuid.Nil)
	}
	if !IsSegmentDir(poolID, rel) {
		return fmt.Errorf("%w: %q is not a segment directory", ErrInvalidPath, rel)
	}
	if err := s.removeSegmentDir(poolID, rel); err != nil {
		return fmt.Errorf("failed to remove the segment directory in pool %s: %w", poolID, err)
	}
	for dir := filepath.Dir(rel); dir != "." && removedEmptyDir(filepath.Join(s.mounts[poolID], dir)); {
		dir = filepath.Dir(dir)
	}

	return nil
}

// Inspect は、Scan が返すものと同じ形で、プールのファイルか生成物のディレクトリ rel を調べ直す。
// Scan が返さないもの (隠しファイル、lost+found、普通のファイルでないもの、segmentDir と種類の合わないもの) なら ErrInvalidPath を返す。
// なければ os.ErrNotExist を包んで返す。
func (s *Store) Inspect(poolID, rel string, segmentDir bool) (*Entry, error) {
	if _, err := s.Path(poolID, rel); err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	// Scan が返すものだけを受け付ける。隠しファイルと、生成物のディレクトリの中は返さない。
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ".") {
			return nil, fmt.Errorf("%w: %q is hidden", ErrInvalidPath, rel)
		}
	}
	if len(parts) > 1 && IsSegmentDir(poolID, parts[0]) {
		return nil, fmt.Errorf("%w: %q is in a segment directory", ErrInvalidPath, rel)
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
	case segmentDir && info.IsDir() && IsSegmentDir(poolID, rel):
		return segmentDirEntry(root, rel)
	case !segmentDir && info.Mode().IsRegular():
		return newEntry(root, rel, info), nil
	default:
		return nil, fmt.Errorf("%w: %q is not a file of the expected kind", ErrInvalidPath, rel)
	}
}
