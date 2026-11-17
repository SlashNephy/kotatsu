package filelib

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
)

var (
	// ErrNoDataDir は、データディレクトリを設定していないので、生成物の置き場がないことを表す。
	ErrNoDataDir = errors.New("the data directory is not configured")
	// ErrClearing は、ほかの片付けが終わっていないことを表す。
	ErrClearing = errors.New("the library artifacts are being cleared")
)

const (
	// detachedPrefix と detachedSuffix は、片付けるときに、生成物の置き場 (kago.LibraryIndexDir) を移す先の名前の前後。
	// 「.」で始まる名前は、DB にないファイルの走査 (設計書 5.6) が調べないので、消しきれずに残っても一覧に出ない。次に片付けるときに一緒に消す。
	detachedPrefix = "." + kago.LibraryIndexDir + "-"
	detachedSuffix = ".removing"
)

// Usage は、ライブラリのファイルの生成物の量。
type Usage struct {
	// Size は、ファイルの大きさの合計 (バイト)。
	Size int64
	// Files は、ファイルの数。
	Files int
}

// Artifacts は、データディレクトリの、ライブラリのファイルの生成物 (索引、画像、CM の区間、字幕) の量を返す。
// 前に片付けたときに消しきれずに残ったものも含める。
func (x *Indexer) Artifacts() (*Usage, error) {
	if x.dataDir == "" {
		return nil, ErrNoDataDir
	}
	usage := &Usage{Size: 0, Files: 0}
	dirs, err := x.detachedDirs()
	if err != nil {
		return nil, err
	}
	for _, dir := range append([]string{filepath.Join(x.dataDir, kago.LibraryIndexDir)}, dirs...) {
		if err := measure(dir, usage); err != nil {
			return nil, err
		}
	}

	return usage, nil
}

// ClearArtifacts は、データディレクトリの、ライブラリのファイルの生成物の置き場を、別の名前に移して使わないようにする (設計書 5.8、#1896)。
// 移した置き場は、PurgeDetached で消す。消した後にファイルを開けば、索引と画像を作り直す。
//
// この tenban で作っている途中の索引は、取りやめて止まるのを待ってから移し、移し終えるまで作り始めない。record には、消す量を渡す。
// record が失敗すれば、移さない。監査ログを書けなければ消さないためである。生成物がなければ、record を呼ばずに、空の量を返す。
//
// 置き場を消すのでなく移すのは、nekonote のジョブなどが書いている途中でも、記録 (source.json、images.json など) と中身のずれを
// 残さないためである。移した後に書き続けたものは、移した先に入るか、置き場がなくて失敗するので、新しい置き場には残らない。
func (x *Indexer) ClearArtifacts(ctx context.Context, record func(*Usage) error) (*Usage, error) {
	if x.dataDir == "" {
		return nil, ErrNoDataDir
	}
	resume, err := x.stopBuilds(ctx)
	if err != nil {
		return nil, err
	}
	defer resume()
	usage, err := x.Artifacts()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(x.dataDir, kago.LibraryIndexDir)
	_, statErr := os.Lstat(dir)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return nil, fmt.Errorf("failed to read the library artifacts: %w", statErr)
	}
	detached, err := x.detachedDirs()
	if err != nil {
		return nil, err
	}
	if !exists && len(detached) == 0 {
		return usage, nil
	}
	if err := record(usage); err != nil {
		return nil, err
	}
	if !exists {
		return usage, nil
	}
	to := filepath.Join(x.dataDir, detachedPrefix+strconv.FormatInt(x.clock.Now().UnixNano(), 10)+detachedSuffix)
	if err := os.Rename(dir, to); err != nil {
		return nil, fmt.Errorf("failed to detach the library artifacts: %w", err)
	}

	return usage, nil
}

// PurgeDetached は、ClearArtifacts が移した置き場を消す。消しきれなかったものは、次に片付けるときに消す。
func (x *Indexer) PurgeDetached() error {
	if x.dataDir == "" {
		return ErrNoDataDir
	}
	dirs, err := x.detachedDirs()
	if err != nil {
		return err
	}
	var errs []error
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Errorf("failed to remove the library artifacts: %w", err))
		}
	}

	return errors.Join(errs...)
}

// stopBuilds は、この tenban で作っている途中の索引を取りやめて、止まるのを待つ。返した関数を呼ぶまで、索引を作り始めない。
func (x *Indexer) stopBuilds(ctx context.Context) (func(), error) {
	x.mu.Lock()
	if x.clearing {
		x.mu.Unlock()

		return nil, ErrClearing
	}
	x.clearing = true
	builds := make([]*indexBuild, 0, len(x.building))
	for _, b := range x.building {
		builds = append(builds, b)
	}
	x.mu.Unlock()
	resume := func() {
		x.mu.Lock()
		x.clearing = false
		x.mu.Unlock()
	}
	for _, b := range builds {
		b.cancel()
	}
	for _, b := range builds {
		select {
		case <-b.done:
		case <-ctx.Done():
			resume()

			return nil, ctx.Err()
		}
	}

	return resume, nil
}

// detachedDirs は、ClearArtifacts が移した置き場 (消しきれずに残ったものを含む) のパスを返す。
func (x *Indexer) detachedDirs() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(x.dataDir, detachedPrefix+"*"+detachedSuffix))
	if err != nil {
		return nil, fmt.Errorf("failed to list the detached library artifacts: %w", err)
	}
	if matches == nil {
		return []string{}, nil
	}

	return matches, nil
}

// measure は、dir の下のファイルの数と大きさを usage に足す。dir がなければ何もしない。数えている間に消えたファイルは数えない。
func measure(dir string, usage *Usage) error {
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		usage.Size += info.Size()
		usage.Files++

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to measure the library artifacts: %w", err)
	}

	return nil
}

// DeleteWaitingJobs は、ライブラリのファイルの画像、CM の区間、字幕の自動生成のジョブのうち、まだ動いていないものの行を消し、消した数を返す。
// 生成物を片付けると索引がなくなり、画像と CM のジョブは、索引を作り直すまで待ち続け、字幕のジョブは失敗するためである。
// 取りやめる (cancelled にする) と、画像と CM のジョブは、River が行を消すまで同じファイルのジョブを入れ直せないので、行を消す。
// 消した後にファイルを開けば、画像と CM のジョブを入れ直す。動いているジョブは、そのまま終えさせる。
func DeleteWaitingJobs(ctx context.Context, db sqlc.DBTX) (int64, error) {
	kinds := []string{new(ImagesArgs).Kind(), new(CMArgs).Kind(), new(CaptionsArgs).Kind()}
	// River の表は sqlc のスキーマにないので、直接書く。まだ動いていないジョブの状態だけを消す。
	tag, err := db.Exec(ctx, `DELETE FROM river_job WHERE kind = ANY($1::text[]) AND state IN ('available', 'pending', 'retryable', 'scheduled')`, kinds)
	if err != nil {
		return 0, fmt.Errorf("failed to delete the waiting library jobs: %w", err)
	}

	return tag.RowsAffected(), nil
}
