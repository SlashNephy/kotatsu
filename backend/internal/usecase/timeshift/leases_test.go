package timeshift_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/timeshift"
)

// TestLeasesRenewExpiredLease は、リースの期限が切れていても、ほかの mikan が取っていなければ延長できることを確かめる。
// DB が止まっていた間に期限が切れても、戻った後に延ばして録り続けるため (#1073)。
func TestLeasesRenewExpiredLease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		takenOver bool
		want      int64
	}{
		{name: "nobody took it over", takenOver: false, want: 1},
		{name: "another mikan took it over", takenOver: true, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newEnv(t)
			require.NoError(t, e.q.InsertTimeshiftAssignment(t.Context(), sqlc.InsertTimeshiftAssignmentParams{
				ID: id.New(), ServiceID: grService(0), Priority: timeshift.Priority, Now: e.clk.Now(),
			}))
			leases := timeshift.NewLeases(e.pool)
			holder := id.New()
			now := e.clk.Now()
			assignment, ok, err := leases.Acquire(t.Context(), holder, now, now.Add(10*time.Second), []uuid.UUID{})
			require.NoError(t, err)
			require.True(t, ok)

			// DB が止まっている間に、リースの期限が切れたことにする。
			later := now.Add(time.Minute)
			if tt.takenOver {
				_, ok, err := leases.Acquire(t.Context(), id.New(), later, later.Add(10*time.Second), []uuid.UUID{})
				require.NoError(t, err)
				require.True(t, ok)
			}

			n, err := leases.Renew(t.Context(), assignment, holder, later, later.Add(10*time.Second))
			require.NoError(t, err)
			assert.Equal(t, tt.want, n)
		})
	}
}
