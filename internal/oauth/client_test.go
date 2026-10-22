package oauth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/oauth"
)

// TestParseClientID は、CIMD の client_id として、https でパスを持つ URL だけを受け付けることを確かめる。
func TestParseClientID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: claudeCodeName, raw: "https://claude.ai/oauth/claude-code-client-metadata", wantErr: false},
		{name: "ChatGPT", raw: "https://chatgpt.com/oauth/client.json", wantErr: false},
		{name: "with port", raw: "https://example.com:8443/client.json", wantErr: false},
		{name: "empty", raw: "", wantErr: true},
		{name: "http", raw: "http://example.com/client.json", wantErr: true},
		{name: "no path", raw: "https://example.com", wantErr: true},
		{name: "root path", raw: "https://example.com/", wantErr: true},
		{name: "dot segment", raw: "https://example.com/a/./client.json", wantErr: true},
		{name: "parent segment", raw: "https://example.com/a/../client.json", wantErr: true},
		{name: "fragment", raw: "https://example.com/client.json#a", wantErr: true},
		{name: "empty fragment", raw: "https://example.com/client.json#", wantErr: true},
		{name: "user info", raw: "https://user@example.com/client.json", wantErr: true},
		{name: "relative", raw: "/client.json", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u, err := oauth.ParseClientID(tt.raw)
			if tt.wantErr {
				require.ErrorIs(t, err, oauth.ErrInvalidClientID)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.raw, u.String())
		})
	}
}

// TestMatchRedirectURI は、登録したリダイレクト URI と完全に一致するものだけを受け付け、
// ループバックの http だけはポートを無視することを確かめる。
func TestMatchRedirectURI(t *testing.T) {
	t.Parallel()

	claudeCode := []string{loopbackCallback, "http://127.0.0.1/callback"}
	tests := []struct {
		name       string
		registered []string
		requested  string
		want       bool
	}{
		{name: "exact https", registered: []string{"https://chatgpt.com/connector_platform_oauth_redirect"}, requested: "https://chatgpt.com/connector_platform_oauth_redirect", want: true},
		{name: "loopback with ephemeral port", registered: claudeCode, requested: "http://localhost:53682/callback", want: true},
		{name: "loopback IPv4 with port", registered: claudeCode, requested: "http://127.0.0.1:41234/callback", want: true},
		{name: "loopback IPv6 with port", registered: []string{"http://[::1]/callback"}, requested: "http://[::1]:8080/callback", want: true},
		{name: "loopback different path", registered: claudeCode, requested: "http://localhost:53682/other", want: false},
		{name: "loopback different host", registered: []string{loopbackCallback}, requested: "http://127.0.0.1:53682/callback", want: false},
		{name: "loopback different query", registered: claudeCode, requested: "http://localhost:53682/callback?a=1", want: false},
		{name: "https ignores no port", registered: []string{exampleCallback}, requested: "https://example.com:8443/cb", want: false},
		{name: "https trailing slash", registered: []string{exampleCallback}, requested: "https://example.com/cb/", want: false},
		{name: "non-loopback http", registered: []string{"http://example.com/cb"}, requested: "http://example.com/cb", want: false},
		{name: "fragment", registered: []string{"https://example.com/cb#a"}, requested: "https://example.com/cb#a", want: false},
		{name: "private-use scheme", registered: []string{"com.example.app:/oauth"}, requested: "com.example.app:/oauth", want: true},
		{name: "javascript scheme", registered: []string{"javascript:alert(1)"}, requested: "javascript:alert(1)", want: false},
		{name: "not registered", registered: []string{exampleCallback}, requested: "https://evil.example/cb", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, oauth.MatchRedirectURI(tt.registered, tt.requested))
		})
	}
}

// TestVerifyCodeVerifier は、RFC 7636 の付録 B の例で、S256 の照合ができることを確かめる。
func TestVerifyCodeVerifier(t *testing.T) {
	t.Parallel()

	const (
		verifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	)
	tests := []struct {
		name      string
		verifier  string
		challenge string
		want      bool
	}{
		{name: "RFC 7636 appendix B", verifier: verifier, challenge: challenge, want: true},
		{name: "wrong verifier", verifier: "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXm", challenge: challenge, want: false},
		{name: "too short verifier", verifier: "short", challenge: challenge, want: false},
		{name: "invalid character", verifier: "dBjftJeZ4CVP+mB92K27uhbUJU1p1r_wW1gFWFOEjXk", challenge: challenge, want: false},
		{name: "plain is not accepted", verifier: verifier, challenge: verifier, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, oauth.VerifyCodeVerifier(tt.verifier, tt.challenge))
		})
	}
}

// TestValidCodeChallenge は、S256 の code_challenge の形だけを受け付けることを確かめる。
func TestValidCodeChallenge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		challenge string
		want      bool
	}{
		{name: "S256", challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", want: true},
		{name: "too short", challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-c", want: false},
		{name: "padding", challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-c=", want: false},
		{name: "standard base64", challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw+cM", want: false},
		{name: "empty", challenge: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, oauth.ValidCodeChallenge(tt.challenge))
		})
	}
}
