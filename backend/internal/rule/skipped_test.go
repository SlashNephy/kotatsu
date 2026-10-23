package rule_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
)

// TestFindSkippedDuplicates は、重複の回避で予約しなかった番組を、外したルールと前に録った日時と一緒に返し、
// 予約のある番組と、重複の回避で外れなかった番組は返さないことを確かめる (#469)。
// こたつテスト1 の「テスト番組 5」を、2 日前に録画済みにしておく。
func TestFindSkippedDuplicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// window は、重複の回避の日数。
		window *int
		// otherRule は、重複の回避をしないルールにも一致させるかどうか。
		otherRule bool
		// manual は、手動で予約しておくかどうか。
		manual       bool
		wantSkipped  bool
		wantReserved bool
	}{
		{name: "期間内に録画済みなら見送る", window: new(7), otherRule: false, manual: false, wantSkipped: true, wantReserved: false},
		{name: "期間より前に録画したなら予約する", window: new(1), otherRule: false, manual: false, wantSkipped: false, wantReserved: true},
		{name: "重複の回避をしないルールにも一致すれば予約する", window: new(7), otherRule: true, manual: false, wantSkipped: false, wantReserved: true},
		{name: "手動で予約した番組は返さない", window: new(7), otherRule: false, manual: true, wantSkipped: false, wantReserved: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newEngineFixture(t)
			location, err := time.LoadLocation("Asia/Tokyo")
			require.NoError(t, err)
			const name = "こたつテスト1 テスト番組 5"
			recordedEnd := testNow().Add(-47 * time.Hour)
			_, err = f.pool.Exec(t.Context(),
				"INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $6)",
				id.New(), serviceID(), rule.TitleKey(name), name, testNow().Add(-48*time.Hour), recordedEnd)
			require.NoError(t, err)
			avoiding := f.createRule(t, `テスト番組 5$`, func(in *rule.Input) {
				in.AvoidDuplicates = true
				in.DuplicateWindowDays = tt.window
			})
			if tt.otherRule {
				f.createRule(t, `テスト番組 5$`, nil)
			}
			if tt.manual {
				_, _, err := f.reservations.CreateProgram(t.Context(), f.pool, f.user,
					&reserve.ProgramInput{ServiceID: serviceID(), EventID: 5, FollowEventRelay: false, Overrides: nil})
				require.NoError(t, err)
			}
			f.run(t)

			skipped, err := rule.FindSkippedDuplicates(t.Context(), f.pool, f.clock, location)
			require.NoError(t, err)
			if tt.wantSkipped {
				require.Len(t, skipped, 1)
				assert.Equal(t, serviceID(), skipped[0].ServiceID)
				assert.Equal(t, int64(5), skipped[0].EventID)
				assert.Equal(t, []*rule.SkippedRule{{ID: avoiding.ID, Name: avoiding.Name}}, skipped[0].Rules)
				assert.WithinDuration(t, recordedEnd, skipped[0].LastRecordedAt, time.Second)
			} else {
				assert.Empty(t, skipped)
			}
			assert.Equal(t, tt.wantReserved, f.reserved(t)[5])
		})
	}
}
