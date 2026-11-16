package graph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// DramaTitleOverrideInput の項目。
const (
	fieldProgramName  = "programName"
	fieldTitleKey     = "titleKey"
	fieldReset        = "reset"
	fieldTmdbSeriesID = "tmdbSeriesId"
)

type dramaTitleOverride struct {
	TitleKey     string `json:"titleKey"`
	TmdbSeriesID *int   `json:"tmdbSeriesId"`
}

// TestDramaTitleOverrides は、管理者だけが、ドラマの作品名ごとの TMDb の作品の上書きを読み、変えられることを確かめる (#1651)。
// 上書きは家族の中で完結するので、再認証は求めない (設計書 9.5)。
func TestDramaTitleOverrides(t *testing.T) {
	t.Parallel()

	const (
		listQuery = `{ dramaLinking { tokenConfigured } dramaTitleOverrides { titleKey tmdbSeriesId } }`
		update    = `mutation($input: DramaTitleOverrideInput!) { updateDramaTitleOverride(input: $input) { titleKey tmdbSeriesId } }`
		program   = "もうパパ！　＃７　【心底ムカついています】"
		titleKey  = "もうパパ"
	)
	f := newFixture(t, nil)
	input := map[string]any{varInput: map[string]any{fieldProgramName: program, fieldTmdbSeriesID: 331304}}
	assert.Equal(t, []string{codeForbidden, codeForbidden}, f.query(t, listQuery, nil, nil), "only administrators can read the overrides")
	assert.Equal(t, []string{codeForbidden}, f.query(t, update, input, nil), "only administrators can change the overrides")

	admin := f.as(t, adminName, domain.RoleAdmin)
	f.clock.Advance(auth.RecentAuthenticationWindow * 2)
	var updated struct {
		UpdateDramaTitleOverride []dramaTitleOverride `json:"updateDramaTitleOverride"`
	}
	require.Empty(t, admin.query(t, update, input, &updated), "no reauthentication is needed")
	assert.Equal(t, []dramaTitleOverride{{TitleKey: titleKey, TmdbSeriesID: new(331304)}}, updated.UpdateDramaTitleOverride,
		"the title is read from the program name the same way as the matching")

	require.Empty(t, admin.query(t, update, map[string]any{varInput: map[string]any{fieldTitleKey: titleKey, "exclude": true}}, &updated))
	assert.Equal(t, []dramaTitleOverride{{TitleKey: titleKey, TmdbSeriesID: nil}}, updated.UpdateDramaTitleOverride)

	var listed struct {
		DramaLinking struct {
			TokenConfigured bool `json:"tokenConfigured"`
		} `json:"dramaLinking"`
		DramaTitleOverrides []dramaTitleOverride `json:"dramaTitleOverrides"`
	}
	require.Empty(t, admin.query(t, listQuery, nil, &listed))
	assert.False(t, listed.DramaLinking.TokenConfigured, "no TMDb token has been entered")
	assert.Len(t, listed.DramaTitleOverrides, 1)

	for _, bad := range []map[string]any{
		{fieldProgramName: program},
		{fieldProgramName: program, fieldTmdbSeriesID: 1, "exclude": true},
		{fieldProgramName: program, fieldTmdbSeriesID: 0},
		{fieldProgramName: program, fieldTmdbSeriesID: 1 << 40},
		{fieldProgramName: program, fieldTitleKey: titleKey, fieldReset: true},
		{fieldTitleKey: " ・ ", fieldReset: true},
	} {
		assert.Equal(t, []string{codeBadUserInput}, admin.query(t, update, map[string]any{varInput: bad}, nil), "%v", bad)
	}

	require.Empty(t, admin.query(t, update, map[string]any{varInput: map[string]any{fieldTitleKey: titleKey, fieldReset: true}}, &updated))
	assert.Empty(t, updated.UpdateDramaTitleOverride)
}

// TestDramaLinkingToken は、管理者だけが TMDb のトークンを入れられ、トークンそのものは返さず末尾の数文字だけを返すこと、
// 空にすれば消えること、変えるときは直近の再認証を求めることを確かめる (#1651、設計書 9.5)。
func TestDramaLinkingToken(t *testing.T) {
	t.Parallel()

	const (
		status   = `{ dramaLinking { tokenConfigured tokenHint } }`
		update   = `mutation($token: String!) { updateDramaLinkingToken(token: $token) { tokenConfigured tokenHint } }`
		varToken = "token"
		entered  = "eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJ0ZXN0In0.signature-wxyz"
	)
	type linking struct {
		TokenConfigured bool    `json:"tokenConfigured"`
		TokenHint       *string `json:"tokenHint"`
	}
	f := newFixture(t, nil)
	input := map[string]any{varToken: entered}
	assert.Equal(t, []string{codeForbidden}, f.query(t, status, nil, nil), "members cannot see the token")
	assert.Equal(t, []string{codeForbidden}, f.query(t, update, input, nil), "members cannot change the token")

	admin := f.as(t, adminName, domain.RoleAdmin)
	var got struct {
		DramaLinking linking `json:"dramaLinking"`
	}
	require.Empty(t, admin.query(t, status, nil, &got))
	assert.Equal(t, linking{TokenConfigured: false, TokenHint: nil}, got.DramaLinking)

	var updated struct {
		UpdateDramaLinkingToken linking `json:"updateDramaLinkingToken"`
	}
	require.Empty(t, admin.query(t, update, input, &updated))
	assert.Equal(t, linking{TokenConfigured: true, TokenHint: new("wxyz")}, updated.UpdateDramaLinkingToken)
	require.Empty(t, admin.query(t, status, nil, &got))
	assert.Equal(t, linking{TokenConfigured: true, TokenHint: new("wxyz")}, got.DramaLinking)

	assert.Equal(t, []string{codeBadUserInput}, admin.query(t, update, map[string]any{varToken: "two words"}, nil))

	require.Empty(t, admin.query(t, update, map[string]any{varToken: ""}, &updated))
	assert.Equal(t, linking{TokenConfigured: false, TokenHint: nil}, updated.UpdateDramaLinkingToken, "an empty token deletes it")

	f.clock.Advance(auth.RecentAuthenticationWindow * 2)
	assert.Equal(t, []string{codeReauthRequired}, admin.query(t, update, input, nil))
}
