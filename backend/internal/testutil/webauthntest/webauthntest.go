// Package webauthntest は、テストでパスキーの登録とログインを行う、ソフトウェアの認証器を提供する。
//
// ブラウザーと認証器の代わりに、go-webauthn が受け取る PublicKeyCredential の JSON を作る。
// 鍵は P-256 の ECDSA で、attestation は none にする。
package webauthntest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-webauthn/webauthn/protocol"
)

// 認証器のデータのフラグ (WebAuthn 6.1)。
const (
	flagUserPresent  byte = 0x01
	flagUserVerified byte = 0x04
	flagAttested     byte = 0x40
)

// COSE の鍵の項目 (RFC 9053)。
const (
	coseKeyType   = 1
	coseAlgorithm = 3
	coseCurve     = -1
	coseX         = -2
	coseY         = -3
	coseKeyTypeEC = 2
	coseES256     = -7
	coseCurveP256 = 1
	ecCoordinate  = 32
)

// credentialIDLength は、作るパスキーの ID の長さ。
const credentialIDLength = 32

// Authenticator は、1 つのパスキーを持つソフトウェアの認証器。
type Authenticator struct {
	// Origin は、ブラウザーが clientDataJSON に入れるオリジン。
	Origin string
	// UserVerified が false なら、生体認証か PIN での本人の確認をしなかったことにする。
	UserVerified bool
	// Counter は、署名の回数。同期するパスキーのように 0 のままにするなら、IncrementCounter を false にする。
	Counter          uint32
	IncrementCounter bool

	key          *ecdsa.PrivateKey
	credentialID []byte
	userHandle   []byte
	rpID         string
}

// New は、origin のブラウザーで使う認証器を返す。本人の確認をし、署名の回数を増やす。
func New(origin string) (*Authenticator, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate a key: %w", err)
	}
	credentialID := make([]byte, credentialIDLength)
	if _, err := rand.Read(credentialID); err != nil {
		return nil, fmt.Errorf("failed to generate a credential ID: %w", err)
	}

	return &Authenticator{
		Origin: origin, UserVerified: true, Counter: 0, IncrementCounter: true,
		key: key, credentialID: credentialID, userHandle: nil, rpID: "",
	}, nil
}

// CredentialID は、パスキーの ID を返す。
func (a *Authenticator) CredentialID() []byte {
	return a.credentialID
}

// Create は、navigator.credentials.create の結果として、パスキーを作った応答の JSON を返す。
func (a *Authenticator) Create(options *protocol.PublicKeyCredentialCreationOptions) ([]byte, error) {
	userHandle, err := bytesOf(options.User.ID)
	if err != nil {
		return nil, err
	}
	a.userHandle, a.rpID = userHandle, options.RelyingParty.ID

	clientData, err := a.clientData("webauthn.create", options.Challenge)
	if err != nil {
		return nil, err
	}
	publicKey, err := a.publicKey()
	if err != nil {
		return nil, err
	}
	authData := a.authenticatorData(flagAttested)
	authData = append(authData, make([]byte, 16)...) // AAGUID
	authData = binary.BigEndian.AppendUint16(authData, credentialIDLength)
	authData = append(authData, a.credentialID...)
	authData = append(authData, publicKey...)
	attestation, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": authData})
	if err != nil {
		return nil, fmt.Errorf("failed to encode the attestation: %w", err)
	}

	return a.credential(map[string]any{
		"clientDataJSON":    encode(clientData),
		"attestationObject": encode(attestation),
		"transports":        []string{"internal"},
	})
}

// Get は、navigator.credentials.get の結果として、パスキーで署名した応答の JSON を返す。
func (a *Authenticator) Get(options *protocol.PublicKeyCredentialRequestOptions) ([]byte, error) {
	if a.IncrementCounter {
		a.Counter++
	}
	clientData, err := a.clientData("webauthn.get", options.Challenge)
	if err != nil {
		return nil, err
	}
	authData := a.authenticatorData(0)
	clientDataHash := sha256.Sum256(clientData)
	digest := sha256.Sum256(append(authData, clientDataHash[:]...))
	signature, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	return a.credential(map[string]any{
		"clientDataJSON":    encode(clientData),
		"authenticatorData": encode(authData),
		"signature":         encode(signature),
		"userHandle":        encode(a.userHandle),
	})
}

func (a *Authenticator) clientData(kind string, challenge protocol.URLEncodedBase64) ([]byte, error) {
	data, err := json.Marshal(map[string]any{"type": kind, "challenge": encode(challenge), "origin": a.Origin, "crossOrigin": false})
	if err != nil {
		return nil, fmt.Errorf("failed to encode the client data: %w", err)
	}

	return data, nil
}

// authenticatorData は、RP ID のハッシュ、フラグ、署名の回数を並べる。
func (a *Authenticator) authenticatorData(flags byte) []byte {
	rpIDHash := sha256.Sum256([]byte(a.rpID))
	flags |= flagUserPresent
	if a.UserVerified {
		flags |= flagUserVerified
	}
	data := append(rpIDHash[:], flags)

	return binary.BigEndian.AppendUint32(data, a.Counter)
}

func (a *Authenticator) publicKey() ([]byte, error) {
	raw, err := a.key.PublicKey.ECDH()
	if err != nil {
		return nil, fmt.Errorf("failed to convert the key: %w", err)
	}
	// 非圧縮の点 (0x04 || X || Y) から、座標を取り出す。
	point := raw.Bytes()
	key, err := cbor.Marshal(map[int]any{
		coseKeyType:   coseKeyTypeEC,
		coseAlgorithm: coseES256,
		coseCurve:     coseCurveP256,
		coseX:         point[1 : 1+ecCoordinate],
		coseY:         point[1+ecCoordinate:],
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode the public key: %w", err)
	}

	return key, nil
}

func (a *Authenticator) credential(response map[string]any) ([]byte, error) {
	data, err := json.Marshal(map[string]any{
		"id":                     encode(a.credentialID),
		"rawId":                  encode(a.credentialID),
		"type":                   "public-key",
		"response":               response,
		"clientExtensionResults": map[string]any{},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode the credential: %w", err)
	}

	return data, nil
}

func bytesOf(v any) ([]byte, error) {
	switch v := v.(type) {
	case protocol.URLEncodedBase64:
		return v, nil
	case []byte:
		return v, nil
	case string:
		// JSON から読んだオプションでは、base64url の文字列になる。
		b, err := base64.RawURLEncoding.DecodeString(v)
		if err != nil {
			return nil, fmt.Errorf("failed to decode the user ID: %w", err)
		}

		return b, nil
	default:
		return nil, fmt.Errorf("unexpected user ID type %T", v)
	}
}

func encode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
