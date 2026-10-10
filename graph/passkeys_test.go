package graph_test

import (
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/webauthntest"
)

const (
	listKeysQuery     = `{ passkeys { id name rpId lastUsedAt } }`
	renameKeyMutation = `mutation($id: ID!, $name: String!) { renamePasskey(id: $id, name: $name) { id name } }`
	deleteKeyMutation = `mutation($id: ID!) { deletePasskey(id: $id) }`
	rpHost            = "kotatsu.example"
	deviceName        = "ノートパソコン"
	renamedLabel      = "スマートフォン"
)

type passkey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	RPID       string     `json:"rpId"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

// registerPasskey は、ログイン中のユーザーに、ソフトウェアの認証器のパスキーを登録する。
func (f *fixture) registerPasskey(t *testing.T) {
	t.Helper()

	client := &auth.ClientInfo{UserAgent: "Mozilla/5.0", SourceIP: netip.Addr{}, Host: rpHost, Secure: true}
	a, err := webauthntest.New("https://" + rpHost)
	require.NoError(t, err)
	creation, err := f.passkeys.BeginRegistration(t.Context(), f.pool, f.userID, client)
	require.NoError(t, err)
	response, err := a.Create(creation.Options)
	require.NoError(t, err)
	_, err = f.passkeys.FinishRegistration(t.Context(), f.pool, audit.User(f.userID), f.userID, creation.Token, deviceName, response, client)
	require.NoError(t, err)
}

func (f *fixture) listPasskeys(t *testing.T) []passkey {
	t.Helper()

	var got struct {
		Passkeys []passkey `json:"passkeys"`
	}
	require.Empty(t, f.query(t, listKeysQuery, nil, &got))

	return got.Passkeys
}

// reauthenticate は、同じユーザーでログインし直したセッションに切り替える。
func (f *fixture) reauthenticate(t *testing.T) {
	t.Helper()

	token, _, err := f.sessions.Create(t.Context(), f.pool, f.userID, nil)
	require.NoError(t, err)
	f.token = token
}

// TestManagePasskeys は、ログイン中のユーザーが、自分のパスキーを一覧し、名前を変え、消せることを確かめる。
func TestManagePasskeys(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	assert.Empty(t, f.listPasskeys(t))
	f.registerPasskey(t)
	f.registerPasskey(t)
	listed := f.listPasskeys(t)
	require.Len(t, listed, 2)
	assert.Equal(t, passkey{ID: listed[0].ID, Name: deviceName, RPID: rpHost, LastUsedAt: nil}, listed[0])

	var renamed struct {
		RenamePasskey passkey `json:"renamePasskey"`
	}
	require.Empty(t, f.query(t, renameKeyMutation, map[string]any{varID: listed[0].ID, varName: renamedLabel}, &renamed))
	assert.Equal(t, renamedLabel, renamed.RenamePasskey.Name)
	assert.Equal(t, []string{codeBadUserInput}, f.query(t, renameKeyMutation, map[string]any{varID: listed[0].ID, varName: " "}, nil))
	assert.Equal(t, []string{codeBadUserInput}, f.query(t, renameKeyMutation, map[string]any{varID: graph.GlobalID("GuideView", f.userID.String()), varName: renamedLabel}, nil))

	// ほかのユーザーは、名前を変えも、消しもできない。
	other := f.as(t, "other", auth.RoleMember)
	assert.Equal(t, []string{codeNotFound}, other.query(t, renameKeyMutation, map[string]any{varID: listed[0].ID, varName: "x"}, nil))
	assert.Equal(t, []string{codeNotFound}, other.query(t, deleteKeyMutation, map[string]any{varID: listed[0].ID}, nil))
	assert.Empty(t, other.listPasskeys(t))

	// 消すには、直前の再認証が要る。
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Equal(t, []string{codeReauthRequired}, f.query(t, deleteKeyMutation, map[string]any{varID: listed[0].ID}, nil))
	f.reauthenticate(t)

	var deleted struct {
		DeletePasskey string `json:"deletePasskey"`
	}
	require.Empty(t, f.query(t, deleteKeyMutation, map[string]any{varID: listed[0].ID}, &deleted))
	assert.Equal(t, listed[0].ID, deleted.DeletePasskey)
	// パスワードを持たないユーザーの最後のパスキーは、消せない。
	assert.Equal(t, []string{codeBadUserInput}, f.query(t, deleteKeyMutation, map[string]any{varID: listed[1].ID}, nil))
	assert.Len(t, f.listPasskeys(t), 1)
}
