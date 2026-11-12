package ts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// テストの文字列の、ARIB STD-B24 の 8 単位符号。漢字系 (GL) のカタカナと、ひらがな (GR) で書く。
// テスト、＃６ (話数)、アニメ、ない、よう。
const (
	textTest  = "\x25\x46\x25\x39\x25\x48"
	textNo6   = "\x21\x74\x23\x36"
	textAnime = "\x25\x22\x25\x4B\x25\x61"
	textNai   = "\xCA\xA4"
	textYou   = "\xE8\xA6"
)

// lengthByte は、記述子の長さの 1 バイトを返す。
func lengthByte(n int) byte {
	if n < 0 || n > 0xFF {
		panic("the field is too long")
	}

	return byte(n)
}

// shortEventDescriptor は、短形式イベント記述子を作る。
func shortEventDescriptor(name, text string) []byte {
	body := append([]byte("jpn"), lengthByte(len(name)))
	body = append(body, name...)
	body = append(body, lengthByte(len(text)))
	body = append(body, text...)

	return append([]byte{0x4D, lengthByte(len(body))}, body...)
}

// extendedItem は、拡張形式イベント記述子の項目。heading が空なら、前の項目の続き。
type extendedItem struct {
	heading string
	text    string
}

// extendedEventDescriptor は、拡張形式イベント記述子を作る。
func extendedEventDescriptor(number byte, items ...extendedItem) []byte {
	loop := []byte{}
	for _, item := range items {
		loop = append(loop, lengthByte(len(item.heading)))
		loop = append(loop, item.heading...)
		loop = append(loop, lengthByte(len(item.text)))
		loop = append(loop, item.text...)
	}
	body := append([]byte{number<<4 | 0x01}, "jpn"...)
	body = append(body, lengthByte(len(loop)))
	body = append(body, loop...)
	// text_length は 0。
	body = append(body, 0)

	return append([]byte{0x4E, lengthByte(len(body))}, body...)
}

// seriesDescriptor は、シリーズ記述子を作る。
func seriesDescriptor(repeat, episode byte) []byte {
	body := []byte{0x00, 0x01, repeat<<4 | 0x01, 0x00, 0x00, episode >> 4, episode << 4, 0x00}

	return append([]byte{0xD5, lengthByte(len(body))}, body...)
}

// TestAnalyzerEventInfo は、EIT[p/f] の放送中と次の番組の、番組名、概要、詳細、シリーズの記述子を読むことを確かめる (#1500)。
// 詳細の項目は、見出しのない項目で、記述子をまたいで続く。
func TestAnalyzerEventInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sections [][]byte
		eventID  uint16
		want     *ts.EventInfo
	}{
		{name: "まだ読んでいない", sections: nil, eventID: 101, want: nil},
		{
			name: "次の番組の番組名、概要、詳細、シリーズ",
			sections: [][]byte{
				eitSection(0, 100, shortEventDescriptor(textTest, textNai)),
				eitSection(1, 101,
					shortEventDescriptor(textAnime, textNo6),
					extendedEventDescriptor(0, extendedItem{heading: textTest, text: textAnime}),
					extendedEventDescriptor(1, extendedItem{heading: "", text: textYou}, extendedItem{heading: textNai, text: textTest}),
					seriesDescriptor(1, 6),
				),
			},
			eventID: 101,
			want: &ts.EventInfo{
				Name: "アニメ", Description: "＃６",
				Extended: []*ts.ExtendedItem{{Heading: "テスト", Text: "アニメよう"}, {Heading: "ない", Text: "テスト"}},
				Series:   &ts.EventSeries{Episode: 6, Repeat: 1},
			},
		},
		{
			name:     "記述子がなければ空",
			sections: [][]byte{eitSection(0, 100)},
			eventID:  100,
			want:     &ts.EventInfo{Name: "", Description: "", Extended: []*ts.ExtendedItem{}, Series: nil},
		},
		{
			name: "最後に読んだものにする",
			sections: [][]byte{
				eitSection(1, 101, shortEventDescriptor(textAnime, "")),
				eitSection(0, 101, shortEventDescriptor(textAnime, textNo6)),
			},
			eventID: 101,
			want:    &ts.EventInfo{Name: "アニメ", Description: "＃６", Extended: []*ts.ExtendedItem{}, Series: nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := ts.NewAnalyzer(nil)
			cc := byte(0)
			for _, p := range psiPackets(0x12, &cc, tt.sections...) {
				_, err := analyzer.Write(p)
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, analyzer.EventInfo(testServiceID, tt.eventID))
		})
	}
}
