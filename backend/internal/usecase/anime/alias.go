package anime

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// titleLink は、番組名の作品名に対応する作品 (TID) と、話数のずれ (しょぼいカレンダーの話数から番組名の話数を引いたもの)。
type titleLink struct {
	tid    int32
	offset float64
}

// titleAlias は、作品名の対応 (anime_title_aliases) の 1 行。
type titleAlias struct {
	key    string
	tid    int32
	offset float64
}

// titleOf は、番組名 name から、作品名と話数を読む。番組名に話数が書かれていなければ、シリーズの記述子の話数 seriesEpisode を使い (#1311)、
// マークを除いた番組名を作品名とする。どちらにも話数がなければ、ok は false。
func titleOf(name string, seriesEpisode *int32) (*ParsedTitle, bool) {
	if parsed, ok := ParseTitle(name); ok {
		return parsed, true
	}
	if seriesEpisode == nil || *seriesEpisode <= 0 {
		return nil, false
	}
	title := stripMarks(name)
	if title == "" {
		return nil, false
	}

	return &ParsedTitle{Title: title, CountStart: float64(*seriesEpisode), CountEnd: float64(*seriesEpisode), Subtitle: ""}, true
}

// titleKeyOfName は、番組名の解析で結びつけた番組の番組名 name から、作品名のキーを返す。
// 番組名に話数が書かれていなければ、シリーズの記述子の話数で結びつけたものなので、マークを除いた番組名を作品名とする。
func titleKeyOfName(name string) string {
	if parsed, ok := ParseTitle(name); ok {
		return TitleKey(parsed.Title)
	}

	return TitleKey(stripMarks(name))
}

// resolveTitle は、作品名のキー key に対応する作品と話数のずれを返す。決められなければ ok は false (設計書 3.8)。
// しょぼいカレンダーで結びついた番組の作品名の対応 (anime_title_aliases) を先に使い、1 つだけなら、その作品と話数のずれを使う。
// 作品か話数のずれが食い違う対応が 2 つ以上あれば、どれか決められないので、タイトルの一覧も引かない。
// 対応がなければ、しょぼいカレンダーのタイトルの一覧から、作品名が 1 つのタイトルだけに一致すれば、その作品を、話数をずらさずに使う。
func resolveTitle(ctx context.Context, q *sqlc.Queries, key string) (titleLink, bool, error) {
	if key == "" {
		return titleLink{tid: 0, offset: 0}, false, nil
	}
	aliases, err := q.ListAnimeTitleAliases(ctx, key)
	if err != nil {
		return titleLink{tid: 0, offset: 0}, false, fmt.Errorf("failed to look up the anime title alias: %w", err)
	}
	switch len(aliases) {
	case 0:
	case 1:
		return titleLink{tid: aliases[0].SyobocalTid, offset: aliases[0].CountOffset}, true, nil
	default:
		return titleLink{tid: 0, offset: 0}, false, nil
	}
	titles, err := q.ListSyobocalTitlesByKey(ctx, key)
	if err != nil {
		return titleLink{tid: 0, offset: 0}, false, fmt.Errorf("failed to look up the syobocal title: %w", err)
	}
	// 同じ作品名のタイトルが複数あれば、どれか決められないので、TID は付けない。
	if len(titles) != 1 {
		return titleLink{tid: 0, offset: 0}, false, nil
	}

	return titleLink{tid: titles[0].Tid, offset: 0}, true, nil
}

// shiftCount は、話数 count を offset だけずらす。count が nil なら nil を返す。
func shiftCount(count *float64, offset float64) *float64 {
	if count == nil {
		return nil
	}

	return new(*count + offset)
}

// rebuildTitleAliases は、作品名の対応 (anime_title_aliases) を、しょぼいカレンダーで 1 つの話だけに結びついた、
// 番組表と過去の番組の番組から作り直し、作った対応の数を返す (設計書 3.8)。番組名から作品名と話数を読めない番組と、
// 複数の話を書いた番組名 (#1-#2 など) の番組は使わない。
func (m *Matcher) rebuildTitleAliases(ctx context.Context) (int, error) {
	rows, err := sqlc.New(m.db).ListAnimeTitleAliasSources(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list the programs for the anime title aliases: %w", err)
	}
	found := map[titleAlias]bool{}
	for _, row := range rows {
		parsed, ok := titleOf(row.Name, row.SeriesEpisode)
		if !ok || parsed.CountStart != parsed.CountEnd {
			continue
		}
		key := TitleKey(parsed.Title)
		if key == "" {
			continue
		}
		found[titleAlias{key: key, tid: row.SyobocalTid, offset: row.CountStart - parsed.CountStart}] = true
	}
	aliases := slices.SortedFunc(maps.Keys(found), func(a, b titleAlias) int {
		return cmp.Or(cmp.Compare(a.key, b.key), cmp.Compare(a.tid, b.tid), cmp.Compare(a.offset, b.offset))
	})
	params := sqlc.InsertAnimeTitleAliasesParams{
		TitleKeys: make([]string, 0, len(aliases)), SyobocalTids: make([]int32, 0, len(aliases)), CountOffsets: make([]float64, 0, len(aliases)),
	}
	for _, alias := range aliases {
		params.TitleKeys = append(params.TitleKeys, alias.key)
		params.SyobocalTids = append(params.SyobocalTids, alias.tid)
		params.CountOffsets = append(params.CountOffsets, alias.offset)
	}
	if err := pgx.BeginFunc(ctx, m.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.DeleteAnimeTitleAliases(ctx); err != nil {
			return fmt.Errorf("failed to delete the anime title aliases: %w", err)
		}
		if err := q.InsertAnimeTitleAliases(ctx, params); err != nil {
			return fmt.Errorf("failed to save the anime title aliases: %w", err)
		}

		return nil
	}); err != nil {
		return 0, err
	}

	return len(aliases), nil
}

// relinkTitles は、番組名の解析で結びつけ、TID の付かなかった番組と録画の話を、今の作品名の対応とタイトルの一覧で引き直し、
// 作品に結びつけた話の数を返す (設計書 3.8)。確定させた番組と、アーカイブへ移した番組と、その録画も引き直す。
// リピート放送は、しょぼいカレンダーで結びつく本放送より前に放送されることもあり、照合したときにはまだ対応がないことがあるためである。
func (m *Matcher) relinkTitles(ctx context.Context) (int, error) {
	q := sqlc.New(m.db)
	// resolved は、作品名のキーごとの、引いた結果。決められなかったキーは nil で持つ。
	resolved := map[string]*titleLink{}
	resolve := func(name string) (*titleLink, error) {
		key := titleKeyOfName(name)
		if link, ok := resolved[key]; ok {
			return link, nil
		}
		link, ok, err := resolveTitle(ctx, q, key)
		if err != nil {
			return nil, err
		}
		resolved[key] = nil
		if ok {
			resolved[key] = &link
		}

		return resolved[key], nil
	}

	relinked := 0
	programs, err := q.ListUnlinkedProgramTitleEpisodes(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list the unlinked program episodes: %w", err)
	}
	for _, row := range programs {
		link, err := resolve(row.Name)
		if err != nil {
			return 0, err
		}
		if link == nil {
			continue
		}
		if err := q.LinkProgramTitleEpisode(ctx, sqlc.LinkProgramTitleEpisodeParams{
			SyobocalTid: new(link.tid), CountStart: shiftCount(row.CountStart, link.offset), CountEnd: shiftCount(row.CountEnd, link.offset),
			ServiceID: row.ServiceID, StartAt: row.StartAt, Position: row.Position,
		}); err != nil {
			return 0, fmt.Errorf("failed to link a program episode: %w", err)
		}
		relinked++
	}
	recordings, err := q.ListUnlinkedRecordingTitleEpisodes(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list the unlinked recording episodes: %w", err)
	}
	for _, row := range recordings {
		link, err := resolve(row.Name)
		if err != nil {
			return 0, err
		}
		if link == nil {
			continue
		}
		if err := q.LinkRecordingTitleEpisode(ctx, sqlc.LinkRecordingTitleEpisodeParams{
			SyobocalTid: new(link.tid), CountStart: shiftCount(row.CountStart, link.offset), CountEnd: shiftCount(row.CountEnd, link.offset),
			RecordingID: row.RecordingID, Position: row.Position,
		}); err != nil {
			return 0, fmt.Errorf("failed to link a recording episode: %w", err)
		}
		relinked++
	}

	return relinked, nil
}
