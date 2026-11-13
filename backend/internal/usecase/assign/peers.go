package assign

import (
	"cmp"
	"slices"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

// Peers は、競合か一部のみになった予約ごとに、チューナーを取り合っている相手の予約の ID を返す (#1502、設計書 4.5)。
// assignments は、Assign が reservations に返した結果で、戻り値も reservations と同じ順に並べる。
//
// 相手は、次のすべてを満たす予約である。画面に出す相手とスケジューラの判断が食い違わないよう、割り当ての結果から求める。
//   - その予約の種別を受けられるチューナーに割り当てられている
//   - 区間 (マージン込み) が、相手の録画する予定の時刻と重なる
//   - 物理チャンネルが違う。同じチャンネルは 1 本のチューナーを共有するので、諦めても何も空かない
//
// 相手は、録画する予定の始まりの早い順に並べ、同じなら予約 ID の順にする。正常な予約と、相手のない予約には空の並びを返す。
func Peers(reservations []*Reservation, tuners []*Tuner, assignments []*Assignment) [][]uuid.UUID {
	types := make(map[int][]string, len(tuners))
	for _, tuner := range tuners {
		types[tuner.Index] = tuner.Types
	}

	peers := make([][]uuid.UUID, 0, len(reservations))
	for i, r := range reservations {
		a := assignments[i]
		if a.State != domain.AssignmentStateConflict && a.State != domain.AssignmentStatePartial {
			peers = append(peers, []uuid.UUID{})

			continue
		}
		found := []int{}
		for j, other := range reservations {
			b := assignments[j]
			if b.Tuner == NoTuner || other.channel() == r.channel() || !slices.Contains(types[b.Tuner], r.ChannelType) {
				continue
			}
			if overlap(r.fullStart(), r.fullEnd(), b.PlannedStart, b.PlannedEnd) > 0 {
				found = append(found, j)
			}
		}
		slices.SortFunc(found, func(x, y int) int {
			return cmp.Or(
				assignments[x].PlannedStart.Compare(assignments[y].PlannedStart),
				slices.Compare(reservations[x].ID[:], reservations[y].ID[:]),
			)
		})
		ids := make([]uuid.UUID, 0, len(found))
		for _, j := range found {
			ids = append(ids, reservations[j].ID)
		}
		peers = append(peers, ids)
	}

	return peers
}
