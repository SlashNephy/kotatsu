package notification_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/notification"
)

// TestPrune は、配り終えた通知のうち、保持期間を過ぎたものだけを、既読か未読かにかかわらず消すことを確かめる (#551)。
func TestPrune(t *testing.T) {
	t.Parallel()

	const day = 24 * time.Hour
	tests := []struct {
		name      string
		retention time.Duration
		// wantKinds は、消した後に残るユーザーの通知の種類 (新しい順)。
		wantKinds []notification.Kind
		// wantDeleted は、消した通知のイベントの数。
		wantDeleted int64
	}{
		{
			name:        "保持期間を過ぎた通知は、既読でも未読でも消す",
			retention:   90 * day,
			wantKinds:   []notification.Kind{notification.KindStorageLow},
			wantDeleted: 2,
		},
		{
			name:        "無期限なら消さない",
			retention:   -1,
			wantKinds:   []notification.Kind{notification.KindStorageLow, notification.KindRecordingFailed, notification.KindRecordingStarted},
			wantDeleted: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			userID := f.user(t, "kotatsu")
			// 100 日前に 2 件配り、1 件を既読にする。
			f.emit(t, notification.KindRecordingStarted)
			f.emit(t, notification.KindRecordingFailed)
			require.Equal(t, 2, f.dispatch(t))
			old := f.list(t, userID, &notification.ListQuery{UnreadOnly: false, Before: nil, Limit: 50})
			_, err := f.inbox.MarkRead(t.Context(), f.pool, userID, []uuid.UUID{old[0].ID})
			require.NoError(t, err)
			// まだ配っていない通知は、古くても消さない。配るジョブが止まっていた間の通知を失わないため。
			f.emit(t, notification.KindTunerServerUnreachable)
			// 1 日前に 1 件配る (上で配っていなかった分は、配らないまま残す)。
			f.clock.Advance(99 * day)
			_, err = f.pool.Exec(t.Context(), `UPDATE notification_events SET dispatched_at = NULL WHERE kind = 'tuner_server_unreachable'`)
			require.NoError(t, err)
			require.NoError(t, notification.Emit(t.Context(), f.pool, f.clock, notification.KindStorageLow, nil))
			_, err = f.pool.Exec(t.Context(), `UPDATE notification_events SET dispatched_at = $1 WHERE kind = 'storage_low'`, f.clock.Now())
			require.NoError(t, err)
			_, err = f.pool.Exec(t.Context(),
				`INSERT INTO notifications (id, user_id, event_id, created_at) SELECT gen_random_uuid(), $1, id, $2 FROM notification_events WHERE kind = 'storage_low'`,
				userID, f.clock.Now())
			require.NoError(t, err)
			f.clock.Advance(day)

			deleted, err := notification.Prune(t.Context(), f.pool, f.clock, tt.retention)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDeleted, deleted)
			assert.Equal(t, tt.wantKinds, kinds(f.list(t, userID, &notification.ListQuery{UnreadOnly: false, Before: nil, Limit: 50})))
			var pending int
			require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM notification_events WHERE dispatched_at IS NULL`).Scan(&pending))
			assert.Equal(t, 1, pending, "undispatched events are kept")
		})
	}
}
