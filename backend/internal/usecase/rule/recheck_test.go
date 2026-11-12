package rule_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

// TestEngineKeepsReservationsStoppedAsRecordedEpisodes は、録り始めに同じ回を録画済みと分かって録画をやめ、Kotatsu が無効にしたルール予約を、
// ルールの照合が有効に戻したり、消して作り直したりしないことを確かめる (#1500)。録画をやめるのは開始マージンの間か、放送が始まった後である。
// 照合は、無効の予約をルールがある間は残すので、録り始めたことを残す列を別に足さなくても、もう一度は録り始めない。
func TestEngineKeepsReservationsStoppedAsRecordedEpisodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// at は、照合する時刻の、testNow からの時間。地上波の放送 (eventId 300) は 5 時間後に始まる。
		at time.Duration
	}{
		{name: "開始マージンの間", at: 5*time.Hour - 10*time.Second},
		{name: "放送が始まった後", at: 5*time.Hour + time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newEngineFixture(t, earlyAndLateEpisodes()...)
			f.createEpisodeRule(t, true)
			f.run(t)
			require.Equal(t, map[int64]bool{300: true}, f.reservedEpisodes(t))

			// 地上波の放送の録り始めに、別の局で先に録った同じ回があると分かり、録画をやめた。
			start := testNow().Add(-24 * time.Hour)
			const recordedName = "灯台の地図 #3"
			_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $6)`,
				id.New(), faketuner.ServiceItemID(grNetworkID, subServiceID), rule.TitleKey(recordedName), recordedName, start, start.Add(time.Hour))
			require.NoError(t, err)
			_, err = f.pool.Exec(t.Context(), `UPDATE reservations SET enabled = false, disabled_reason = 'recorded_episode', disabled_service_id = $1
				WHERE service_id = $2 AND event_id = 300`, faketuner.ServiceItemID(grNetworkID, subServiceID), serviceID())
			require.NoError(t, err)

			f.clock.Advance(tt.at)
			result := f.run(t)

			assert.Zero(t, result.Created)
			assert.Zero(t, result.Deleted)
			assert.Equal(t, map[int64]bool{300: false}, f.reservedEpisodes(t), "the stopped reservation stays disabled")
			var reason *string
			require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT disabled_reason FROM reservations WHERE service_id = $1 AND event_id = 300", serviceID()).
				Scan(&reason))
			assert.Equal(t, new("recorded_episode"), reason)
		})
	}
}
