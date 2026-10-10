// Package password は、ローカルアカウントのパスワードを扱う (設計書 9.3、2.2)。
//
// パスワードは Argon2id でハッシュ化し、NIST SP 800-63B-4 に沿って、長さ、同梱のブロックリスト、
// zxcvbn-ts の強度で検証する。Have I Been Pwned への照会は、設定で有効にしたときだけ行う。
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id のパラメーター (付録 A)。OWASP の Password Storage Cheat Sheet が挙げる最小の構成
// (メモリ 19 MiB、反復 2 回、並列 1) にする。低スペック機でも、1 回の検証が数十ミリ秒で済む。
const (
	argon2Memory      uint32 = 19 * 1024
	argon2Iterations  uint32 = 2
	argon2Parallelism uint8  = 1
	argon2SaltLength         = 16
	argon2KeyLength   uint32 = 32
)

// ErrInvalidHash は、保存したハッシュの形式が正しくないことを表す。
var ErrInvalidHash = errors.New("invalid password hash")

type params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func currentParams() params {
	return params{memory: argon2Memory, iterations: argon2Iterations, parallelism: argon2Parallelism}
}

// Hash は、パスワードを Argon2id でハッシュ化し、PHC 文字列形式で返す。
// パラメーターを文字列に含めるので、後からパラメーターを強めても、古いハッシュを検証できる。
func Hash(password string) (string, error) {
	salt := make([]byte, argon2SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate a salt: %w", err)
	}
	p := currentParams()
	key := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, argon2KeyLength)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.iterations, p.parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verification は、パスワードの検証の結果。
type Verification struct {
	// OK は、パスワードがハッシュに一致したかどうか。
	OK bool
	// NeedsRehash は、ハッシュのパラメーターが今のものと違うかどうか。一致したときに、ハッシュを作り直すのに使う。
	NeedsRehash bool
}

// maxKeyLength は、保存したハッシュの鍵の長さの上限。壊れた値で大きな計算をしないために設ける。
const maxKeyLength = 1024

// Verify は、パスワードがハッシュに一致するかを返す。
func Verify(password, encoded string) (*Verification, error) {
	p, salt, key, err := decode(encoded)
	if err != nil {
		return nil, err
	}
	keyLength := len(key)
	if keyLength == 0 || keyLength > maxKeyLength {
		return nil, fmt.Errorf("%w: unexpected key length %d", ErrInvalidHash, keyLength)
	}
	actual := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, uint32(keyLength))
	if subtle.ConstantTimeCompare(actual, key) != 1 {
		return &Verification{OK: false, NeedsRehash: false}, nil
	}

	return &Verification{OK: true, NeedsRehash: p != currentParams()}, nil
}

// dummyHash は、どのパスワードにも一致しない、今のパラメーターでのハッシュ。
const dummyHash = "$argon2id$v=19$m=19456,t=2,p=1$gelerYLlvuZwLQgmpnC7ew$cwpP7WwAtgHSqMp8xskoovDBEn07D3g2x+XuqAIDQQ8"

// VerifyDummy は、ユーザーがいないときに、Verify と同じだけ時間をかける。
// ユーザー名が存在するかどうかを、応答の時間から推測させないためである。
func VerifyDummy(password string) error {
	_, err := Verify(password, dummyHash)

	return err
}

func decode(encoded string) (params, []byte, []byte, error) {
	var p params
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return p, nil, nil, fmt.Errorf("%w: not an argon2id hash", ErrInvalidHash)
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return p, nil, nil, fmt.Errorf("%w: unsupported version", ErrInvalidHash)
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.iterations, &p.parallelism); err != nil {
		return p, nil, nil, fmt.Errorf("%w: invalid parameters: %w", ErrInvalidHash, err)
	}
	if p.memory == 0 || p.iterations == 0 || p.parallelism == 0 {
		return p, nil, nil, fmt.Errorf("%w: invalid parameters", ErrInvalidHash)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return p, nil, nil, fmt.Errorf("%w: invalid salt: %w", ErrInvalidHash, err)
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return p, nil, nil, fmt.Errorf("%w: invalid key: %w", ErrInvalidHash, err)
	}

	return p, salt, key, nil
}
