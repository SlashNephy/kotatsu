package oauth_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/oauth"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	grantDeviceCode = "urn:ietf:params:oauth:grant-type:device_code"
	testDeviceID    = "com.example.tv"
	testEPGResource = testOrigin + "/api/epgstation"
	// testSource は、端末の接続元のアドレス。
	testSource = "192.0.2.10:40000"

	errorAuthorizationPending = "authorization_pending"
	errorSlowDown             = "slow_down"
	errorExpiredToken         = "expired_token"
	// normalizedCode は、ABCD-EFGH を正規化した user_code。
	normalizedCode = "ABCDEFGH"
	// perSourceCap は、接続元ごとのコードの上限。
	perSourceCap = 5
)

// deviceFixture は、EPGStation 互換の API を有効にした fixture。member と viewer は、セッションでログインしている。
type deviceFixture struct {
	*fixture
}

func newDeviceFixture(t *testing.T) *deviceFixture {
	t.Helper()

	f := newFixture(t)
	setEPGStationEnabled(t, f.db, true)
	f.member.SessionID = uuid.New()
	f.member.AuthenticatedAt = testNow()
	f.viewer.SessionID = uuid.New()
	f.viewer.AuthenticatedAt = testNow()

	return &deviceFixture{fixture: f}
}

func setEPGStationEnabled(t *testing.T, db *pgxpool.Pool, enabled bool) {
	t.Helper()

	clk := clocktest.NewFake(testNow())
	_, err := settings.NewStore(clk, audit.NewRecorder(clk)).Update(t.Context(), db, audit.System("test"), &settings.Changes{EPGStationAPIEnabled: &enabled})
	require.NoError(t, err)
}

// postFrom は、フォームを remoteAddr から handler に送り、状態コードと JSON の本文を返す。
func postFrom(t *testing.T, handler http.Handler, path, remoteAddr string, form url.Values) (int, map[string]any) {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, testOrigin+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec.Code, decodeBody(t, rec)
}

// requestCode は、remoteAddr の端末としてコードを求め、状態コードと応答を返す。
func (f *deviceFixture) requestCode(t *testing.T, remoteAddr string) (int, map[string]any) {
	t.Helper()

	return postFrom(t, f.server.DeviceAuthorizationHandler(slog.New(slog.DiscardHandler)), oauth.DeviceAuthorizationPath, remoteAddr, url.Values{
		paramClientID: {testDeviceID}, "device_name": {"リビングのテレビ"},
	})
}

// issue は、testSource の端末としてコードを求め、device_code と user_code を返す。
func (f *deviceFixture) issue(t *testing.T) (string, string) {
	t.Helper()

	status, body := f.requestCode(t, testSource)
	require.Equal(t, http.StatusOK, status, body)
	deviceCode, _ := body["device_code"].(string)
	userCode, _ := body["user_code"].(string)

	return deviceCode, userCode
}

// poll は、端末として device_code で問い合わせる。
func (f *deviceFixture) poll(t *testing.T, deviceCode string) (int, map[string]any) {
	t.Helper()

	return postFrom(t, f.server.TokenHandler(slog.New(slog.DiscardHandler)), oauth.ExchangePath, testSource, url.Values{
		paramGrantType: {grantDeviceCode}, "device_code": {deviceCode}, paramClientID: {testDeviceID},
	})
}

// pollError は、問い合わせて、エラーのコードを返す。
func (f *deviceFixture) pollError(t *testing.T, deviceCode string) string {
	t.Helper()

	status, body := f.poll(t, deviceCode)
	require.Equal(t, http.StatusBadRequest, status, body)
	code, _ := body["error"].(string)

	return code
}

// TestDeviceAuthorizationFlow は、コードの発行から、承認、トークンを 1 回だけ返すまでと、トークンの使い道を確かめる。
func TestDeviceAuthorizationFlow(t *testing.T) {
	t.Parallel()

	f := newDeviceFixture(t)
	status, body := f.requestCode(t, testSource)
	require.Equal(t, http.StatusOK, status, body)
	deviceCode, _ := body["device_code"].(string)
	userCode, _ := body["user_code"].(string)
	assert.True(t, strings.HasPrefix(deviceCode, "kotatsu_dc_"))
	assert.Regexp(t, `^[2-9A-HJ-NP-Z]{4}-[2-9A-HJ-NP-Z]{4}$`, userCode)
	assert.Equal(t, testOrigin+"/device", body["verification_uri"])
	assert.Equal(t, testOrigin+"/device?user_code="+userCode, body["verification_uri_complete"])
	assert.InDelta(t, 600, body["expires_in"], 0)
	assert.InDelta(t, 5, body["interval"], 0)

	assert.Equal(t, errorAuthorizationPending, f.pollError(t, deviceCode))

	// 承認の画面に出すために引いても、承認しない。
	f.clock.Advance(5 * time.Second)
	device, err := f.server.LookupDevice(t.Context(), f.member, strings.ToLower(userCode))
	require.NoError(t, err)
	assert.Equal(t, &oauth.DeviceAuthorization{DeviceName: "リビングのテレビ", UserCode: userCode, Approved: false}, device)
	assert.Equal(t, errorAuthorizationPending, f.pollError(t, deviceCode))

	// 承認は何度押しても同じで、トークンを 2 つ作らない。
	for range 2 {
		approved, err := f.server.ApproveDevice(t.Context(), audit.User(f.member.UserID), f.member, userCode)
		require.NoError(t, err)
		assert.True(t, approved.Approved)
	}
	_, err = f.server.ApproveDevice(t.Context(), audit.User(f.viewer.UserID), f.viewer, userCode)
	require.ErrorIs(t, err, oauth.ErrDeviceApprovedByOther)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'oauth_device.approve'"))
	assert.Zero(t, f.count(t, "SELECT count(*) FROM oauth_tokens"))

	// 承認したら、受け取りの時点でトークンを作り、1 回だけ返す。
	f.clock.Advance(5 * time.Second)
	status, body = f.poll(t, deviceCode)
	require.Equal(t, http.StatusOK, status, body)
	access, _ := body["access_token"].(string)
	refresh, _ := body[grantRefreshToken].(string)
	assert.True(t, strings.HasPrefix(access, "kotatsu_at_"))
	assert.True(t, strings.HasPrefix(refresh, "kotatsu_rt_"))
	assert.Equal(t, oauth.ScopeEPGStation, body["scope"])
	f.clock.Advance(5 * time.Second)
	assert.Equal(t, errorInvalidGrant, f.pollError(t, deviceCode))
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM oauth_grants WHERE kind = 'device'"))

	// トークンは、EPGStation 互換の API にだけ使え、MCP サーバーには使えない。
	principal, grant, err := f.server.AuthenticateDevice(t.Context(), access, testEPGResource)
	require.NoError(t, err)
	assert.Equal(t, f.member.UserID, principal.UserID)
	assert.Equal(t, []string{oauth.ScopeEPGStation}, grant.Scopes)
	_, _, err = f.server.AuthenticateDevice(t.Context(), access, "https://other.example/api/epgstation")
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
	_, _, err = f.server.Authenticate(t.Context(), access, testResource)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)

	// リフレッシュトークンで入れ替えられる。
	status, body = postFrom(t, f.server.TokenHandler(slog.New(slog.DiscardHandler)), oauth.ExchangePath, testSource, url.Values{
		paramGrantType: {grantRefreshToken}, grantRefreshToken: {refresh}, paramClientID: {testDeviceID},
	})
	require.Equal(t, http.StatusOK, status, body)
	access, _ = body["access_token"].(string)

	// 取り消しても行は消さず、取り消した日時を書く。トークンは使えなくなる。
	grants, err := f.server.ListGrants(t.Context(), f.member.UserID)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	assert.True(t, grants[0].Device)
	require.NoError(t, f.server.DeleteGrant(t.Context(), audit.User(f.member.UserID), f.member.UserID, grants[0].ID))
	_, _, err = f.server.AuthenticateDevice(t.Context(), access, testEPGResource)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
	_, err = f.server.DeviceGrantRole(t.Context(), grants[0].ID)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM oauth_grants WHERE revoked_at IS NOT NULL"))
	grants, err = f.server.ListGrants(t.Context(), f.member.UserID)
	require.NoError(t, err)
	assert.Empty(t, grants)
}

// decodeBody は、JSON の本文を読む。JSON でなければ空にする。
func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	body := map[string]any{}
	if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	}

	return body
}

func (f *deviceFixture) count(t *testing.T, query string) int {
	t.Helper()

	var n int
	require.NoError(t, f.db.QueryRow(t.Context(), query).Scan(&n))

	return n
}

// TestDevicePollInterval は、前回からの間が、問い合わせの間隔 (5 秒) からタイマーのずれのゆとり (2 秒) を引いた 3 秒より短い
// 問い合わせにだけ slow_down を返すことを、閾値の直前と、ちょうど閾値で確かめる。
func TestDevicePollInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		elapsed time.Duration
		want    string
	}{
		{name: "閾値の直前", elapsed: 3*time.Second - time.Microsecond, want: errorSlowDown},
		{name: "ちょうど閾値", elapsed: 3 * time.Second, want: errorAuthorizationPending},
		{name: "問い合わせの間隔のとおり", elapsed: 5 * time.Second, want: errorAuthorizationPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newDeviceFixture(t)
			deviceCode, _ := f.issue(t)
			require.Equal(t, errorAuthorizationPending, f.pollError(t, deviceCode))

			f.clock.Advance(tt.elapsed)
			assert.Equal(t, tt.want, f.pollError(t, deviceCode))
		})
	}
}

// TestDeviceCodeErrors は、端末の問い合わせに、RFC 8628 の名前で答えることを確かめる。
func TestDeviceCodeErrors(t *testing.T) {
	t.Parallel()

	f := newDeviceFixture(t)
	deviceCode, _ := f.issue(t)

	assert.Equal(t, errorInvalidGrant, f.pollError(t, "kotatsu_dc_unknown"))
	status, body := postFrom(t, f.server.TokenHandler(slog.New(slog.DiscardHandler)), oauth.ExchangePath, testSource, url.Values{
		paramGrantType: {grantDeviceCode}, "device_code": {deviceCode}, paramClientID: {"com.example.other"},
	})
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, errorInvalidGrant, body["error"])

	f.clock.Advance(oauth.DeviceCodeLifetime)
	assert.Equal(t, errorExpiredToken, f.pollError(t, deviceCode))
}

// TestDeviceCodeCaps は、有効なコードの数の上限に達したら、まだ承認していないコードを古い方から失効させて空け、
// 全部承認済みで空けられないときだけ 429 を返すことを確かめる。接続元は、IPv4 ならアドレス、IPv6 なら /64 で数える。
func TestDeviceCodeCaps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// sources は、上限まで埋める端末の接続元。順に 1 つずつ求める。
		sources []string
		// next は、埋まった後に求める接続元。
		next string
		// approveAll は、埋めたコードをすべて承認してから求めるかどうか。
		approveAll bool
		// wantExpired は、next が求めた後に失効した、sources のコードの番号。
		wantExpired []int
		wantStatus  int
	}{
		{
			name:    "接続元ごとの上限では、その接続元のいちばん古いものを失効させる",
			sources: fill("192.0.2.10:1"), next: "192.0.2.10:2",
			wantExpired: []int{0}, wantStatus: http.StatusOK,
		},
		{
			name:    "IPv6 は /64 で数える",
			sources: fill("[2001:db8:1:2::1]:1"), next: "[2001:db8:1:2:ffff::9]:1",
			wantExpired: []int{0}, wantStatus: http.StatusOK,
		},
		{
			name:    "IPv6 の別の /64 は別に数える",
			sources: fill("[2001:db8:1:2::1]:1"), next: "[2001:db8:1:3::1]:1",
			wantExpired: []int{}, wantStatus: http.StatusOK,
		},
		{
			name:    "IPv4 のアドレスが違えば別に数える",
			sources: fill("192.0.2.10:1"), next: "192.0.2.11:1",
			wantExpired: []int{}, wantStatus: http.StatusOK,
		},
		{
			name:    "全体の上限では、全体でいちばん古いものを失効させる",
			sources: distinctSources(100), next: "198.51.100.1:1",
			wantExpired: []int{0}, wantStatus: http.StatusOK,
		},
		{
			name:    "全部承認済みなら 429",
			sources: fill("192.0.2.10:1"), next: "192.0.2.10:2", approveAll: true,
			wantExpired: []int{}, wantStatus: http.StatusTooManyRequests,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newDeviceFixture(t)
			codes := make([]string, 0, len(tt.sources))
			for _, source := range tt.sources {
				status, body := f.requestCode(t, source)
				require.Equal(t, http.StatusOK, status, body)
				deviceCode, _ := body["device_code"].(string)
				codes = append(codes, deviceCode)
				if tt.approveAll {
					userCode, _ := body["user_code"].(string)
					_, err := f.server.ApproveDevice(t.Context(), audit.User(f.member.UserID), f.member, userCode)
					require.NoError(t, err)
				}
				// 古い順を、作った日時で決めるので、少しずつずらす。
				f.clock.Advance(time.Second)
			}

			status, body := f.requestCode(t, tt.next)
			require.Equal(t, tt.wantStatus, status, body)

			expired := []int{}
			for i, deviceCode := range codes {
				// 承認済みのコードは、トークンを受け取れる。
				if _, body := f.poll(t, deviceCode); body["error"] == errorExpiredToken {
					expired = append(expired, i)
				}
			}
			assert.Equal(t, tt.wantExpired, expired)
		})
	}
}

// fill は、1 つの接続元から、接続元ごとの上限まで求める接続元の並びを返す。
func fill(source string) []string {
	sources := make([]string, perSourceCap)
	for i := range sources {
		sources[i] = source
	}

	return sources
}

// distinctSources は、接続元ごとの上限に当たらないよう、5 つずつ別の接続元に分けた n 個の接続元を返す。
func distinctSources(n int) []string {
	sources := make([]string, n)
	for i := range sources {
		sources[i] = fmt.Sprintf("10.0.%d.1:1", i/perSourceCap)
	}

	return sources
}

// TestApproveDevice は、承認を断る場合を確かめる。
func TestApproveDevice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		principal func(f *deviceFixture) *auth.Principal
		userCode  func(issued string) string
		wantErr   error
	}{
		{
			name:      "小文字、ハイフンの抜け、空白を許す",
			principal: func(f *deviceFixture) *auth.Principal { return f.member },
			userCode:  func(issued string) string { return " " + strings.ToLower(strings.ReplaceAll(issued, "-", "")) + " " },
			wantErr:   nil,
		},
		{
			name: "トークンで認証したリクエストからは承認しない",
			principal: func(f *deviceFixture) *auth.Principal {
				return &auth.Principal{UserID: f.member.UserID, Role: domain.RoleMember, SessionID: uuid.Nil(), AuthenticatedAt: time.Time{}, OAuthClientID: testDeviceID}
			},
			userCode: func(issued string) string { return issued },
			wantErr:  oauth.ErrTokenCannotApprove,
		},
		{
			name:      "使わない字を含む",
			principal: func(f *deviceFixture) *auth.Principal { return f.member },
			userCode:  func(string) string { return "0OIL-2345" },
			wantErr:   oauth.ErrInvalidUserCode,
		},
		{
			name:      "知らないコード",
			principal: func(f *deviceFixture) *auth.Principal { return f.member },
			userCode: func(issued string) string {
				if issued == "2222-2222" {
					return "3333-3333"
				}

				return "2222-2222"
			},
			wantErr: oauth.ErrDeviceCodeNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newDeviceFixture(t)
			deviceCode, userCode := f.issue(t)
			_, err := f.server.ApproveDevice(t.Context(), audit.User(f.member.UserID), tt.principal(f), tt.userCode(userCode))
			f.clock.Advance(5 * time.Second)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, errorAuthorizationPending, f.pollError(t, deviceCode))

				return
			}
			require.NoError(t, err)
			status, body := f.poll(t, deviceCode)
			assert.Equal(t, http.StatusOK, status, body)
		})
	}
}

// TestNormalizeUserCode は、人が打った user_code の正規化を確かめる。
func TestNormalizeUserCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "そのまま", input: "ABCD-EFGH", want: normalizedCode},
		{name: "小文字とハイフンの抜け", input: "abcdefgh", want: normalizedCode},
		{name: "空白と余分なハイフン", input: " AB CD--EF GH ", want: normalizedCode},
		{name: "全角", input: "ＡＢＣＤ－ＥＦＧＨ", want: normalizedCode},
		{name: "見間違えやすい字", input: "ABCD-EFG0", wantErr: true},
		{name: "短い", input: "ABCD-EFG", wantErr: true},
		{name: "長い", input: "ABCD-EFGHJ", wantErr: true},
		{name: "記号", input: "ABCD_EFGH", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := oauth.NormalizeUserCode(tt.input)
			if tt.wantErr {
				require.ErrorIs(t, err, oauth.ErrInvalidUserCode)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestDeviceDisabled は、EPGStation 互換の API が無効なら、デバイス認可がないものとして振る舞い、発行済みのトークンも使えないことを確かめる。
func TestDeviceDisabled(t *testing.T) {
	t.Parallel()

	f := newDeviceFixture(t)
	deviceCode, userCode := f.issue(t)
	_, err := f.server.ApproveDevice(t.Context(), audit.User(f.member.UserID), f.member, userCode)
	require.NoError(t, err)
	_, body := f.poll(t, deviceCode)
	access, _ := body["access_token"].(string)

	setEPGStationEnabled(t, f.db, false)
	status, _ := f.requestCode(t, testSource)
	assert.Equal(t, http.StatusNotFound, status)
	// MCP は有効なので、トークンのエンドポイントは答えるが、device_code は受け付けない。
	status, body = f.poll(t, deviceCode)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "unsupported_grant_type", body["error"])
	_, _, err = f.server.AuthenticateDevice(t.Context(), access, testEPGResource)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
	_, err = f.server.LookupDevice(t.Context(), f.member, userCode)
	require.ErrorIs(t, err, oauth.ErrDeviceDisabled)
}
