package epg

import (
	"cmp"
	"slices"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
)

// relatedShared は、同じイベントを複数のサービスで放送する関係 (イベント共有)。
const relatedShared = "shared"

// stream は、同じ TS で同じ種別の番組を流すサービスのまとまり。イベント共有を補うときに、番組を比べる範囲にする。
type stream struct {
	networkID   int64
	channelType string
	channel     string
	serviceType int64
}

// streams は、サービスの Mirakurun のサービス ID から、そのサービスの stream を引く表。
type streams map[int64]stream

// addUpstream は、上流のサービスを表に足す。チャンネルを持たないサービスは足さない。
func (s streams) addUpstream(services []*mirakurun.Service) {
	for _, service := range services {
		if service.Channel == nil {
			continue
		}
		s[service.ID] = stream{
			networkID:   int64(service.NetworkID),
			channelType: service.Channel.Type,
			channel:     service.Channel.Channel,
			serviceType: int64(service.Type),
		}
	}
}

// addRows は、DB のサービスを表に足す。上流に同じサービスがあれば、上流を優先する。
func (s streams) addRows(rows []sqlc.ListServicesRow) {
	for _, row := range rows {
		if _, ok := s[row.ID]; ok {
			continue
		}
		s[row.ID] = stream{networkID: row.NetworkID, channelType: row.ChannelType, channel: row.Channel, serviceType: row.ServiceType}
	}
}

// siblings は、serviceIDs のサービスと同じ stream のサービスを、serviceIDs に足して返す。
// イベント共有を補うのは地上波だけなので、BS と CS のサービスには足さない。1 つの TS に多くのサービスが入り、突き合わせが増えるだけになる。
func (s streams) siblings(serviceIDs []int64) []int64 {
	wanted := map[stream]bool{}
	for _, id := range serviceIDs {
		if st, ok := s[id]; ok && domain.IsTerrestrialNetwork(st.networkID) {
			wanted[st] = true
		}
	}
	result := slices.Clone(serviceIDs)
	for id, st := range s {
		if wanted[st] {
			result = append(result, id)
		}
	}

	return slices.Compact(slices.Sorted(slices.Values(result)))
}

// sameEvent は、イベント共有の記述子がない番組を、同じ番組とみなすときに比べる値。
type sameEvent struct {
	stream   stream
	startAt  int64
	duration int64
	name     string
}

// inferSharedEvents は、地上波の同じ TS のサブチャンネルで、イベント共有の記述子がない同じ番組に、互いを指す shared の関連を足す (設計書 3.3、#1024)。
// 放送の EIT には、event_id が違い、event_group_descriptor もない同じ番組がある (2026 年 10 月に NHK 総合 1 と 2 で確かめた)。
// 同じ stream で、開始、長さ、番組名が同じで、どちらも shared の関連を持たない番組を、同じ番組とみなす。
// BS と CS は、1 つの TS に別の放送局のサービスが入り、「インフォメーション」のような番組名が重なるので、対象にしない。
// 関連を足す番組は複製して返し、programs の要素は書き換えない。
// 放送中と次の番組 (EIT[p/f]) の延長が片方のサービスにだけ先に届くと、その間は同じ番組とみなさず、届いたら戻る。
func inferSharedEvents(programs []*mirakurun.Program, streams streams) []*mirakurun.Program {
	groups := map[sameEvent][]int{}
	for i, p := range programs {
		st, ok := streams[serviceKey(p)]
		if !ok || !domain.IsTerrestrialNetwork(st.networkID) || p.Name == "" || hasShared(p) {
			continue
		}
		key := sameEvent{stream: st, startAt: p.StartAt, duration: p.Duration, name: p.Name}
		groups[key] = append(groups[key], i)
	}

	result := slices.Clone(programs)
	for _, indexes := range groups {
		if len(indexes) < 2 || hasDuplicateService(programs, indexes) {
			continue
		}
		for _, i := range indexes {
			p := *programs[i]
			others := []*domain.RelatedItem{}
			for _, j := range indexes {
				if j == i {
					continue
				}
				// 上流の shared の関連と同じく、同じネットワークなのでネットワーク ID は書かない。
				others = append(others, &domain.RelatedItem{Type: relatedShared, NetworkID: 0, ServiceID: programs[j].ServiceID, EventID: programs[j].EventID})
			}
			slices.SortFunc(others, func(a, b *domain.RelatedItem) int {
				return cmp.Or(cmp.Compare(a.ServiceID, b.ServiceID), cmp.Compare(a.EventID, b.EventID))
			})
			p.RelatedItems = slices.Concat(p.RelatedItems, others)
			result[i] = &p
		}
	}

	return result
}

func serviceKey(p *mirakurun.Program) int64 {
	return int64(p.NetworkID)*serviceIDBase + int64(p.ServiceID)
}

func hasShared(p *mirakurun.Program) bool {
	return slices.ContainsFunc(p.RelatedItems, func(item *domain.RelatedItem) bool { return item.Type == relatedShared })
}

// hasDuplicateService は、同じサービスの番組が 2 つ以上あるかどうかを返す。同じサービスの中の番組は、共有とみなさない。
func hasDuplicateService(programs []*mirakurun.Program, indexes []int) bool {
	seen := map[int64]bool{}
	for _, i := range indexes {
		key := serviceKey(programs[i])
		if seen[key] {
			return true
		}
		seen[key] = true
	}

	return false
}
