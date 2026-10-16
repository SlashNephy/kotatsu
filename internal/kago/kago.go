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
	// MinFreeBytes は容量の閾値。録画を書いた後の空き容量がこれを下回らないプールを優先して選ぶ (#511)。
	MinFreeBytes uint64
}

// FreeSpaceFunc は、path を含むファイルシステムの、使える空き容量 (バイト) を返す。
type FreeSpaceFunc func(path string) (uint64, error)

// TotalSpaceFunc は、path を含むファイルシステムの容量 (バイト) を返す。
type TotalSpaceFunc func(path string) (uint64, error)

// Store は、このロールから見えるプールのマウント先を持つ。
type Store struct {
	mounts map[string]string
	// order は、設定に書いたプールの順。優先順位が同じプールは、この順に選ぶ。
	order     []string
	freeSpace FreeSpaceFunc
	// sidecarDir は、付属ファイルを録画ファイルの横でなく、まとめて置くディレクトリ。空なら、録画ファイルの横に置く。
	sidecarDir string
}

// SidecarPoolID は、付属ファイルをまとめて置くディレクトリ (WithSidecarDir) を、プールと同じく扱うときの ID。
// recording_files の pool_id に記録する。設定のプールの ID とは重ならないよう、_ で始める。
const SidecarPoolID = "_sidecar"

// Location は、プールの中のファイルの場所。
type Location struct {
	PoolID string
	Path   string
}

var (
	// ErrPoolNotMounted は、プール ID に対応するマウント先が設定されていないことを表す。
	ErrPoolNotMounted = errors.New("pool is not mounted on this instance")
	// ErrNoPoolFits は、推定サイズを収められるプールがないことを表す。
	ErrNoPoolFits = errors.New("no pool has enough free space")
	// ErrPoolUnavailable は、空き容量を調べられたプールが 1 つもないことを表す (#526)。
	ErrPoolUnavailable = errors.New("no pool could be measured")
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
	return &Store{mounts: maps.Clone(mounts), order: ordered, freeSpace: freeSpace, sidecarDir: ""}
}

// WithSidecarDir は、新しい録画の付属ファイルを、dir の中の「プール ID/録画ファイルのパス.kotatsu/」に置く Store を返す (設計書 5.6)。
// すでにある録画の付属ファイルは移さず、録画ファイルの横にあれば、そこを使い続ける。dir が空なら、s をそのまま返す。
func (s *Store) WithSidecarDir(dir string) *Store {
	if dir == "" {
		return s
	}
	mounts := maps.Clone(s.mounts)
	mounts[SidecarPoolID] = dir

	return &Store{mounts: mounts, order: s.order, freeSpace: s.freeSpace, sidecarDir: dir}
}

// IndexLocations は、録画ファイル rel の索引の場所の候補を返す。記録から索引を探すときは、どれかに当たるものを使う。
func (s *Store) IndexLocations(poolID, rel string) []*Location {
	dirs := s.sidecarLocations(poolID, rel)
	locations := make([]*Location, 0, len(dirs))
	for _, dir := range dirs {
		locations = append(locations, &Location{PoolID: dir.PoolID, Path: dir.Path + "/" + IndexFileName})
	}

	return locations
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
// どのプールも閾値を下回るなら、推定サイズが入るプールのうち、優先順位が最も高いものを選ぶ。
// 閾値は録画を始めるかどうかの条件ではなく、容量の管理が古い録画を消して保つ空き容量なので、録画を失敗させない (#511)。
// このロールにマウントされていないプールと、空き容量を調べられなかったプールは選ばない。
// どれも選べなければ ErrNoPoolFits を返す。空き容量を調べられたプールが 1 つもなければ、ErrPoolUnavailable を返す。
func (s *Store) Select(pools []*Pool, estimated uint64) (*Pool, error) {
	ordered := slices.Clone(pools)
	slices.SortStableFunc(ordered, func(a, b *Pool) int {
		return cmp.Or(cmp.Compare(b.Priority, a.Priority), cmp.Compare(s.rank(a.ID), s.rank(b.ID)), cmp.Compare(a.ID, b.ID))
	})

	errs := []error{}
	measured := 0
	var fallback *Pool
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
		measured++
		if free < estimated {
			continue
		}
		if free-estimated >= pool.MinFreeBytes {
			return pool, nil
		}
		if fallback == nil {
			fallback = pool
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	// 空き容量を調べられたプールがなければ、容量が足りないのではない。調べられなかった理由を返す (#526)。
	if measured == 0 {
		return nil, errors.Join(append([]error{ErrPoolUnavailable}, errs...)...)
	}

	return nil, errors.Join(append([]error{fmt.Errorf("%w: estimated %d bytes", ErrNoPoolFits, estimated)}, errs...)...)
}

// sidecarSuffix は、録画ファイルの横に作る、付属ファイル用のディレクトリの名前に付ける接尾辞。
const sidecarSuffix = ".kotatsu"

// IndexFileName は、付属ファイルのディレクトリに置く、セグメントの索引のファイルの名前 (設計書 5.6)。
const IndexFileName = "index"

// IndexPath は、録画ファイル rel の索引の、プール内の相対パスを返す。
func IndexPath(rel string) string {
	return rel + sidecarSuffix + "/" + IndexFileName
}

// CreateSidecarDir は、録画ファイル rel の付属ファイル (索引やサムネイル) を置くディレクトリを作り、その場所を返す。
// 録画ファイルの横 (以前の置き場所) にすでにあれば、そこを使い続ける。なければ、まとめて置くディレクトリがあればそこに、
// なければ録画ファイルの横 (例: 番組名.m2ts.kotatsu/) に作る。すでにあれば、そのまま使う。
func (s *Store) CreateSidecarDir(poolID, rel string) (*Location, error) {
	candidates := s.sidecarLocations(poolID, rel)
	legacy := candidates[len(candidates)-1]
	target := candidates[0]
	if len(candidates) > 1 {
		if dir, err := s.Path(legacy.PoolID, legacy.Path); err == nil {
			if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
				target = legacy
			}
		}
	}
	mount, ok := s.mounts[target.PoolID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPoolNotMounted, target.PoolID)
	}
	if !filepath.IsLocal(target.Path) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidPath, target.Path)
	}
	root, err := os.OpenRoot(mount)
	if err != nil {
		return nil, fmt.Errorf("failed to open pool %s: %w", target.PoolID, err)
	}
	defer root.Close()
	if err := root.MkdirAll(target.Path, 0o755); err != nil {
		// 同じ名前のディレクトリでないものがあれば、付属ファイルを置けない。
		if info, statErr := root.Lstat(target.Path); statErr == nil && !info.IsDir() {
			return nil, fmt.Errorf("%w: %q exists and is not a directory", ErrInvalidPath, target.Path)
		}

		return nil, fmt.Errorf("failed to create the sidecar directory: %w", err)
	}

	return target, nil
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
	for _, sidecar := range s.sidecarLocations(poolID, rel) {
		if err := s.removeSidecarDir(sidecar.PoolID, sidecar.Path); err != nil {
			return fmt.Errorf("failed to remove the sidecar directory in pool %s: %w", sidecar.PoolID, err)
		}
		// まとめて置くディレクトリの中に掘ったディレクトリも、空になれば遡って消す。
		if sidecar.PoolID == SidecarPoolID {
			for dir := filepath.Dir(sidecar.Path); dir != "." && removedEmptyDir(filepath.Join(s.mounts[SidecarPoolID], dir)); {
				dir = filepath.Dir(dir)
			}
		}
	}
	// ファイル名のテンプレートで掘ったディレクトリが空になれば、プールの直下まで遡って消す。
	// 消せなければ (空でないか、ほかの理由で)、そこで止める。録画のファイルは消せているので、エラーにはしない。
	dir := filepath.Dir(rel)
	for dir != "." && removedEmptyDir(filepath.Join(s.mounts[poolID], dir)) {
		dir = filepath.Dir(dir)
	}

	return nil
}

// Exists は、プールにファイル rel があるかを返す。プールがこのインスタンスにマウントされていなければ ErrPoolNotMounted を返す。
// プールの中に限って調べるので、シンボリックリンクでもプールの外は調べない。続けて多くのファイルを調べるときは NewChecker を使う。
func (s *Store) Exists(poolID, rel string) (bool, error) {
	checker := s.NewChecker()
	defer checker.Close()

	return checker.Exists(poolID, rel)
}

// Checker は、プールを開き直さずに、ファイルの有無を続けて調べる。使い終わったら Close する。
type Checker struct {
	store *Store
	roots map[string]*os.Root
}

// NewChecker は Checker を返す。
func (s *Store) NewChecker() *Checker {
	return &Checker{store: s, roots: map[string]*os.Root{}}
}

// Exists は、Store.Exists と同じく、プールにファイル rel があるかを返す。
func (c *Checker) Exists(poolID, rel string) (bool, error) {
	if !filepath.IsLocal(rel) {
		return false, fmt.Errorf("%w: %q", ErrInvalidPath, rel)
	}
	root, ok := c.roots[poolID]
	if !ok {
		mount, mounted := c.store.mounts[poolID]
		if !mounted {
			return false, fmt.Errorf("%w: %s", ErrPoolNotMounted, poolID)
		}
		var err error
		if root, err = os.OpenRoot(mount); err != nil {
			return false, fmt.Errorf("failed to open pool %s: %w", poolID, err)
		}
		c.roots[poolID] = root
	}
	_, err := root.Stat(rel)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("failed to inspect a file in pool %s: %w", poolID, err)
	}
}

// Close は、開いたプールを閉じる。
func (c *Checker) Close() {
	for _, root := range c.roots {
		_ = root.Close()
	}
	c.roots = map[string]*os.Root{}
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

// removeSidecarDir は、付属ファイルのディレクトリ dir (プール内の相対パス) と、その中のファイルを消す。
// 中にディレクトリがあれば、付属ファイルのディレクトリではない (ほかの録画を置いたディレクトリと重なった) とみなし、中身ごとは消さない。
// プールの中に限って消すので、シンボリックリンクをたどってプールの外を消すことはない (リンクそのものは消す)。
func (s *Store) removeSidecarDir(poolID, dir string) error {
	root, err := os.OpenRoot(s.mounts[poolID])
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return root.Remove(dir)
	}
	f, err := root.Open(dir)
	if err != nil {
		return err
	}
	entries, err := f.ReadDir(-1)
	_ = f.Close()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return fmt.Errorf("%w: %q contains a directory", ErrInvalidPath, dir)
		}
	}
	for _, entry := range entries {
		if err := root.Remove(filepath.Join(dir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := root.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// sidecarLocations は、録画ファイル rel の付属ファイルのディレクトリの場所の候補を、使う順に返す。
// まとめて置くディレクトリがあれば、そこを先にし、録画ファイルの横 (以前の置き場所) を後にする。
func (s *Store) sidecarLocations(poolID, rel string) []*Location {
	legacy := &Location{PoolID: poolID, Path: rel + sidecarSuffix}
	if s.sidecarDir == "" {
		return []*Location{legacy}
	}

	return []*Location{{PoolID: SidecarPoolID, Path: filepath.ToSlash(filepath.Join(poolID, rel)) + sidecarSuffix}, legacy}
}
