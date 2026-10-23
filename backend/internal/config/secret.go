package config

import (
	"encoding/json"
	"log/slog"
)

const redacted = "[REDACTED]"

// Secret は秘密情報の値。fmt、slog、JSON のどれで出力しても値を伏せる。
// 値そのものは Value で取り出す。
type Secret string

var (
	_ slog.LogValuer = Secret("")
	_ json.Marshaler = Secret("")
)

// Value は伏せていない値を返す。
func (s Secret) Value() string {
	return string(s)
}

func (s Secret) String() string {
	if s == "" {
		return ""
	}

	return redacted
}

// GoString は %#v での出力にも値を出さないために定義する。
func (s Secret) GoString() string {
	return s.String()
}

func (s Secret) LogValue() slog.Value {
	return slog.StringValue(s.String())
}

func (s Secret) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}
