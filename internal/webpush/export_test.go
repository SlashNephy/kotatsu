package webpush

import (
	"crypto/ecdh"
	"net/netip"
)

// EncryptWith は、送り手の鍵とソルトを決めて暗号化する。RFC 8291 の付録 A の値と比べるのに使う。
func EncryptWith(plaintext, uaPublic, authSecret []byte, asPrivate *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	return encrypt(plaintext, uaPublic, authSecret, asPrivate, salt)
}

// IsPublic は isPublic をテストから呼ぶ。
func IsPublic(addr netip.Addr) bool {
	return isPublic(addr)
}
