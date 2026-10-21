// Package halfwidth は、EPG の文字列の全角の英数字を、人が読む表示のために半角にする。
package halfwidth

import "strings"

// Convert は、全角の英数字と、英数字に添えて使う記号と、全角の空白を半角にする。
// 画面の表示で使う半角化 (web/src/futon/halfWidth.ts) と同じ範囲にする。「！」「？」「（）」「～」は、和文の約物として使うので残す。
func Convert(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '　':
			return ' '
		case r >= 'Ａ' && r <= 'Ｚ', r >= 'ａ' && r <= 'ｚ', r >= '０' && r <= '９', strings.ContainsRune("＃＄％＆＊＋－．／：；＝＠＿", r):
			return r - 0xfee0
		default:
			return r
		}
	}, s)
}
