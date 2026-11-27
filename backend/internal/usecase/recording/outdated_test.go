package recording_test

import (
	"context"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// TestWorkerDefersToCompatiblePeers は、DB のスキーマがこのバイナリより新しく (動かしてよいバイナリの下限より古い)、下限を満たす
// mikan がほかに動いていれば、割り振りを取らずに任せることを確認する (#2195)。任せる先がいなければ、古いバイナリでも取る。
func TestWorkerDefersToCompatiblePeers(t *testing.T) {
	t.Parallel()

	latest := database.LatestVersion()
	tests := []struct {
		name string
		// outdated は、このバイナリを、DB の下限より古くするかどうか。
		outdated bool
		// peerVersion は、ほかに動いている mikan が登録簿に書いた、そのバイナリの最新のマイグレーションのバージョン。nil なら書いていない。
		peerVersion *int64
		// peerStale は、ほかの mikan のハートビートが途絶えているかどうか。
		peerStale   bool
		wantAcquire bool
	}{
		{name: "下限を満たしていれば取る", outdated: false, peerVersion: new(latest), peerStale: false, wantAcquire: true},
		{name: "下限を満たす mikan がほかにいれば任せる", outdated: true, peerVersion: new(latest + 1), peerStale: false, wantAcquire: false},
		{name: "ほかの mikan も下限を満たさなければ取る", outdated: true, peerVersion: new(latest), peerStale: false, wantAcquire: true},
		{name: "ほかの mikan がバージョンを書いていなければ取る", outdated: true, peerVersion: nil, peerStale: false, wantAcquire: true},
		{name: "下限を満たす mikan が止まっていれば取る", outdated: true, peerVersion: new(latest + 1), peerStale: true, wantAcquire: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			if tt.outdated {
				// 新しいバイナリの kotatsu migrate が、このバイナリの知らないマイグレーションを当て、下限を上げたことにする。
				_, err := f.pool.Exec(t.Context(), "UPDATE schema_compatibility SET min_binary_version = $1", latest+1)
				require.NoError(t, err)
			}
			// 割り振りを書く時刻まで時計を進めてから、ほかの mikan のハートビートを書く。
			reservationID := f.assignOne(t)
			heartbeat := f.clock.Now()
			if tt.peerStale {
				heartbeat = heartbeat.Add(-time.Hour)
			}
			_, err := f.pool.Exec(t.Context(), `INSERT INTO instances (id, role, address, started_at, heartbeat_at, schema_version)
				VALUES ($1, 'mikan', '127.0.0.1:0', $2, $2, $3)`, id.New(), heartbeat, tt.peerVersion)
			require.NoError(t, err)

			holder := id.New()
			acquired := make(chan struct{}, 1)
			_, _ = f.startWorkerWithOptions(t, holder, recording.NewRecordingLeases(f.pool), &recording.WorkerOptions{
				LeaseTTL: 10 * time.Second, RenewInterval: 20 * time.Millisecond, AcquireInterval: 50 * time.Millisecond,
				HandoverTimeout: shortHandoverTimeout, ReleaseCooldown: 0, DatabaseClock: false, PollOnly: true, PollInterval: 50 * time.Millisecond,
				DeferAcquire: recording.DeferToCompatiblePeers(f.pool, f.clock, holder, latest),
			}, func(ctx context.Context, _ *recording.Assignment) error {
				acquired <- struct{}{}
				<-ctx.Done()

				return nil
			})

			// 取らないことは、取れる間隔の何回分か待って確かめる。
			select {
			case <-acquired:
				assert.True(t, tt.wantAcquire, "the outdated mikan acquired the assignment")
				assert.Equal(t, &holder, f.assignment(t, reservationID).holder)
			case <-time.After(500 * time.Millisecond):
				assert.False(t, tt.wantAcquire, "the mikan did not acquire the assignment")
				assert.Equal(t, (*uuid.UUID)(nil), f.assignment(t, reservationID).holder)
			}
		})
	}
}
