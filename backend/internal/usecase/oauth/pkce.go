package oauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

const (
	// minCodeVerifier と maxCodeVerifier は、PKCE の code_verifier の長さ (RFC 7636 4.1)。
	minCodeVerifier = 43
	maxCodeVerifier = 128
	// codeChallengeLength は、S256 の code_challenge の長さ。SHA-256 の 32 バイトを、パディングなしの base64url にしたもの。
	codeChallengeLength = 43
	// CodeChallengeMethodS256 は、Kotatsu が受け付ける、PKCE の code_challenge の作り方。plain は受け付けない。
	CodeChallengeMethodS256 = "S256"
)

// ValidCodeChallenge は、S256 の code_challenge として正しい形かどうかを返す。
func ValidCodeChallenge(challenge string) bool {
	if len(challenge) != codeChallengeLength {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(challenge)

	return err == nil && len(decoded) == sha256.Size
}

// VerifyCodeVerifier は、code_verifier から作った S256 の値が、認可のときに受け取った code_challenge と合うかどうかを返す。
func VerifyCodeVerifier(verifier, challenge string) bool {
	if !validCodeVerifier(verifier) {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])

	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// validCodeVerifier は、code_verifier が、決まった長さで、使ってよい文字 (英数字と -._~) だけでできているかどうかを返す。
func validCodeVerifier(verifier string) bool {
	if len(verifier) < minCodeVerifier || len(verifier) > maxCodeVerifier {
		return false
	}
	for _, c := range verifier {
		isAlnum := ('A' <= c && c <= 'Z') || ('a' <= c && c <= 'z') || ('0' <= c && c <= '9')
		if !isAlnum && c != '-' && c != '.' && c != '_' && c != '~' {
			return false
		}
	}

	return true
}
