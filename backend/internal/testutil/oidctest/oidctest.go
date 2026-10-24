// Package oidctest は、テストで OIDC のログインを行う、最小限の IdP を提供する。
//
// 認可コードフローと PKCE (S256) に対応し、RS256 で署名した ID トークンを発行する。
// discovery (/.well-known/openid-configuration)、JWKS、認可、トークンのエンドポイントを持つ。
package oidctest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
)

const (
	keyID     = "oidctest"
	rsaBits   = 2048
	codeBytes = 16
	tokenTTL  = 5 * time.Minute
	grantType = "authorization_code"
	s256      = "S256"
)

// Provider は、テスト用の IdP。Subject と Claims を変えると、次に発行する ID トークンの中身が変わる。
type Provider struct {
	ClientID     string
	ClientSecret string

	mu sync.Mutex
	// Subject は、次にログインするユーザーの sub。
	Subject string
	// Claims は、ID トークンに足す claim (groups、preferred_username など)。
	Claims map[string]any
	// WrongNonce なら、認可のリクエストと違う nonce を ID トークンに入れる。
	WrongNonce bool
	// WrongAudience なら、ほかのクライアントに向けた ID トークンを発行する。
	WrongAudience bool
	// Now は、ID トークンの発行と期限の時刻。nil なら time.Now。
	Now func() time.Time
	// LastAuthorization は、最後に受け取った認可のリクエストのパラメーター。
	LastAuthorization url.Values

	redirectURIs []string
	server       *httptest.Server
	key          *rsa.PrivateKey
	grants       map[string]*grant
}

type grant struct {
	challenge   string
	nonce       string
	redirectURI string
}

// New は、IdP を起動して返す。Close で止める。redirectURIs は、クライアントに登録したリダイレクト URI。
// 実際の IdP と同じく、登録したものにだけ戻す。
func New(clientID, clientSecret string, redirectURIs ...string) (*Provider, error) {
	key, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate a signing key: %w", err)
	}
	p := &Provider{
		ClientID: clientID, ClientSecret: clientSecret, redirectURIs: redirectURIs, mu: sync.Mutex{},
		Subject: "", Claims: map[string]any{}, WrongNonce: false, WrongAudience: false, Now: nil, LastAuthorization: nil,
		server: nil, key: key, grants: map[string]*grant{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("GET /keys", p.keys)
	mux.HandleFunc("GET /authorize", p.authorizeHandler)
	mux.HandleFunc("POST /token", p.token)
	p.server = httptest.NewServer(mux)

	return p, nil
}

// Issuer は、IdP の issuer の URL を返す。
func (p *Provider) Issuer() string {
	return p.server.URL
}

// Close は IdP を止める。
func (p *Provider) Close() {
	p.server.Close()
}

// Authorize は、ブラウザーが認可のエンドポイント authURL を開き、ユーザーがログインして同意したときの振る舞いをまねる。
// クライアントに戻るリダイレクト先 (code と state を付けたリダイレクト URI) を返す。
func (p *Provider) Authorize(authURL string) (string, error) {
	u, err := url.Parse(authURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse the authorization URL: %w", err)
	}

	return p.authorize(u.Query())
}

func (p *Provider) authorize(query url.Values) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.LastAuthorization = query
	switch {
	case query.Get("client_id") != p.ClientID:
		return "", errors.New("unknown client_id")
	case query.Get("response_type") != "code":
		return "", errors.New("response_type must be code")
	case query.Get("code_challenge_method") != s256 || query.Get("code_challenge") == "":
		return "", errors.New("PKCE with S256 is required")
	}
	registered := slices.Index(p.redirectURIs, query.Get("redirect_uri"))
	if registered < 0 {
		return "", errors.New("redirect_uri is not registered")
	}
	raw := make([]byte, codeBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate a code: %w", err)
	}
	code := base64.RawURLEncoding.EncodeToString(raw)
	redirectURI := p.redirectURIs[registered]
	p.grants[code] = &grant{challenge: query.Get("code_challenge"), nonce: query.Get("nonce"), redirectURI: redirectURI}

	redirect, err := url.Parse(redirectURI)
	if err != nil {
		return "", fmt.Errorf("failed to parse redirect_uri: %w", err)
	}
	values := redirect.Query()
	values.Set("code", code)
	values.Set("state", query.Get("state"))
	redirect.RawQuery = values.Encode()

	return redirect.String(), nil
}

func (p *Provider) authorizeHandler(w http.ResponseWriter, r *http.Request) {
	redirect, err := p.authorize(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

func (p *Provider) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                p.Issuer(),
		"authorization_endpoint":                p.Issuer() + "/authorize",
		"token_endpoint":                        p.Issuer() + "/token",
		"jwks_uri":                              p.Issuer() + "/keys",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"code_challenge_methods_supported":      []string{s256},
	})
}

func (p *Provider) keys(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
		{Key: &p.key.PublicKey, KeyID: keyID, Algorithm: string(jose.RS256), Use: "sig"},
	}})
}

func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}
	if !p.authenticateClient(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})

		return
	}
	idToken, err := p.exchange(r.PostForm)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": err.Error()})

		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": "oidctest-access", "token_type": "Bearer", "expires_in": int(tokenTTL.Seconds()), "id_token": idToken,
	})
}

// authenticateClient は、クライアントの ID と秘密を、Basic 認証か、本文のパラメーターで確かめる。
func (p *Provider) authenticateClient(r *http.Request) bool {
	id, secret, ok := r.BasicAuth()
	if !ok {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	// oauth2 は、Basic 認証では ID と秘密を URL エンコードしてから送る。
	if decoded, err := url.QueryUnescape(id); err == nil {
		id = decoded
	}
	if decoded, err := url.QueryUnescape(secret); err == nil {
		secret = decoded
	}

	return id == p.ClientID && subtle.ConstantTimeCompare([]byte(secret), []byte(p.ClientSecret)) == 1
}

func (p *Provider) exchange(form url.Values) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if form.Get("grant_type") != grantType {
		return "", errors.New("unsupported grant_type")
	}
	g, ok := p.grants[form.Get("code")]
	if !ok {
		return "", errors.New("unknown or used code")
	}
	// コードは 1 回しか使えない。
	delete(p.grants, form.Get("code"))
	if form.Get("redirect_uri") != g.redirectURI {
		return "", errors.New("redirect_uri does not match")
	}
	sum := sha256.Sum256([]byte(form.Get("code_verifier")))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
		return "", errors.New("code_verifier does not match the code_challenge")
	}

	return p.idToken(g.nonce)
}

func (p *Provider) idToken(nonce string) (string, error) {
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	audience := p.ClientID
	if p.WrongAudience {
		audience = "another-client"
	}
	if p.WrongNonce {
		nonce = "another-nonce"
	}
	claims := maps.Clone(p.Claims)
	if claims == nil {
		claims = map[string]any{}
	}
	claims["iss"], claims["sub"], claims["aud"] = p.Issuer(), p.Subject, audience
	claims["iat"], claims["exp"], claims["nonce"] = now.Unix(), now.Add(tokenTTL).Unix(), nonce
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to encode the claims: %w", err)
	}
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: p.key, KeyID: keyID, Algorithm: string(jose.RS256), Use: "sig"}},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create a signer: %w", err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		return "", fmt.Errorf("failed to sign the ID token: %w", err)
	}

	return signed.CompactSerialize()
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// ヘッダーは送ってしまったので、これ以上は伝えられない。
		return
	}
}
