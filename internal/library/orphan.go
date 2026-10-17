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

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/segjoin"
)

const (
	// orphanGrace は、DB にないファイルとみなすまでに、最後に書いてから空ける時間。
	// 録画はファイルを作ってから DB に記録し、サムネイルは一時ファイルに書いてから名前を変えるので、書いたばかりのファイルは DB にないことがある。
	orphanGrace = time.Hour
	// orphanAuditTarget は、DB にないファイルを消したときの監査ログの対象の種類。
	orphanAuditTarget = "storage_file"
)

// ErrNotOrphan は、消そうとしたファイルが、DB にあるか、録画やつなぐ処理で使っているか、書いたばかりであることを表す。
var ErrNotOrphan = errors.New("the file is in use or recently written")

// OrphanFile は、プールにあるのに DB に記録のないファイルか、持ち主の録画ファイルが DB にない付属ファイルのディレクトリ (設計書 5.6)。
type OrphanFile struct {
	PoolID string
	Entry  *kago.Entry
}

// FindOrphans は、このインスタンスにマウントしたプールと、付属ファイルをまとめて置くディレクトリを調べ、DB にないファイルを、プールとパスの順に返す。
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

// DeleteOrphan は、FindOrphans が返したファイルか付属ファイルのディレクトリを確かめ直して消し、監査ログに残す。
// DB にあるか、使っているか、書いたばかりなら ErrNotOrphan を、なければ ErrNotFound を返す。
func (l *Library) DeleteOrphan(ctx context.Context, db Database, actor *audit.Actor, poolID, path string, sidecar bool) error {
	entry, err := l.store.Inspect(poolID, path, sidecar)
	switch {
	case errors.Is(err, kago.ErrInvalidPath), errors.Is(err, kago.ErrPoolNotMounted):
		return fmt.Errorf("%w: %w", ErrNotOrphan, err)
	case errors.Is(err, fs.ErrNotExist):
		return ErrNotFound
	case err != nil:
		return err
	}
	used, err := l.usedFiles(ctx, db)
	if err != nil {
		return err
	}
	if !used.orphan(poolID, entry, l.clock.Now()) {
		return ErrNotOrphan
	}

	// 監査ログを書けなければ消さず、消せなければ監査ログを残さない。
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		err := l.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "storage_file.deleted",
			TargetType: orphanAuditTarget,
			TargetID:   poolID,
			Summary:    map[string]any{"path": entry.Path, "sidecar": entry.Sidecar, "size": entry.Size},
			SourceIP:   netip.Addr{},
			UserAgent:  "",
		})
		if err != nil {
			return err
		}
		if err := l.store.RemoveEntry(poolID, entry.Path, entry.Sidecar); err != nil {
			return fmt.Errorf("failed to remove an orphan file: %w", err)
		}

		return nil
	})
}

// usedFiles は、DB にある録画ファイルと、つなぐ処理が使っているファイルの置き場所。
type usedFiles map[kago.Location]struct{}

// usedFiles は、DB にある録画ファイル (録画中のものも含む) と、終わっていないつなぐ処理が置き換えるファイルと消すファイルを返す。
func (l *Library) usedFiles(ctx context.Context, db sqlc.DBTX) (usedFiles, error) {
	rows, err := sqlc.New(db).ListRecordingFileLocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recording files: %w", err)
	}
	used := usedFiles{}
	for _, row := range rows {
		used[kago.Location{PoolID: row.PoolID, Path: row.Path}] = struct{}{}
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
		used[kago.Location{PoolID: args.PoolID, Path: args.Path}] = struct{}{}
		for _, removed := range args.Removed {
			used[kago.Location{PoolID: removed.PoolID, Path: removed.Path}] = struct{}{}
		}
	}
	if err := jobs.Err(); err != nil {
		return nil, fmt.Errorf("failed to list the join jobs: %w", err)
	}

	return used, nil
}

// orphan は、プール poolID の entry が、DB にも、つなぐ処理にもなく、書いてから orphanGrace が過ぎたかを返す。
func (u usedFiles) orphan(poolID string, entry *kago.Entry, now time.Time) bool {
	if now.Sub(entry.ModifiedAt) < orphanGrace {
		return false
	}
	owner := &kago.Location{PoolID: poolID, Path: entry.Path}
	if entry.Sidecar {
		var err error
		if owner, err = kago.SidecarOwner(poolID, entry.Path); err != nil {
			// 持ち主の分からないディレクトリは、消さないよう、使っているとみなす。
			return false
		}
	}
	candidates := []kago.Location{*owner}
	// つないでいる途中のファイルは、つないだ後のファイル (最初のセグメント) の名前に接尾辞を付けたもの。
	if trimmed, ok := strings.CutSuffix(owner.Path, joiningSuffix); ok {
		candidates = append(candidates, kago.Location{PoolID: owner.PoolID, Path: trimmed})
	}

	return !slices.ContainsFunc(candidates, func(location kago.Location) bool {
		_, ok := u[location]

		return ok
	})
}
