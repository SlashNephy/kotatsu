package graph_test

import (
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/googletest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/google"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	googleIntegrationQuery  = `{ googleIntegration { clientId configured redirectUri drive { accountName } } }`
	updateGoogleApp         = `mutation($input: GoogleAppInput!) { updateGoogleApp(input: $input) { clientId configured redirectUri } }`
	beginGoogleDrive        = `mutation { beginGoogleDriveConnection }`
	disconnectGoogleDrive   = `mutation { disconnectGoogleDrive { configured } }`
	addGoogleDriveFolder    = `mutation($id: String!, $name: String!) { addGoogleDriveFolder(id: $id, libraryName: $name) { folders { id } } }`
	renameGoogleFolder      = `mutation($id: String!, $name: String!) { renameGoogleDriveFolder(id: $id, libraryName: $name) { folders { id } } }`
	removeGoogleDriveFolder = `mutation($id: String!) { removeGoogleDriveFolder(id: $id) { folders { id } } }`
	googleFolderChoices     = `{ googleDriveFolderChoices { id name } }`
	// driveFolderID は、テストで選ぶ Drive のフォルダの ID。
	driveFolderID = "videos"
	// googleClientID は、モックの Google に登録したクライアント ID。
	googleClientID = "google-client"
	// fieldClientID は、クライアント ID を渡す入力の項目の名前。
	fieldClientID     = "clientId"
	fieldClientSecret = "clientSecret"
)

// TestGoogleIntegration は、管理者だけが Google の OAuth クライアントを登録して Google Drive の接続を始められ、
// シークレットを返さず、登録と接続を始めるときは直近の再認証を求めることを確認する (#1902、設計書 9.5)。
func TestGoogleIntegration(t *testing.T) {
	t.Parallel()
	fake := googletest.New(googleClientID, "top-secret-value")
	googleServer := httptest.NewServer(fake.Handler())
	t.Cleanup(googleServer.Close)
	clk := clocktest.NewFake(time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC))
	applicationKey := "kotatsu-graph-test-application-key-0123456789"
	service, err := google.NewService(&google.Options{
		Clock: clk, Recorder: audit.NewRecorder(clk), AppSecret: applicationKey, HTTPClient: googleServer.Client(),
		Logger: slog.New(slog.DiscardHandler), URL: googleServer.URL,
	})
	require.NoError(t, err)
	f := newFixture(t, &graph.Options{Google: service})
	input := map[string]any{varInput: map[string]any{fieldClientID: googleClientID, fieldClientSecret: "top-secret-value"}}
	folder := map[string]any{"id": driveFolderID, "name": driveFolderID}
	folderID := map[string]any{"id": driveFolderID}

	member := f.as(t, "member", domain.RoleMember)
	viewer := f.as(t, "viewer", domain.RoleViewer)
	for _, other := range []*fixture{member, viewer} {
		assert.Equal(t, []string{codeForbidden}, other.query(t, googleIntegrationQuery, nil, nil))
		assert.Equal(t, []string{codeForbidden}, other.query(t, updateGoogleApp, input, nil))
		assert.Equal(t, []string{codeForbidden}, other.query(t, beginGoogleDrive, nil, nil))
		assert.Equal(t, []string{codeForbidden}, other.query(t, disconnectGoogleDrive, nil, nil))
		assert.Equal(t, []string{codeForbidden}, other.query(t, addGoogleDriveFolder, folder, nil))
		assert.Equal(t, []string{codeForbidden}, other.query(t, removeGoogleDriveFolder, folderID, nil))
		assert.Equal(t, []string{codeForbidden}, other.query(t, renameGoogleFolder, folder, nil))
		assert.Equal(t, []string{codeForbidden}, other.query(t, googleFolderChoices, nil, nil))
	}

	admin := f.as(t, adminName, domain.RoleAdmin)
	type integration struct {
		ClientID    *string `json:"clientId"`
		Configured  bool    `json:"configured"`
		RedirectURI string  `json:"redirectUri"`
	}
	var got struct {
		GoogleIntegration integration `json:"googleIntegration"`
	}
	require.Empty(t, admin.query(t, googleIntegrationQuery, nil, &got))
	// リダイレクト URI は、Google の連携で共有するパスにする。
	assert.Equal(t, integration{ClientID: nil, Configured: false, RedirectURI: f.server.URL + "/internal/auth/google/callback"}, got.GoogleIntegration)
	assert.Equal(t, []string{codeConflict}, admin.query(t, beginGoogleDrive, nil, nil), "cannot connect before the client is registered")

	var updated struct {
		UpdateGoogleApp integration `json:"updateGoogleApp"`
	}
	require.Empty(t, admin.query(t, updateGoogleApp, input, &updated))
	assert.Equal(t, new(googleClientID), updated.UpdateGoogleApp.ClientID)
	assert.True(t, updated.UpdateGoogleApp.Configured)
	var summary string
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT summary::text FROM audit_events WHERE action = 'google_app.update'`).Scan(&summary))
	assert.NotContains(t, summary, "top-secret-value", "the audit log has no secret")
	var stored []byte
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT client_secret_encrypted FROM google_apps`).Scan(&stored))
	assert.NotContains(t, string(stored), "top-secret-value", "the secret is encrypted")

	var begun struct {
		BeginGoogleDriveConnection string `json:"beginGoogleDriveConnection"`
	}
	require.Empty(t, admin.query(t, beginGoogleDrive, nil, &begun))
	assert.True(t, strings.HasPrefix(begun.BeginGoogleDriveConnection, googleServer.URL+"/o/oauth2/v2/auth?"), begun.BeginGoogleDriveConnection)
	assert.Contains(t, begun.BeginGoogleDriveConnection, "code_challenge_method=S256")
	assert.Equal(t, []string{codeNotFound}, admin.query(t, disconnectGoogleDrive, nil, nil), "nothing to disconnect")
	// 接続する前は、見せるフォルダを選べない。
	assert.Equal(t, []string{codeNotFound}, admin.query(t, googleFolderChoices, nil, nil))
	assert.Equal(t, []string{codeNotFound}, admin.query(t, addGoogleDriveFolder, folder, nil))
	assert.Equal(t, []string{codeNotFound}, admin.query(t, removeGoogleDriveFolder, folderID, nil), "nothing to remove")
	assert.Equal(t, []string{codeNotFound}, admin.query(t, renameGoogleFolder, folder, nil), "nothing to rename")

	f.clock.Advance(auth.RecentAuthenticationWindow * 2)
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, updateGoogleApp, input, nil))
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, beginGoogleDrive, nil, nil))
	// 見せるフォルダを足すのは、Drive のファイルを家族に見せ始める操作なので、再認証を求める。外すのは求めない。
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, addGoogleDriveFolder, folder, nil))
	assert.Equal(t, []string{codeNotFound}, admin.query(t, removeGoogleDriveFolder, folderID, nil))
}
