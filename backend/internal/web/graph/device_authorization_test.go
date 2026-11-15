package graph_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

const (
	deviceAuthorizationQuery = `query($userCode: String!) { deviceAuthorization(userCode: $userCode) { deviceName userCode approved } }`
	approveDeviceMutation    = `mutation($userCode: String!) { approveDeviceAuthorization(userCode: $userCode) { deviceName userCode approved } }`
	// testUserCode は、端末の画面に出ている user_code。
	testUserCode = "ABCD-EFGH"
)

type deviceAuthorization struct {
	DeviceName string `json:"deviceName"`
	UserCode   string `json:"userCode"`
	Approved   bool   `json:"approved"`
}

// addDeviceCode は、端末が求めた、まだ承認していないコードを足す。再認証の期限を過ぎても使えるよう、期限は 1 時間後にする。
func (f *fixture) addDeviceCode(t *testing.T) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO oauth_device_codes (
			device_code_hash, user_code, client_id, device_name, scopes, resource, source_network, created_at, expires_at
		) VALUES ('device', 'ABCDEFGH', 'com.example.tv', 'リビングのテレビ', '{epgstation:read}', 'http://127.0.0.1/api/epgstation',
			'192.0.2.10/32', $1, $2)`, f.clock.Now(), f.clock.Now().Add(time.Hour))
	require.NoError(t, err)
}

// deviceApprover は、コードを承認したユーザーを返す。承認していなければ nil。
func (f *fixture) deviceApprover(t *testing.T) any {
	t.Helper()

	var approver any
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT approved_by FROM oauth_device_codes").Scan(&approver))

	return approver
}

// TestDeviceAuthorization は、承認の画面の GraphQL を確かめる (#1527)。引くだけでは承認せず、GET でも承認しない。
// 承認には直近の認証が要り、何度押しても同じで、ほかのユーザーが承認したものは承認できない。
func TestDeviceAuthorization(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	variables := map[string]any{"userCode": "abcd efgh"}
	// EPGStation 互換の API が無効なら、デバイス認可はない。
	assert.Equal(t, []string{codeNotFound}, f.query(t, deviceAuthorizationQuery, variables, nil))
	f.enableEPGStationAPI(t)

	var got struct {
		DeviceAuthorization *deviceAuthorization `json:"deviceAuthorization"`
	}
	require.Empty(t, f.query(t, deviceAuthorizationQuery, variables, &got))
	assert.Nil(t, got.DeviceAuthorization, "no code is issued yet")
	assert.Equal(t, []string{codeBadUserInput}, f.query(t, deviceAuthorizationQuery, map[string]any{"userCode": "0000-0000"}, nil))

	f.addDeviceCode(t)
	// 引くだけでは承認しない。
	require.Empty(t, f.query(t, deviceAuthorizationQuery, variables, &got))
	assert.Equal(t, &deviceAuthorization{DeviceName: "リビングのテレビ", UserCode: testUserCode, Approved: false}, got.DeviceAuthorization)
	assert.Nil(t, f.deviceApprover(t))

	// GET では承認しない。ブラウザの先読みやリンクの下見で、黙って承認されないためである。
	get, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		f.server.URL+"/graphql?query="+url.QueryEscape(`mutation { approveDeviceAuthorization(userCode: "ABCD-EFGH") { approved } }`), http.NoBody)
	require.NoError(t, err)
	get.Header.Set("Cookie", auth.CookieName+"="+f.token)
	resp, err := httpclienttest.New(t).Do(get)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	assert.Nil(t, f.deviceApprover(t))

	// 承認には、直近の認証が要る。
	stale := f.as(t, "stale", domain.RoleMember)
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Equal(t, []string{codeReauthRequired}, stale.query(t, approveDeviceMutation, variables, nil))
	assert.Nil(t, f.deviceApprover(t))

	// 何度押しても同じ。
	member := f.as(t, "member", domain.RoleMember)
	for range 2 {
		var approved struct {
			ApproveDeviceAuthorization deviceAuthorization `json:"approveDeviceAuthorization"`
		}
		require.Empty(t, member.query(t, approveDeviceMutation, variables, &approved))
		assert.Equal(t, deviceAuthorization{DeviceName: "リビングのテレビ", UserCode: testUserCode, Approved: true}, approved.ApproveDeviceAuthorization)
	}
	require.Empty(t, member.query(t, deviceAuthorizationQuery, variables, &got))
	assert.True(t, got.DeviceAuthorization.Approved)
	// ほかのユーザーが承認したものは、承認できず、承認済みとも出さない。
	other := f.as(t, otherUser, domain.RoleViewer)
	assert.Equal(t, []string{codeConflict}, other.query(t, approveDeviceMutation, variables, nil))
	require.Empty(t, other.query(t, deviceAuthorizationQuery, variables, &got))
	assert.False(t, got.DeviceAuthorization.Approved)
}
