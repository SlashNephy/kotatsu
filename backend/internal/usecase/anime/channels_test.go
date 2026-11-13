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
	importer := anime.NewImporter(db, clk, newFakeFetcher(), newFakeTitles(t), nil, slog.New(slog.DiscardHandler))
	_, err := importer.Import(t.Context(), false)
	require.NoError(t, err)
	channels := anime.NewChannels(clk, audit.NewRecorder(clk), nil)
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
	channels := anime.NewChannels(clk, audit.NewRecorder(clk), nil)
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

// TestListChannelsTerrestrial は、地上波のサービスに、取り込んだ対応を当てる規則を確かめる (#1566)。
// 種別が地上波でない (他の地域を受ける EXT などの) 地上波のサービスには、NHK と独立局の対応だけを当て、系列局には当てない。
// サブチャンネルにはメインの対応を当て、しょぼいカレンダーで別のチャンネルを持つサブチャンネルには、そのチャンネルを当てる。
func TestListChannelsTerrestrial(t *testing.T) {
	t.Parallel()

	// extType は、地上波の他の地域の局を受ける、Mirakurun の地上波でない種別。
	const extType = "EXT1"
	db := newDB(t)
	_, err := db.Exec(t.Context(), `INSERT INTO channel_types (type, display_name, sort_order) VALUES ('EXT1', 'EXT1', 3), ('BS4K', 'BS4K', 4)`)
	require.NoError(t, err)
	// saya-definitions の形のまま、地上波はネットワーク ID 0 で入れる。TBS の項目は、系列局の MBS (2064) を含む。
	_, err = db.Exec(t.Context(), `INSERT INTO anime_channel_definitions (channel_type, network_id, sid, name, syobocal_channel_id, annict_channel_id) VALUES
		('GR', 0, 1024, 'NHK 総合', 1, 1), ('GR', 0, 41984, 'NHK 総合', 1, 1),
		('GR', 0, 1032, 'Eテレ', 2, 2), ('GR', 0, 2056, 'Eテレ', 2, 2),
		('GR', 0, 1048, 'TBS', 5, 5), ('GR', 0, 2064, 'TBS', 5, 5),
		('GR', 0, 42032, 'KBS京都', 66, 66), ('GR', 0, 23608, 'TOKYO MX', 19, 19),
		('BS', 4, 101, 'NHKBS1', 9, 9)`)
	require.NoError(t, err)

	tests := []struct {
		name        string
		channelType string
		networkID   int64
		sid         int64
		// wantName が nil なら、対応を当てない。
		wantName     *string
		wantSyobocal *int32
		wantAnnict   *int32
	}{
		{name: "GR main", channelType: "GR", networkID: 32736, sid: 1024, wantName: new("NHK 総合"), wantSyobocal: new(int32(1)), wantAnnict: new(int32(1))},
		{name: "GR key station", channelType: "GR", networkID: 32739, sid: 1048, wantName: new("TBS"), wantSyobocal: new(int32(5)), wantAnnict: new(int32(5))},
		{name: "EXT NHK of another area", channelType: extType, networkID: 32096, sid: 41984, wantName: new("NHK 総合"), wantSyobocal: new(int32(1)), wantAnnict: new(int32(1))},
		{name: "EXT Eテレ of another area", channelType: extType, networkID: 32721, sid: 2056, wantName: new("Eテレ"), wantSyobocal: new(int32(2)), wantAnnict: new(int32(2))},
		{name: "EXT independent station", channelType: extType, networkID: 32102, sid: 42032, wantName: new("KBS京都"), wantSyobocal: new(int32(66)), wantAnnict: new(int32(66))},
		// 系列局は、キー局のチャンネルの放送時間データで照合すると、別の作品に結びつくことがある。
		{name: "EXT affiliate", channelType: extType, networkID: 32722, sid: 2064, wantName: nil, wantSyobocal: nil, wantAnnict: nil},
		{name: "EXT affiliate subchannel", channelType: extType, networkID: 32722, sid: 2065, wantName: nil, wantSyobocal: nil, wantAnnict: nil},
		{name: "GR subchannel", channelType: "GR", networkID: 32736, sid: 1025, wantName: new("NHK 総合"), wantSyobocal: new(int32(1)), wantAnnict: new(int32(1))},
		{name: "EXT subchannel of NHK", channelType: extType, networkID: 32096, sid: 41985, wantName: new("NHK 総合"), wantSyobocal: new(int32(1)), wantAnnict: new(int32(1))},
		{name: "Eテレ2", channelType: "GR", networkID: 32737, sid: 1033, wantName: new("Eテレ"), wantSyobocal: new(int32(64)), wantAnnict: nil},
		{name: "Eテレ3", channelType: "GR", networkID: 32737, sid: 1034, wantName: new("Eテレ"), wantSyobocal: new(int32(65)), wantAnnict: nil},
		{name: "TOKYO MX subchannel", channelType: "GR", networkID: 32391, sid: 23609, wantName: new("TOKYO MX"), wantSyobocal: new(int32(19)), wantAnnict: new(int32(19))},
		{name: "TOKYO MX2", channelType: "GR", networkID: 32391, sid: 23610, wantName: new("TOKYO MX"), wantSyobocal: new(int32(187)), wantAnnict: nil},
		// サブチャンネルは、メインの 2 つ後まで。
		{name: "beyond subchannels", channelType: "GR", networkID: 32736, sid: 1027, wantName: nil, wantSyobocal: nil, wantAnnict: nil},
		// 地上波でないネットワークのサービスには、サービス ID が同じでも、地上波の対応を当てない。
		{name: "EXT not terrestrial", channelType: extType, networkID: 65534, sid: 1024, wantName: nil, wantSyobocal: nil, wantAnnict: nil},
		{name: "BS", channelType: "BS", networkID: 4, sid: 101, wantName: new("NHKBS1"), wantSyobocal: new(int32(9)), wantAnnict: new(int32(9))},
		{name: "BS4K", channelType: "BS4K", networkID: 11, sid: 101, wantName: nil, wantSyobocal: nil, wantAnnict: nil},
	}
	// newDB のサービスと ID がぶつからないよう、ずらす。
	idOf := func(networkID, sid int64) int64 { return 1_000_000_000_000 + networkID*100000 + sid }
	for _, tt := range tests {
		_, err := db.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
			VALUES ($1, $2, $3, 'テスト', 'テスト', 1, $4, '1', now(), now())`, idOf(tt.networkID, tt.sid), tt.networkID, tt.sid, tt.channelType)
		require.NoError(t, err)
	}
	list, err := anime.ListChannels(t.Context(), db)
	require.NoError(t, err)
	channels := map[int64]*anime.Channel{}
	for _, channel := range list {
		channels[channel.ServiceID] = channel
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := channels[idOf(tt.networkID, tt.sid)]
			require.True(t, ok)
			assert.Equal(t, tt.wantName, got.DefinitionName)
			assert.Equal(t, tt.wantSyobocal, got.ImportedSyobocalChannelID)
			assert.Equal(t, tt.wantSyobocal, got.SyobocalChannelID)
			assert.Equal(t, tt.wantAnnict, got.AnnictChannelID)
		})
	}
}
