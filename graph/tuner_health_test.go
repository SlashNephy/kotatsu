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
