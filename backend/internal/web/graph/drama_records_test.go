package graph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

// TestDramaRecordSettings は、見たドラマの話にする割合を、Simkl と連携していなくても、TMDb との連携が有効なら
// ユーザーごとに変えられ、見たアニメを記録する割合とは別に持つことを確かめる (#1649)。
func TestDramaRecordSettings(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	viewer := f.as(t, "viewer", domain.RoleViewer)
	other := f.as(t, "other", domain.RoleViewer)

	const (
		read   = `{ dramaRecordSettings { enabled thresholdPercent destinations { destination available connection { accountName } } } }`
		update = `mutation($input: DramaRecordSettingsInput!) { updateDramaRecordSettings(input: $input) { thresholdPercent } }`
		anime  = `{ animeRecordSettings { thresholdPercent } }`
	)
	type settings struct {
		DramaRecordSettings struct {
			Enabled          bool `json:"enabled"`
			ThresholdPercent int  `json:"thresholdPercent"`
			Destinations     []struct {
				Destination string `json:"destination"`
				Available   bool   `json:"available"`
				Connection  *struct {
					AccountName string `json:"accountName"`
				} `json:"connection"`
			} `json:"destinations"`
		} `json:"dramaRecordSettings"`
	}
	var got settings
	require.Empty(t, viewer.query(t, read, nil, &got))
	assert.False(t, got.DramaRecordSettings.Enabled, "TMDb linking is disabled by default")
	assert.Equal(t, 20, got.DramaRecordSettings.ThresholdPercent)
	require.Len(t, got.DramaRecordSettings.Destinations, 1)
	assert.Equal(t, "SIMKL", got.DramaRecordSettings.Destinations[0].Destination)
	assert.False(t, got.DramaRecordSettings.Destinations[0].Available, "no Simkl app is registered")
	assert.Nil(t, got.DramaRecordSettings.Destinations[0].Connection)

	tests := []struct {
		name    string
		percent int
		want    []string
		saved   int
	}{
		{name: "saves a percent", percent: 50, want: []string{}, saved: 50},
		{name: "refuses zero", percent: 0, want: []string{codeBadUserInput}, saved: 50},
		{name: "refuses over 100", percent: 101, want: []string{codeBadUserInput}, saved: 50},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, viewer.query(t, update, map[string]any{varInput: map[string]any{"thresholdPercent": tt.percent}}, nil), tt.name)
		require.Empty(t, viewer.query(t, read, nil, &got))
		assert.Equal(t, tt.saved, got.DramaRecordSettings.ThresholdPercent, tt.name)
	}

	var animeSettings struct {
		AnimeRecordSettings struct {
			ThresholdPercent int `json:"thresholdPercent"`
		} `json:"animeRecordSettings"`
	}
	require.Empty(t, viewer.query(t, anime, nil, &animeSettings))
	assert.Equal(t, 20, animeSettings.AnimeRecordSettings.ThresholdPercent, "the anime percent stays as it was")
	require.Empty(t, other.query(t, read, nil, &got))
	assert.Equal(t, 20, got.DramaRecordSettings.ThresholdPercent, "the setting is kept per user")
}
