package metrics_test

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/metrics"
)

// TestFullSyncCollector は、最後に EPG の全量を突き合わせた時刻と、それから経った秒数を返すことを確かめる (設計書 11.3、#2010)。
func TestFullSyncCollector(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		// synced は、突き合わせ終えた時刻を start からの長さで並べたもの。最後のものが残る。
		synced []time.Duration
		// elapsed は、最後の突き合わせから、収集するまでに進める長さ。
		elapsed time.Duration
		want    string
	}{
		{
			name: "まだ突き合わせていなければ、時刻は 0 で、経った秒数は出さない",
			want: `
# HELP kotatsu_epg_last_full_sync_timestamp_seconds Unix time when this instance last synced the whole EPG as the neko leader.
# TYPE kotatsu_epg_last_full_sync_timestamp_seconds gauge
kotatsu_epg_last_full_sync_timestamp_seconds 0
`,
		},
		{
			name:    "突き合わせた後は、最後の時刻と、そこから経った秒数を出す",
			synced:  []time.Duration{0, 30 * time.Minute},
			elapsed: 45 * time.Minute,
			want: `
# HELP kotatsu_epg_full_sync_age_seconds Seconds since this instance last synced the whole EPG as the neko leader.
# TYPE kotatsu_epg_full_sync_age_seconds gauge
kotatsu_epg_full_sync_age_seconds 2700
# HELP kotatsu_epg_last_full_sync_timestamp_seconds Unix time when this instance last synced the whole EPG as the neko leader.
# TYPE kotatsu_epg_last_full_sync_timestamp_seconds gauge
kotatsu_epg_last_full_sync_timestamp_seconds 1.7916354e+09
`,
		},
		{
			name:    "時計が戻って、突き合わせた時刻より前になっても、経った秒数は負にしない",
			synced:  []time.Duration{time.Hour},
			elapsed: -time.Minute,
			want: `
# HELP kotatsu_epg_full_sync_age_seconds Seconds since this instance last synced the whole EPG as the neko leader.
# TYPE kotatsu_epg_full_sync_age_seconds gauge
kotatsu_epg_full_sync_age_seconds 0
# HELP kotatsu_epg_last_full_sync_timestamp_seconds Unix time when this instance last synced the whole EPG as the neko leader.
# TYPE kotatsu_epg_last_full_sync_timestamp_seconds gauge
kotatsu_epg_last_full_sync_timestamp_seconds 1.7916372e+09
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clk := clocktest.NewFake(start)
			collector := metrics.NewFullSyncCollector(clk)
			for _, d := range tt.synced {
				clk.Set(start.Add(d))
				collector.Observe(clk.Now())
			}
			clk.Advance(tt.elapsed)

			require.NoError(t, testutil.CollectAndCompare(collector, strings.NewReader(tt.want),
				"kotatsu_epg_last_full_sync_timestamp_seconds", "kotatsu_epg_full_sync_age_seconds"))
		})
	}
}
