package halfwidth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/halfwidth"
)

// TestConvert は、画面の半角化 (frontend/src/futon/halfWidth.test.ts) と同じ例で、同じ結果になることを確かめる。
func TestConvert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "全角の英数字を半角にする", input: "ＮＨＫ総合１", want: "NHK総合1"},
		{name: "英数字に添える記号と、全角の空白も半角にする", input: "相棒１４　＃４", want: "相棒14 #4"},
		{name: "時刻や割合の記号", input: "１２：３０　５０％　Ａ＆Ｂ", want: "12:30 50% A&B"},
		{name: "和文の約物は全角のまま", input: "参戦！（２８）～？", want: "参戦！（28）～？"},
		{name: "半角の文字はそのまま", input: "NHK 1", want: "NHK 1"},
		{name: "仮名や漢字はそのまま", input: "ニュース・天気", want: "ニュース・天気"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, halfwidth.Convert(tt.input))
		})
	}
}
