package edcb_test

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
)

func TestTunerServerSharesNetworkTVIDWithOtherInstances(t *testing.T) {
	t.Parallel()

	ts := make([]byte, 2*188)
	ts[0], ts[188] = 0x47, 0x47
	stopped := make(chan struct{})
	close(stopped)
	tests := []struct {
		name string
		// shared なら、ほかのインスタンスが開いている ID (2000) に加わる。holders は、ほかに残る持ち主の数。
		shared  bool
		holders int
		// relayBusy は、中継に失敗する回数。加わった ID の中継に空きがなければ、自分で別の ID (1000) を取り直す。
		relayBusy int
		// stopping なら、プロセスが止まり始めている。joined なら、引き継ぎを待つ間に、ほかのインスタンスが加わる。
		stopping bool
		joined   bool

		wantExclusive []bool
		wantCloses    int
		wantForgotten []int32
		wantKept      []int32
	}{
		// ほかのインスタンスと同じ ID を持ち、ほかの持ち主が残っていれば、閉じずに外れる (#1850)。
		{name: "leave a shared ID", shared: true, holders: 1, wantExclusive: []bool{false}, wantCloses: 0, wantForgotten: []int32{}},
		// 最後の持ち主なら、加わった ID でも閉じる。
		{name: "close a shared ID as the last holder", shared: true, holders: 0, wantExclusive: []bool{false}, wantCloses: 1, wantForgotten: []int32{2000}},
		// 加わった ID の中継に空きがなければ (EDCB は 3 つ目の中継を断る)、別の ID を取り直す。
		{
			name: "relay of the shared ID is busy", shared: true, holders: 1, relayBusy: 25,
			wantExclusive: []bool{false, true}, wantCloses: 0, wantForgotten: []int32{},
		},
		// 止まるときは、最後の持ち主でもすぐには閉じず、ほかのインスタンスが加わるのを待つ。加われば閉じない。
		{
			name: "hand over to a joining instance", stopping: true, joined: true,
			wantExclusive: []bool{false}, wantCloses: 0, wantForgotten: []int32{}, wantKept: []int32{1000},
		},
		// 加わらなければ、待った後に閉じる。
		{
			name: "close after nobody took over", stopping: true, joined: false,
			wantExclusive: []bool{false}, wantCloses: 1, wantForgotten: []int32{1000}, wantKept: []int32{1000},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := streamServer(t, ts, tt.relayBusy)
			ids := &fakeIDs{
				mu: sync.Mutex{}, acquired: []edcb.ServiceKey{}, released: []int32{},
				shared: tt.shared, holders: tt.holders, joinedDuringHandover: tt.joined,
			}
			opts := &edcb.TunerServerOptions{NetworkTVIDs: ids, PreemptionCheckInterval: time.Hour, HandoverGrace: 10 * time.Millisecond}
			if tt.stopping {
				opts.Stopped = stopped
			}
			stream, err := edcb.NewTunerServer(s.client(t), opts).OpenServiceStream(t.Context(), nhkSougou, nil)
			require.NoError(t, err)
			got, err := io.ReadAll(stream)
			require.NoError(t, err)
			assert.Equal(t, ts, got)
			require.NoError(t, stream.Close())

			ids.mu.Lock()
			defer ids.mu.Unlock()
			assert.Equal(t, tt.wantExclusive, ids.exclusive)
			assert.Equal(t, tt.wantForgotten, ids.released)
			assert.Equal(t, tt.wantKept, ids.kept)
			assert.Equal(t, tt.wantCloses, countCommand(s, 1074))
		})
	}
}
