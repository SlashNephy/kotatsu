package graph_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	webhookFields   = `id name url format kinds enabled`
	listWebhooks    = `{ webhooks { ` + webhookFields + ` } }`
	createWebhook   = `mutation($input: CreateWebhookInput!) { createWebhook(input: $input) { webhook { ` + webhookFields + ` } secret } }`
	updateWebhook   = `mutation($id: ID!, $input: UpdateWebhookInput!) { updateWebhook(id: $id, input: $input) { ` + webhookFields + ` } }`
	deleteWebhook   = `mutation($id: ID!) { deleteWebhook(id: $id) }`
	discordURL      = "https://discord.example/api/webhooks/1/token"
	formatDiscord   = "DISCORD"
	fieldURL        = "url"
	fieldKinds      = "kinds"
	fieldEnabled    = "enabled"
	fieldFormat     = "format"
	webhookName     = "家族の Discord"
	kindConflict    = "RESERVATION_CONFLICT"
	kindUnreachable = "TUNER_SERVER_UNREACHABLE"
)

type webhookResult struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Format  string   `json:"format"`
	Kinds   []string `json:"kinds"`
	Enabled bool     `json:"enabled"`
}

func webhookInput() map[string]any {
	return map[string]any{
		varName: webhookName, fieldURL: discordURL, fieldFormat: formatDiscord, fieldKinds: []string{kindUnreachable, kindConflict},
	}
}

// TestWebhooks は、管理者だけが Webhook を作り、変え、消せることを確かめる (設計書 6.2、9.7)。
func TestWebhooks(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	create := map[string]any{varInput: webhookInput()}
	for _, query := range []string{listWebhooks} {
		assert.Equal(t, []string{codeForbidden}, f.query(t, query, nil, nil), "members cannot read webhooks")
	}
	assert.Equal(t, []string{codeForbidden}, f.query(t, createWebhook, create, nil), "members cannot create webhooks")

	admin := f.as(t, adminName, auth.RoleAdmin)
	var created struct {
		CreateWebhook struct {
			Webhook webhookResult `json:"webhook"`
			Secret  string        `json:"secret"`
		} `json:"createWebhook"`
	}
	require.Empty(t, admin.query(t, createWebhook, create, &created))
	got := created.CreateWebhook.Webhook
	want := webhookResult{
		ID: got.ID, Name: webhookName, URL: discordURL, Format: formatDiscord,
		// 種類は、通知設定の画面と同じ順にそろえる。既定では有効にする。
		Kinds: []string{kindConflict, kindUnreachable}, Enabled: true,
	}
	assert.Equal(t, want, got)
	assert.True(t, strings.HasPrefix(created.CreateWebhook.Secret, "whsec_"))

	var list struct {
		Webhooks []webhookResult `json:"webhooks"`
	}
	require.Empty(t, admin.query(t, listWebhooks, nil, &list))
	assert.Equal(t, []webhookResult{want}, list.Webhooks)

	// 省いた項目は変えない。
	var updated struct {
		UpdateWebhook webhookResult `json:"updateWebhook"`
	}
	update := map[string]any{"id": got.ID, varInput: map[string]any{fieldFormat: "SLACK", fieldEnabled: false}}
	require.Empty(t, admin.query(t, updateWebhook, update, &updated))
	want.Format, want.Enabled = "SLACK", false
	assert.Equal(t, want, updated.UpdateWebhook)
	assert.Equal(t, []string{codeForbidden}, f.query(t, updateWebhook, update, nil), "members cannot update webhooks")

	assert.Equal(t, []string{codeForbidden}, f.query(t, deleteWebhook, map[string]any{"id": got.ID}, nil), "members cannot delete webhooks")
	var deleted struct {
		DeleteWebhook string `json:"deleteWebhook"`
	}
	require.Empty(t, admin.query(t, deleteWebhook, map[string]any{"id": got.ID}, &deleted))
	assert.Equal(t, got.ID, deleted.DeleteWebhook)
	require.Empty(t, admin.query(t, listWebhooks, nil, &list))
	assert.Empty(t, list.Webhooks)

	// 作り、変え、消すのは重要な操作なので、直前の再認証を求める。読むだけなら求めない。
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, createWebhook, create, nil))
	require.Empty(t, admin.query(t, listWebhooks, nil, &list))
}

func TestWebhooksRejectInvalidInput(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	admin := f.as(t, adminName, auth.RoleAdmin)
	withInput := func(modify func(map[string]any)) map[string]any {
		input := webhookInput()
		modify(input)

		return map[string]any{varInput: input}
	}
	missing := graph.GlobalID("Webhook", "0199d000-0000-7000-8000-000000000001")
	tests := []struct {
		name      string
		query     string
		variables map[string]any
		want      string
	}{
		{name: "空の名前", query: createWebhook, variables: withInput(func(in map[string]any) { in[varName] = " " }), want: codeBadUserInput},
		{name: "http でも https でもない URL", query: createWebhook, variables: withInput(func(in map[string]any) { in[fieldURL] = "ftp://example.com" }), want: codeBadUserInput},
		{name: "種類がない", query: createWebhook, variables: withInput(func(in map[string]any) { in[fieldKinds] = []string{} }), want: codeBadUserInput},
		{name: "ない Webhook を変える", query: updateWebhook, variables: map[string]any{"id": missing, varInput: map[string]any{fieldEnabled: false}}, want: codeNotFound},
		{name: "ない Webhook を消す", query: deleteWebhook, variables: map[string]any{"id": missing}, want: codeNotFound},
		{name: "Webhook でない ID", query: deleteWebhook, variables: map[string]any{"id": graph.GlobalID("User", f.userID.String())}, want: codeBadUserInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []string{tt.want}, admin.query(t, tt.query, tt.variables, nil))
		})
	}
}
