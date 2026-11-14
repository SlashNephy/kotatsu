package graph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	issuedStreamURLsQuery = `{ issuedStreamUrls { id userDisplayName targetName live createdAt expiresAt lastUsedAt } }`
	revokeStreamURL       = `mutation($id: ID!) { revokeStreamUrl(id: $id) }`
)

type issuedStreamURL struct {
	ID              string  `json:"id"`
	UserDisplayName string  `json:"userDisplayName"`
	TargetName      string  `json:"targetName"`
	Live            bool    `json:"live"`
	LastUsedAt      *string `json:"lastUsedAt"`
}

// TestManageIssuedStreamURLs は、管理者が、発行された外部プレイヤー用の URL を一覧し、1 本ずつ取り消せ、
// 管理者でなければ一覧も取り消しもできないことを確かめる (#1757)。
func TestManageIssuedStreamURLs(t *testing.T) {
	t.Parallel()

	signer, err := stream.NewSigner("kotatsu-graph-test-application-key-0123456789")
	require.NoError(t, err)
	f := newFixture(t, &graph.Options{StreamSigner: signer})
	f.syncEPG(t)
	admin := f.as(t, "stream-admin", domain.RoleAdmin)
	require.Empty(t, admin.query(t, enableExternalPlayerURLs, nil, nil))
	viewer := f.as(t, "yuzu", domain.RoleViewer)
	require.Empty(t, viewer.query(t, createStreamURL, map[string]any{varTarget: graph.GlobalID("Service", "3273601024")}, nil))

	list := func(t *testing.T, as *fixture) ([]issuedStreamURL, []string) {
		t.Helper()

		var got struct {
			IssuedStreamUrls []issuedStreamURL `json:"issuedStreamUrls"`
		}
		errs := as.query(t, issuedStreamURLsQuery, nil, &got)

		return got.IssuedStreamUrls, errs
	}

	urls, errs := list(t, admin)
	require.Empty(t, errs)
	require.Len(t, urls, 1)
	assert.Equal(t, "yuzu", urls[0].UserDisplayName)
	assert.Equal(t, "こたつテスト1", urls[0].TargetName)
	assert.True(t, urls[0].Live)
	assert.Nil(t, urls[0].LastUsedAt, "the URL is not opened yet")

	tests := []struct {
		name string
		as   *fixture
		want []string
	}{
		{name: "視聴者は取り消せない", as: viewer, want: []string{codeForbidden}},
		{name: "管理者が取り消す", as: admin, want: nil},
		{name: "取り消した URL", as: admin, want: []string{codeNotFound}},
	}
	// 順に取り消すので、行ごとに並行させない。
	for _, tt := range tests {
		got := tt.as.query(t, revokeStreamURL, map[string]any{"id": urls[0].ID}, nil)
		if tt.want == nil {
			assert.Empty(t, got, tt.name)

			continue
		}
		assert.Equal(t, tt.want, got, tt.name)
	}

	remaining, errs := list(t, admin)
	require.Empty(t, errs)
	assert.Empty(t, remaining, "the revoked URL is not listed")
	_, errs = list(t, viewer)
	assert.Equal(t, []string{codeForbidden}, errs, "only administrators can list the URLs")
}
