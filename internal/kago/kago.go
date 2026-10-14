// Package kago は、録画の保存先プール (kago) を扱う (設計書 5.6)。
//
// プールは、ディレクトリと、その設定 (容量の閾値と優先順位) の組である。
// 録画のパスは「プール ID ＋ プール内の相対パス」で記録し、プール ID とマウント先の対応はロールごとの設定で与える。
// そのため、ロールごとにマウント先のパスが違っていても、同じ録画を指せる。
package kago

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// Pool は保存先プールの設定。
type Pool struct {
	ID string
	// Priority は選ばれる優先順位。大きいほど先に選ぶ。
	Priority int
	// MinFreeBytes は容量の閾値。録画を書いた後の空き容量がこれを下回るプールは選ばない。
	MinFreeBytes uint64
}

// FreeSpaceFunc は、path を含むファイルシステムの、使える空き容量 (バイト) を返す。
type FreeSpaceFunc func(path string) (uint64, error)

// Store は、このロールから見えるプールのマウント先を持つ。
type Store struct {
	mounts map[string]string
	// order は、設定に書いたプールの順。優先順位が同じプールは、この順に選ぶ。
	order     []string
	freeSpace FreeSpaceFunc
}

var (
	// ErrPoolNotMounted は、プール ID に対応するマウント先が設定されていないことを表す。
	ErrPoolNotMounted = errors.New("pool is not mounted on this instance")
	// ErrNoPoolFits は、推定サイズを収められるプールがないことを表す。
	ErrNoPoolFits = errors.New("no pool has enough free space")
	// ErrInvalidPath は、プール内の相対パスとして使えないパスであることを表す。
	ErrInvalidPath = errors.New("invalid path in a pool")
)

// New は Store を作る。mounts はプール ID からマウント先への対応 (config.Storage.Pools)。
// プールの順は ID の昇順にする。設定に書いた順を使うなら NewOrdered を使う。
// freeSpace が nil なら、ファイルシステムに問い合わせる FreeSpace を使う。
func New(mounts map[string]string, freeSpace FreeSpaceFunc) *Store {
	return NewOrdered(mounts, nil, freeSpace)
}

// NewOrdered は、プールの順を order (config.Storage.PoolOrder) にした Store を作る。
// 優先順位が同じプールは、この順に選ぶ。order にないプールは、order にあるものの後に、ID の昇順で並べる。
func NewOrdered(mounts map[string]string, order []string, freeSpace FreeSpaceFunc) *Store {
	if freeSpace == nil {
		freeSpace = FreeSpace
	}
	ordered := make([]string, 0, len(mounts))
	for _, id := range order {
		if _, ok := mounts[id]; ok && !slices.Contains(ordered, id) {
			ordered = append(ordered, id)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(mounts)) {
		if !slices.Contains(ordered, id) {
			ordered = append(ordered, id)
		}
	}

	// 呼び出し側が後から書き換えても影響しないよう、複製して持つ。
	return &Store{mounts: maps.Clone(mounts), order: ordered, freeSpace: freeSpace}
}

// PoolIDs は、このロールにマウントされているプールの ID を、プールの順に返す。
func (s *Store) PoolIDs() []string {
	return slices.Clone(s.order)
}

// Path は、プール ID とプール内の相対パスから、このロールでのパスを返す。
// 相対パスがプールの外を指すとき (絶対パスや .. を含むとき) は ErrInvalidPath を返す。
func (s *Store) Path(poolID, rel string) (string, error) {
	mount, ok := s.mounts[poolID]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrPoolNotMounted, poolID)
	}
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%w: %q", ErrInvalidPath, rel)
	}

	return filepath.Join(mount, rel), nil
}

// Open は、プール ID とプール内の相対パスのファイルを、読むために開く。
// ファイルは、プールのマウント先の中に限って開くので、シンボリックリンクでもプールの外は開かない。
func (s *Store) Open(poolID, rel string) (*os.File, error) {
	mount, ok := s.mounts[poolID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPoolNotMounted, poolID)
	}
	if !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidPath, rel)
	}

	return os.OpenInRoot(mount, rel)
}

// Select は、推定サイズ estimated (バイト) を書いても空き容量が閾値を下回らないプールのうち、
// 優先順位が最も高いものを選ぶ。優先順位が同じなら、プールの順 (設定に書いた順) で先のものを選ぶ。
// このロールにマウントされていないプールと、空き容量を調べられなかったプールは選ばない。
// どれも選べなければ ErrNoPoolFits を返す。
func (s *Store) Select(pools []*Pool, estimated uint64) (*Pool, error) {
	ordered := slices.Clone(pools)
	slices.SortStableFunc(ordered, func(a, b *Pool) int {
		return cmp.Or(cmp.Compare(b.Priority, a.Priority), cmp.Compare(s.rank(a.ID), s.rank(b.ID)), cmp.Compare(a.ID, b.ID))
	})

	errs := []error{}
	for _, pool := range ordered {
		mount, ok := s.mounts[pool.ID]
		if !ok {
			continue
		}
		free, err := s.freeSpace(mount)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get the free space of pool %s: %w", pool.ID, err))

			continue
		}
		if free >= estimated && free-estimated >= pool.MinFreeBytes {
			return pool, nil
		}
	}

	return nil, errors.Join(append([]error{fmt.Errorf("%w: estimated %d bytes", ErrNoPoolFits, estimated)}, errs...)...)
}

// sidecarSuffix は、録画ファイルの横に作る、付属ファイル用のディレクトリの名前に付ける接尾辞。
const sidecarSuffix = ".kotatsu"

// indexFileName は、付属ファイルのディレクトリに置く、セグメントの索引のファイルの名前 (設計書 5.6)。
const indexFileName = "index"

// IndexPath は、録画ファイル rel の索引の、プール内の相対パスを返す。
func IndexPath(rel string) string {
	return rel + sidecarSuffix + "/" + indexFileName
}

// CreateSidecarDir は、録画ファイルの横に、索引やサムネイルを置くディレクトリ (例: 番組名.m2ts.kotatsu/) を作り、
// そのプール内の相対パスを返す。すでにあれば、そのまま使う。
func (s *Store) CreateSidecarDir(poolID, rel string) (string, error) {
	dirRel := rel + sidecarSuffix
	dir, err := s.Path(poolID, dirRel)
	if err != nil {
		return "", err
	}
	err = os.Mkdir(dir, 0o755)
	if errors.Is(err, os.ErrExist) {
		// 同じ名前のディレクトリでないものがあれば、付属ファイルを置けない。
		info, statErr := os.Stat(dir)
		if statErr != nil {
			return "", fmt.Errorf("failed to inspect the sidecar directory: %w", statErr)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%w: %q exists and is not a directory", ErrInvalidPath, dirRel)
		}

		return dirRel, nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to create the sidecar directory: %w", err)
	}

	return dirRel, nil
}

// PoolFreeSpace は、プール poolID の空き容量 (バイト) を返す。
func (s *Store) PoolFreeSpace(poolID string) (uint64, error) {
	mount, ok := s.mounts[poolID]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrPoolNotMounted, poolID)
	}
	free, err := s.freeSpace(mount)
	if err != nil {
		return 0, fmt.Errorf("failed to get the free space of pool %s: %w", poolID, err)
	}

	return free, nil
}

// Remove は、プールの録画ファイル rel と、その付属ファイルのディレクトリを消す。どちらもなければ、何もしない。
func (s *Store) Remove(poolID, rel string) error {
	path, err := s.Path(poolID, rel)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove a file in pool %s: %w", poolID, err)
	}
	if err := removeSidecarDir(path + sidecarSuffix); err != nil {
		return fmt.Errorf("failed to remove the sidecar directory in pool %s: %w", poolID, err)
	}
	// ファイル名のテンプレートで掘ったディレクトリが空になれば、プールの直下まで遡って消す。
	// 消せなければ (空でないか、ほかの理由で)、そこで止める。録画のファイルは消せているので、エラーにはしない。
	dir := filepath.Dir(rel)
	for dir != "." && removedEmptyDir(filepath.Join(s.mounts[poolID], dir)) {
		dir = filepath.Dir(dir)
	}

	return nil
}

// rank は、プールの順での位置を返す。マウントされていないプールは、どれよりも後にする。
func (s *Store) rank(id string) int {
	if i := slices.Index(s.order, id); i >= 0 {
		return i
	}

	return len(s.order)
}

// removedEmptyDir は、空のディレクトリ path を消し、消せたかどうかを返す。空でなければ消さない。
func removedEmptyDir(path string) bool {
	return os.Remove(path) == nil
}

// removeSidecarDir は、付属ファイルのディレクトリと、その中のファイルを消す。中にディレクトリがあれば、付属ファイルの
// ディレクトリではない (ほかの録画を置いたディレクトリと重なった) とみなし、中身ごとは消さない。
func removeSidecarDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return fmt.Errorf("%w: %q contains a directory", ErrInvalidPath, dir)
		}
	}
	for _, entry := range entries {
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}
