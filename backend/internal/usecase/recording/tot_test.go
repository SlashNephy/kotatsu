package recording_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// TestBroadcastClockSkew は、録画している TS の TOT が示す放送の時刻と、届いた時点の OS の時計の差を、
// 書いているセグメントの様子と一緒に、チューナーサーバーの状態に書くことを確かめる (#1309)。測り直していなければ書き直さない。
func TestBroadcastClockSkew(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recorder, _ := f.newRecorder(t, onceStream{data: nil}, nil)
	read := func() (*int64, *time.Time) {
		t.Helper()

		var skew *int64
		var checked *time.Time
		err := f.pool.QueryRow(t.Context(), "SELECT broadcast_clock_skew_ms, broadcast_clock_checked_at FROM tuner_server_status WHERE id").
			Scan(&skew, &checked)
		require.NoError(t, err)

		return skew, checked
	}

	// まだ TOT を読んでいなければ、何も書かない。
	recording.WriteBroadcastSkew(t.Context(), recorder)
	var written int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM tuner_server_status WHERE broadcast_clock_skew_ms IS NOT NULL").
		Scan(&written))
	assert.Zero(t, written)

	// OS の時計が、放送の時刻より 15 秒進んでいる。
	recording.ObserveTOT(recorder, f.clock.Now().Add(-15*time.Second))
	recording.WriteBroadcastSkew(t.Context(), recorder)
	skew, checked := read()
	require.NotNil(t, skew)
	assert.Equal(t, int64(15_000), *skew)
	require.NotNil(t, checked)
	assert.True(t, f.clock.Now().Equal(*checked), checked)

	// 放送の時刻に追いついたら、書き直す。
	f.clock.Advance(time.Minute)
	recording.ObserveTOT(recorder, f.clock.Now())
	recording.WriteBroadcastSkew(t.Context(), recorder)
	skew, _ = read()
	require.NotNil(t, skew)
	assert.Zero(t, *skew)
}
