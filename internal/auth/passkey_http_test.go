package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
)

// 送信元とログインの状態を、テストのリクエストごとに選ぶヘッダー。
const (
	testAuthHeader = "X-Test-Auth"
	testHostHeader = "X-Test-Host"
	credentialKey  = "credential"
	badRequest     = "BAD_REQUEST"
)

type passkeyServer struct {
	t      *testing.T
	f      *passkeyFixture
	server *httptest.Server
	jar    *cookiejar.Jar
}

func newPasskeyServer(t *testing.T, f *passkeyFixture) *passkeyServer {
	t.Helper()

	// Cookie の期限は時計から決まる。過去の時刻のままだと、Cookie の入れ物が期限切れとして捨てる。
	f.clock.Advance(time.Since(f.clock.Now()))
	client := func(r *http.Request) *auth.ClientInfo {
		info := *f.client
		if host := r.Header.Get(testHostHeader); host != "" {
			info.Host, info.Secure = host, false
		}

		return &info
	}
	mux := http.NewServeMux()
	mux.Handle("POST /auth/login", f.logins.PasswordHandler(f.db, client))
	mux.Handle("POST /auth/login/passkey", f.logins.PasskeySecondFactorHandler(f.db, client))
	mux.Handle("POST /auth/login/passkey/confirm", f.logins.PasskeySecondFactorConfirmHandler(f.db, client))
	mux.Handle("POST /auth/passkey/login", f.logins.PasskeyLoginHandler(f.db, client))
	mux.Handle("POST /auth/passkey/login/confirm", f.logins.PasskeyLoginConfirmHandler(f.db, client))
	mux.Handle("POST /auth/passkey/registration", f.passkeys.RegistrationHandler(f.db, client))
	mux.Handle("POST /auth/passkey/registration/confirm", f.passkeys.RegistrationConfirmHandler(f.db, client))
	// ログイン中のユーザーは、本来はセッションのミドルウェアが持たせる。テストでは、ヘッダーで状態を選ぶ。
	withPrincipal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authenticatedAt := f.clock.Now()
		switch r.Header.Get(testAuthHeader) {
		case "none":
			mux.ServeHTTP(w, r)

			return
		case "stale":
			authenticatedAt = authenticatedAt.Add(-auth.RecentAuthenticationWindow - time.Minute)
		}
		mux.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), &auth.Principal{
			UserID: f.user.ID, Role: f.user.Role, SessionID: f.user.ID, AuthenticatedAt: authenticatedAt,
		})))
	})
	server := httptest.NewServer(withPrincipal)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	return &passkeyServer{t: t, f: f, server: server, jar: jar}
}

// post は、body を JSON で送り、応答を v に読む。headers は、名前と値を交互に並べる。
func (s *passkeyServer) post(path string, body any, v any, headers ...string) int {
	s.t.Helper()

	encoded, err := json.Marshal(body)
	require.NoError(s.t, err)
	req, err := http.NewRequestWithContext(s.t.Context(), http.MethodPost, s.server.URL+path, bytes.NewReader(encoded))
	require.NoError(s.t, err)
	req.Header.Set("Content-Type", jsonType)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := (&http.Client{Jar: s.jar}).Do(req)
	require.NoError(s.t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	if v != nil {
		require.NoError(s.t, json.NewDecoder(resp.Body).Decode(v))
	}

	return resp.StatusCode
}

func (s *passkeyServer) cookie(name string) string {
	s.t.Helper()

	for _, c := range s.jar.Cookies(mustParseURL(s.t, s.server.URL+"/auth/")) {
		if c.Name == name {
			return c.Value
		}
	}

	return ""
}

type errorBody struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

func credentialBody(t *testing.T, credential []byte, name string) map[string]any {
	t.Helper()

	return map[string]any{credentialKey: json.RawMessage(credential), "name": name}
}

// TestPasskeyHandlers は、パスキーの登録、パスキーだけでのログイン、第二要素としての確認を、HTTP で通して確かめる。
func TestPasskeyHandlers(t *testing.T) {
	t.Parallel()

	f := newPasskeyFixture(t)
	s := newPasskeyServer(t, f)
	a := newAuthenticator(t)

	// 登録には、ログインと、直前の再認証が要る。
	var failed errorBody
	assert.Equal(t, http.StatusUnauthorized, s.post("/auth/passkey/registration", map[string]any{}, &failed, testAuthHeader, "none"))
	assert.Equal(t, "UNAUTHENTICATED", failed.Error.Code)
	assert.Equal(t, http.StatusForbidden, s.post("/auth/passkey/registration", map[string]any{}, &failed, testAuthHeader, "stale"))
	assert.Equal(t, "REAUTHENTICATION_REQUIRED", failed.Error.Code)
	// HTTP で LAN のホスト名に接続していれば、パスキーを使えないことを伝える。
	assert.Equal(t, http.StatusBadRequest, s.post("/auth/passkey/registration", map[string]any{}, &failed, testHostHeader, "kotatsu.lan:8080"))
	assert.Equal(t, "PASSKEY_UNAVAILABLE", failed.Error.Code)

	var creation struct {
		Options   protocol.PublicKeyCredentialCreationOptions `json:"options"`
		ExpiresAt time.Time                                   `json:"expiresAt"`
	}
	require.Equal(t, http.StatusOK, s.post("/auth/passkey/registration", map[string]any{}, &creation))
	assert.Equal(t, testHost, creation.Options.RelyingParty.ID)
	assert.Equal(t, protocol.VerificationRequired, creation.Options.AuthenticatorSelection.UserVerification)
	assert.NotEmpty(t, s.cookie(auth.WebAuthnCookieName))
	created, err := a.Create(&creation.Options)
	require.NoError(t, err)

	// 名前が違うだけなら、手続きは残り、直して送り直せる。
	assert.Equal(t, http.StatusBadRequest, s.post("/auth/passkey/registration/confirm", credentialBody(t, created, " "), &failed))
	assert.Equal(t, "INVALID_PASSKEY_NAME", failed.Error.Code)
	var passkey struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		RPID string `json:"rpId"`
	}
	require.Equal(t, http.StatusCreated, s.post("/auth/passkey/registration/confirm", credentialBody(t, created, deviceLabel), &passkey))
	assert.Equal(t, deviceLabel, passkey.Name)
	assert.Equal(t, testHost, passkey.RPID)
	assert.Empty(t, s.cookie(auth.WebAuthnCookieName), "the ceremony cookie must be cleared")
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.passkey_registered' AND source_ip = '192.168.1.10'"))

	// パスキーだけでログインする。
	var assertion struct {
		Options protocol.PublicKeyCredentialRequestOptions `json:"options"`
	}
	require.Equal(t, http.StatusOK, s.post("/auth/passkey/login", map[string]any{}, &assertion, testAuthHeader, "none"))
	signed, err := a.Get(&assertion.Options)
	require.NoError(t, err)
	var completed struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	require.Equal(t, http.StatusOK, s.post("/auth/passkey/login/confirm", credentialBody(t, signed, ""), &completed, testAuthHeader, "none"))
	assert.Empty(t, completed.RecoveryCodes)
	assert.NotEmpty(t, s.cookie(auth.CookieName), "the session cookie must be set")
	assert.Empty(t, s.cookie(auth.WebAuthnCookieName))
	// 同じ手続きは、二度と使えない。
	assert.Equal(t, http.StatusUnauthorized, s.post("/auth/passkey/login/confirm", credentialBody(t, signed, ""), &failed, testAuthHeader, "none"))
	assert.Equal(t, "PASSKEY_CEREMONY_EXPIRED", failed.Error.Code)

	// パスワードの後の第二要素として確かめる。
	var login struct {
		Next    string   `json:"next"`
		Methods []string `json:"methods"`
	}
	require.Equal(t, http.StatusOK, s.post("/auth/login", map[string]any{"username": loginName, passwordText: loginPhrase}, &login, testAuthHeader, "none"))
	assert.Equal(t, "second_factor", login.Next)
	assert.Equal(t, []string{"passkey"}, login.Methods)
	require.Equal(t, http.StatusOK, s.post("/auth/login/passkey", map[string]any{}, &assertion, testAuthHeader, "none"))
	require.Len(t, assertion.Options.AllowedCredentials, 1)
	a.UserVerified = false
	signed, err = a.Get(&assertion.Options)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, s.post("/auth/login/passkey/confirm", credentialBody(t, signed, ""), &failed, testAuthHeader, "none"))
	assert.Equal(t, "INVALID_SECOND_FACTOR", failed.Error.Code)

	a.UserVerified = true
	require.Equal(t, http.StatusOK, s.post("/auth/login/passkey", map[string]any{}, &assertion, testAuthHeader, "none"))
	signed, err = a.Get(&assertion.Options)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, s.post("/auth/login/passkey/confirm", credentialBody(t, signed, ""), &completed, testAuthHeader, "none"))
	assert.Empty(t, s.cookie(auth.ChallengeCookieName), "the challenge cookie must be cleared")
	assert.Equal(t, 2, f.count(t, "SELECT count(*) FROM audit_events WHERE action = 'user.login' AND summary->>'method' = 'passkey'"))
}

// TestPasskeyHandlersRejectMissingInput は、手続きのトークンや応答がなければ、受け付けないことを確かめる。
func TestPasskeyHandlersRejectMissingInput(t *testing.T) {
	t.Parallel()

	f := newPasskeyFixture(t)
	s := newPasskeyServer(t, f)

	tests := []struct {
		name       string
		path       string
		body       map[string]any
		wantStatus int
		wantCode   string
	}{
		{name: "no credential", path: "/auth/passkey/login/confirm", body: map[string]any{}, wantStatus: http.StatusBadRequest, wantCode: badRequest},
		{
			name: "no ceremony cookie", path: "/auth/passkey/login/confirm", body: map[string]any{credentialKey: map[string]any{}},
			wantStatus: http.StatusUnauthorized, wantCode: "PASSKEY_CEREMONY_EXPIRED",
		},
		{
			name: "no registration ceremony cookie", path: "/auth/passkey/registration/confirm", body: map[string]any{credentialKey: map[string]any{}, "name": deviceLabel},
			wantStatus: http.StatusUnauthorized, wantCode: "PASSKEY_CEREMONY_EXPIRED",
		},
		{name: "no password step", path: "/auth/login/passkey", body: map[string]any{}, wantStatus: http.StatusUnauthorized, wantCode: "LOGIN_REQUIRED"},
	}
	for _, tt := range tests {
		var failed errorBody
		assert.Equal(t, tt.wantStatus, s.post(tt.path, tt.body, &failed), tt.name)
		assert.Equal(t, tt.wantCode, failed.Error.Code, tt.name)
	}

	// パスキーのないユーザーには、第二要素としての確認を始めさせない。
	require.Equal(t, http.StatusOK, s.post("/auth/login", map[string]any{"username": loginName, passwordText: loginPhrase}, nil))
	var failed errorBody
	assert.Equal(t, http.StatusBadRequest, s.post("/auth/login/passkey", map[string]any{}, &failed))
	assert.Equal(t, "NO_PASSKEY", failed.Error.Code)
}
