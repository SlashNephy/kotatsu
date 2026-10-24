package notification_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/notification"
)

// TestLoadReservationContext は、手動の予約は予約したユーザーに、ルール予約はルールを作ったユーザーに届け、
// 分からなければ全員に届けることを確かめる。
func TestLoadReservationContext(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	alice, bob := f.user(t, "alice"), f.user(t, "bob")
	now := f.clock.Now()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)`)
	require.NoError(t, err)
	const serviceID = 3273601024
	_, err = f.pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $2, $2)`, serviceID, now)
	require.NoError(t, err)
	// 番組予約とルール予約には eventId が要り、時刻予約には付けない。
	nextEvent := int64(0)
	eventID := func(kind string) *int64 {
		if kind == "time" {
			return nil
		}
		nextEvent++

		return new(nextEvent)
	}
	reservation := func(kind string, createdBy *uuid.UUID) uuid.UUID {
		reservationID := id.New()
		_, err := f.pool.Exec(t.Context(), `INSERT INTO reservations (id, kind, service_id, event_id, start_at, end_at, name, description,
			extended, genres, settings, created_at, created_by, updated_at)
			VALUES ($1, $2, $3, $7, $4, $5, '番組', '', '{}', '[]', '{}', $4, $6, $4)`,
			reservationID, kind, serviceID, now, now.Add(time.Hour), createdBy, eventID(kind))
		require.NoError(t, err)

		return reservationID
	}
	rule := func(name string, createdAt time.Time, createdBy *uuid.UUID) uuid.UUID {
		ruleID := id.New()
		_, err := f.pool.Exec(t.Context(), `INSERT INTO rules (id, name, memo, enabled, conditions, settings, avoid_duplicates,
			follow_event_relay, created_at, created_by, updated_at) VALUES ($1, $2, '', true, '{}', '{}', false, true, $3, $4, $3)`,
			ruleID, name, createdAt, createdBy)
		require.NoError(t, err)

		return ruleID
	}
	link := func(reservationID uuid.UUID, ruleIDs ...uuid.UUID) {
		for _, ruleID := range ruleIDs {
			_, err := f.pool.Exec(t.Context(), `INSERT INTO reservation_rules (reservation_id, rule_id) VALUES ($1, $2)`, reservationID, ruleID)
			require.NoError(t, err)
		}
	}

	byAlice := reservation("program", &alice)
	imported := reservation("time", nil)
	older := rule("古いルール", now.Add(-time.Hour), &bob)
	newer := rule("新しいルール", now, &alice)
	byRules := reservation("rule", nil)
	link(byRules, newer, older)
	orphanRule := rule("作者のいないルール", now, nil)
	byOrphanRule := reservation("rule", nil)
	link(byOrphanRule, orphanRule)

	tests := []struct {
		name          string
		reservationID uuid.UUID
		want          *notification.ReservationContext
	}{
		{
			name: "手動の予約は、予約したユーザーにだけ届ける", reservationID: byAlice,
			want: &notification.ReservationContext{
				Audience: &notification.Audience{UserIDs: []uuid.UUID{alice}, ByRule: false}, Kind: "program", Rules: []notification.ReservationRule{},
			},
		},
		{
			name: "予約したユーザーが分からなければ、全員に届ける", reservationID: imported,
			want: &notification.ReservationContext{Audience: nil, Kind: "time", Rules: []notification.ReservationRule{}},
		},
		{
			name: "ルール予約は、ルールを作ったユーザーに届け、ルールを作った順に載せる", reservationID: byRules,
			want: &notification.ReservationContext{
				Audience: &notification.Audience{UserIDs: []uuid.UUID{alice, bob}, ByRule: true}, Kind: "rule",
				Rules: []notification.ReservationRule{{ID: older.String(), Name: "古いルール"}, {ID: newer.String(), Name: "新しいルール"}},
			},
		},
		{
			name: "ルールを作ったユーザーが分からなければ、全員に届ける", reservationID: byOrphanRule,
			want: &notification.ReservationContext{
				Audience: &notification.Audience{UserIDs: nil, ByRule: true}, Kind: "rule",
				Rules: []notification.ReservationRule{{ID: orphanRule.String(), Name: "作者のいないルール"}},
			},
		},
		{
			name: "予約が消えていれば、全員に届ける", reservationID: id.New(),
			want: &notification.ReservationContext{Audience: nil, Kind: "", Rules: []notification.ReservationRule{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := notification.LoadReservationContext(t.Context(), f.pool, tt.reservationID)
			require.NoError(t, err)
			if got.Audience != nil && tt.want.Audience != nil {
				assert.ElementsMatch(t, tt.want.Audience.UserIDs, got.Audience.UserIDs)
				got.Audience.UserIDs, tt.want.Audience.UserIDs = nil, nil
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
