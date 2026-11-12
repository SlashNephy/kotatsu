package ts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// TestDecodeARIBString は、ARIB STD-B24 の 8 単位符号の文字列を、チューナーサーバーの番組情報と同じ書き方で読むことを確かめる (#1500)。
func TestDecodeARIBString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{
			// 初めの状態では、GL に漢字系 (G0)、GR にひらがな (G2) を呼び出している。
			name: "漢字系とひらがな",
			in:   []byte{0x25, 0x46, 0x25, 0x39, 0x25, 0x48, 0xCE, 0xD0, 0xF3},
			want: "テストのばん",
		},
		{
			// 標準の大きさの英数は、チューナーサーバーと同じく全角にする。空白も全角にする。
			name: "英数は標準の大きさで全角",
			in:   []byte{0x0E, 0x41, 0x42, 0x20, 0x31, 0x0F, 0x25, 0x46},
			want: "ＡＢ　１テ",
		},
		{
			name: "中型の英数と空白は半角",
			in:   []byte{0x89, 0x0E, 0x41, 0x42, 0x20, 0x31, 0x8A, 0x32},
			want: "AB 1２",
		},
		{
			// 0x2141 は、EUC-JP の復号器が U+FF5E にするが、チューナーサーバーにそろえて U+301C にする。
			name: "波ダッシュは JIS X 0208 の対応",
			in:   []byte{0x21, 0x41, 0x25, 0x46, 0x21, 0x41},
			want: "〜テ〜",
		},
		{
			// 0x213D は、EUC-JP の復号器が U+2015 にするが、チューナーサーバーにそろえて U+2014 にする。
			name: "ダッシュは JIS X 0208 の対応",
			in:   []byte{0x21, 0x3D, 0x21, 0x3D},
			want: "——",
		},
		{
			// 標準の大きさの英数の引用符は、チューナーサーバーにそろえて右の引用符にする。
			name: "英数の引用符",
			in:   []byte{0x0E, 0x22, 0x41, 0x22, 0x27},
			want: "”Ａ”’",
		},
		{
			name: "追加記号",
			in:   []byte{0x7A, 0x6A, 0x7A, 0x56},
			want: "🈞🈑",
		},
		{
			// G1 にカタカナを指示して GL に呼び出し、SS2 で次の 1 文字だけ G2 (ひらがな) にする。
			name: "集合の指示とシングルシフト",
			in:   []byte{0x1B, 0x29, 0x31, 0x0E, 0x22, 0x19, 0x22, 0x22},
			want: "アあア",
		},
		{
			// LS3R で GR に G3 (カタカナ) を呼び出す。
			name: "GR への呼び出し",
			in:   []byte{0xA2, 0x1B, 0x7C, 0xA2},
			want: "あア",
		},
		{
			// 色の指定は飛ばし、APR は改行にする。
			name: "制御符号",
			in:   []byte{0x90, 0x41, 0xA2, 0x0D, 0x9B, 0x30, 0x20, 0x53, 0xA4},
			want: "あ\nい",
		},
		{
			// 外字 (DRCS) は読まずに飛ばす。
			name: "外字は飛ばす",
			in:   []byte{0x1B, 0x28, 0x20, 0x41, 0x21, 0x0F, 0xA2},
			want: "あ",
		},
		{
			name: "途中で切れた 2 バイトの文字は読まない",
			in:   []byte{0xA2, 0x25},
			want: "あ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ts.DecodeARIBString(tt.in))
		})
	}
}
