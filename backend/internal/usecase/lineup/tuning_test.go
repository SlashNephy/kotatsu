package lineup_test

import (
	"log/slog"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/lineup"
)

// tuningBase は、選局の結果の時刻の起点を返す。
func tuningBase() time.Time {
	return time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
}

// refused は、地上波のチャンネル channel を、起点から at に選局して拒否された結果。
func refused(channel string, at time.Duration) lineup.TuningOutcome {
	return lineup.TuningOutcome{ChannelType: "GR", Channel: channel, At: tuningBase().Add(at), Received: false}
}

// received は、地上波のチャンネル channel を、起点から at に選局して受信できた結果。
func received(channel string, at time.Duration) lineup.TuningOutcome {
	return lineup.TuningOutcome{ChannelType: "GR", Channel: channel, At: tuningBase().Add(at), Received: true}
}

// TestUnreachable は、続けて選局できないチャンネルを決める規則を確認する (#1519)。
func TestUnreachable(t *testing.T) {
	t.Parallel()

	rule := lineup.UnreachableRule{SameRoundInterval: time.Hour, Threshold: 3}
	tests := []struct {
		name     string
		outcomes []lineup.TuningOutcome
		want     map[string][]string
	}{
		{
			name: "回数に届き、ほかのチャンネルが受信できていれば決める",
			outcomes: []lineup.TuningOutcome{
				refused("22", 0), received("27", 30*time.Minute), refused("22", 2*time.Hour), refused("22", 4*time.Hour),
			},
			want: map[string][]string{"GR": {"22"}},
		},
		{
			name:     "回数に届かなければ決めない",
			outcomes: []lineup.TuningOutcome{refused("22", 0), received("27", 30*time.Minute), refused("22", 2*time.Hour)},
			want:     map[string][]string{},
		},
		{
			name: "種別のほかのチャンネルも受信できていなければ、アンテナかチューナーの不調とみなして決めない",
			outcomes: []lineup.TuningOutcome{
				refused("22", 0), refused("27", 10*time.Minute), refused("22", 2*time.Hour), refused("22", 4*time.Hour),
			},
			want: map[string][]string{},
		},
		{
			name: "ほかのチャンネルの受信は、続き始めた後のものだけを数える",
			outcomes: []lineup.TuningOutcome{
				received("27", 0), refused("22", time.Hour), refused("22", 3*time.Hour), refused("22", 5*time.Hour),
			},
			want: map[string][]string{},
		},
		{
			name: "ほかの種別の受信は数えない",
			outcomes: []lineup.TuningOutcome{
				refused("22", 0),
				{ChannelType: "BS", Channel: "BS15_0", At: tuningBase().Add(30 * time.Minute), Received: true},
				refused("22", 2*time.Hour), refused("22", 4*time.Hour),
			},
			want: map[string][]string{},
		},
		{
			name: "前に数えた失敗から間隔の内の失敗は、同じ 1 回と数える",
			outcomes: []lineup.TuningOutcome{
				refused("22", 0), received("27", 10*time.Minute), refused("22", 30*time.Minute), refused("22", time.Hour),
				refused("22", 90*time.Minute),
			},
			// 0 分と 90 分の 2 回。30 分と 60 分は、0 分と同じ回。
			want: map[string][]string{},
		},
		{
			name: "同じ回の失敗が続いても、前に数えた失敗から間隔を過ぎたら、次の回と数える",
			outcomes: []lineup.TuningOutcome{
				refused("22", 0), received("27", 10*time.Minute), refused("22", 30*time.Minute), refused("22", time.Hour),
				refused("22", 90*time.Minute), refused("22", 2*time.Hour), refused("22", 150*time.Minute),
				refused("22", 3*time.Hour+time.Minute),
			},
			// 0 分、90 分、181 分の 3 回。
			want: map[string][]string{"GR": {"22"}},
		},
		{
			name: "受信できたら 0 に戻す",
			outcomes: []lineup.TuningOutcome{
				refused("22", 0), received("27", 10*time.Minute), refused("22", 2*time.Hour), received("22", 3*time.Hour),
				refused("22", 4*time.Hour),
			},
			want: map[string][]string{},
		},
		{
			name: "受信できた後の最初の失敗は、前の失敗から間がなくても 1 回と数える",
			outcomes: []lineup.TuningOutcome{
				refused("22", 0), received("22", 10*time.Minute), refused("22", 20*time.Minute), received("27", 30*time.Minute),
				refused("22", 2*time.Hour), refused("22", 4*time.Hour),
			},
			want: map[string][]string{"GR": {"22"}},
		},
		{
			name: "チャンネルの昇順に並べる",
			outcomes: []lineup.TuningOutcome{
				refused("25", 0), refused("22", 0), received("27", 30*time.Minute),
				refused("25", 2*time.Hour), refused("22", 2*time.Hour), refused("25", 4*time.Hour), refused("22", 4*time.Hour),
			},
			want: map[string][]string{"GR": {"22", "25"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, lineup.Unreachable(tt.outcomes, rule))
		})
	}
}

// grChannelTV は、チューナーサーバーの地上波のチャンネル channel のテレビのサービス。
func grChannelTV(networkID, serviceID int, channel string) *tunerserver.Service {
	return &tunerserver.Service{
		ID: int64(networkID*100000 + serviceID), ServiceID: serviceID, NetworkID: networkID, Name: "GR" + channel, Type: 0x01,
		LogoID: nil, RemoteControlKeyID: nil, Channel: &tunerserver.ChannelRef{Type: "GR", Channel: channel},
	}
}

// TestCheckerTuning は、録画の結果から続けて選局できないチャンネルを決め、管理者に 1 回だけ通知することを確認する (#1519)。
func TestCheckerTuning(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	q := sqlc.New(pool)
	clk := clocktest.NewFake(tuningBase().Add(24 * time.Hour))
	createAdmin(t, pool, clk)
	require.NoError(t, q.EnsureChannelType(t.Context(), sqlc.EnsureChannelTypeParams{Type: "GR", DisplayName: "地上波", SortOrder: 0}))
	dead := grChannelTV(32000, 1, "22")
	alive := grChannelTV(32001, 1, "27")
	for _, service := range []*tunerserver.Service{dead, alive} {
		require.NoError(t, q.UpsertService(t.Context(), sqlc.UpsertServiceParams{
			ID: service.ID, NetworkID: int64(service.NetworkID), Sid: int64(service.ServiceID), Name: service.Name,
			NameNormalized: service.Name, ServiceType: int64(service.Type), ChannelType: "GR", Channel: service.Channel.Channel,
			RemoteControlKeyID: nil, LogoID: nil, Now: tuningBase(),
		}))
	}
	record := func(serviceID int64, at time.Duration, state domain.RecordingState, reason *domain.RecordingFailureReason) {
		t.Helper()

		_, err := pool.Exec(t.Context(), `INSERT INTO recordings (
  id, service_id, name, description, start_at, end_at, state, failure_reason, recorded_from, created_at, updated_at
) VALUES ($1, $2, 'test', '', $3, $3, $4, $5, $3, $3, $3)`, uuid.New(), serviceID, tuningBase().Add(at), state, reason)
		require.NoError(t, err)
	}
	refusedReason := new(domain.RecordingFailureReasonTuningRefused)
	tuner := new(fakeServices)
	tuner.set(dead, alive)
	checker := lineup.NewChecker(pool, tuner, clk, slog.New(slog.DiscardHandler), 0)

	// チューナーの空きがなかった失敗は数えない。
	record(dead.ID, 0, domain.RecordingStateFailed, refusedReason)
	record(alive.ID, 30*time.Minute, domain.RecordingStateCompleted, nil)
	record(dead.ID, 2*time.Hour, domain.RecordingStateFailed, new(domain.RecordingFailureReasonTunerUnavailable))
	record(dead.ID, 4*time.Hour, domain.RecordingStateFailed, refusedReason)
	require.NoError(t, checker.Check(t.Context()))
	assert.Empty(t, lineupNotifications(t, pool))

	// 3 回目で通知する。続いている間は繰り返さない。
	record(dead.ID, 6*time.Hour, domain.RecordingStateFailed, refusedReason)
	require.NoError(t, checker.Check(t.Context()))
	require.NoError(t, checker.Check(t.Context()))
	got := lineupNotifications(t, pool)
	require.Len(t, got, 1)
	assert.Equal(t, "GR", got[0][keyKind])
	assert.Equal(t, []any{"22"}, got[0]["unreachable"])

	// チューナーサーバーの一覧から消えたチャンネルは通知せず、忘れる。戻ったら、また通知する。
	tuner.set(alive)
	require.NoError(t, checker.Check(t.Context()))
	tuner.set(dead, alive)
	require.NoError(t, checker.Check(t.Context()))
	assert.Len(t, lineupNotifications(t, pool), 2)

	// 受信できたら忘れ、また続けて拒否されたら通知する。
	record(dead.ID, 8*time.Hour, domain.RecordingStatePartial, nil)
	require.NoError(t, checker.Check(t.Context()))
	assert.Len(t, lineupNotifications(t, pool), 2)
	record(dead.ID, 9*time.Hour, domain.RecordingStateFailed, refusedReason)
	record(alive.ID, 9*time.Hour+30*time.Minute, domain.RecordingStateCompletedWithErrors, nil)
	record(dead.ID, 11*time.Hour, domain.RecordingStateFailed, refusedReason)
	record(dead.ID, 13*time.Hour, domain.RecordingStateFailed, refusedReason)
	require.NoError(t, checker.Check(t.Context()))
	assert.Len(t, lineupNotifications(t, pool), 3)

	// 受信できた録画を消しても、録画履歴に残っていれば、受信できたものに数えて忘れる。続いて 3 回断られたら、また通知する。
	_, err = pool.Exec(t.Context(), `INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at)
VALUES ($1, $2, 'test', 'test', $3, $3, $3)`, uuid.New(), dead.ID, tuningBase().Add(14*time.Hour))
	require.NoError(t, err)
	require.NoError(t, checker.Check(t.Context()))
	record(dead.ID, 15*time.Hour, domain.RecordingStateFailed, refusedReason)
	record(alive.ID, 15*time.Hour+30*time.Minute, domain.RecordingStateCompleted, nil)
	record(dead.ID, 17*time.Hour, domain.RecordingStateFailed, refusedReason)
	require.NoError(t, checker.Check(t.Context()))
	assert.Len(t, lineupNotifications(t, pool), 3)
	record(dead.ID, 19*time.Hour, domain.RecordingStateFailed, refusedReason)
	require.NoError(t, checker.Check(t.Context()))
	assert.Len(t, lineupNotifications(t, pool), 4)
}
