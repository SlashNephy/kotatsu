package dramalibrary

import (
	"fmt"

	"github.com/SlashNephy/kotatsu/internal/infra/tmdb"
)

// NextEpisode は、TMDb のキャッシュ (作品のシーズンの一覧 seasons と、シーズンごとの話の番号 episodes) で、作品の話
// (season, number) の次に見る話を求め、"シーズン:話" の形で返す。次の話がなければ "-" を返す。
func NextEpisode(seasons []*tmdb.Season, episodes map[int32][]int32, season, number int32) string {
	const series = 1
	c := &cache{series: nil, seasons: map[int32][]*tmdb.Season{series: seasons}, episodes: map[seasonKey][]*tmdb.Episode{}}
	for s, numbers := range episodes {
		for _, n := range numbers {
			c.episodes[seasonKey{series: series, season: s}] = append(c.episodes[seasonKey{series: series, season: s}],
				&tmdb.Episode{EpisodeNumber: n, AirDate: "", Name: ""})
		}
	}
	next := c.nextEpisode(series, season, number)
	if next == nil {
		return "-"
	}

	return fmt.Sprintf("%d:%d", next.Season, next.Number)
}
