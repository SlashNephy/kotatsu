package neko_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/match"
	"github.com/SlashNephy/kotatsu/internal/neko"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
)

// TestLeaderSyncsEPGAndMatchesRules は、リーダーになった neko が、チューナーサーバーの EPG を DB に同期し、
// ルールが作られると、その変更の通知を受けて予約を作ることを確かめる。
func TestLeaderSyncsEPGAndMatchesRules(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	options := faketuner.DefaultOptions(time.Now(), nil)
	server := httptest.NewServer(faketuner.New(options))
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- neko.Run(ctx, slog.New(slog.DiscardHandler), &neko.Options{
			DB:         pool,
			Clock:      clock.Real{},
			InstanceID: id.New(),
			Tuner:      client,
			EPG:        nil,
			Archive:    nil,
			Tuners:     nil,
			Location:   time.UTC,
			Assign:     nil,
			Follow:     nil,
			Rules:      &rule.RunnerOptions{Debounce: 10 * time.Millisecond, Interval: time.Hour, PollOnly: false, PollInterval: 0},
		})
	}()

	require.Eventually(t, func() bool {
		var n int
		require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM programs").Scan(&n))

		return n == len(options.Programs)
	}, 10*time.Second, 20*time.Millisecond)

	// こたつテスト1 の eventId 10 は、今から数時間後に放送される。
	rules := rule.NewRules(clock.Real{}, audit.NewRecorder(clock.Real{}))
	_, err = rules.Create(t.Context(), pool, audit.System("test"), &rule.Input{
		Name: "テスト", Memo: "", Enabled: true,
		Conditions: &match.Conditions{
			Keywords:   []*match.Keyword{{Text: "こたつテスト1 テスト番組 10$", Regexp: true, CaseSensitive: false, Fields: []match.Field{match.FieldName}}},
			ServiceIDs: []int64{faketuner.ServiceItemID(0x7FE0, 0x0400)},
		},
		Overrides: nil, ValidFrom: nil, ValidUntil: nil, AvoidDuplicates: false, DuplicateWindowDays: nil, FollowEventRelay: true,
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var n int
		require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM reservations WHERE kind = 'rule' AND event_id = 10").Scan(&n))

		return n == 1
	}, 10*time.Second, 20*time.Millisecond)

	cancel()
	assert.NoError(t, <-done)
}
