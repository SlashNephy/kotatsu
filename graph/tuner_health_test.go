package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTunerServerHealth は、管理者でなくても、チューナーサーバーにつながっているかを読めることを確かめる。
func TestTunerServerHealth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// checked は、チューナーサーバーの状態を確かめたことがあるかどうか。connected は、そのときにつながったかどうか。
		checked      bool
		connected    bool
		wantDegraded bool
	}{
		{name: "まだ確かめていない", checked: false, connected: false, wantDegraded: false},
		{name: "つながっている", checked: true, connected: true, wantDegraded: false},
		{name: "縮退中", checked: true, connected: false, wantDegraded: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			now := f.clock.Now()
			if tt.checked {
				var degradedSince *time.Time
				if !tt.connected {
					degradedSince = &now
				}
				_, err := f.pool.Exec(t.Context(),
					"INSERT INTO tuner_server_status (degraded_since, last_error, clock_skew_ms, checked_at) VALUES ($1, '', NULL, $2)",
					degradedSince, now)
				require.NoError(t, err)
			}

			var got struct {
				TunerServerHealth struct {
					Degraded      bool       `json:"degraded"`
					DegradedSince *time.Time `json:"degradedSince"`
				} `json:"tunerServerHealth"`
			}
			// fixture のユーザーはメンバーで、管理者ではない。
			require.Empty(t, f.query(t, `{ tunerServerHealth { degraded degradedSince } }`, nil, &got))
			assert.Equal(t, tt.wantDegraded, got.TunerServerHealth.Degraded)
			assert.Equal(t, tt.wantDegraded, got.TunerServerHealth.DegradedSince != nil)
		})
	}
}

// TestTunerServerHealthClockSkew は、管理者でなくても、チューナーサーバーの時計が閾値 (10 秒) を超えてずれているかを読めることを
// 確かめる (#505)。録画の開始と終了がずれうるので、ホームで全員に知らせるのに使う。
func TestTunerServerHealthClockSkew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		skewMs     *int64
		wantSkewed bool
		wantSkewMs *int
	}{
		{name: "まだ測っていない", skewMs: nil, wantSkewed: false, wantSkewMs: nil},
		{name: "閾値ちょうどは警告しない", skewMs: new(int64(10_000)), wantSkewed: false, wantSkewMs: new(10_000)},
		{name: "進んでいる", skewMs: new(int64(10_001)), wantSkewed: true, wantSkewMs: new(10_001)},
		{name: "遅れている", skewMs: new(int64(-30_000)), wantSkewed: true, wantSkewMs: new(-30_000)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			_, err := f.pool.Exec(t.Context(),
				"INSERT INTO tuner_server_status (degraded_since, last_error, clock_skew_ms, checked_at) VALUES (NULL, '', $1, $2)",
				tt.skewMs, f.clock.Now())
			require.NoError(t, err)

			var got struct {
				TunerServerHealth struct {
					ClockSkewed bool `json:"clockSkewed"`
					ClockSkewMs *int `json:"clockSkewMs"`
				} `json:"tunerServerHealth"`
			}
			require.Empty(t, f.query(t, `{ tunerServerHealth { clockSkewed clockSkewMs } }`, nil, &got))
			assert.Equal(t, tt.wantSkewed, got.TunerServerHealth.ClockSkewed)
			assert.Equal(t, tt.wantSkewMs, got.TunerServerHealth.ClockSkewMs)
		})
	}
}
