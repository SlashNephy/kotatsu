package ts_test

import (
	"encoding/binary"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// captionPID は、合成した TS の、字幕の PID。
const captionPID = 0x0130

// 字幕の文の 8 単位符号 (ARIB STD-B24)。初めは GL が漢字、GR がひらがな、G1 が英数、G3 がマクロ。
const (
	// nihon は「日本」(漢字、GL)。
	nihon = "\x46\x7C\x4B\x5C"
	// hiraganaA は「あ」(ひらがな、GR)。
	hiraganaA = "\xA2"
	// alnumA は、G1 を GL に呼び出して (LS1) 書く「A」。GL を G0 に戻す (LS0)。
	alnumA = "\x0E\x41\x0F"
	// katakanaPa は、G1 にカタカナを指示し (ESC 0x29 0x31)、呼び出して書く「パ」。GL を G0 に戻す。
	katakanaPa = "\x1B\x29\x31\x0E\x51\x0F"
)

const (
	captionCS    = 0x0C
	captionAPR   = 0x0D
	captionSSZ   = 0x88
	captionMSZ   = 0x89
	captionNSZ   = 0x8A
	captionMgmtA = 0x00
	captionMgmtB = 0x20 // グループ B の字幕管理 (data_group_id 0x20)
	captionTextA = 0x01 // グループ A の第 1 言語の字幕文
)

// captionUnit は、データユニットを作る。
func captionUnit(parameter byte, data []byte) []byte {
	unit := make([]byte, 0, 5+len(data))
	unit = append(unit, 0x1F, parameter, byte(len(data)>>16&0xFF), byte(len(data)>>8&0xFF), byte(len(data)&0xFF))

	return append(unit, data...)
}

// captionStatementGroup は、時刻の指定のない字幕文のデータグループの内容を作る。
func captionStatementGroup(units ...[]byte) []byte {
	loop := slices.Concat(units...)

	return append([]byte{0x00, byte(len(loop) >> 16 & 0xFF), byte(len(loop) >> 8 & 0xFF), byte(len(loop) & 0xFF)}, loop...)
}

// captionManagementGroup は、日本語の 1 言語の字幕管理のデータグループの内容を作る。
func captionManagementGroup() []byte {
	return []byte{0x00, 0x01, 0x00, 'j', 'p', 'n', 0x00, 0x00, 0x00, 0x00}
}

// captionPESPackets は、data_group_id が groupID のデータグループを入れた字幕の PES を、184 バイトずつのパケットに分けて作る。
func captionPESPackets(cc *byte, pts uint64, groupID byte, group []byte) [][]byte {
	dataGroup := append([]byte{groupID << 2, 0x00, 0x00, byte(len(group) >> 8 & 0xFF), byte(len(group) & 0xFF)}, group...)
	// CRC_16 は読まないので、0 で埋める。
	dataGroup = append(dataGroup, 0x00, 0x00)
	data := append([]byte{0x80, 0xFF, 0xF0}, dataGroup...)
	header := append([]byte{0x80, 0x80, 0x05}, encodePTS(pts)...)
	pes := []byte{0x00, 0x00, 0x01, 0xBD}
	pes = binary.BigEndian.AppendUint16(pes, uint16((len(header)+len(data))&0xFFFF))
	pes = append(append(pes, header...), data...)

	pkts := [][]byte{}
	for first := true; len(pes) > 0; first = false {
		p := make([]byte, ts.PacketSize)
		for i := range p {
			p[i] = 0xFF
		}
		p[0] = 0x47
		binary.BigEndian.PutUint16(p[1:3], captionPID)
		if first {
			p[1] |= 0x40
		}
		p[3] = 0x10 | *cc&0x0F
		*cc++
		n := copy(p[4:], pes)
		pes = pes[n:]
		pkts = append(pkts, p)
	}

	return pkts
}

// captionStream は、字幕の ES (component_tag 0x30) を持つ番組の PAT と PMT を作る。
func captionStream() [][]byte {
	return captionStreamWithTag(0x30)
}

// captionStreamWithTag は、component_tag が tag の字幕の ES を持つ番組の PAT と PMT を作る。
func captionStreamWithTag(tag byte) [][]byte {
	var patCC, pmtCC byte
	streams := []esEntry{
		{streamType: 0x02, pid: videoPID, infoLength: 0, descriptors: nil},
		{streamType: 0x06, pid: captionPID, infoLength: 0, descriptors: []byte{0x52, 0x01, tag}},
	}
	pkts := psiPackets(0x0000, &patCC, patSection(0, map[uint16]uint16{programNum: pmtPID}))

	return append(pkts, psiPackets(pmtPID, &pmtCC, pmtSection(0, streams...))...)
}

// drcsUnit は、1 バイトの DRCS-1 (終端の符号 0x41) の符号 code に、2 階調で幅 width と高さ height の外字の画像 pattern を定義する。
func drcsUnit(code byte, width, height byte, pattern []byte) []byte {
	data := make([]byte, 0, 8+len(pattern))
	data = append(data, 0x01, 0x41, code, 0x01, 0x00, 0x00, width, height)

	return captionUnit(0x30, append(data, pattern...))
}

// unalignedPatternMD5 は、3 画素四方の 2 階調の外字の画像 (9 ビット、2 バイト) の、端数を除いた 1 バイト (0xAB) の MD5。
const unalignedPatternMD5 = "2408ad11f9eb830da749e2a36a29eff7"

// drcsPatternMD5 は、テストの外字の画像 (0xF0 0x0F) の MD5 を小文字の 16 進にしたもの。
const drcsPatternMD5 = "1ff0f4acb6ef4d317a2eebf40ccb26ff"

func TestCaptionScanner(t *testing.T) {
	t.Parallel()

	statement := func(body ...[]byte) []byte {
		return captionStatementGroup(captionUnit(0x20, slices.Concat(body...)))
	}
	pattern := []byte{0xF0, 0x0F}
	// DRCS-1 を G1 に指示し (ESC 0x29 0x20 0x41)、呼び出して符号 0x21 を書く。
	drcsCharacter := []byte{0x1B, 0x29, 0x20, 0x41, 0x0E, 0x21, 0x0F}
	replace := func(md5 string) (string, bool) {
		switch md5 {
		case drcsPatternMD5:
			return "★", true
		case unalignedPatternMD5:
			return "◆", true
		}

		return "", false
	}
	type group = captionGroup
	tests := []struct {
		name   string
		groups []group
		want   []ts.Caption
	}{
		{
			name: "字幕管理を読むまでの字幕文は読まない",
			groups: []group{
				{pts: 100, id: captionTextA, data: statement([]byte(hiraganaA))},
				{pts: 200, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 300, id: captionTextA, data: statement([]byte(hiraganaA))},
			},
			want: []ts.Caption{{PTS: 200, Text: "", Cleared: true}, {PTS: 300, Text: "あ", Cleared: false}},
		},
		{
			name: "字幕管理は、グループが変わったときだけ画面を消す",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 300, id: captionMgmtB, data: captionManagementGroup()},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 300, Text: "", Cleared: true}},
		},
		{
			name: "画面を消してから書いた文だけを返す",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement([]byte(hiraganaA), []byte{captionCS}, []byte(nihon))},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "日本", Cleared: true}},
		},
		{
			// 1 つ目の字幕文で G1 にカタカナを指示しても、2 つ目の字幕文の G1 は英数に戻る。
			name: "文字の指示は字幕文ごとに初めの状態から読む",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement([]byte(katakanaPa))},
				{pts: 300, id: captionTextA, data: statement([]byte(alnumA))},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "パ", Cleared: false}, {PTS: 300, Text: "Ａ", Cleared: false}},
		},
		{
			name: "小型の文字 (ルビ) は除き、中型の文字は半角にする",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement(
					[]byte(nihon), []byte{captionSSZ}, []byte(hiraganaA), []byte{captionMSZ}, []byte(alnumA), []byte{captionNSZ}, []byte(alnumA),
					[]byte{captionMSZ}, []byte(katakanaPa),
				)},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "日本AＡﾊﾟ", Cleared: false}},
		},
		{
			name: "外字は画像の MD5 で置き換え、引けなければ〓にする",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: captionStatementGroup(drcsUnit(0x21, 4, 4, pattern), captionUnit(0x20, drcsCharacter))},
				{pts: 300, id: captionTextA, data: captionStatementGroup(drcsUnit(0x21, 4, 4, []byte{0x00, 0x00}), captionUnit(0x20, drcsCharacter))},
				// 外字は、同じ字幕文で送られたものだけを使う。
				{pts: 400, id: captionTextA, data: statement(drcsCharacter)},
			},
			want: []ts.Caption{
				{PTS: 100, Text: "", Cleared: true},
				{PTS: 200, Text: "★", Cleared: false},
				{PTS: 300, Text: "〓", Cleared: false},
				{PTS: 400, Text: "", Cleared: false},
			},
		},
		{
			// 3 画素四方の外字 (9 ビット) は 2 バイトで送られる。MD5 は aribb24.js と同じく端数を除いて取り、次の外字は 2 バイトの後から読む。
			name: "画素のビット数がバイトにそろわない外字",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: captionStatementGroup(
					captionUnit(0x30, []byte{
						0x02,
						0x41, 0x21, 0x01, 0x00, 0x00, 3, 3, 0xAB, 0x80,
						0x41, 0x22, 0x01, 0x00, 0x00, 4, 4, 0xF0, 0x0F,
					}),
					captionUnit(0x20, []byte{0x1B, 0x29, 0x20, 0x41, 0x0E, 0x21, 0x22, 0x0F}),
				)},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "◆★", Cleared: false}},
		},
		{
			// TMD が 01 の字幕文は、data_unit_loop_length の前に STM (5 バイト) を持つ。
			name: "時刻を指定した字幕文",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: append([]byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00}, statement([]byte(hiraganaA))[1:]...)},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "あ", Cleared: false}},
		},
		{
			name: "グループ B の字幕文",
			groups: []group{
				{pts: 100, id: captionMgmtB, data: captionManagementGroup()},
				{pts: 200, id: 0x21, data: statement([]byte(hiraganaA))},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "あ", Cleared: false}},
		},
		{
			// 既定のマクロ 0x61 は、G1 にカタカナを指示する。マクロの集合 (G3) は、SS3 で呼び出す。
			name: "既定のマクロを読む",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement([]byte{0x1D, 0x61, 0x0E, 0x2B})},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "カ", Cleared: false}},
		},
		{
			name: "待った後の画面の消去は、文を消さない",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement([]byte(hiraganaA), []byte{0x9D, 0x20, 0x45, captionCS}, []byte(nihon))},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "あ日本", Cleared: false}},
		},
		{
			// 番組情報では 0x2141 を U+301C にするが、字幕はブラウザの字幕と同じく EUC-JP のデコーダーの対応 (U+FF5E) にする。
			name: "波ダッシュは EUC-JP のデコーダーの対応にする",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement([]byte{0x21, 0x41})},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "～", Cleared: false}},
		},
		{
			name: "第 2 言語の字幕文は読まない",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: 0x02, data: statement([]byte(hiraganaA))},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, scanCaptionGroups(t, tt.groups, replace))
		})
	}
}

// TestCaptionScannerNewlines は、字幕文の中で文字を書く縦の位置が変わったら改行を入れ、書く位置を aribb24.js と同じく
// 画素で計算することを確認する。
func TestCaptionScannerNewlines(t *testing.T) {
	t.Parallel()

	statement := func(body ...[]byte) []byte {
		return captionStatementGroup(captionUnit(0x20, slices.Concat(body...)))
	}
	type group = captionGroup
	tests := []struct {
		name   string
		groups []group
		want   []ts.Caption
	}{
		{
			name: "字幕文の中で行が変わったら改行を入れる",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement(
					// APS で 1 行目に置いて書き、APR で次の行に移って書く。ACPS (CSI 220;509 0x20 0x61) で別の行に移って書く。
					[]byte{0x1C, 0x41, 0x40, captionAPR}, []byte(hiraganaA), []byte{captionAPR}, []byte(nihon),
					[]byte{0x9B, '2', '2', '0', ';', '5', '0', '9', 0x20, 0x61}, []byte(hiraganaA),
				)},
				{pts: 300, id: captionTextA, data: statement([]byte(hiraganaA))},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "あ\n日本\nあ", Cleared: false}, {PTS: 300, Text: "あ", Cleared: false}},
		},
		{
			// 放送にある、ルビの大きさ (SSZ) にしてから APS で位置を決め、字の間を空ける字幕。APS の行は、そのときの文字の大きさで
			// 位置にするので、小型の 2 行目 (1) は、標準の大きさの 1 行目 (初めの位置) と同じ高さになり、改行を入れない。
			name: "APS の行は、そのときの文字の大きさで位置にする",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement(
					[]byte(nihon), []byte{captionSSZ, 0x1C, 0x41, 0x48, captionMSZ, 0x20, captionSSZ, 0x1C, 0x41, 0x49, captionNSZ}, []byte(nihon),
				)},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "日本 日本", Cleared: false}},
		},
		{
			// 表示領域 (SDF、CSI 80;360 0x20 0x56) を、標準の大きさの文字 2 つ分の幅にする。
			name: "行の終わりで折り返したら改行を入れる",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement(
					[]byte{0x9B, '8', '0', ';', '3', '6', '0', 0x20, 0x56}, []byte(hiraganaA+hiraganaA+hiraganaA),
				)},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "ああ\nあ", Cleared: false}},
		},
		{
			// 書く位置を決める前の SDP (CSI 100;100 0x20 0x5F) は、位置を動かさない。ACPS (CSI 0;159 0x20 0x61) は画面の上の
			// 位置で決めるので、表示領域の中では表示領域の位置の分だけ上になり、初めの位置と同じ高さになる。
			name: "ACPS は表示領域の位置からの位置にする",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement(
					[]byte{0x9B, '1', '0', '0', ';', '1', '0', '0', 0x20, 0x5F}, []byte(hiraganaA),
					[]byte{0x9B, '0', ';', '1', '5', '9', 0x20, 0x61}, []byte(nihon),
				)},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "あ日本", Cleared: false}},
		},
		{
			// 書く位置を決めた後の SDP は、画面の上の位置を保つので、表示領域が動いた分だけ、表示領域の中の位置が変わる。
			name: "書く位置を決めた後の SDP は、画面の上の位置を保つ",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement(
					[]byte{0x9B, '1', '0', '0', ';', '1', '0', '0', 0x20, 0x5F}, []byte(hiraganaA),
					[]byte{0x9B, '1', '0', '0', ';', '1', '3', '0', 0x20, 0x5F}, []byte(nihon),
				)},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "あ\n日本", Cleared: false}},
		},
		{
			// 表示領域を標準の文字 2 つ分の幅にし、小型の文字を 2 つ書くと行の終わりに届く。
			name: "ルビ (小型の文字) も書く位置を進める",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement(
					[]byte{0x9B, '8', '0', ';', '3', '6', '0', 0x20, 0x56}, []byte(hiraganaA), []byte{captionSSZ}, []byte(hiraganaA+hiraganaA),
					[]byte{captionNSZ}, []byte(hiraganaA),
				)},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "あ\nあ", Cleared: false}},
		},
		{
			// 文字の大きさ (SSM、CSI 16;16 0x20 0x57) と字間 (SHS、CSI 4 0x20 0x58) で、1 行に 4 文字になる。
			name: "文字の大きさと字間で折り返す位置が変わる",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement(
					[]byte{0x9B, '8', '0', ';', '3', '6', '0', 0x20, 0x56}, []byte{0x9B, '1', '6', ';', '1', '6', 0x20, 0x57, 0x9B, '4', 0x20, 0x58},
					[]byte(hiraganaA+hiraganaA+hiraganaA+hiraganaA+hiraganaA),
				)},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "ああああ\nあ", Cleared: false}},
		},
		{
			// 行の初めで APB を 2 回送ると、前の行の終わりへ戻り、縦の位置が変わる。
			name: "後ろへ動かして行の初めを越えたら前の行へ戻る",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement([]byte(hiraganaA), []byte{0x08, 0x08}, []byte(hiraganaA))},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "あ\nあ", Cleared: false}},
		},
		{
			// 特殊1 (SZX 0x6B) の大きさでは、aribb24.js と同じく、1 文字を書いて進めた後の位置が NaN になり、続く文字ごとに改行が入る。
			name: "特殊な大きさの文字は、2 文字目から文字ごとに改行を入れる",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement([]byte(hiraganaA), []byte{0x8B, 0x6B}, []byte(hiraganaA+hiraganaA+hiraganaA))},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "ああ\nあ\nあ", Cleared: false}},
		},
		{
			// aribb24.js は、画面を消しても、最後に書いた位置を忘れないので、別の行に書くと文の初めに改行が入る。
			name: "画面を消した後も、最後に書いた縦の位置で改行を決める",
			groups: []group{
				{pts: 100, id: captionMgmtA, data: captionManagementGroup()},
				{pts: 200, id: captionTextA, data: statement([]byte(hiraganaA), []byte{captionAPR, captionCS}, []byte(nihon))},
			},
			want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "\n日本", Cleared: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, scanCaptionGroups(t, tt.groups, nil))
		})
	}
}

// captionGroup は、字幕の PES 1 つで送るデータグループ。
type captionGroup struct {
	pts  uint64
	id   byte
	data []byte
}

// scanCaptionGroups は、groups を字幕の PES にして読み、取り出した字幕文を返す。
func scanCaptionGroups(t *testing.T, groups []captionGroup, replace func(md5 string) (string, bool)) []ts.Caption {
	t.Helper()

	var cc byte
	pkts := captionStream()
	for _, g := range groups {
		pkts = append(pkts, captionPESPackets(&cc, g.pts, g.id, g.data)...)
	}
	scanner := ts.NewCaptionScanner(programNum, replace)
	_, err := scanner.Write(join(pkts))
	require.NoError(t, err)

	return scanner.Captions()
}

// TestCaptionScannerAssemblesPES は、複数のパケットにまたがる字幕の PES を組み立て、パケットが欠けた PES は読まないことを確認する。
func TestCaptionScannerAssemblesPES(t *testing.T) {
	t.Parallel()

	// 3 つ以上のパケットにまたがる長さの字幕文 (ひらがなの「あ」を 600 文字)。
	long := slices.Repeat([]byte(hiraganaA), 600)
	// 既定の表示領域 (横 960 画素) と文字の大きさ (36 画素と字間 4 画素) では、24 文字で折り返すので、改行が入る。
	want := strings.TrimSuffix(strings.Repeat(strings.Repeat("あ", 24)+"\n", 25), "\n")
	var cc byte
	pkts := captionStream()
	pkts = append(pkts, captionPESPackets(&cc, 100, captionMgmtA, captionManagementGroup())...)
	pkts = append(pkts, captionPESPackets(&cc, 200, captionTextA, captionStatementGroup(captionUnit(0x20, long)))...)
	broken := captionPESPackets(&cc, 300, captionTextA, captionStatementGroup(captionUnit(0x20, long)))
	require.Greater(t, len(broken), 2)
	// 2 つ目のパケットが欠けた PES。
	pkts = append(pkts, broken[0])
	pkts = append(pkts, broken[2:]...)

	scanner := ts.NewCaptionScanner(programNum, nil)
	// パケットの区切りとそろわない大きさで書いても、同じに読める。
	data := join(pkts)
	for len(data) > 0 {
		n := min(len(data), 100)
		_, err := scanner.Write(data[:n])
		require.NoError(t, err)
		data = data[n:]
	}
	assert.Equal(t, []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: want, Cleared: false}}, scanner.Captions())
}

// TestCaptionScannerComponentTagFallback は、component_tag 0x30 の字幕がなければ、0x31〜0x37 の最初の字幕を読み、
// 字幕でない component_tag の ES は読まないことを確認する。
func TestCaptionScannerComponentTagFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tag  byte
		want []ts.Caption
	}{
		{name: "0x31 の字幕", tag: 0x31, want: []ts.Caption{{PTS: 100, Text: "", Cleared: true}, {PTS: 200, Text: "あ", Cleared: false}}},
		{name: "文字スーパー (0x38)", tag: 0x38, want: []ts.Caption{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cc byte
			pkts := captionStreamWithTag(tt.tag)
			pkts = append(pkts, captionPESPackets(&cc, 100, captionMgmtA, captionManagementGroup())...)
			pkts = append(pkts, captionPESPackets(&cc, 200, captionTextA, captionStatementGroup(captionUnit(0x20, []byte(hiraganaA))))...)
			scanner := ts.NewCaptionScanner(programNum, nil)
			_, err := scanner.Write(join(pkts))
			require.NoError(t, err)
			assert.Equal(t, tt.want, scanner.Captions())
		})
	}
}
