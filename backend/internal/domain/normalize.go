package domain

import (
	"maps"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Normalize は、検索用のカラムに入れる文字列を作る (設計書 3.5)。
// NFKC で全角の英数字と記号を半角に、半角のカタカナを全角にそろえる。
// Mirakurun はサービス名や番組名を全角で返すので、そろえないと半角で入力した語が一致しない。
// 🈟 や 🈑 のような ARIB の囲み文字は、EPGStation と同じく [新] や [字] にする。
// EPGStation から移したルールの [再] のようなキーワードが、そのまま一致するようにするためである。
// 波ダッシュ (〜) は ~ に、ダッシュの類 (‐ ‑ ‒ – — ― −) は - にそろえる (#2041)。NFKC は全角チルダ (～) を ~ に、
// 全角のハイフンマイナス (－) を - にするが、これらは残すので、局や回で書き方が違うと、同じ作品名でも一致しなかった。
// 長音符 (ー) は言葉の一部なので、そろえない。
// 大文字と小文字は区別したまま残す。区別するかどうかは照合の条件で決める。
func Normalize(s string) string {
	return strings.Map(foldDash, norm.NFKC.String(replaceEnclosedMarks(s)))
}

// foldDash は、波ダッシュを ~ に、ダッシュの類を - にする。ほかの文字は変えない。
func foldDash(r rune) rune {
	switch r {
	case '\u301C': // 〜 WAVE DASH
		return '~'
	case '\u2010', // ‐ HYPHEN (NFKC は ‑ NON-BREAKING HYPHEN をこれにする)
		'\u2012', // ‒ FIGURE DASH
		'\u2013', // – EN DASH
		'\u2014', // — EM DASH
		'\u2015', // ― HORIZONTAL BAR
		'\u2212': // − MINUS SIGN
		return '-'
	default:
		return r
	}
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
