package epg

import (
	"context"
	"errors"
	"fmt"

	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

const (
	// searchGramsFillBatchSize は、番組の検索の gram の列を埋めるときに、1 回に読み書きする番組の数。
	searchGramsFillBatchSize = 1000
	// searchGramsFillLimit は、1 回の全量の突き合わせで gram を埋める番組の数の上限。番組とアーカイブで別に数える。
	// 列を足した直後は、アーカイブに数十万件の番組がある。全量の突き合わせのトランザクションを長く延ばさないよう、何回かに分けて埋める。
	searchGramsFillLimit = 5 * searchGramsFillBatchSize
)

// fillDerivedColumns は、Go で作る列 (シリーズ名と話数 (#1564)、番組の検索の gram (#966)) を埋めていない番組を埋める。
// 全量の突き合わせのときだけ呼ぶ。
func fillDerivedColumns(ctx context.Context, q *sqlc.Queries) error {
	if err := fillProgramSeries(ctx, q); err != nil {
		return err
	}

	return fillSearchGrams(ctx, q)
}

// fillSearchGrams は、番組の検索の gram の列がない番組とアーカイブの、gram を埋める (#966)。
// 列を足す前に書いた番組と、列を知らない古いバイナリがローリング更新の間に書いた番組、EIT[p/f] で番組名か概要を書き直した番組を、
// 全量の突き合わせのたびに searchGramsFillLimit 件ずつ埋める。埋めるまでの番組は、検索の候補に残るので、検索の結果は変わらない。
// 番組名も概要も空の番組には、空の配列を書き、読み直さないようにする。番組の中身は変わらないので、EPG の変更としては記録しない。
func fillSearchGrams(ctx context.Context, q *sqlc.Queries) error {
	for filled := 0; filled < searchGramsFillLimit; {
		rows, err := q.ListProgramsWithoutSearchGrams(ctx, searchGramsFillBatchSize)
		if err != nil {
			return fmt.Errorf("failed to list the programs without search grams: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		params := make([]sqlc.SetProgramSearchGramsParams, 0, len(rows))
		for i := range rows {
			params = append(params, sqlc.SetProgramSearchGramsParams{
				SearchGrams: match.SearchGrams(rows[i].NameNormalized, rows[i].DescriptionNormalized),
				ServiceID:   rows[i].ServiceID, EventID: rows[i].EventID,
			})
		}
		errs := []error{}
		q.SetProgramSearchGrams(ctx, params).Exec(func(i int, err error) {
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to set the search grams of program %d/%d: %w", params[i].ServiceID, params[i].EventID, err))
			}
		})
		if err := errors.Join(errs...); err != nil {
			return err
		}
		filled += len(rows)
	}

	return fillArchiveSearchGrams(ctx, q)
}

// fillArchiveSearchGrams は、番組の検索の gram の列がないアーカイブの、gram を searchGramsFillLimit 件まで埋める (#966)。
func fillArchiveSearchGrams(ctx context.Context, q *sqlc.Queries) error {
	for filled := 0; filled < searchGramsFillLimit; {
		rows, err := q.ListProgramArchivesWithoutSearchGrams(ctx, searchGramsFillBatchSize)
		if err != nil {
			return fmt.Errorf("failed to list the archived programs without search grams: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		params := make([]sqlc.SetProgramArchiveSearchGramsParams, 0, len(rows))
		for i := range rows {
			params = append(params, sqlc.SetProgramArchiveSearchGramsParams{
				SearchGrams: match.SearchGrams(rows[i].NameNormalized, rows[i].DescriptionNormalized),
				ServiceID:   rows[i].ServiceID, EventID: rows[i].EventID, StartAt: rows[i].StartAt,
			})
		}
		errs := []error{}
		q.SetProgramArchiveSearchGrams(ctx, params).Exec(func(i int, err error) {
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to set the search grams of archived program %d/%d: %w", params[i].ServiceID, params[i].EventID, err))
			}
		})
		if err := errors.Join(errs...); err != nil {
			return err
		}
		filled += len(rows)
	}

	return nil
}
