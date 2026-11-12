package episode

import "time"

// SeriesOf は、番組名から読んだシリーズ名 (作品名と編と期を揃えたもの) を返す。読めなければ空。
// シリーズ名は番組名だけから読むので、概要や記述子の話数は要らない。
func SeriesOf(name string) string {
	return Parse(&Source{Name: name, Description: "", Extended: "", SeriesEpisode: nil, Anime: nil}).Series
}

// Aired は、前の回を録らない (#1499) ときに比べる、始まりの時刻の付いた回 (放送、予約、録画)。
type Aired struct {
	Episode *Episode
	StartAt time.Time
}

// numbered は、シリーズ名と話数が読めたかどうかを返す。どちらかが読めない回は、外しもせず、比べる相手にもしない。
func (a *Aired) numbered() bool {
	return a.Episode.Series != "" && a.Episode.Number > 0
}

// LaterEpisode は、target より前に始まり、シリーズ名が完全に一致し、話数が target より大きい相手のうち、一番大きい話数を返す。
// そうした相手が無いか、target のシリーズ名か話数が読めなければ 0 を返す (設計書 4.4、#1499)。
//
// シリーズ名は、局ごとの飾りの違いで別の期や編の回を前の回とみないよう、完全一致だけで比べる (SameSeries のように緩めない)。
// 開始で絞るのは、番組表に並ぶ翌週の 6 話で、今週の 5 話を外さないためである。
func LaterEpisode(target *Aired, opponents []*Aired) int {
	if !target.numbered() {
		return 0
	}
	later := 0
	for _, o := range opponents {
		if !o.numbered() || o.Episode.Series != target.Episode.Series || !o.StartAt.Before(target.StartAt) {
			continue
		}
		if o.Episode.Number > target.Episode.Number {
			later = max(later, o.Episode.Number)
		}
	}

	return later
}
