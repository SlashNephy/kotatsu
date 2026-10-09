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
		// ARIB の囲み文字は、NFKC で中の文字に分解される。「新」や「字」での検索に一致するようになる。
		{name: "enclosed characters used in EPG", in: "🈟番組名🈑", want: "新番組名字"},
		{name: "keeps letter case", in: "Ｎｅｗｓ", want: "News"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, epg.Normalize(tt.in))
		})
	}
}
