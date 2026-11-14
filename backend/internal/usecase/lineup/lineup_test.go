package lineup_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/lineup"
)

func TestTransponder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		kind   lineup.Kind
		kHz    int
		want   int
		wantOK bool
	}{
		{name: "BS-1", kind: lineup.KindBS, kHz: 11_727_480, want: 1, wantOK: true},
		{name: "BS-3", kind: lineup.KindBS, kHz: 11_765_840, want: 3, wantOK: true},
		{name: "BS-23", kind: lineup.KindBS, kHz: 12_149_440, want: 23, wantOK: true},
		// 4K・8K の中継器は、構成から除く。
		{name: "BS-7 (4K)", kind: lineup.KindBS, kHz: 11_842_560, want: 0, wantOK: false},
		{name: "BS-17 (4K)", kind: lineup.KindBS, kHz: 12_034_360, want: 0, wantOK: false},
		{name: "CS-2", kind: lineup.KindCS, kHz: 12_291_000, want: 2, wantOK: true},
		{name: "CS-24", kind: lineup.KindCS, kHz: 12_731_000, want: 24, wantOK: true},
		// 表に当てはまらない周波数。
		{name: "between transponders", kind: lineup.KindBS, kHz: 11_746_000, want: 0, wantOK: false},
		{name: "below the first", kind: lineup.KindCS, kHz: 12_000_000, want: 0, wantOK: false},
		{name: "above the last", kind: lineup.KindBS, kHz: 12_200_000, want: 0, wantOK: false},
		// 計算した周波数との差が、許す誤差 (2 MHz) の内なら同じ中継器とみなし、外なら除く。
		{name: "within the tolerance", kind: lineup.KindBS, kHz: 11_727_480 + 2_000, want: 1, wantOK: true},
		{name: "beyond the tolerance", kind: lineup.KindBS, kHz: 11_727_480 + 2_010, want: 0, wantOK: false},
		// 衛星でない種別は、読まない。
		{name: "terrestrial", kind: lineup.Kind("GR"), kHz: 11_727_480, want: 0, wantOK: false},
	}
	for _, tt := range tests {
		number, ok := lineup.Transponder(tt.kind, tt.kHz)
		assert.Equal(t, tt.wantOK, ok, tt.name)
		assert.Equal(t, tt.want, number, tt.name)
	}
}

// observation は、BS-1 と BS-3 に、テレビのサービス (と、テレビでないサービス) を載せた構成。
func observation(version uint8, bs1 ...uint16) *lineup.Observation {
	services := make([]lineup.Service, 0, len(bs1)+1)
	services = append(services, lineup.Service{ServiceID: 900, ServiceType: 0xC0})
	for _, id := range bs1 {
		services = append(services, lineup.Service{ServiceID: id, ServiceType: 0x01})
	}

	return &lineup.Observation{
		Kind: lineup.KindBS, NetworkID: 4, Version: version,
		Streams: []lineup.Stream{
			{TransportStreamID: 0x4010, FrequencyKHz: 11_727_480, Services: services},
			{TransportStreamID: 0x4031, FrequencyKHz: 11_765_840, Services: []lineup.Service{{ServiceID: 141, ServiceType: 0x01}}},
			// 4K の中継器のサービスは比べない。
			{TransportStreamID: 0x4671, FrequencyKHz: 11_842_560, Services: []lineup.Service{{ServiceID: 701, ServiceType: 0x01}}},
		},
	}
}

func known(ids ...uint16) []lineup.Known {
	out := make([]lineup.Known, 0, len(ids)+1)
	for _, id := range ids {
		out = append(out, lineup.Known{NetworkID: 4, ServiceID: id})
	}
	// ほかのネットワークのサービスは比べない。
	out = append(out, lineup.Known{NetworkID: 6, ServiceID: 55})

	return out
}

const interval = 30 * time.Minute

func TestComparer(t *testing.T) {
	t.Parallel()

	type step struct {
		advance time.Duration
		obs     *lineup.Observation
		known   []lineup.Known
		// want は知らせる食い違い。nil なら知らせない。
		want *lineup.Change
	}
	added := &lineup.Change{Kind: lineup.KindBS, NetworkID: 4, Added: 1, Removed: 1}
	tests := []struct {
		name  string
		steps []step
	}{
		{
			name:  "the first sighting is not notified",
			steps: []step{{advance: 0, obs: observation(1, 101, 103), known: known(101, 102, 141), want: nil}},
		},
		{
			name: "notified after the interval with the same version",
			steps: []step{
				{advance: 0, obs: observation(1, 101, 103), known: known(101, 102, 141), want: nil},
				{advance: interval - time.Second, obs: observation(1, 101, 103), known: known(101, 102, 141), want: nil},
				{advance: time.Second, obs: observation(1, 101, 103), known: known(101, 102, 141), want: added},
				// 知らせた version は確かめ済みなので、二重に知らせない。
				{advance: interval, obs: observation(1, 101, 103), known: known(101, 102, 141), want: nil},
			},
		},
		{
			name: "a new version restarts the count",
			steps: []step{
				{advance: 0, obs: observation(1, 101, 103), known: known(101, 102, 141), want: nil},
				{advance: interval, obs: observation(2, 101, 103), known: known(101, 102, 141), want: nil},
				{advance: interval, obs: observation(2, 101, 103), known: known(101, 102, 141), want: added},
			},
		},
		{
			name: "a resolved mismatch is forgotten",
			steps: []step{
				{advance: 0, obs: observation(1, 101, 103), known: known(101, 102, 141), want: nil},
				// スキャンし直して一覧が合ったら、食い違いを忘れる。
				{advance: interval, obs: observation(2, 101, 103), known: known(101, 103, 141), want: nil},
				{advance: interval, obs: observation(3, 101), known: known(101, 103, 141), want: nil},
			},
		},
		{
			name: "a lineup without streams is ignored",
			steps: []step{
				{advance: 0, obs: &lineup.Observation{Kind: lineup.KindBS, NetworkID: 4, Version: 1, Streams: nil}, known: known(101), want: nil},
				{advance: interval, obs: &lineup.Observation{Kind: lineup.KindBS, NetworkID: 4, Version: 1, Streams: nil}, known: known(101), want: nil},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clk := clocktest.NewFake(time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC))
			comparer := lineup.NewComparer(clk, interval)
			for i, s := range tt.steps {
				clk.Advance(s.advance)
				change, ok := comparer.Observe(s.obs, s.known)
				if s.want == nil {
					assert.False(t, ok, "step %d", i)

					continue
				}
				require.True(t, ok, "step %d", i)
				assert.Equal(t, s.want, change, "step %d", i)
			}
		})
	}
}

// TestComparerForget は、チューナーサーバーの一覧が変わったら、確かめ済みの version を忘れて比べ直すことを確かめる。
func TestComparerForget(t *testing.T) {
	t.Parallel()

	clk := clocktest.NewFake(time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC))
	comparer := lineup.NewComparer(clk, interval)
	// 合っていれば、version 1 を確かめ済みにする。
	_, ok := comparer.Observe(observation(1, 101), known(101, 141))
	require.False(t, ok)
	// 一覧が変わっても、確かめ済みの version は比べない。
	_, ok = comparer.Observe(observation(1, 101), known(141))
	require.False(t, ok)
	clk.Advance(interval)
	_, ok = comparer.Observe(observation(1, 101), known(141))
	require.False(t, ok)

	comparer.Forget()
	_, ok = comparer.Observe(observation(1, 101), known(141))
	require.False(t, ok)
	clk.Advance(interval)
	change, ok := comparer.Observe(observation(1, 101), known(141))
	require.True(t, ok)
	assert.Equal(t, &lineup.Change{Kind: lineup.KindBS, NetworkID: 4, Added: 1, Removed: 0}, change)
}

// TestComparerCS は、CS も同じ規則で比べ、構成がないとき (nil) は比べないことを確かめる。
func TestComparerCS(t *testing.T) {
	t.Parallel()

	clk := clocktest.NewFake(time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC))
	comparer := lineup.NewComparer(clk, interval)
	_, ok := comparer.Observe(nil, nil)
	require.False(t, ok)

	cs := &lineup.Observation{
		Kind: lineup.KindCS, NetworkID: 6, Version: 1,
		Streams: []lineup.Stream{{TransportStreamID: 0x6020, FrequencyKHz: 12_291_000, Services: []lineup.Service{{ServiceID: 55, ServiceType: 0x01}, {ServiceID: 56, ServiceType: 0x01}}}},
	}
	known := []lineup.Known{{NetworkID: 6, ServiceID: 55}}
	_, ok = comparer.Observe(cs, known)
	require.False(t, ok)
	clk.Advance(interval)
	change, ok := comparer.Observe(cs, known)
	require.True(t, ok)
	assert.Equal(t, &lineup.Change{Kind: lineup.KindCS, NetworkID: 6, Added: 1, Removed: 0}, change)
}
