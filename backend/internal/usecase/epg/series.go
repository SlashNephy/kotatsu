package epg

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// seriesFillBatchSize は、シリーズ名と話数のカラムを埋めるときに、1 回に読み書きする番組の数。
const seriesFillBatchSize = 1000

// episodeSeriesOf は、番組を回の読み方 (設計書 4.4) で読み、シリーズ名と話数を返す (#1564)。
// シリーズ名を読めなければ空を、シリーズ名か話数を読めなければ話数に nil を返す。続きの放送は、シリーズ名と話数の両方で検索するからである。
// extendedNormalized は、正規化した詳細。概要が空のときだけ、概要の代わりに読む。
func episodeSeriesOf(name, description, extendedNormalized string, seriesEpisode *int32) (string, *int32) {
	ep := episode.Parse(&episode.Source{
		Name: name, Description: description, Extended: extendedNormalized, SeriesEpisode: seriesEpisode, Anime: nil,
	})
	// 回の読み方は MaxNumberDigits 桁までしか読まないので、上限を超えることはないが、カラムに収まらない値は書かない。
	if ep.Series == "" || ep.Number <= 0 || ep.Number > math.MaxInt32 {
		return ep.Series, nil
	}

	return ep.Series, new(int32(ep.Number))
}

// fillProgramSeries は、シリーズ名のカラムがない番組の、シリーズ名と話数のカラムを埋める (#1564)。
// カラムを足す前に書いた番組と、カラムを知らない古いバイナリがローリング更新の間に書いた番組を、次の全量の照合で埋める。
// 番組の内容は変わらないので、EPG の変更としては記録しない。シリーズ名を読めない番組には空を書き、読み直さないようにする。
func fillProgramSeries(ctx context.Context, q *sqlc.Queries) error {
	for {
		rows, err := q.ListProgramsWithoutSeries(ctx, seriesFillBatchSize)
		if err != nil {
			return fmt.Errorf("failed to list the programs without series: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		params := make([]sqlc.SetProgramSeriesParams, 0, len(rows))
		for i := range rows {
			extended := ""
			if rows[i].ExtendedNormalized != nil {
				extended = *rows[i].ExtendedNormalized
			}
			series, number := episodeSeriesOf(rows[i].Name, rows[i].DescriptionNormalized, extended, rows[i].SeriesEpisode)
			params = append(params, sqlc.SetProgramSeriesParams{
				Series: series, EpisodeNumber: number, ServiceID: rows[i].ServiceID, EventID: rows[i].EventID,
			})
		}
		errs := []error{}
		q.SetProgramSeries(ctx, params).Exec(func(i int, err error) {
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to set the series of program %d/%d: %w", params[i].ServiceID, params[i].EventID, err))
			}
		})
		if err := errors.Join(errs...); err != nil {
			return err
		}
	}
}
