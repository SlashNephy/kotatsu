package graph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

type animeChannel struct {
	ServiceID                 string `json:"serviceId"`
	ImportedSyobocalChannelID *int   `json:"importedSyobocalChannelId"`
	SyobocalChannelIDOverride *int   `json:"syobocalChannelIdOverride"`
	SyobocalChannelID         *int   `json:"syobocalChannelId"`
	AnnictChannelID           *int   `json:"annictChannelId"`
}

// TestAnimeChannels は、管理者だけが、サービスとしょぼいカレンダー、Annict のチャンネルの対応を読み、上書きできることを確かめる (設計書 3.8)。
// 上書きは家族の中で完結するので、再認証は求めない (設計書 9.5)。
func TestAnimeChannels(t *testing.T) {
	t.Parallel()

	const (
		fieldSyobocal = "syobocalChannelId"
		listQuery     = `{ animeChannels { serviceId importedSyobocalChannelId syobocalChannelIdOverride syobocalChannelId annictChannelId } animeImports { source } }`
		update        = `mutation($id: ID!, $input: AnimeChannelInput!) { updateAnimeChannel(serviceId: $id, input: $input) {
			serviceId importedSyobocalChannelId syobocalChannelIdOverride syobocalChannelId annictChannelId } }`
	)
	f := newFixture(t, nil)
	f.syncEPG(t)
	// こたつテスト1 (地上波、サービス ID 0x0400)。
	serviceID := graph.GlobalID("Service", "3273601024")

	assert.Equal(t, []string{codeForbidden, codeForbidden}, f.query(t, listQuery, nil, nil), "only administrators can read the anime channels")
	input := map[string]any{"id": serviceID, varInput: map[string]any{fieldSyobocal: 19}}
	assert.Equal(t, []string{codeForbidden}, f.query(t, update, input, nil), "only administrators can change the anime channels")

	admin := f.as(t, adminName, auth.RoleAdmin)
	var listed struct {
		AnimeChannels []animeChannel   `json:"animeChannels"`
		AnimeImports  []map[string]any `json:"animeImports"`
	}
	require.Empty(t, admin.query(t, listQuery, nil, &listed))
	require.NotEmpty(t, listed.AnimeChannels)
	assert.Empty(t, listed.AnimeImports, "nothing is imported until the linking is enabled")
	for _, channel := range listed.AnimeChannels {
		assert.Nil(t, channel.SyobocalChannelID, "nothing is imported yet: %s", channel.ServiceID)
	}

	f.clock.Advance(auth.RecentAuthenticationWindow * 2)
	var updated struct {
		UpdateAnimeChannel animeChannel `json:"updateAnimeChannel"`
	}
	require.Empty(t, admin.query(t, update, input, &updated), "no reauthentication is needed")
	assert.Equal(t, animeChannel{
		ServiceID: serviceID, ImportedSyobocalChannelID: nil, SyobocalChannelIDOverride: new(19), SyobocalChannelID: new(19), AnnictChannelID: nil,
	}, updated.UpdateAnimeChannel)

	for _, bad := range []map[string]any{
		{"id": serviceID, varInput: map[string]any{fieldSyobocal: 0}},
		{"id": serviceID, varInput: map[string]any{fieldSyobocal: 1, "resetSyobocalChannelId": true}},
		{"id": serviceID, varInput: map[string]any{"annictChannelId": 1 << 40}},
	} {
		assert.Equal(t, []string{codeBadUserInput}, admin.query(t, update, bad, nil), "%v", bad)
	}
	assert.Equal(t, []string{codeNotFound}, admin.query(t, update, map[string]any{"id": graph.GlobalID("Service", "1"), varInput: map[string]any{fieldSyobocal: 1}}, nil))

	require.Empty(t, admin.query(t, update, map[string]any{"id": serviceID, varInput: map[string]any{"resetSyobocalChannelId": true}}, &updated))
	assert.Nil(t, updated.UpdateAnimeChannel.SyobocalChannelID)
}
