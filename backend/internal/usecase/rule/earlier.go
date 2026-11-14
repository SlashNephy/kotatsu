package rule

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// skipEarlierEpisodes は、前の回を録らないルールに一致した放送のうち、追っているシリーズの前の回を外す (設計書 4.4、#1499)。
//
// 比べる相手は、録った回 (シリーズ名が一致する録画履歴の全部と、録画中の録画)、予約してある回、弾かれていない候補の放送である。
// 相手のうち、シリーズ名が完全に一致し、開始がその放送より前で、話数がその放送より大きいものが 1 つでもあれば外す。
// 外した放送からは、この設定を入れたルールだけを外す。どのルールにも一致しなくなった放送は、予約すべき集合から除き、
// skipped が nil でなければ、外したルールと理由と一緒に入れる。bundled にある (同じ回を束ねて一部のルールを外した) 放送は、
// 束ねた理由を優先して出す。
// removed は、この照合で消えるルール予約の番組。消える予約は、比べる相手にしない (#1724)。
func skipEarlierEpisodes(
	ctx context.Context, tx pgx.Tx, now time.Time, matched map[programKey]*desiredProgram, skipped, bundled map[programKey]*skippedProgram,
	removed map[programKey]bool,
) error {
	keys := []programKey{}
	for key, d := range matched {
		if slices.ContainsFunc(d.rules, func(r *compiledRule) bool { return r.SkipEarlierEpisodes }) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	// 人が取り消した (無効にした) 予約の番組は、外しもせず、比べる相手にもしない。外すと取り消しの記録 (無効の予約) を消してしまう。
	cancelled, err := cancelledPrograms(ctx, tx, now)
	if err != nil {
		return err
	}
	keys = slices.DeleteFunc(keys, func(key programKey) bool { return cancelled[key] })
	if len(keys) == 0 {
		return nil
	}
	slices.SortFunc(keys, compareKeys)

	candidates := make(map[programKey]*episode.Aired, len(keys))
	series := map[string]bool{}
	for _, key := range keys {
		row := matched[key].row
		aired := &episode.Aired{Episode: episode.Parse(programSource(&row)), StartAt: row.StartAt}
		candidates[key] = aired
		if aired.Episode.Series != "" && aired.Episode.Number > 0 {
			series[aired.Episode.Series] = true
		}
	}
	if len(series) == 0 {
		return nil
	}

	opponents, recorded, err := listEarlierOpponents(ctx, tx, now, candidates, slices.Sorted(maps.Keys(series)), removed)
	if err != nil {
		return err
	}

	for _, key := range keys {
		target := candidates[key]
		later := episode.LaterEpisode(target, opponents[target.Episode.Series])
		if later == 0 {
			continue
		}
		d := matched[key]
		removed := []*compiledRule{}
		d.rules = slices.DeleteFunc(d.rules, func(r *compiledRule) bool {
			if r.SkipEarlierEpisodes {
				removed = append(removed, r)
			}

			return r.SkipEarlierEpisodes
		})
		if len(d.rules) > 0 {
			// この設定を入れていない別のルールも一致していれば、そのルールが受け持って予約する。
			continue
		}
		delete(matched, key)
		if skipped == nil {
			continue
		}
		if s := bundled[key]; s != nil {
			s.rules = append(s.rules, removed...)
			skipped[key] = s

			continue
		}
		skipped[key] = &skippedProgram{
			row: d.row, rules: removed, reason: SkipReasonEarlierEpisode, lastRecorded: nil, recorded: nil, instead: nil,
			laterEpisode: later, byRecording: episode.LaterEpisode(target, recorded[target.Episode.Series]) > 0,
		}
	}

	return nil
}

// listEarlierOpponents は、前の回かどうかを比べる相手を、シリーズ名ごとに返す。2 つ目の値は、そのうちの録った回 (録画) だけ。
// 相手は、弾かれていない (競合でない) 候補の放送、シリーズ名が series のどれかの録った回、予約してある回である。
func listEarlierOpponents(
	ctx context.Context, tx pgx.Tx, now time.Time, candidates map[programKey]*episode.Aired, series []string,
	removed map[programKey]bool,
) (map[string][]*episode.Aired, map[string][]*episode.Aired, error) {
	rejected, err := conflictingPrograms(ctx, tx, now)
	if err != nil {
		return nil, nil, err
	}
	opponents := map[string][]*episode.Aired{}
	add := func(to map[string][]*episode.Aired, a *episode.Aired) {
		to[a.Episode.Series] = append(to[a.Episode.Series], a)
	}
	for key, c := range candidates {
		if !rejected[key] {
			add(opponents, c)
		}
	}

	q := sqlc.New(tx)
	recordedRows, err := q.ListRecordedEpisodesBySeries(ctx, series)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list the recorded episodes by series: %w", err)
	}
	recorded := map[string][]*episode.Aired{}
	for _, row := range recordedRows {
		a := airedOf(row.Name, row.Description, row.SeriesEpisode, row.StartAt)
		add(opponents, a)
		add(recorded, a)
	}

	reservedRows, err := q.ListReservedEpisodes(ctx, now)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list the reserved episodes: %w", err)
	}
	for _, row := range reservedRows {
		if row.EventID != nil && removed[programKey{serviceID: row.ServiceID, eventID: *row.EventID}] {
			continue
		}
		add(opponents, airedOf(row.Name, row.Description, row.SeriesEpisode, row.StartAt))
	}

	return opponents, recorded, nil
}

// airedOf は、録画か予約の番組名、概要、記述子の話数から回を読む。
func airedOf(name, description string, seriesEpisode *int32, startAt time.Time) *episode.Aired {
	src := &episode.Source{Name: name, Description: description, Extended: "", SeriesEpisode: seriesEpisode, Anime: nil}

	return &episode.Aired{Episode: episode.Parse(src), StartAt: startAt}
}
