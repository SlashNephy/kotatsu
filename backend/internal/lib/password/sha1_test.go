package password_test

import (
	"crypto"
	"encoding/hex"
	"strings"
)

func sha1Hex(s string) string {
	h := crypto.SHA1.New()
	h.Write([]byte(s))

	return strings.ToUpper(hex.EncodeToString(h.Sum(nil)))
}

func sha1Prefix(s string) string {
	return sha1Hex(s)[:5]
}

func sha1Suffix(s string) string {
	return sha1Hex(s)[5:]
}
