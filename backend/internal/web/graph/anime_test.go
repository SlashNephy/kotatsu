package graph_test

import (
	"testing"
	"time"

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

// TestAnimeEpisodes は、番組と録画に結びついた話を、TID に対応する作品のページへのリンクとともに返すことを確かめる (設計書 3.8)。
func TestAnimeEpisodes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	const serviceID = 3273601024
	var startAt time.Time
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT start_at FROM programs WHERE service_id = $1 AND event_id = 1`, serviceID).Scan(&startAt))
	const recordingID = "0192f000-0000-7000-8000-000000000001"
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{sql: `INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at) VALUES ($1, $2, 1, now())`, args: []any{serviceID, startAt}},
		{sql: `INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end, subtitle, is_first, is_last)
			VALUES ($1, $2, 0, 'syobocal', 6776, 1, 1, 'はじまり', true, false), ($1, $2, 1, 'title', NULL, 2, 2, NULL, false, false)`, args: []any{serviceID, startAt}},
		{sql: `INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm', 6776, 0, 10079, 154587, NULL)`, args: []any{}},
		{sql: `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
			VALUES ($3, $1, 'テスト', '', $2, $2, 'completed', $2, now(), now())`, args: []any{serviceID, startAt, recordingID}},
		{sql: `INSERT INTO recording_anime_matches (recording_id, matched_at) VALUES ($1, now())`, args: []any{recordingID}},
		{sql: `INSERT INTO recording_anime_episodes (recording_id, position, source, syobocal_tid, count_start, count_end, is_first, is_last)
			VALUES ($1, 0, 'syobocal', 6776, 3, 4, false, true)`, args: []any{recordingID}},
	} {
		_, err := f.pool.Exec(t.Context(), statement.sql, statement.args...)
		require.NoError(t, err)
	}

	type work struct {
		AnnictURL  *string `json:"annictUrl"`
		AnilistURL *string `json:"anilistUrl"`
		MalURL     *string `json:"malUrl"`
	}
	type episode struct {
		Source      string   `json:"source"`
		SyobocalURL *string  `json:"syobocalUrl"`
		CountStart  *float64 `json:"countStart"`
		CountEnd    *float64 `json:"countEnd"`
		Subtitle    *string  `json:"subtitle"`
		First       bool     `json:"first"`
		Last        bool     `json:"last"`
		Works       []work   `json:"works"`
	}
	const fields = `animeEpisodes { source syobocalUrl countStart countEnd subtitle first last works { annictUrl anilistUrl malUrl } }`
	var got struct {
		Program struct {
			AnimeEpisodes []episode `json:"animeEpisodes"`
		} `json:"program"`
		Recording struct {
			AnimeEpisodes []episode `json:"animeEpisodes"`
		} `json:"recording"`
	}
	require.Empty(t, f.query(t, `query($program: ID!, $recording: ID!) {
		program: node(id: $program) { ... on Program { `+fields+` } }
		recording: node(id: $recording) { ... on Recording { `+fields+` } } }`, map[string]any{
		"program":   graph.GlobalID("Program", "3273601024.1"),
		"recording": graph.GlobalID("Recording", recordingID),
	}, &got))

	tv := work{AnnictURL: new("https://annict.com/works/10079"), AnilistURL: new("https://anilist.co/anime/154587"), MalURL: nil}
	assert.Equal(t, []episode{
		{
			Source: "SYOBOCAL", SyobocalURL: new("https://cal.syoboi.jp/tid/6776"), CountStart: new(1.0), CountEnd: new(1.0),
			Subtitle: new("はじまり"), First: true, Last: false, Works: []work{tv},
		},
		{Source: "TITLE", SyobocalURL: nil, CountStart: new(2.0), CountEnd: new(2.0), Subtitle: nil, First: false, Last: false, Works: []work{}},
	}, got.Program.AnimeEpisodes)
	assert.Equal(t, []episode{
		{
			Source: "SYOBOCAL", SyobocalURL: new("https://cal.syoboi.jp/tid/6776"), CountStart: new(3.0), CountEnd: new(4.0),
			Subtitle: nil, First: false, Last: true, Works: []work{tv},
		},
	}, got.Recording.AnimeEpisodes)
}
