// Package keyrebuild は、Go で番組名から計算して DB に保存したキーを、今のバイナリの計算に合わせて作り直す (#1977、#1978、設計書 11.4)。
//
// キーの計算を直すと、保存したキーと、同じ番組名から今計算したキーが食い違い、重複の回避やドラマの上書きが効かなくなる。
// kotatsu migrate (とバックアップを戻すとき) に、マイグレーションの後で呼ぶ。保存したキーと今のキーが違う行だけを書き直すので、
// 何度呼んでも、2 回目からは何も変えない。キーを知らない古いバイナリが、ローリング更新の間に古いキーで書いた行も、次の migrate で直す。
package keyrebuild

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/drama"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

// batchSize は、1 回に読む行の数。
const batchSize = 500

// Run は、録画履歴のキーと、ドラマの作品名の鍵を作り直す。
func Run(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	if err := rebuildHistoryTitleKeys(ctx, logger, pool); err != nil {
		return err
	}

	return rebuildDramaTitleKeys(ctx, logger, pool)
}

// rebuildHistoryTitleKeys は、録画履歴の重複の回避のキー (title_key) を、番組名から計算し直したキー (rule.TitleKey) に書き直す (#1977)。
func rebuildHistoryTitleKeys(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	q := sqlc.New(pool)
	after := uuid.Nil()
	rebuilt := 0
	for {
		rows, err := q.ListRecordingHistoryTitleKeys(ctx, sqlc.ListRecordingHistoryTitleKeysParams{After: after, MaxCount: batchSize})
		if err != nil {
			return fmt.Errorf("failed to list the recording history title keys: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		after = rows[len(rows)-1].ID
		params := []sqlc.SetRecordingHistoryTitleKeyParams{}
		for _, row := range rows {
			if key := rule.TitleKey(row.Name); key != row.TitleKey {
				params = append(params, sqlc.SetRecordingHistoryTitleKeyParams{ID: row.ID, TitleKey: key})
			}
		}
		if len(params) == 0 {
			continue
		}
		errs := []error{}
		q.SetRecordingHistoryTitleKey(ctx, params).Exec(func(i int, err error) {
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to set the title key of recording history %s: %w", params[i].ID, err))
			}
		})
		if err := errors.Join(errs...); err != nil {
			return err
		}
		rebuilt += len(params)
	}
	if rebuilt > 0 {
		logger.InfoContext(ctx, "recording history title keys rebuilt", slog.Int("count", rebuilt))
	}

	return nil
}

// dramaRekey は、作品名の鍵を書き直す、番組の結びつき。
type dramaRekey struct {
	serviceID int64
	startAt   time.Time
	from, to  string
}

// rebuildDramaTitleKeys は、番組の結びつきの作品名の鍵 (program_drama_matches.title_key) を、番組名から計算し直した鍵
// (drama.TitleKeyOf) に書き直す (#1978)。
//
// 上書き (drama_title_overrides) は作品名の鍵だけを持ち、番組名を持たないので、結びつきの前の鍵と今の鍵の対応から書き直す。
// 前の鍵に上書きがあれば、今の鍵に写す。今の鍵に上書きがあれば、そちらを残す。1 つの前の鍵が、いくつもの今の鍵に分かれることもある
// (作品名 season 1. と season 2. が、どちらも「作品名 season」だったものなど)。写した後、前の鍵の結びつきが残っていなければ、前の鍵の上書きを消す。
// 書き直した結びつきは確定を外し、次の照合で、今の鍵の上書きで照合し直させる。
// 番組表にもアーカイブにもない番組の結びつきは、番組名が分からないので書き直さない。その番組だけで使っていた上書きは、前の鍵のまま残る。
func rebuildDramaTitleKeys(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	rekeys, err := listDramaRekeys(ctx, pool)
	if err != nil {
		return err
	}
	if len(rekeys) == 0 {
		return nil
	}

	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		fromKeys := []string{}
		for _, r := range rekeys {
			if err := q.RekeyProgramDramaMatch(ctx, sqlc.RekeyProgramDramaMatchParams{TitleKey: r.to, ServiceID: r.serviceID, StartAt: r.startAt}); err != nil {
				return fmt.Errorf("failed to rekey the drama match of service %d at %s: %w", r.serviceID, r.startAt, err)
			}
			if err := q.CopyDramaTitleOverride(ctx, sqlc.CopyDramaTitleOverrideParams{ToKey: r.to, FromKey: r.from}); err != nil {
				return fmt.Errorf("failed to copy the drama title override: %w", err)
			}
			if !slices.Contains(fromKeys, r.from) {
				fromKeys = append(fromKeys, r.from)
			}
		}
		for _, key := range fromKeys {
			if err := q.DeleteUnusedDramaTitleOverride(ctx, key); err != nil {
				return fmt.Errorf("failed to delete the unused drama title override: %w", err)
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to rebuild the drama title keys: %w", err)
	}
	logger.InfoContext(ctx, "drama title keys rebuilt", slog.Int("count", len(rekeys)))

	return nil
}

// listDramaRekeys は、保存した作品名の鍵が、番組名から今計算した鍵と違う、番組の結びつきを返す。
func listDramaRekeys(ctx context.Context, pool *pgxpool.Pool) ([]*dramaRekey, error) {
	q := sqlc.New(pool)
	params := sqlc.ListProgramDramaMatchTitleKeysParams{AfterServiceID: math.MinInt64, AfterStartAt: time.Time{}, MaxCount: batchSize}
	rekeys := []*dramaRekey{}
	for {
		rows, err := q.ListProgramDramaMatchTitleKeys(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("failed to list the drama match title keys: %w", err)
		}
		if len(rows) == 0 {
			return rekeys, nil
		}
		last := rows[len(rows)-1]
		params.AfterServiceID, params.AfterStartAt = last.ServiceID, last.StartAt
		for _, row := range rows {
			if key := drama.TitleKeyOf(row.Name); key != row.TitleKey {
				rekeys = append(rekeys, &dramaRekey{serviceID: row.ServiceID, startAt: row.StartAt, from: row.TitleKey, to: key})
			}
		}
	}
}
