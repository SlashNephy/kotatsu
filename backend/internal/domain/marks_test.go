package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

func TestMarks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "no marks", in: "ゆるキャン△ 第1話", want: []string{}},
		{name: "enclosed characters", in: "🈟ゆるキャン△ 第1話🈑🅊", want: []string{"新", "字", "HV"}},
		{name: "enclosed characters without NFKC decomposition", in: "番組名🆞🆠", want: []string{"4K", "5.1"}},
		// 局によっては括弧で書くので、同じ囲み文字とみなす。全角の括弧も同じ。
		{name: "bracketed marks", in: "[再]番組名【字】（二）", want: []string{"再", "字", "二"}},
		{name: "bracketed words are mapped to the enclosed labels", in: "番組名[字幕][吹替][無料]", want: []string{"字", "吹", "無"}},
		{name: "bracketed alphanumeric marks", in: "[HV]番組名[4K][5.1]", want: []string{"HV", "4K", "5.1"}},
		{name: "duplicates are removed", in: "🈞[再]番組名", want: []string{"再"}},
		// 番組名の文字の「新」や、囲み文字でない括弧は、囲み文字ではない。
		{name: "plain characters are not marks", in: "新番組 再放送 [前編] [劇場版]", want: []string{}},
		{name: "enclosed characters not in the table are not marks", in: "番組名🄰🈲", want: []string{}},
		// 🈀 (ほか) は番組名の一部。
		{name: "hoka is not a mark", in: "出演者🈀", want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, domain.Marks(tt.in))
		})
	}
}
