// Package logattr は、利用者が送った値 (リクエストのパスやヘッダー、GraphQL の引数、それを含むエラー) を、ログの属性にする。
// 値の改行 (CR と LF) を \r と \n の 2 文字にして、値が別の行のログに見えないようにする。
// ログは slog の JSON で書くので、改行は JSON の側でもエスケープされるが、JSON 以外の書き方に変えても崩れないよう、属性の側でも落とす。
package logattr

import (
	"log/slog"
	"strings"
)

// String は、value の改行をエスケープした文字列の属性を返す。
func String(key, value string) slog.Attr {
	return slog.String(key, escape(value))
}

// Error は、err の文言の改行をエスケープした属性を、error の名前で返す。err が nil なら、値を nil にする。
func Error(err error) slog.Attr {
	if err == nil {
		return slog.Any("error", nil)
	}

	return slog.String("error", escape(err.Error()))
}

// escape は、value の CR と LF を、\r と \n の 2 文字にする。
func escape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r", `\r`), "\n", `\n`)
}
