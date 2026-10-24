package rule_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

// yurucamp は、記号を取り除いた後の番組名。
const (
	yurucamp        = "ゆるキャン△"
	yurucampSeason3 = "ゆるキャン△ SEASON3"
)

func TestTitleKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "ARIB enclosed marks", in: "🈟ゆるキャン△ 第1話🈑", want: "ゆるキャン△ 第1話"},
		{name: "bracketed marks", in: "[新]ゆるキャン△[字][デ]", want: yurucamp},
		{name: "lenticular brackets", in: "【再】ゆるキャン△　【終】", want: yurucamp},
		{name: "full-width letters and spaces", in: "ゆるキャン△　ＳＥＡＳＯＮ３", want: yurucampSeason3},
		{name: "episode numbers are kept", in: "ゆるキャン△ #2「ふじさん」[字]", want: "ゆるキャン△ #2「ふじさん」"},
		{name: "a 新 that is part of the title is kept", in: "新世界より", want: "新世界より"},
		{name: "unknown bracketed words are kept", in: "[特別編]ゆるキャン△", want: "[特別編]ゆるキャン△"},
		{name: "same key from different notations", in: "🈡ゆるキャン△　ＳＥＡＳＯＮ３ 第１話", want: "ゆるキャン△ SEASON3 第1話"},
		{name: "enclosed marks between words separate them like bracketed marks", in: "ゆるキャン△🈟SEASON3", want: yurucampSeason3},
		{name: "bracketed marks between words", in: "ゆるキャン△[新]SEASON3", want: yurucampSeason3},
		{name: "theatrical version is a different program", in: "[劇場版]ゆるキャン△", want: "[劇場版]ゆるキャン△"},
		{name: "first and second halves are different programs", in: "映画「タイタニック」[前]", want: "映画「タイタニック」[前]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, rule.TitleKey(tt.in))
		})
	}
}
