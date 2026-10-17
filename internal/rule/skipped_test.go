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
// 手動で予約した番組と、ほかのルールで予約した番組は返さないことを確かめる (#469)。
func TestFindSkippedDuplicates(t *testing.T) {
	t.Parallel()

	f := newEngineFixture(t)
	location, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)
	recordedEnd := testNow().Add(-47 * time.Hour)
	for _, name := range []string{"こたつテスト1 テスト番組 5", "こたつテスト1 テスト番組 6", "こたつテスト1 テスト番組 7"} {
		_, err := f.pool.Exec(t.Context(),
			"INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $6)",
			id.New(), serviceID(), rule.TitleKey(name), name, testNow().Add(-48*time.Hour), recordedEnd)
		require.NoError(t, err)
	}
	avoiding := f.createRule(t, `テスト番組 (5|6|7)$`, func(in *rule.Input) { in.AvoidDuplicates = true })
	// 7 は、重複の回避をしないルールにも一致するので、予約する。
	f.createRule(t, `テスト番組 7$`, nil)
	// 6 は、利用者が手動で予約した。
	_, _, err = f.reservations.CreateProgram(t.Context(), f.pool, f.user, &reserve.ProgramInput{ServiceID: serviceID(), EventID: 6, FollowEventRelay: false, Overrides: nil})
	require.NoError(t, err)
	f.run(t)

	skipped, err := rule.FindSkippedDuplicates(t.Context(), f.pool, f.clock, location)
	require.NoError(t, err)
	require.Len(t, skipped, 1)
	assert.Equal(t, int64(5), skipped[0].EventID)
	assert.Equal(t, serviceID(), skipped[0].ServiceID)
	assert.Equal(t, []*rule.SkippedRule{{ID: avoiding.ID, Name: avoiding.Name}}, skipped[0].Rules)
	assert.WithinDuration(t, recordedEnd, skipped[0].LastRecordedAt, time.Second)
	// 見送った番組は予約しない。
	assert.Equal(t, map[int64]bool{6: true, 7: true}, f.reserved(t))
}
