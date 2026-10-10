package notification_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/notification"
)

func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
}

type fixture struct {
	pool       *pgxpool.Pool
	clock      *clocktest.Fake
	dispatcher *notification.Dispatcher
	inbox      *notification.Inbox
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(testNow())

	return &fixture{
		pool:       pool,
		clock:      clk,
		dispatcher: notification.NewDispatcher(pool, clk),
		inbox:      notification.NewInbox(clk),
	}
}

func (f *fixture) user(t *testing.T, username string) uuid.UUID {
	t.Helper()

	userID := id.New()
	require.NoError(t, sqlc.New(f.pool).InsertUser(t.Context(), sqlc.InsertUserParams{
		ID: userID, Username: username, DisplayName: username, Role: "member", PasswordHash: nil, CreatedAt: f.clock.Now(),
	}))

	return userID
}

func (f *fixture) emit(t *testing.T, kind notification.Kind) {
	t.Helper()

	require.NoError(t, notification.Emit(t.Context(), f.pool, f.clock, kind, nil))
}

func (f *fixture) dispatch(t *testing.T) int {
	t.Helper()

	n, err := f.dispatcher.DispatchOnce(t.Context())
	require.NoError(t, err)

	return n
}

func (f *fixture) list(t *testing.T, userID uuid.UUID, query *notification.ListQuery) []*notification.Notification {
	t.Helper()

	notifications, err := f.inbox.List(t.Context(), f.pool, userID, query)
	require.NoError(t, err)

	return notifications
}

func kinds(notifications []*notification.Notification) []notification.Kind {
	result := make([]notification.Kind, 0, len(notifications))
	for _, n := range notifications {
		result = append(result, n.Kind)
	}

	return result
}

func TestEmit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    notification.Kind
		payload map[string]any
		want    map[string]any
		wantErr error
	}{
		{name: "payload を JSON にして記録する", kind: notification.KindStorageLow, payload: map[string]any{"pool": "main", "freeBytes": 1024}, want: map[string]any{"pool": "main", "freeBytes": float64(1024)}, wantErr: nil},
		{name: "payload がなければ空のオブジェクトを記録する", kind: notification.KindRecordingStarted, payload: nil, want: map[string]any{}, wantErr: nil},
		{name: "知らない種類は記録しない", kind: notification.Kind("unknown"), payload: nil, want: nil, wantErr: notification.ErrUnknownKind},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			userID := f.user(t, "alice")
			err := notification.Emit(t.Context(), f.pool, f.clock, tt.kind, tt.payload)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Zero(t, f.dispatch(t))

				return
			}
			require.NoError(t, err)
			assert.Equal(t, 1, f.dispatch(t))

			got := f.list(t, userID, &notification.ListQuery{UnreadOnly: false, Before: nil, Limit: 10})
			require.Len(t, got, 1)
			assert.Equal(t, tt.kind, got[0].Kind)
			assert.Equal(t, tt.want, got[0].Payload)
			assert.True(t, testNow().Equal(got[0].OccurredAt))
		})
	}
}

// TestDispatchOnce は、イベントを、設定で受け取るとした有効なユーザーにだけ、一度だけ配ることを確かめる。
func TestDispatchOnce(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	alice := f.user(t, "alice")
	bob := f.user(t, "bob")
	carol := f.user(t, "carol")
	require.NoError(t, f.inbox.SetPreference(t.Context(), f.pool, bob, &notification.Preference{
		Kind: notification.KindReservationConflict, InApp: false, WebPush: true,
	}))
	_, err := sqlc.New(f.pool).DisableUser(t.Context(), sqlc.DisableUserParams{DisabledAt: new(f.clock.Now()), ID: carol})
	require.NoError(t, err)

	f.emit(t, notification.KindReservationConflict)
	f.clock.Advance(time.Second)
	f.emit(t, notification.KindStorageLow)
	assert.Equal(t, 2, f.dispatch(t))
	// 配ったイベントは、もう一度は配らない。
	assert.Zero(t, f.dispatch(t))

	all := &notification.ListQuery{UnreadOnly: false, Before: nil, Limit: 10}
	tests := []struct {
		name   string
		userID uuid.UUID
		want   []notification.Kind
	}{
		{name: "設定がなければすべて受け取る", userID: alice, want: []notification.Kind{notification.KindStorageLow, notification.KindReservationConflict}},
		{name: "画面内の通知を切った種類は受け取らない", userID: bob, want: []notification.Kind{notification.KindStorageLow}},
		{name: "無効にしたユーザーは受け取らない", userID: carol, want: []notification.Kind{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := f.list(t, tt.userID, all)
			// 同時に配った通知は、作った日時が同じなので、順序は ID で決まる。種類の集合で比べる。
			assert.ElementsMatch(t, tt.want, kinds(got))
		})
	}
}

// TestDispatchOnceBatches は、1 回のバッチに収まらない数のイベントも、すべて配ることを確かめる。
func TestDispatchOnceBatches(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	alice := f.user(t, "alice")
	const events = 250
	for range events {
		f.emit(t, notification.KindRecordingCompleted)
	}
	assert.Equal(t, events, f.dispatch(t))

	n, err := f.inbox.UnreadCount(t.Context(), f.pool, alice)
	require.NoError(t, err)
	assert.Equal(t, events, n)
}

// TestInbox は、通知の一覧のページ送りと、既読にする操作を確かめる。
func TestInbox(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	alice := f.user(t, "alice")
	bob := f.user(t, "bob")
	// 古い順に 3 件を、別々の時刻に配る。
	for _, kind := range []notification.Kind{notification.KindRecordingStarted, notification.KindRecordingCompleted, notification.KindRecordingFailed} {
		f.emit(t, kind)
		f.dispatch(t)
		f.clock.Advance(time.Minute)
	}

	all := f.list(t, alice, &notification.ListQuery{UnreadOnly: false, Before: nil, Limit: 10})
	assert.Equal(t, []notification.Kind{notification.KindRecordingFailed, notification.KindRecordingCompleted, notification.KindRecordingStarted}, kinds(all))

	// 最初の 2 件の次のページは、最も古い 1 件だけである。
	page := f.list(t, alice, &notification.ListQuery{UnreadOnly: false, Before: nil, Limit: 2})
	require.Len(t, page, 2)
	next := f.list(t, alice, &notification.ListQuery{UnreadOnly: false, Before: &notification.Position{CreatedAt: page[1].CreatedAt, ID: page[1].ID}, Limit: 2})
	assert.Equal(t, []notification.Kind{notification.KindRecordingStarted}, kinds(next))

	// ほかのユーザーの通知は既読にできず、既読の通知は数えない。
	bobs := f.list(t, bob, &notification.ListQuery{UnreadOnly: false, Before: nil, Limit: 1})
	read, err := f.inbox.MarkRead(t.Context(), f.pool, alice, []uuid.UUID{all[0].ID, bobs[0].ID})
	require.NoError(t, err)
	assert.Equal(t, 1, read)
	read, err = f.inbox.MarkRead(t.Context(), f.pool, alice, []uuid.UUID{all[0].ID})
	require.NoError(t, err)
	assert.Zero(t, read)

	unread := f.list(t, alice, &notification.ListQuery{UnreadOnly: true, Before: nil, Limit: 10})
	assert.Equal(t, []notification.Kind{notification.KindRecordingCompleted, notification.KindRecordingStarted}, kinds(unread))
	marked := f.list(t, alice, &notification.ListQuery{UnreadOnly: false, Before: nil, Limit: 1})
	require.NotNil(t, marked[0].ReadAt)
	assert.True(t, f.clock.Now().Equal(*marked[0].ReadAt))

	read, err = f.inbox.MarkRead(t.Context(), f.pool, alice, []uuid.UUID{})
	require.NoError(t, err)
	assert.Zero(t, read)
	_, err = f.inbox.List(t.Context(), f.pool, alice, &notification.ListQuery{UnreadOnly: false, Before: nil, Limit: 0})
	require.ErrorIs(t, err, notification.ErrInvalidLimit)

	read, err = f.inbox.MarkAllRead(t.Context(), f.pool, alice)
	require.NoError(t, err)
	assert.Equal(t, 2, read)
	for userID, want := range map[uuid.UUID]int{alice: 0, bob: 3} {
		n, err := f.inbox.UnreadCount(t.Context(), f.pool, userID)
		require.NoError(t, err)
		assert.Equal(t, want, n)
	}
}

func TestPreferences(t *testing.T) {
	t.Parallel()

	defaults := func() []*notification.Preference {
		preferences := make([]*notification.Preference, 0, len(notification.Kinds()))
		for _, kind := range notification.Kinds() {
			preferences = append(preferences, &notification.Preference{Kind: kind, InApp: true, WebPush: true})
		}

		return preferences
	}
	tests := []struct {
		name    string
		set     []*notification.Preference
		want    func() []*notification.Preference
		wantErr error
	}{
		{name: "設定しなければどの通知先でも受け取る", set: nil, want: defaults, wantErr: nil},
		{
			name: "設定した種類だけを変え、設定し直すと上書きする",
			set: []*notification.Preference{
				{Kind: notification.KindStorageLow, InApp: false, WebPush: false},
				{Kind: notification.KindStorageLow, InApp: true, WebPush: false},
			},
			want: func() []*notification.Preference {
				preferences := defaults()
				preferences[4].WebPush = false

				return preferences
			},
			wantErr: nil,
		},
		{name: "知らない種類は設定できない", set: []*notification.Preference{{Kind: "unknown", InApp: false, WebPush: false}}, want: defaults, wantErr: notification.ErrUnknownKind},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			alice := f.user(t, "alice")
			for _, preference := range tt.set {
				err := f.inbox.SetPreference(t.Context(), f.pool, alice, preference)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)

					continue
				}
				require.NoError(t, err)
			}

			got, err := f.inbox.Preferences(t.Context(), f.pool, alice)
			require.NoError(t, err)
			assert.Equal(t, tt.want(), got)
		})
	}
}
