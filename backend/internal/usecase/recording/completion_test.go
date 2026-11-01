package recording_test

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// TestRecorderResolvesDetectCM は、録画の完了のイベントに載せる、CM の区間を見分けるかどうか (設計書 6.1、#1209) を、
// 予約やルールで選んでいればその値、なければ録画を終えるときの全体の設定から求めることを確かめる。
// 全体の設定は、予約を作った後に変える。予約を作ったときではなく、録画を終えるときの値に従うことも確かめるため。
func TestRecorderResolvesDetectCM(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	tests := []struct {
		name string
		// global は全体の設定。nil なら一度も変えていない (既定は見分けない)。
		global *bool
		// rule はルールの録画設定。nil ならルール予約にしない。ルールの上書きが nil のときは ruleFollows を true にする。
		rule        *bool
		ruleFollows bool
		reservation *bool
		want        bool
	}{
		{name: "全体を変えていなければ、見分けない", global: nil, reservation: nil, want: false},
		{name: "全体が使わないで、予約が全体に従う", global: new(false), reservation: nil, want: false},
		{name: "全体が使わないで、予約が解析する", global: new(false), reservation: new(true), want: true},
		{name: "全体が使わないで、予約が解析しない", global: new(false), reservation: new(false), want: false},
		{name: "全体が使うで、予約が全体に従う", global: new(true), reservation: nil, want: true},
		{name: "全体が使うで、予約が解析する", global: new(true), reservation: new(true), want: true},
		{name: "全体が使うで、予約が解析しない", global: new(true), reservation: new(false), want: false},
		{name: "全体が使わないで、ルールが解析し、予約が従う", global: new(false), rule: new(true), reservation: nil, want: true},
		{name: "全体が使うで、ルールが解析せず、予約が従う", global: new(true), rule: new(false), reservation: nil, want: false},
		{name: "全体が使うで、ルールも予約も従う", global: new(true), ruleFollows: true, reservation: nil, want: true},
		{name: "ルールが解析しても、予約の解析しないを優先する", global: new(false), rule: new(true), reservation: new(false), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			overrides, err := json.Marshal(&domain.RecordingOverrides{DetectCM: tt.reservation})
			require.NoError(t, err)
			_, err = f.pool.Exec(t.Context(), "UPDATE reservations SET settings = $1 WHERE id = $2", overrides, reservationID)
			require.NoError(t, err)
			if tt.rule != nil || tt.ruleFollows {
				ruleSettings, err := json.Marshal(&domain.RecordingOverrides{DetectCM: tt.rule})
				require.NoError(t, err)
				ruleID := id.New()
				_, err = f.pool.Exec(t.Context(), `INSERT INTO rules (id, name, memo, enabled, conditions, settings, avoid_duplicates, follow_event_relay, created_at, updated_at)
					VALUES ($1, 'ルール', '', true, '{"version":1,"conditions":{}}', $2, false, true, $3, $3)`, ruleID, ruleSettings, testNow())
				require.NoError(t, err)
				_, err = f.pool.Exec(t.Context(), "INSERT INTO reservation_rules (reservation_id, rule_id) VALUES ($1, $2)", reservationID, ruleID)
				require.NoError(t, err)
				_, err = f.pool.Exec(t.Context(), "UPDATE reservations SET kind = $1 WHERE id = $2", string(domain.ReservationKindRule), reservationID)
				require.NoError(t, err)
			}
			if tt.global != nil {
				_, err := settings.NewStore(f.clock, audit.NewRecorder(f.clock)).Update(t.Context(), f.pool, audit.System("test"), &settings.Changes{
					Recording: &domain.RecordingOverrides{DetectCM: tt.global},
				})
				require.NoError(t, err)
			}

			stream := newDrainedStream(data)
			recorder, _ := f.newRecorderWithDefaults(t, f.pool, stream, nil, nil, slog.New(slog.DiscardHandler), settings.RecordingDefaults)
			_, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			stream.wait(t)
			f.clock.Advance(time.Hour)
			require.NoError(t, wait(t, done))

			events := f.events(t)
			require.NotEmpty(t, events)
			last := events[len(events)-1]
			require.Equal(t, string(domain.NotificationKindRecordingCompleted), last.Kind)
			assert.Equal(t, tt.want, last.Payload["detectCm"])
		})
	}
}
