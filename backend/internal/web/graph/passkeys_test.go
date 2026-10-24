package graph_test

import (
	"log/slog"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/testutil/webauthntest"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	listKeysQuery     = `{ passkeys { id name rpId lastUsedAt } }`
	renameKeyMutation = `mutation($id: ID!, $name: String!) { renamePasskey(id: $id, name: $name) { id name } }`
	deleteKeyMutation = `mutation($id: ID!) { deletePasskey(id: $id) }`
	rpHost            = "kotatsu.example"
	deviceName        = "ノートパソコン"
	codeInternal      = "INTERNAL"
	otherUser         = "other"
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

	var deleted struct {
		DeletePasskey string `json:"deletePasskey"`
	}
	require.Empty(t, f.query(t, deleteKeyMutation, map[string]any{varID: listed[0].ID}, &deleted))
	assert.Equal(t, listed[0].ID, deleted.DeletePasskey)
	assert.Equal(t, []passkey{listed[1]}, f.listPasskeys(t))
}

// TestManagePasskeysRejects は、パスキーの名前の変更と削除を、受け付けない場合を確かめる。
func TestManagePasskeysRejects(t *testing.T) {
	t.Parallel()

	unknownID := graph.GlobalID("Passkey", uuid.NewString())
	tests := []struct {
		name string
		// prepare は、ログイン中のユーザーのパスキーの ID を受け取り、操作する fixture と、送るクエリと変数を返す。
		prepare  func(t *testing.T, f *fixture, id string) (*fixture, string, map[string]any)
		wantCode string
	}{
		{
			name: "empty name", wantCode: codeBadUserInput,
			prepare: func(_ *testing.T, f *fixture, id string) (*fixture, string, map[string]any) {
				return f, renameKeyMutation, map[string]any{varID: id, varName: " "}
			},
		},
		{
			name: "too long name", wantCode: codeBadUserInput,
			prepare: func(_ *testing.T, f *fixture, id string) (*fixture, string, map[string]any) {
				return f, renameKeyMutation, map[string]any{varID: id, varName: strings.Repeat("あ", 65)}
			},
		},
		{
			name: "ID of another type", wantCode: codeBadUserInput,
			prepare: func(_ *testing.T, f *fixture, _ string) (*fixture, string, map[string]any) {
				return f, renameKeyMutation, map[string]any{varID: graph.GlobalID("GuideView", f.userID.String()), varName: renamedLabel}
			},
		},
		{
			name: "malformed ID", wantCode: codeBadUserInput,
			prepare: func(_ *testing.T, f *fixture, _ string) (*fixture, string, map[string]any) {
				return f, deleteKeyMutation, map[string]any{varID: "not-an-id"}
			},
		},
		{
			name: "unknown passkey", wantCode: codeNotFound,
			prepare: func(_ *testing.T, f *fixture, _ string) (*fixture, string, map[string]any) {
				return f, deleteKeyMutation, map[string]any{varID: unknownID}
			},
		},
		{
			name: "rename passkey of another user", wantCode: codeNotFound,
			prepare: func(t *testing.T, f *fixture, id string) (*fixture, string, map[string]any) {
				t.Helper()

				return f.as(t, otherUser, auth.RoleMember), renameKeyMutation, map[string]any{varID: id, varName: renamedLabel}
			},
		},
		{
			name: "delete passkey of another user", wantCode: codeNotFound,
			prepare: func(t *testing.T, f *fixture, id string) (*fixture, string, map[string]any) {
				t.Helper()

				return f.as(t, otherUser, auth.RoleMember), deleteKeyMutation, map[string]any{varID: id}
			},
		},
		{
			name: "delete without recent authentication", wantCode: codeReauthRequired,
			prepare: func(_ *testing.T, f *fixture, id string) (*fixture, string, map[string]any) {
				f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)

				return f, deleteKeyMutation, map[string]any{varID: id}
			},
		},
		{
			// fixture のユーザーは、パスワードを持たない。
			name: "delete the last passkey of a user without a password", wantCode: codeBadUserInput,
			prepare: func(_ *testing.T, f *fixture, id string) (*fixture, string, map[string]any) {
				return f, deleteKeyMutation, map[string]any{varID: id}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			f.registerPasskey(t)
			listed := f.listPasskeys(t)
			require.Len(t, listed, 1)

			actor, query, variables := tt.prepare(t, f, listed[0].ID)
			assert.Equal(t, []string{tt.wantCode}, actor.query(t, query, variables, nil))
			assert.Equal(t, listed, f.listPasskeys(t), "the passkey must be left as it was")
		})
	}
}

// withoutPasskeys は、パスキーを扱う設定のない /graphql を、同じ DB とセッションで用意する。
func (f *fixture) withoutPasskeys(t *testing.T) *fixture {
	t.Helper()

	handler := graph.NewHandler(&graph.Options{DB: f.pool, Clock: f.clock, Logger: slog.New(slog.DiscardHandler)})
	server := httptest.NewServer(f.sessions.Middleware(f.pool)(handler))
	t.Cleanup(server.Close)
	g := *f
	g.server = server

	return &g
}

// TestPasskeysUnavailable は、パスキーを扱う設定がなければ、内部のエラーにすることを確かめる。
func TestPasskeysUnavailable(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.registerPasskey(t)
	id := f.listPasskeys(t)[0].ID
	// 設定のない Resolver で、同じ DB を引く。
	g := f.withoutPasskeys(t)

	tests := []struct {
		name      string
		query     string
		variables map[string]any
	}{
		{name: "list", query: listKeysQuery},
		{name: "rename", query: renameKeyMutation, variables: map[string]any{varID: id, varName: renamedLabel}},
		{name: "delete", query: deleteKeyMutation, variables: map[string]any{varID: id}},
	}
	for _, tt := range tests {
		assert.Equal(t, []string{codeInternal}, g.query(t, tt.query, tt.variables, nil), tt.name)
	}
}
