// Package medialib は、録画として登録していない TS を再生するために、ファイルを読んでよいフォルダ (ライブラリ) を扱う (設計書 5.8、#1734)。
//
// ライブラリの中身は、一覧 (List)、情報 (Stat)、開く (Open) の 3 つの口だけで読む。今はローカルのディレクトリ (Local) だけを実装し、
// クラウドストレージは、後でこの口に当てはめる。パスは、ライブラリの中の / 区切りの相対パスで表し、ライブラリの外へは出られない。
package medialib

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"time"
)

// Kind は、ライブラリの種類。
type Kind string

const (
	// KindPool は、保存先プール。設定に書かなくても、ライブラリとして見せる。
	KindPool Kind = "pool"
	// KindFolder は、設定 (storage.libraries) で足したフォルダ。
	KindFolder Kind = "folder"
)

var (
	// ErrInvalidPath は、ライブラリの中の相対パスとして使えないパス (絶対パス、..、\ を含むもの、隠しファイル) であることを表す。
	ErrInvalidPath = errors.New("invalid path in a library")
	// ErrNotFound は、ライブラリかファイルがないことを表す。fs.ErrNotExist でもある。
	ErrNotFound = errors.New("not found in the library")
)

// Entry は、ライブラリの中のファイルかディレクトリ。
type Entry struct {
	// Name は、名前。
	Name string
	// Path は、ライブラリの中の / 区切りの相対パス。
	Path string
	// Dir は、ディレクトリかどうか。
	Dir bool
	// Size は、ファイルの大きさ (バイト)。ディレクトリでは 0。
	Size int64
	// ModifiedAt は、最後に書いた日時。
	ModifiedAt time.Time
}

// File は、ライブラリの中の、読むために開いたファイル。途中から読める (ReaderAt と Seeker)。
// *os.File が満たす。クラウドストレージでは、範囲を指定した取得で満たす。
type File interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
	Stat() (fs.FileInfo, error)
}

// Library は、ライブラリの中身を読む口。
type Library interface {
	// Name は、ライブラリの名前。
	Name() string
	// Kind は、ライブラリの種類。
	Kind() Kind
	// List は、ディレクトリ dir (空ならライブラリの根元) の中のファイルとディレクトリを、名前の順に返す。
	// 隠しファイルと、普通のファイルでもディレクトリでもないもの、ライブラリの外を指すシンボリックリンクは返さない。
	List(ctx context.Context, dir string) ([]*Entry, error)
	// Stat は、ファイル p の情報を返す。
	Stat(ctx context.Context, p string) (*Entry, error)
	// Open は、ファイル p を読むために開く。
	Open(ctx context.Context, p string) (File, error)
}

// Set は、このインスタンスから読めるライブラリの一覧。
type Set struct {
	libraries []Library
}

// NewSet は、保存先プール (pools、設定に書いた順 poolOrder) と、設定で足したフォルダ (folders) のライブラリの一覧を返す。
// プールを先に設定の順で並べ、フォルダを名前の順に続ける。
func NewSet(pools map[string]string, poolOrder []string, folders map[string]string) *Set {
	libraries := []Library{}
	added := map[string]bool{}
	for _, id := range slices.Concat(poolOrder, slices.Sorted(maps.Keys(pools))) {
		dir, ok := pools[id]
		if !ok || added[id] {
			continue
		}
		added[id] = true
		libraries = append(libraries, NewLocal(id, dir, KindPool))
	}
	for _, name := range slices.Sorted(maps.Keys(folders)) {
		if added[name] {
			continue
		}
		added[name] = true
		libraries = append(libraries, NewLocal(name, folders[name], KindFolder))
	}

	return &Set{libraries: libraries}
}

// All は、ライブラリを、プール、フォルダの順に返す。
func (s *Set) All() []Library {
	return slices.Clone(s.libraries)
}

// Get は、名前が name のライブラリを返す。
func (s *Set) Get(name string) (Library, bool) {
	for _, library := range s.libraries {
		if library.Name() == name {
			return library, true
		}
	}

	return nil, false
}

// Playable は、名前が name のファイルを、TS として再生の候補にするか (拡張子が .ts、.m2ts、.mts か) を返す。
// 拡張子の大文字と小文字は区別しない。
func Playable(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".ts", ".m2ts", ".mts":
		return true
	default:
		return false
	}
}

// CleanPath は、ライブラリの中の相対パス p を、/ 区切りの正規の形にする。空と . は根元 ("") にする。
// 絶対パス、.. を含むもの、\ を含むもの、隠しファイル (. で始まる名前) を含むものは ErrInvalidPath を返す。
// \ を拒むのは、Windows でパスの区切りとして読まれ、ほかの OS と違うファイルを指すからである。
func CleanPath(p string) (string, error) {
	if p == "" || p == "." {
		return "", nil
	}
	if strings.ContainsAny(p, "\\\x00") || strings.HasPrefix(p, "/") {
		return "", ErrInvalidPath
	}
	for part := range strings.SplitSeq(p, "/") {
		if part == "" || part == ".." || part == "." || strings.HasPrefix(part, ".") {
			return "", ErrInvalidPath
		}
	}

	return p, nil
}
