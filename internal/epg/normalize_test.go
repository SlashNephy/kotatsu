package epg_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/epg"
)

func TestNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "full-width service name", in: "ＴＯＫＹＯ　ＭＸ１", want: "TOKYO MX1"},
		{name: "full-width digits and symbols", in: "第１話「ａｂｃ！」", want: "第1話「abc!」"},
		{name: "half-width katakana", in: "ｱﾆﾒ ﾆｭｰｽ", want: "アニメ ニュース"},
		// ARIB の囲み文字は、EPGStation と同じく括弧の書き方にする。「[新]」でも「新」でも検索に一致する。
		{name: "enclosed characters used in EPG", in: "🈟番組名🈑", want: "[新]番組名[字]"},
		{name: "enclosed characters without NFKC decomposition", in: "番組名🆞⚿", want: "番組名[4K][鍵]"},
		{name: "enclosed alphabets", in: "番組名🅊🅂", want: "番組名[HV][S]"},
		// 🈀 (ほか) は番組名の一部なので、括弧にしない。
		{name: "hoka is part of the name", in: "出演者🈀", want: "出演者ほか"},
		{name: "keeps letter case", in: "Ｎｅｗｓ", want: "News"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, epg.Normalize(tt.in))
		})
	}
}

func TestNormalizeExtended(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		extended map[string]string
		want     string
	}{
		{name: "no extended", extended: map[string]string{}, want: ""},
		{name: "one heading", extended: map[string]string{"出演者": "ＡＢＣ　太郎"}, want: "出演者\nABC 太郎\n"},
		// 見出しの順に並べる。上流の JSON のキーの順には頼らない。
		{name: "headings in order", extended: map[string]string{"番組内容": "ｘ", "出演者": "ｙ"}, want: "出演者\ny\n番組内容\nx\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, epg.NormalizeExtended(tt.extended))
		})
	}
}
