package anime_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

// TestUpdateChannel は、管理者の上書きを項目ごとに取り込んだ対応に重ね、取り込み直しても残すことを確かめる。
// 上書きを消すと取り込んだ対応に戻り、変えたことは監査ログに残す。
func TestUpdateChannel(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	enable(t, db, clk)
	importer := anime.NewImporter(db, clk, newFakeFetcher(), slog.New(slog.DiscardHandler))
	_, err := importer.Import(t.Context(), false)
	require.NoError(t, err)
	channels := anime.NewChannels(clk, audit.NewRecorder(clk))
	actor := audit.System("test")

	channelOf := func(serviceID int64) *anime.Channel {
		t.Helper()

		list, err := anime.ListChannels(t.Context(), db)
		require.NoError(t, err)
		for _, channel := range list {
			if channel.ServiceID == serviceID {
				return channel
			}
		}
		require.Failf(t, "no channel", "service %d", serviceID)

		return nil
	}

	// しょぼいカレンダーの ChID だけを上書きする。Annict のチャンネルは取り込んだ対応のまま。
	require.NoError(t, channels.Update(t.Context(), db, actor, grService, &anime.ChannelChanges{
		SyobocalChannelID: new(int32(19)), ResetSyobocalChannelID: false, AnnictChannelID: nil, ResetAnnictChannelID: false,
	}))
	got := channelOf(grService)
	assert.Equal(t, new(int32(19)), got.SyobocalChannelID)
	assert.Equal(t, new(int32(19)), got.OverrideSyobocalChannelID)
	assert.Equal(t, new(int32(1)), got.ImportedSyobocalChannelID)
	assert.Equal(t, new(int32(1)), got.AnnictChannelID)

	// 取り込んだ対応がないサービスにも、上書きで対応を付けられる。
	require.NoError(t, channels.Update(t.Context(), db, actor, csService, &anime.ChannelChanges{
		SyobocalChannelID: nil, ResetSyobocalChannelID: false, AnnictChannelID: new(int32(7)), ResetAnnictChannelID: false,
	}))
	assert.Equal(t, new(int32(7)), channelOf(csService).AnnictChannelID)
	assert.Nil(t, channelOf(csService).SyobocalChannelID)

	// 取り込み直しても、上書きは残る。
	_, err = importer.Import(t.Context(), true)
	require.NoError(t, err)
	assert.Equal(t, new(int32(19)), channelOf(grService).SyobocalChannelID)

	// 上書きを消すと、取り込んだ対応に戻り、上書きの行も消える。
	require.NoError(t, channels.Update(t.Context(), db, actor, grService, &anime.ChannelChanges{
		SyobocalChannelID: nil, ResetSyobocalChannelID: true, AnnictChannelID: nil, ResetAnnictChannelID: false,
	}))
	assert.Equal(t, new(int32(1)), channelOf(grService).SyobocalChannelID)
	var overrides int
	require.NoError(t, db.QueryRow(t.Context(), "SELECT count(*) FROM anime_channel_overrides WHERE service_id = $1", grService).Scan(&overrides))
	assert.Zero(t, overrides)

	var audited int
	require.NoError(t, db.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE action = 'service.anime_channel_updated'").Scan(&audited))
	assert.Equal(t, 3, audited)
}

// TestUpdateChannelRejects は、正しくない上書きと、ないサービスを拒むことを確かめる。
func TestUpdateChannelRejects(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	channels := anime.NewChannels(clk, audit.NewRecorder(clk))
	tests := []struct {
		name      string
		serviceID int64
		changes   *anime.ChannelChanges
		want      error
	}{
		{
			name: "zero", serviceID: grService, want: anime.ErrInvalidChannel,
			changes: &anime.ChannelChanges{SyobocalChannelID: new(int32(0)), ResetSyobocalChannelID: false, AnnictChannelID: nil, ResetAnnictChannelID: false},
		},
		{
			name: "set and reset", serviceID: grService, want: anime.ErrInvalidChannel,
			changes: &anime.ChannelChanges{SyobocalChannelID: nil, ResetSyobocalChannelID: false, AnnictChannelID: new(int32(1)), ResetAnnictChannelID: true},
		},
		{
			name: "unknown service", serviceID: 1, want: anime.ErrServiceNotFound,
			changes: &anime.ChannelChanges{SyobocalChannelID: new(int32(1)), ResetSyobocalChannelID: false, AnnictChannelID: nil, ResetAnnictChannelID: false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := channels.Update(t.Context(), db, audit.System("test"), tt.serviceID, tt.changes)
			require.ErrorIs(t, err, tt.want)
		})
	}
}
