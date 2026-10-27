package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/segjoin"
)

const (
	// orphanGrace は、DB にないファイルとみなすまでに、最後に書くか名前を変えてから空ける時間。
	// 録画はファイルを作ってから DB に記録し、サムネイルは一時ファイルに書いてから名前を変えるので、書いたばかりのファイルは DB にないことがある。
	orphanGrace = time.Hour
	// orphanAuditTarget は、DB にないファイルを消したときの監査ログの対象の種類。
	orphanAuditTarget = "storage_file"
)

// ErrNotOrphan は、消そうとしたファイルが、DB にあるか、録画やつなぐ処理で使っているか、書いたばかりであることを表す。
var ErrNotOrphan = errors.New("the file is in use or recently written")

// OrphanFile は、プールにあるのに DB に記録のないファイルか、持ち主の録画ファイルが DB にない生成物のディレクトリ (設計書 5.6)。
type OrphanFile struct {
	PoolID string
	Entry  *kago.Entry
}

// FindOrphans は、このインスタンスにマウントしたプールと、データディレクトリを調べ、DB にないファイルを、プールとパスの順に返す。
// 録画中のファイル、つないでいる途中のファイルとつなぐ元のファイル、書いたばかりのファイルは返さない。
func (l *Library) FindOrphans(ctx context.Context, db sqlc.DBTX) ([]*OrphanFile, error) {
	used, err := l.usedFiles(ctx, db)
	if err != nil {
		return nil, err
	}
	now := l.clock.Now()
	result := []*OrphanFile{}
	for _, poolID := range l.store.ScanPoolIDs() {
		entries, err := l.store.Scan(poolID)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if used.orphan(poolID, entry, now) {
				result = append(result, &OrphanFile{PoolID: poolID, Entry: entry})
			}
		}
	}

	return result, nil
}

// OrphanTarget は、消す、DB にないファイルか生成物のディレクトリ。FindOrphans が返したものを渡す。
type OrphanTarget struct {
	PoolID     string
	Path       string
	SegmentDir bool
}

// DeleteOrphansResult は、DeleteOrphans の結果。
type DeleteOrphansResult struct {
	// Deleted は、消したファイルとディレクトリの数。
	Deleted int
	// Skipped は、確かめ直すと、DB にあるか、使っているか、書いたばかりか、もうなかったので、消さなかった数。
	Skipped int
}

// DeleteOrphans は、FindOrphans が返したファイルか生成物のディレクトリを、確かめ直して消し、1 つずつ監査ログに残す。
func (l *Library) DeleteOrphans(ctx context.Context, db Database, actor *audit.Actor, targets []*OrphanTarget) (*DeleteOrphansResult, error) {
	result := &DeleteOrphansResult{Deleted: 0, Skipped: 0}
	used, err := l.usedFiles(ctx, db)
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		err := l.deleteOrphan(ctx, db, actor, used, target)
		switch {
		case errors.Is(err, ErrNotOrphan), errors.Is(err, ErrNotFound):
			result.Skipped++
		case err != nil:
			return nil, err
		default:
			result.Deleted++
		}
	}

	return result, nil
}

// deleteOrphan は、target を確かめ直して消す。DB にあるか、使っているか、書いたばかりなら ErrNotOrphan を、なければ ErrNotFound を返す。
func (l *Library) deleteOrphan(ctx context.Context, db Database, actor *audit.Actor, used *usedFiles, target *OrphanTarget) error {
	// 走査の対象でないもの (タイムシフトのバッファなど) は、DB にないのが正常なので、孤児として消さない。
	if !slices.Contains(l.store.ScanPoolIDs(), target.PoolID) {
		return fmt.Errorf("%w: pool %s is not scanned", ErrNotOrphan, target.PoolID)
	}
	entry, err := l.store.Inspect(target.PoolID, target.Path, target.SegmentDir)
	switch {
	case errors.Is(err, kago.ErrInvalidPath), errors.Is(err, kago.ErrPoolNotMounted):
		return fmt.Errorf("%w: %w", ErrNotOrphan, err)
	case errors.Is(err, fs.ErrNotExist):
		return ErrNotFound
	case err != nil:
		return err
	}
	if !used.orphan(target.PoolID, entry, l.clock.Now()) {
		return ErrNotOrphan
	}

	// 監査ログを書けなければ消さず、消せなければ監査ログを残さない。
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		err := l.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "storage_file.deleted",
			TargetType: orphanAuditTarget,
			TargetID:   target.PoolID,
			Summary:    map[string]any{"path": entry.Path, "segment_dir": entry.SegmentDir, "size": entry.Size},
			SourceIP:   netip.Addr{},
			UserAgent:  "",
		})
		if err != nil {
			return err
		}
		if err := l.store.RemoveEntry(target.PoolID, entry.Path, entry.SegmentDir); err != nil {
			return fmt.Errorf("failed to remove an orphan file: %w", err)
		}

		return nil
	})
}

// usedFiles は、DB にある録画ファイルと、つなぐ処理が使っているファイルの置き場所と ID。
// ID は、生成物のディレクトリの持ち主を確かめるのに使う。
type usedFiles struct {
	locations map[kago.Location]struct{}
	ids       map[uuid.UUID]struct{}
}

func (u *usedFiles) add(id uuid.UUID, poolID, path string) {
	u.locations[kago.Location{PoolID: poolID, Path: path}] = struct{}{}
	if id != uuid.Nil {
		u.ids[id] = struct{}{}
	}
}

// usedFiles は、DB にある録画ファイル (録画中のものも含む) と、終わっていないつなぐ処理が置き換えるファイルと消すファイルを返す。
func (l *Library) usedFiles(ctx context.Context, db sqlc.DBTX) (*usedFiles, error) {
	rows, err := sqlc.New(db).ListRecordingFileLocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recording files: %w", err)
	}
	used := &usedFiles{locations: map[kago.Location]struct{}{}, ids: map[uuid.UUID]struct{}{}}
	for _, row := range rows {
		used.add(row.ID, row.PoolID, row.Path)
	}
	// タイムシフトのチャンクの索引は、録画のセグメントと同じく、データディレクトリの「ID/」に置く。チャンクの行があるうちは使っているとみなす。
	// チャンクのファイルはタイムシフトのディレクトリにあり、プールとして調べないので、置き場所は足さない。
	chunks, err := sqlc.New(db).ListTimeshiftChunkIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the timeshift chunks: %w", err)
	}
	for _, chunk := range chunks {
		used.add(chunk.ID, "", "")
	}
	// つないだ後は、DB の記録はつないだファイルに置き換わるが、つなぐ元のファイルは、置き換えるジョブが消すまで残る。
	// ジョブは、マウントが戻るまで何日もやり直すことがあるので、終わっていないジョブのファイルは使っているとみなす。
	jobs, err := db.Query(ctx, `SELECT args FROM river_job WHERE kind = $1 AND state NOT IN ('completed', 'cancelled', 'discarded')`,
		new(segjoin.FinalizeArgs).Kind())
	if err != nil {
		return nil, fmt.Errorf("failed to list the join jobs: %w", err)
	}
	defer jobs.Close()
	for jobs.Next() {
		var raw []byte
		if err := jobs.Scan(&raw); err != nil {
			return nil, fmt.Errorf("failed to read a join job: %w", err)
		}
		var args segjoin.FinalizeArgs
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("failed to decode a join job: %w", err)
		}
		used.add(uuid.Nil, args.PoolID, args.Path)
		for _, removed := range args.Removed {
			used.add(removed.ID, removed.PoolID, removed.Path)
		}
	}
	if err := jobs.Err(); err != nil {
		return nil, fmt.Errorf("failed to list the join jobs: %w", err)
	}

	return used, nil
}

// orphan は、プール poolID の entry が、DB にも、つなぐ処理にもなく、書くか名前を変えてから orphanGrace が過ぎたかを返す。
// 録画のファイル名を付け直すときは、ファイルを移してから記録を書き換えるので、移したばかりのファイルは DB にないことがある。
func (u *usedFiles) orphan(poolID string, entry *kago.Entry, now time.Time) bool {
	if now.Sub(entry.ChangedAt) < orphanGrace {
		return false
	}
	if entry.SegmentDir {
		owner, err := kago.SegmentDirOwner(poolID, entry.Path)
		if err != nil {
			// 持ち主の分からないディレクトリは、消さないよう、使っているとみなす。
			return false
		}
		_, ok := u.ids[owner]

		return !ok
	}
	owner := &kago.Location{PoolID: poolID, Path: entry.Path}
	candidates := []kago.Location{*owner}
	// つないでいる途中のファイルは、つないだ後のファイル (最初のセグメント) の名前に接尾辞を付けたもの。
	if trimmed, ok := strings.CutSuffix(owner.Path, joiningSuffix); ok {
		candidates = append(candidates, kago.Location{PoolID: owner.PoolID, Path: trimmed})
	}

	return !slices.ContainsFunc(candidates, func(location kago.Location) bool {
		_, ok := u.locations[location]

		return ok
	})
}
