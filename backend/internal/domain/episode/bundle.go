package episode

import (
	"cmp"
	"slices"
	"time"
)

// Broadcast は、束ねる候補の放送。
type Broadcast struct {
	Episode   *Episode
	ServiceID int64
	EventID   int64
	StartAt   time.Time
	EndAt     time.Time
	// Captioned は、番組名に字幕付きの印があるかどうか。Satellite は、地上波でない局の放送かどうか。
	Captioned bool
	Satellite bool
	// Rejected は、割り当てで競合になり、録れない見込みの放送かどうか。
	Rejected bool
}

// Recorded は、録った回 (録画履歴か、録画中の録画)。
type Recorded struct {
	Episode *Episode
}

// Decision は、放送 1 本の扱い。どちらも nil なら、予約する。
type Decision struct {
	// Recorded は、録った回と同じ回なので外したときの、その録画。
	Recorded *Recorded
	// Instead は、同じ回を別の放送で録るので外したときの、その放送。
	Instead *Broadcast
}

// Resolve は、候補の放送を局をまたいで同じ回で束ね、束ごとに 1 本だけを残す (設計書 4.4)。
// 返す値は broadcasts と同じ並びで、それぞれの扱いを表す。
// 毎週同じ概要は、候補と録った回を合わせて見分けるので、broadcasts と recorded の回の指紋を書き換える。
func Resolve(broadcasts []*Broadcast, recorded []*Recorded) []*Decision {
	episodes := make([]*Episode, 0, len(broadcasts)+len(recorded))
	for _, b := range broadcasts {
		episodes = append(episodes, b.Episode)
	}
	for _, r := range recorded {
		episodes = append(episodes, r.Episode)
	}
	SuppressWeeklySummaries(episodes)

	decisions := make([]*Decision, len(broadcasts))
	for i := range decisions {
		decisions[i] = &Decision{Recorded: nil, Instead: nil}
	}
	known, unknown := bundleKnown(broadcasts)
	unknown = attachUnknown(broadcasts, known, unknown)
	for _, bundle := range known {
		if r := findRecorded(broadcasts, bundle.members, recorded); r != nil {
			for _, i := range bundle.members {
				decisions[i].Recorded = r
			}

			continue
		}
		pick(broadcasts, bundle.members, decisions)
	}
	for _, members := range bundleUnknown(broadcasts, unknown) {
		pick(broadcasts, members, decisions)
	}

	return decisions
}

// bundle は、同じ回の放送の束。members は broadcasts の添字。
type bundle struct {
	members []int
}

// bundleKnown は、回の分かる放送を束ね、束と、回の分からない放送の添字を返す。
func bundleKnown(broadcasts []*Broadcast) ([]*bundle, []int) {
	known, unknown := []int{}, []int{}
	for i, b := range broadcasts {
		if b.Episode.Known() {
			known = append(known, i)
		} else {
			unknown = append(unknown, i)
		}
	}

	// 確かに同じ組を推移的につなぐ。
	uf := newUnionFind(len(broadcasts))
	for x, i := range known {
		for _, j := range known[x+1:] {
			if certainlySame(broadcasts[i].Episode, broadcasts[j].Episode) {
				uf.union(i, j)
			}
		}
	}
	clusters := uf.groups(known)

	// どれか 1 組でも同じ回と判定される束どうしを相手とし、互いにつながる束の集まりが、どの 2 つも互いに相手のときだけまとめる。
	// 1 組でも相手でなければ、取り違えて録り逃すより 1 本多く録るほうを選び、どれもまとめない。
	adjacent := make([][]bool, len(clusters))
	for x := range clusters {
		adjacent[x] = make([]bool, len(clusters))
	}
	cuf := newUnionFind(len(clusters))
	for x := range clusters {
		for y := x + 1; y < len(clusters); y++ {
			if anySame(broadcasts, clusters[x], clusters[y]) {
				adjacent[x][y], adjacent[y][x] = true, true
				cuf.union(x, y)
			}
		}
	}
	indices := make([]int, len(clusters))
	for x := range indices {
		indices[x] = x
	}
	bundles := []*bundle{}
	for _, component := range cuf.groups(indices) {
		if isClique(component, adjacent) {
			members := []int{}
			for _, x := range component {
				members = append(members, clusters[x]...)
			}
			bundles = append(bundles, &bundle{members: members})

			continue
		}
		for _, x := range component {
			bundles = append(bundles, &bundle{members: clusters[x]})
		}
	}

	return bundles, unknown
}

func anySame(broadcasts []*Broadcast, a, b []int) bool {
	for _, i := range a {
		for _, j := range b {
			if Same(broadcasts[i].Episode, broadcasts[j].Episode) {
				return true
			}
		}
	}

	return false
}

func isClique(component []int, adjacent [][]bool) bool {
	for x, i := range component {
		for _, j := range component[x+1:] {
			if !adjacent[i][j] {
				return false
			}
		}
	}

	return true
}

// attachUnknown は、回の分からない放送を、時間が重なり、作品名が完全に一致する放送を持つ束が 1 つだけなら、その束に入れる。
// 番組名にも概要にも話数を載せずに、別の局が話数を付けて流す回と同じ時刻に流す局があるからである。
// どの束にも入れなかった放送の添字を返す。
func attachUnknown(broadcasts []*Broadcast, bundles []*bundle, unknown []int) []int {
	rest := []int{}
	for _, u := range unknown {
		ub := broadcasts[u]
		var target *bundle
		count := 0
		if ub.Episode.Work != "" && !ub.Episode.Special {
			for _, bu := range bundles {
				if slices.ContainsFunc(bu.members, func(i int) bool { return attachable(ub, broadcasts[i]) }) {
					target = bu
					count++
				}
			}
		}
		if count != 1 {
			rest = append(rest, u)

			continue
		}
		target.members = append(target.members, u)
	}

	return rest
}

func attachable(u, b *Broadcast) bool {
	return b.Episode.Known() && !b.Episode.Special && b.Episode.Work == u.Episode.Work && overlaps(u, b)
}

// bundleUnknown は、残った回の分からない放送を、飾りの印を除いて揃えた番組名ごとに分け、時間が重なるものだけを束ねる。
// 時刻をまたいでは束ねない。束ねると、毎日の「ニュース」が初回しか録れなくなる。2 本以上の束だけを返す。
// 間の放送を介してつながるだけで、どれかの組が重ならない束は、束ねない。回が分からないので、どれも録る。
func bundleUnknown(broadcasts []*Broadcast, unknown []int) [][]int {
	uf := newUnionFind(len(broadcasts))
	adjacent := make([][]bool, len(broadcasts))
	for i := range adjacent {
		adjacent[i] = make([]bool, len(broadcasts))
	}
	for x, i := range unknown {
		for _, j := range unknown[x+1:] {
			a, b := broadcasts[i], broadcasts[j]
			if a.Episode.Key != "" && a.Episode.Key == b.Episode.Key && overlaps(a, b) {
				uf.union(i, j)
				adjacent[i][j], adjacent[j][i] = true, true
			}
		}
	}
	result := [][]int{}
	for _, members := range uf.groups(unknown) {
		if len(members) >= 2 && isClique(members, adjacent) {
			result = append(result, members)
		}
	}

	return result
}

func overlaps(a, b *Broadcast) bool {
	return a.StartAt.Before(b.EndAt) && b.StartAt.Before(a.EndAt)
}

// findRecorded は、束の回の分かる放送のどれかと同じ回の、録った回を返す。無ければ nil。
func findRecorded(broadcasts []*Broadcast, members []int, recorded []*Recorded) *Recorded {
	for _, i := range members {
		ep := broadcasts[i].Episode
		if !ep.Known() {
			continue
		}
		for _, r := range recorded {
			if r.Episode.Known() && Same(ep, r.Episode) {
				return r
			}
		}
	}

	return nil
}

// pick は、束から 1 本を選び、選ばなかった放送を「別の放送で録る」として外す。
// 選んだ放送が競合で録れない見込みなら、それも予約したまま残し、残りから同じ手順でもう 1 本選ぶ (空けば録れるからである)。
func pick(broadcasts []*Broadcast, members []int, decisions []*Decision) {
	if len(members) < 2 {
		return
	}
	remaining := slices.Clone(members)
	slices.SortFunc(remaining, func(i, j int) int { return compareStart(broadcasts[i], broadcasts[j]) })
	var chosen *Broadcast
	for len(remaining) > 0 {
		first := broadcasts[remaining[0]]
		best := -1
		for x, i := range remaining {
			if x > 0 && !overlaps(first, broadcasts[i]) {
				continue
			}
			if best < 0 || comparePreference(broadcasts[i], broadcasts[remaining[best]]) < 0 {
				best = x
			}
		}
		chosen = broadcasts[remaining[best]]
		remaining = slices.Delete(remaining, best, best+1)
		if !chosen.Rejected {
			break
		}
	}
	for _, i := range remaining {
		decisions[i].Instead = chosen
	}
}

func compareStart(a, b *Broadcast) int {
	return cmp.Or(a.StartAt.Compare(b.StartAt), cmp.Compare(a.ServiceID, b.ServiceID), cmp.Compare(a.EventID, b.EventID))
}

// comparePreference は、一番早い放送と時間が重なる放送から 1 本を選ぶ順 (字幕付き、衛星、開始の早さ、サービス ID、番組 ID)。
func comparePreference(a, b *Broadcast) int {
	return cmp.Or(compareTrue(a.Captioned, b.Captioned), compareTrue(a.Satellite, b.Satellite), compareStart(a, b))
}

// compareTrue は、true を先にする。
func compareTrue(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	default:
		return 1
	}
}

// unionFind は、素集合。
type unionFind struct {
	parent []int
}

func newUnionFind(n int) *unionFind {
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}

	return &unionFind{parent: parent}
}

func (u *unionFind) find(i int) int {
	for u.parent[i] != i {
		u.parent[i] = u.parent[u.parent[i]]
		i = u.parent[i]
	}

	return i
}

func (u *unionFind) union(i, j int) {
	ri, rj := u.find(i), u.find(j)
	if ri != rj {
		u.parent[max(ri, rj)] = min(ri, rj)
	}
}

// groups は、members を同じ集合ごとに分ける。集合の並びと集合の中の並びは、members での最初の出現の順。
func (u *unionFind) groups(members []int) [][]int {
	index := map[int]int{}
	result := [][]int{}
	for _, i := range members {
		root := u.find(i)
		x, ok := index[root]
		if !ok {
			x = len(result)
			index[root] = x
			result = append(result, []int{})
		}
		result[x] = append(result[x], i)
	}

	return result
}
