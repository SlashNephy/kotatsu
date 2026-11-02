package graph_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	animeRecordApps      = `{ animeRecordApps { destination clientId configured redirectUri } }`
	updateAnimeRecordApp = `mutation($destination: AnimeRecordDestination!, $input: AnimeRecordAppInput!) {
		updateAnimeRecordApp(destination: $destination, input: $input) { destination clientId configured redirectUri } }`
	animeRecordSettings = `{ animeRecordSettings { enabled thresholdPercent watchingOnFirst destinations { destination available connection { accountName } } } }`
	beginAnimeRecord    = `mutation($destination: AnimeRecordDestination!) { beginAnimeRecordConnection(destination: $destination) }`
	enableAnimeLinking  = `mutation { updateSystemSettings(input: {animeLinkingEnabled: true}) { animeLinkingEnabled } }`
	destinationAnnict   = "ANNICT"
	varDestination      = "destination"
)

type animeRecordApp struct {
	Destination string  `json:"destination"`
	ClientID    *string `json:"clientId"`
	Configured  bool    `json:"configured"`
	RedirectURI string  `json:"redirectUri"`
}

// TestAnimeRecordApps は、管理者だけが記録先のアプリを登録でき、クライアントシークレットを返さず、
// 変えるときは直近の再認証を求めることを確かめる (#927、設計書 9.5)。
func TestAnimeRecordApps(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	input := map[string]any{varDestination: destinationAnnict, varInput: map[string]any{"clientId": "client", "clientSecret": "top-secret-value"}}

	assert.Equal(t, []string{codeForbidden}, f.query(t, animeRecordApps, nil, nil), "members cannot see the apps")
	assert.Equal(t, []string{codeForbidden}, f.query(t, updateAnimeRecordApp, input, nil), "members cannot change the apps")

	admin := f.as(t, adminName, auth.RoleAdmin)
	var updated struct {
		UpdateAnimeRecordApp animeRecordApp `json:"updateAnimeRecordApp"`
	}
	require.Empty(t, admin.query(t, updateAnimeRecordApp, input, &updated))
	assert.Equal(t, animeRecordApp{
		Destination: destinationAnnict, ClientID: new("client"), Configured: true,
		RedirectURI: f.server.URL + "/auth/anime-records/annict/callback",
	}, updated.UpdateAnimeRecordApp)
	var listed struct {
		AnimeRecordApps []animeRecordApp `json:"animeRecordApps"`
	}
	require.Empty(t, admin.query(t, animeRecordApps, nil, &listed))
	require.Len(t, listed.AnimeRecordApps, 2)
	assert.False(t, listed.AnimeRecordApps[1].Configured, "AniList is not registered yet")

	var summary string
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT summary::text FROM audit_events WHERE action = 'anime_record_app.update'`).Scan(&summary))
	assert.NotContains(t, summary, "top-secret-value", "the audit log has no secret")
	var stored []byte
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT client_secret_encrypted FROM anime_record_apps`).Scan(&stored))
	assert.NotContains(t, string(stored), "top-secret-value", "the secret is encrypted")

	f.clock.Advance(auth.RecentAuthenticationWindow * 2)
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, updateAnimeRecordApp, input, nil))
}

// TestAnimeRecordSettings は、視聴者も自分の記録の設定を変え、連携を始められ、管理者が連携を無効にしている間は始められないことを確かめる (#927)。
func TestAnimeRecordSettings(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	viewer := f.as(t, "viewer", auth.RoleViewer)
	admin := f.as(t, adminName, auth.RoleAdmin)
	annict := map[string]any{varDestination: destinationAnnict}

	type settings struct {
		AnimeRecordSettings struct {
			Enabled          bool `json:"enabled"`
			ThresholdPercent int  `json:"thresholdPercent"`
			WatchingOnFirst  bool `json:"watchingOnFirst"`
			Destinations     []struct {
				Destination string `json:"destination"`
				Available   bool   `json:"available"`
			} `json:"destinations"`
		} `json:"animeRecordSettings"`
	}
	var got settings
	require.Empty(t, viewer.query(t, animeRecordSettings, nil, &got))
	assert.False(t, got.AnimeRecordSettings.Enabled, "the linking is disabled by default")
	assert.Equal(t, 20, got.AnimeRecordSettings.ThresholdPercent)
	assert.Equal(t, []string{codeConflict}, viewer.query(t, beginAnimeRecord, annict, nil), "cannot connect while the linking is disabled")

	require.Empty(t, admin.query(t, enableAnimeLinking, nil, nil))
	assert.Equal(t, []string{codeConflict}, viewer.query(t, beginAnimeRecord, annict, nil), "cannot connect before the app is registered")
	require.Empty(t, admin.query(t, updateAnimeRecordApp, map[string]any{
		varDestination: destinationAnnict, varInput: map[string]any{"clientId": "client", "clientSecret": "secret"},
	}, nil))
	require.Empty(t, viewer.query(t, animeRecordSettings, nil, &got))
	assert.True(t, got.AnimeRecordSettings.Enabled)
	assert.True(t, got.AnimeRecordSettings.Destinations[0].Available)
	assert.False(t, got.AnimeRecordSettings.Destinations[1].Available, "AniList is not registered")

	var begun struct {
		BeginAnimeRecordConnection string `json:"beginAnimeRecordConnection"`
	}
	require.Empty(t, viewer.query(t, beginAnimeRecord, annict, &begun))
	assert.True(t, strings.HasPrefix(begun.BeginAnimeRecordConnection, "http://127.0.0.1:9/oauth/authorize?"), begun.BeginAnimeRecordConnection)

	const update = `mutation($input: AnimeRecordSettingsInput!) { updateAnimeRecordSettings(input: $input) { thresholdPercent watchingOnFirst } }`
	input := map[string]any{"thresholdPercent": 30, "watchingOnFirst": true, "watchingOnEpisode": false, "watchedOnLast": false}
	require.Empty(t, viewer.query(t, update, map[string]any{varInput: input}, nil))
	require.Empty(t, viewer.query(t, animeRecordSettings, nil, &got))
	assert.Equal(t, 30, got.AnimeRecordSettings.ThresholdPercent)
	assert.True(t, got.AnimeRecordSettings.WatchingOnFirst)
	var others settings
	require.Empty(t, admin.query(t, animeRecordSettings, nil, &others))
	assert.Equal(t, 20, others.AnimeRecordSettings.ThresholdPercent, "the settings are per user")
	input["thresholdPercent"] = 0
	assert.Equal(t, []string{codeBadUserInput}, viewer.query(t, update, map[string]any{varInput: input}, nil))

	const report = `mutation($service: ID, $recording: ID, $intervals: [PlaybackIntervalInput!]!) {
		reportAnimePlayback(serviceId: $service, recordingId: $recording, intervals: $intervals) }`
	intervals := []map[string]any{{"from": "2026-10-10T12:00:00Z", "to": "2026-10-10T12:05:00Z"}}
	var reported struct {
		ReportAnimePlayback bool `json:"reportAnimePlayback"`
	}
	require.Empty(t, viewer.query(t, report, map[string]any{"service": graph.GlobalID("Service", "3273601024"), "intervals": intervals}, &reported))
	assert.False(t, reported.ReportAnimePlayback, "nothing is recorded before connecting")
	assert.Equal(t, []string{codeBadUserInput}, viewer.query(t, report, map[string]any{"intervals": intervals}, nil), "a target is required")

	// 手で記録するときは、番組、録画、作品のどれか 1 つを指し、連携していなければ NOT_FOUND を返す (#1232)。
	const (
		record     = `mutation($input: RecordAnimeEpisodesInput!) { recordAnimeEpisodes(input: $input) }`
		varTid     = "tid"
		varEpisode = "episode"
	)
	assert.Equal(t, []string{codeBadUserInput}, viewer.query(t, record, map[string]any{varInput: map[string]any{}}, nil), "a target is required")
	assert.Equal(t, []string{codeBadUserInput}, viewer.query(t, record, map[string]any{varInput: map[string]any{
		varTid: 6776, varEpisode: 1, "recording": graph.GlobalID("Recording", "0192f000-0000-7000-8000-000000000001"),
	}}, nil), "only one target is allowed")
	assert.Equal(t, []string{codeBadUserInput}, viewer.query(t, record, map[string]any{varInput: map[string]any{varTid: 6776}}, nil),
		"an episode is required for a work")
	assert.Equal(t, []string{codeBadUserInput}, viewer.query(t, record, map[string]any{varInput: map[string]any{
		varTid: 6776, varEpisode: 1, "title": strings.Repeat("あ", 201),
	}}, nil), "the title is at most 200 characters")
	assert.Equal(t, []string{codeNotFound}, viewer.query(t, record, map[string]any{varInput: map[string]any{varTid: 6776, varEpisode: 1}}, nil),
		"viewers can record, but not before connecting")
}

// TestAnimeImportToken は、管理者だけが作品 ID を引くための Annict のトークンを設定でき、トークンを返さず、
// 変えるときは直近の再認証を求めることを確かめる (#1223、設計書 9.5)。
func TestAnimeImportToken(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	const (
		animeImportToken       = `{ animeImportToken { configured } }`
		updateAnimeImportToken = `mutation($token: String!) { updateAnimeImportToken(annictToken: $token) { configured } }`
		varToken               = "token"
	)
	input := map[string]any{varToken: "top-secret-token"}

	assert.Equal(t, []string{codeForbidden}, f.query(t, animeImportToken, nil, nil), "members cannot see the token")
	assert.Equal(t, []string{codeForbidden}, f.query(t, updateAnimeImportToken, input, nil), "members cannot change the token")

	admin := f.as(t, adminName, auth.RoleAdmin)
	type status struct {
		Configured bool `json:"configured"`
	}
	var updated struct {
		UpdateAnimeImportToken status `json:"updateAnimeImportToken"`
	}
	require.Empty(t, admin.query(t, updateAnimeImportToken, input, &updated))
	assert.True(t, updated.UpdateAnimeImportToken.Configured)
	var got struct {
		AnimeImportToken status `json:"animeImportToken"`
	}
	require.Empty(t, admin.query(t, animeImportToken, nil, &got))
	assert.True(t, got.AnimeImportToken.Configured)
	assert.Equal(t, []string{codeBadUserInput}, admin.query(t, updateAnimeImportToken, map[string]any{varToken: strings.Repeat("a", 257)}, nil))

	require.Empty(t, admin.query(t, updateAnimeImportToken, map[string]any{varToken: ""}, &updated))
	assert.False(t, updated.UpdateAnimeImportToken.Configured, "an empty token deletes it")

	f.clock.Advance(auth.RecentAuthenticationWindow * 2)
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, updateAnimeImportToken, input, nil))
}
