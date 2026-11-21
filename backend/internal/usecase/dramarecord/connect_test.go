package dramarecord_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/dramarecord"
)

// TestConnect は、Simkl から戻ってきた認可を、始めたセッションのまま、Simkl の認可サーバーから戻ってきたときだけ受け付け、
// トークンを暗号化して保存することを確かめる (#1649)。
func TestConnect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// tamper は、戻ってきたリクエストを書き換える。
		tamper func(f *fixture, query url.Values)
		// as は、戻ってきたリクエストのユーザー。nil なら alice。
		as      func(f *fixture) *auth.Principal
		scope   string
		wantErr error
	}{
		{name: "connects", tamper: func(*fixture, url.Values) {}, as: nil, scope: "", wantErr: nil},
		{
			name: "another user cannot complete the authorization", tamper: func(*fixture, url.Values) {},
			as: func(f *fixture) *auth.Principal { return f.bob }, scope: "", wantErr: dramarecord.ErrAuthorizationMismatch,
		},
		{
			name: "a response from another issuer is rejected", tamper: func(_ *fixture, q url.Values) { q.Set("iss", "https://evil.example") },
			as: nil, scope: "", wantErr: dramarecord.ErrAuthorizationMismatch,
		},
		{
			name: "a response without the issuer is rejected", tamper: func(_ *fixture, q url.Values) { q.Del("iss") },
			as: nil, scope: "", wantErr: dramarecord.ErrAuthorizationMismatch,
		},
		{
			name: "an unknown state is rejected", tamper: func(_ *fixture, q url.Values) { q.Set("state", "unknown") },
			as: nil, scope: "", wantErr: dramarecord.ErrAuthorizationExpired,
		},
		{
			name: "a denial is reported", tamper: func(_ *fixture, q url.Values) { q.Del("code"); q.Set("error", "access_denied") },
			as: nil, scope: "", wantErr: dramarecord.ErrAccessDenied,
		},
		{
			name: "a read-only token is rejected", tamper: func(*fixture, url.Values) {},
			as: nil, scope: "media:read", wantErr: dramarecord.ErrPermanent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.simkl.Lock()
			f.simkl.Scope = tt.scope
			f.simkl.Unlock()
			query := f.authorize(t, f.alice)
			tt.tamper(f, query)
			principal := f.alice
			if tt.as != nil {
				principal = tt.as(f)
			}
			err := f.service.Complete(t.Context(), f.pool, principal, dramarecord.DestinationSimkl, callbackOf(query), f.client)
			connections, listErr := f.service.Connections(t.Context(), f.pool, f.alice.UserID)
			require.NoError(t, listErr)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, connections)
				assert.Empty(t, f.inserter.watchlists, "no import is enqueued")

				return
			}
			require.NoError(t, err)
			require.Len(t, connections, 1)
			assert.Equal(t, "simkl-user", connections[0].AccountName)
			assert.Equal(t, []dramarecord.WatchlistArgs{{UserID: f.alice.UserID, Destination: dramarecord.DestinationSimkl}}, f.inserter.watchlists,
				"connecting imports the watchlist right away")
			// トークンは暗号化して持つ。
			var plain int
			require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM drama_record_connections
				WHERE position('simkl_'::bytea in access_token_encrypted) > 0
				   OR position('simkl_'::bytea in refresh_token_encrypted) > 0`).Scan(&plain))
			assert.Zero(t, plain, "tokens are stored encrypted")
			// 同じ state は 2 度使えない。
			err = f.service.Complete(t.Context(), f.pool, f.alice, dramarecord.DestinationSimkl, callbackOf(query), f.client)
			require.ErrorIs(t, err, dramarecord.ErrAuthorizationExpired)
		})
	}
}

// TestConnectRequiresDramaLinking は、管理者が TMDB との連携を無効にしているか、アプリを登録していなければ、連携を始められないことを確かめる。
func TestConnectRequiresDramaLinking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(t *testing.T, f *fixture)
	}{
		{name: "drama linking is disabled", prepare: func(t *testing.T, f *fixture) {
			t.Helper()
			f.exec(t, `UPDATE system_settings SET drama_linking_enabled = false`)
		}},
		{name: "no app is registered", prepare: func(t *testing.T, f *fixture) {
			t.Helper()
			require.NoError(t, f.service.UpdateApp(t.Context(), f.pool, audit.User(f.bob.UserID), dramarecord.DestinationSimkl,
				&dramarecord.AppChanges{ClientID: "", ClientSecret: nil}))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			tt.prepare(t, f)
			_, err := f.service.Begin(t.Context(), f.pool, f.alice, dramarecord.DestinationSimkl, origin)
			require.ErrorIs(t, err, dramarecord.ErrUnavailable)
		})
	}
}

// TestDisconnect は、連携を外すと、Kotatsu からトークンを消し、Simkl で更新のトークンを取り消すことを確かめる。
// 見ている作品に写した作品は、Kotatsu の作品の一覧として残す。
func TestDisconnect(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.link(t, 12, 0, series, 3)
	f.simkl.Lock()
	f.simkl.Items = []*simklItem{{SimklID: 1, TMDBID: series, Status: dramarecord.StatusWatching, UpdatedAt: at(9, 0)}}
	f.simkl.Unlock()
	f.connect(t, f.alice)
	require.NoError(t, f.service.ImportWatchlist(t.Context(), f.pool, f.alice.UserID, dramarecord.DestinationSimkl))
	require.Equal(t, []string{seriesWatching}, f.entries(t, f.alice.UserID))

	require.NoError(t, f.service.Disconnect(t.Context(), f.pool, audit.User(f.alice.UserID), f.alice.UserID, dramarecord.DestinationSimkl))
	connections, err := f.service.Connections(t.Context(), f.pool, f.alice.UserID)
	require.NoError(t, err)
	assert.Empty(t, connections)
	f.simkl.Lock()
	revoked := append([]string{}, f.simkl.Revoked...)
	f.simkl.Unlock()
	require.Len(t, revoked, 1)
	assert.Contains(t, revoked[0], "simkl_rt_", "the refresh token is revoked, which also revokes the access token")
	assert.Equal(t, []string{seriesWatching}, f.entries(t, f.alice.UserID), "imported works stay in Kotatsu")
	var imports int
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM drama_watchlist_imports`).Scan(&imports))
	assert.Zero(t, imports, "the imported Simkl states are deleted")

	err = f.service.Disconnect(t.Context(), f.pool, audit.User(f.alice.UserID), f.alice.UserID, dramarecord.DestinationSimkl)
	require.ErrorIs(t, err, dramarecord.ErrNotConnected)
}

// TestLogSecrets は、ログの報告から伏せる値として、アプリのシークレットと、連携のアクセストークンと更新のトークンを返すことを確かめる (#1521)。
func TestLogSecrets(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.connect(t, f.alice)
	secrets, err := f.service.LogSecrets(t.Context(), f.pool)
	require.NoError(t, err)
	require.Len(t, secrets, 3)
	assert.Equal(t, clientSecret, secrets[0])
	assert.True(t, strings.HasPrefix(secrets[1], "simkl_at_"), "the access token")
	assert.True(t, strings.HasPrefix(secrets[2], "simkl_rt_"), "the refresh token")
}
