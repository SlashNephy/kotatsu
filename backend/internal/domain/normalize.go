// Package epg は、チューナーサーバーの EPG を DB に同期する (設計書 3.3)。
package domain

import (
	"maps"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Normalize は、検索用の列に入れる文字列を作る (設計書 3.5)。
// NFKC で全角の英数字と記号を半角に、半角のカタカナを全角にそろえる。
// Mirakurun はサービス名や番組名を全角で返すので、そろえないと半角で入力した語が一致しない。
// 🈟 や 🈑 のような ARIB の囲み文字は、EPGStation と同じく [新] や [字] にする。
// EPGStation から移したルールの [再] のようなキーワードが、そのまま一致するようにするためである。
// 大文字と小文字は区別したまま残す。区別するかどうかは照合の条件で決める。
func Normalize(s string) string {
	return norm.NFKC.String(replaceEnclosedMarks(s))
}

// NormalizeExtended は、詳細 (見出しから本文への対応) を、照合のための 1 つの文字列にして正規化する (設計書 3.5)。
// 見出しの順に「見出し\n本文\n」をつなぐ。
func NormalizeExtended(extended map[string]string) string {
	var b strings.Builder
	for _, heading := range slices.Sorted(maps.Keys(extended)) {
		b.WriteString(heading)
		b.WriteString("\n")
		b.WriteString(extended[heading])
		b.WriteString("\n")
	}

	return Normalize(b.String())
}
