package medialib

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
)

// lostAndFound は、ext4 などがファイルシステムの直下に作るディレクトリ。ライブラリの内容ではないので出さない。
const lostAndFound = "lost+found"

// Local は、ローカルのディレクトリのライブラリ。os.Root で開くので、シンボリックリンクでもディレクトリの外は読まない。
type Local struct {
	name string
	dir  string
	kind Kind
}

var (
	_ Library  = new(Local)
	_ Searcher = new(Local)
)

// maxSearchVisits は、ローカルのライブラリで名前で探すときに、たどるファイルとディレクトリの数の上限。
// 大きなフォルダを根元から探しても、ディスクを長く奪わないようにする。
const maxSearchVisits = 50000

// NewLocal は、ディレクトリ dir を、名前 name のライブラリとして返す。
func NewLocal(name, dir string, kind Kind) *Local {
	return &Local{name: name, dir: dir, kind: kind}
}

func (l *Local) Name() string { return l.name }

func (l *Local) Kind() Kind { return l.kind }

func (l *Local) List(_ context.Context, dir string) ([]*Entry, error) {
	dir, err := CleanPath(dir)
	if err != nil {
		return nil, err
	}
	root, err := l.openRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(relName(dir))
	if err != nil {
		return nil, notFound(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect %q in library %s: %w", dir, l.name, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %q is not a directory", ErrNotFound, dir)
	}
	children, err := f.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("failed to list %q in library %s: %w", dir, l.name, err)
	}
	entries := make([]*Entry, 0, len(children))
	for _, child := range children {
		name := child.Name()
		if strings.HasPrefix(name, ".") || (dir == "" && name == lostAndFound) {
			continue
		}
		rel := path.Join(dir, name)
		// シンボリックリンクは、指す先で決める。ライブラリの外を指すもの (os.Root が拒む) と、指す先のないものは出さない。
		info, err := root.Stat(rel)
		if err != nil {
			continue
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		entries = append(entries, newEntry(rel, info))
	}
	slices.SortFunc(entries, func(a, b *Entry) int { return strings.Compare(a.Name, b.Name) })

	return entries, nil
}

func (l *Local) Stat(_ context.Context, p string) (*Entry, error) {
	p, err := CleanPath(p)
	if err != nil {
		return nil, err
	}
	root, err := l.openRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Stat(relName(p))
	if err != nil {
		return nil, notFound(err)
	}

	return newEntry(p, info), nil
}

func (l *Local) Open(_ context.Context, p string) (File, error) {
	p, err := CleanPath(p)
	if err != nil {
		return nil, err
	}
	if p == "" {
		return nil, fmt.Errorf("%w: the root is not a file", ErrInvalidPath)
	}
	f, err := os.OpenInRoot(l.dir, p)
	if err != nil {
		return nil, notFound(err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()

		return nil, fmt.Errorf("failed to inspect %q in library %s: %w", p, l.name, err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()

		return nil, fmt.Errorf("%w: %q is not a file", ErrNotFound, p)
	}

	return f, nil
}

// Search は、ディレクトリ dir の下を深さ優先でたどり、名前に query を (大文字と小文字を区別せずに) 含む再生できるファイルを、
// 名前の順に limit 件まで返す。隠しファイルとシンボリックリンクはたどらない。maxSearchVisits 個たどったら打ち切る。
func (l *Local) Search(ctx context.Context, dir, query string, limit int) ([]*Entry, bool, error) {
	dir, err := CleanPath(dir)
	if err != nil {
		return nil, false, err
	}
	root, err := l.openRoot()
	if err != nil {
		return nil, false, err
	}
	defer root.Close()
	if info, err := root.Stat(relName(dir)); err != nil || !info.IsDir() {
		return nil, false, fmt.Errorf("%w: %q is not a directory", ErrNotFound, dir)
	}
	needle := strings.ToLower(query)
	entries := []*Entry{}
	truncated := false
	visits := 0
	err = fs.WalkDir(root.FS(), relName(dir), func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			// 読めないディレクトリは飛ばす。
			return fs.SkipDir
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		visits++
		if visits > maxSearchVisits || len(entries) >= limit {
			truncated = true

			return fs.SkipAll
		}
		name := entry.Name()
		if p != relName(dir) && (strings.HasPrefix(name, ".") || (path.Dir(p) == "." && name == lostAndFound)) {
			if entry.IsDir() {
				return fs.SkipDir
			}

			return nil
		}
		if !entry.Type().IsRegular() || !Playable(name) || !strings.Contains(strings.ToLower(name), needle) {
			return nil
		}
		// 探している間に消えたファイルは飛ばす。
		if info, infoErr := entry.Info(); infoErr == nil {
			entries = append(entries, newEntry(p, info))
		}

		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("failed to search %q in library %s: %w", dir, l.name, err)
	}
	slices.SortStableFunc(entries, func(a, b *Entry) int { return strings.Compare(a.Name, b.Name) })

	return entries, truncated, nil
}

// openRoot は、ライブラリのディレクトリを開く。マウントされていないなどでなければ、ErrNotFound を包んで返す。
func (l *Local) openRoot() (*os.Root, error) {
	root, err := os.OpenRoot(l.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to open library %s: %w", l.name, notFound(err))
	}

	return root, nil
}

// relName は、根元 ("") を os.Root の "." にする。
func relName(p string) string {
	if p == "" {
		return "."
	}

	return p
}

// notFound は、ないことを表すエラーと、ライブラリの外を指すエラーを ErrNotFound で包む。外を指すことは、利用者に区別して見せない。
func notFound(err error) error {
	var pathErr *fs.PathError
	if errors.Is(err, fs.ErrNotExist) || (errors.As(err, &pathErr) && strings.Contains(pathErr.Err.Error(), "escapes")) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}

	return err
}

func newEntry(p string, info fs.FileInfo) *Entry {
	size := int64(0)
	if !info.IsDir() {
		size = info.Size()
	}

	return &Entry{Name: path.Base(p), Path: p, Dir: info.IsDir(), Size: size, ModifiedAt: info.ModTime()}
}
