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

	"github.com/google/uuid"
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
	// totalSpace は、プールのファイルシステムの容量を調べる。
	totalSpace TotalSpaceFunc
	// dataDir は、データディレクトリ。
	dataDir string
	// timeshiftDir は、タイムシフトのバッファを置くディレクトリ。
	timeshiftDir string
}

// DataPoolID は、データディレクトリ (WithDataDir) を、Store の中でプールと同じく扱うときの ID。
// DB にも API にも出さない。設定のプールの ID とは重ならないよう、_ で始める。
const DataPoolID = "_data"

// TimeshiftPoolID は、タイムシフトのディレクトリ (WithTimeshiftDir) を、Store の中でプールと同じく扱うときの ID。
// DataPoolID と同じく、DB にも API にも出さず、PoolIDs と ScanPoolIDs にも含めない。
const TimeshiftPoolID = "_timeshift"

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

	// 呼び出し側が後から書き換えても影響しないよう、複製して持つ。プールがなくても、データやタイムシフトのディレクトリを足せるよう、空の対応にする。
	cloned := make(map[string]string, len(mounts))
	maps.Copy(cloned, mounts)

	return &Store{mounts: cloned, order: ordered, freeSpace: freeSpace, totalSpace: TotalSpace, dataDir: "", timeshiftDir: ""}
}

// WithTotalSpace は、プールの容量を totalSpace で調べる Store を返す。totalSpace が nil なら、s をそのまま返す。
func (s *Store) WithTotalSpace(totalSpace TotalSpaceFunc) *Store {
	if totalSpace == nil {
		return s
	}

	return &Store{mounts: s.mounts, order: s.order, freeSpace: s.freeSpace, totalSpace: totalSpace, dataDir: s.dataDir, timeshiftDir: s.timeshiftDir}
}

// WithDataDir は、セグメントの生成物を、dir の中の「セグメントのファイルの ID/」に置く Store を返す (設計書 5.6)。
// dir が空なら、s をそのまま返す。そのときは、生成物を作れない。
func (s *Store) WithDataDir(dir string) *Store {
	if dir == "" {
		return s
	}
	mounts := maps.Clone(s.mounts)
	mounts[DataPoolID] = dir

	return &Store{mounts: mounts, order: s.order, freeSpace: s.freeSpace, totalSpace: s.totalSpace, dataDir: dir, timeshiftDir: s.timeshiftDir}
}

// WithTimeshiftDir は、タイムシフトのバッファを、dir の中に置く Store を返す。
// dir が空なら、s をそのまま返す。そのときは、タイムシフトのどの処理も動かない。
// dir は TimeshiftPoolID のプールとして引けるが、PoolIDs と ScanPoolIDs には出ない。
func (s *Store) WithTimeshiftDir(dir string) *Store {
	if dir == "" {
		return s
	}
	mounts := maps.Clone(s.mounts)
	mounts[TimeshiftPoolID] = dir

	return &Store{mounts: mounts, order: s.order, freeSpace: s.freeSpace, totalSpace: s.totalSpace, dataDir: s.dataDir, timeshiftDir: dir}
}

// HasTimeshiftDir は、タイムシフトのディレクトリを設定してあるかを返す。
func (s *Store) HasTimeshiftDir() bool {
	return s.timeshiftDir != ""
}

// SegmentDir は、セグメントのファイル fileID (recording_files.id) の生成物のディレクトリの場所を返す。
func SegmentDir(fileID uuid.UUID) *Location {
	return &Location{PoolID: DataPoolID, Path: fileID.String()}
}

// DataLocation は、データディレクトリの、セグメントのファイル fileID のディレクトリにある、name のファイルの場所を返す。
func DataLocation(fileID uuid.UUID, name string) *Location {
	return &Location{PoolID: DataPoolID, Path: fileID.String() + "/" + name}
}

// IndexLocation は、セグメントのファイル fileID の索引の場所を返す。
func IndexLocation(fileID uuid.UUID) *Location {
	return DataLocation(fileID, IndexFileName)
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

// Stat は、プール ID とプール内の相対パスのファイルの情報を返す。Open と同じく、プールのマウント先の中に限って調べる。
func (s *Store) Stat(poolID, rel string) (os.FileInfo, error) {
	f, err := s.Open(poolID, rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return f.Stat()
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

// IndexFileName は、生成物のディレクトリに置く、セグメントの索引のファイルの名前 (設計書 5.6)。
const IndexFileName = "index"

// CreateSegmentDir は、セグメントのファイル fileID の生成物 (索引やサムネイル) を置くディレクトリを作り、その場所を返す。
// すでにあれば、そのまま使う。
func (s *Store) CreateSegmentDir(fileID uuid.UUID) (*Location, error) {
	target := SegmentDir(fileID)
	mount, ok := s.mounts[target.PoolID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPoolNotMounted, target.PoolID)
	}
	root, err := os.OpenRoot(mount)
	if err != nil {
		return nil, fmt.Errorf("failed to open pool %s: %w", target.PoolID, err)
	}
	defer root.Close()
	if err := root.MkdirAll(target.Path, 0o755); err != nil {
		// 同じ名前のディレクトリでないものがあれば、生成物を置けない。
		if info, statErr := root.Lstat(target.Path); statErr == nil && !info.IsDir() {
			return nil, fmt.Errorf("%w: %q exists and is not a directory", ErrInvalidPath, target.Path)
		}

		return nil, fmt.Errorf("failed to create the segment directory: %w", err)
	}

	return target, nil
}

// RemoveSegmentDir は、データディレクトリの、セグメントのファイル fileID のディレクトリと、その中のファイルを消す。なければ何もしない。
func (s *Store) RemoveSegmentDir(fileID uuid.UUID) error {
	dir := SegmentDir(fileID)
	if err := s.removeSegmentDir(dir.PoolID, dir.Path); err != nil {
		return fmt.Errorf("failed to remove the segment directory: %w", err)
	}

	return nil
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

// PoolTotalSpace は、プール poolID のファイルシステムの容量 (バイト) を返す。
func (s *Store) PoolTotalSpace(poolID string) (uint64, error) {
	mount, ok := s.mounts[poolID]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrPoolNotMounted, poolID)
	}
	total, err := s.totalSpace(mount)
	if err != nil {
		return 0, fmt.Errorf("failed to get the total space of pool %s: %w", poolID, err)
	}

	return total, nil
}

// Remove は、プールの録画ファイル rel と、そのファイル fileID (recording_files.id) の生成物のディレクトリを消す。
// どちらもなければ、何もしない。fileID が uuid.Nil なら、生成物のディレクトリは消さない。
func (s *Store) Remove(poolID, rel string, fileID uuid.UUID) error {
	path, err := s.Path(poolID, rel)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove a file in pool %s: %w", poolID, err)
	}
	if _, mounted := s.mounts[DataPoolID]; mounted && fileID != uuid.Nil {
		if err := s.RemoveSegmentDir(fileID); err != nil {
			return err
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

// removeSegmentDir は、生成物のディレクトリ dir (プール内の相対パス) と、その中のファイルを消す。
// 中にディレクトリがあれば、生成物のディレクトリではない (ほかの録画を置いたディレクトリと重なった) とみなし、中身ごとは消さない。
// プールの中に限って消すので、シンボリックリンクをたどってプールの外を消すことはない (リンクそのものは消す)。
func (s *Store) removeSegmentDir(poolID, dir string) error {
	mount, ok := s.mounts[poolID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPoolNotMounted, poolID)
	}
	root, err := os.OpenRoot(mount)
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
