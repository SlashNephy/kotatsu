package nextepisode_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/usecase/nextepisode"
)

// harbor は、テストのシリーズ名。
const harbor = "霧笛の港"

func TestKeyOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  *episode.Source
		want *nextepisode.Key
	}{
		{
			name: "番組名の話数とシリーズ名で探す",
			src:  &episode.Source{Name: "霧笛の港　＃３「灯台守」", Description: "", Extended: "", SeriesEpisode: nil, Anime: nil},
			want: &nextepisode.Key{TID: 0, Count: 0, Series: harbor, Number: 3},
		},
		{
			name: "番組名に話数がなければ、記述子の話数で探す",
			src:  &episode.Source{Name: "霧笛の港🈑", Description: "", Extended: "", SeriesEpisode: new(int32(4)), Anime: nil},
			want: &nextepisode.Key{TID: 0, Count: 0, Series: harbor, Number: 4},
		},
		{
			name: "期はシリーズ名に含める",
			src:  &episode.Source{Name: "霧笛の港 第2期 #5", Description: "", Extended: "", SeriesEpisode: nil, Anime: nil},
			want: &nextepisode.Key{TID: 0, Count: 0, Series: "霧笛の港第2期", Number: 5},
		},
		{
			name: "しょぼいカレンダーの話に結びついていれば、番組名でなく TID と話数で探す",
			src: &episode.Source{
				Name: "霧笛の港 #3", Description: "", Extended: "", SeriesEpisode: nil, Anime: &episode.Anime{TID: 6776, Count: 15},
			},
			want: &nextepisode.Key{TID: 6776, Count: 15, Series: "", Number: 0},
		},
		{
			name: "しょぼいカレンダーの話数がなければ、番組名で探す",
			src: &episode.Source{
				Name: "霧笛の港 #3", Description: "", Extended: "", SeriesEpisode: nil, Anime: &episode.Anime{TID: 6776, Count: 0},
			},
			want: &nextepisode.Key{TID: 0, Count: 0, Series: harbor, Number: 3},
		},
		{
			name: "話数を読めない番組 (ニュースなど) では探さない",
			src:  &episode.Source{Name: "ニュース7", Description: "きょうのニュース", Extended: "", SeriesEpisode: nil, Anime: nil},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, nextepisode.KeyOf(tt.src))
		})
	}
}

// grService と subService は、同じネットワークのメインとサブのサービス。bsService は別のネットワークのサービス。
const (
	grService  = int64(32736_01024)
	subService = int64(32736_01025)
	bsService  = int64(4_00101)
)

func base() time.Time {
	return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
}

// candidate は、base から hours 時間後に始まる、話数 number の候補。shared に、同じイベントを共有するサービスの番号を書く。
func candidate(serviceID, eventID int64, hours int, number float64, shared ...int64) *nextepisode.Candidate {
	items := make([]string, 0, len(shared))
	for _, s := range shared {
		items = append(items, fmt.Sprintf(`{"type":"shared","networkId":%d,"serviceId":%d,"eventId":%d}`, s/100_000, s%100_000, eventID))
	}

	return &nextepisode.Candidate{
		ServiceID: serviceID, EventID: eventID, StartAt: base().Add(time.Duration(hours) * time.Hour), Number: number,
		RelatedItems: []byte("[" + strings.Join(items, ",") + "]"),
	}
}

func keys(broadcasts []*nextepisode.Broadcast) [][2]int64 {
	out := make([][2]int64, 0, len(broadcasts))
	for _, b := range broadcasts {
		out = append(out, [2]int64{b.ServiceID, b.EventID})
	}

	return out
}

func TestPick(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		candidates []*nextepisode.Candidate
		reserved   map[nextepisode.ProgramKey]bool
		recorded   map[float64]bool
		want       [][2]int64
	}{
		{
			name:       "予約も録った回もなければ、放送の早い順にすべて返す",
			candidates: []*nextepisode.Candidate{candidate(grService, 10, 1, 4), candidate(bsService, 20, 30, 4), candidate(grService, 11, 169, 5)},
			reserved:   map[nextepisode.ProgramKey]bool{},
			recorded:   map[float64]bool{},
			want:       [][2]int64{{grService, 10}, {bsService, 20}, {grService, 11}},
		},
		{
			name:       "予約のある放送は外す",
			candidates: []*nextepisode.Candidate{candidate(grService, 10, 1, 4), candidate(bsService, 20, 30, 4)},
			reserved:   map[nextepisode.ProgramKey]bool{{ServiceID: grService, EventID: 10}: true},
			recorded:   map[float64]bool{},
			want:       [][2]int64{{bsService, 20}},
		},
		{
			name: "同じイベントを共有する放送に予約があれば外す",
			candidates: []*nextepisode.Candidate{
				candidate(subService, 10, 1, 4, grService),
				candidate(bsService, 20, 30, 4),
			},
			reserved: map[nextepisode.ProgramKey]bool{{ServiceID: grService, EventID: 10}: true},
			recorded: map[float64]bool{},
			want:     [][2]int64{{bsService, 20}},
		},
		{
			name: "同じイベントを共有する放送は、サービス ID の小さい 1 つにする",
			candidates: []*nextepisode.Candidate{
				candidate(subService, 10, 1, 4, grService),
				candidate(grService, 10, 1, 4, subService),
			},
			reserved: map[nextepisode.ProgramKey]bool{},
			recorded: map[float64]bool{},
			want:     [][2]int64{{grService, 10}},
		},
		{
			name:       "録画履歴にある話数の放送は、録画済みとして外す",
			candidates: []*nextepisode.Candidate{candidate(grService, 10, 1, 4), candidate(grService, 11, 169, 5)},
			reserved:   map[nextepisode.ProgramKey]bool{},
			recorded:   map[float64]bool{4: true},
			want:       [][2]int64{{grService, 11}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, keys(nextepisode.Pick(tt.candidates, tt.reserved, tt.recorded)))
		})
	}
}

// TestPickLimitsBroadcasts は、毎日の再放送が並んでも、MaxBroadcasts 件までしか返さないことを確かめる。
func TestPickLimitsBroadcasts(t *testing.T) {
	t.Parallel()

	candidates := make([]*nextepisode.Candidate, 0, nextepisode.MaxBroadcasts+5)
	for i := range nextepisode.MaxBroadcasts + 5 {
		candidates = append(candidates, candidate(grService, int64(100+i), i*24, float64(4+i)))
	}

	got := nextepisode.Pick(candidates, map[nextepisode.ProgramKey]bool{}, map[float64]bool{})

	assert.Len(t, got, nextepisode.MaxBroadcasts)
	assert.Equal(t, int64(100), got[0].EventID)
}
