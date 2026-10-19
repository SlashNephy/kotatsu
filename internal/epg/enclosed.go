package epg

// EnclosedGroup は、囲み文字を画面で選ぶときのまとまり。
type EnclosedGroup string

const (
	// EnclosedGroupIdeographic は、漢字と仮名の囲み文字。
	EnclosedGroupIdeographic EnclosedGroup = "ideographic"
	// EnclosedGroupAlphanumeric は、英数字の囲み文字。
	EnclosedGroupAlphanumeric EnclosedGroup = "alphanumeric"
)

// EnclosedCharacter は、番組名に付く ARIB の囲み文字 (設計書 4.2)。
type EnclosedCharacter struct {
	Character rune
	// Label は、囲み文字の中身。正規化では [Label] にし、ルールの条件ではこれで指定する。
	Label string
	Group EnclosedGroup
}

// enclosedCount は、表の囲み文字の数。
const enclosedCount = 52

// EnclosedCharacters は、変換し、取り出し、条件で選べる囲み文字の表 (設計書 4.2)。画面で並べる順に並べる。
// この表だけが囲み文字を決める。web の表 (enclosedCharacters.gen.ts) は、mise run generate でここから作る。
// 表にない囲み文字 (🈀 や ㊙ など) は、番組名の一部として NFKC で中の文字にするだけにする。
func EnclosedCharacters() []*EnclosedCharacter {
	table := enclosedTable()
	characters := make([]*EnclosedCharacter, 0, len(table))
	for i := range table {
		characters = append(characters, &table[i])
	}

	return characters
}

// enclosedTable は、表を値の配列で返す。照合では番組ごとに表を引くので、ヒープに確保しないよう配列にする。
func enclosedTable() [enclosedCount]EnclosedCharacter {
	ideographic := func(r rune, label string) EnclosedCharacter {
		return EnclosedCharacter{Character: r, Label: label, Group: EnclosedGroupIdeographic}
	}
	alphanumeric := func(r rune, label string) EnclosedCharacter {
		return EnclosedCharacter{Character: r, Label: label, Group: EnclosedGroupAlphanumeric}
	}

	return [enclosedCount]EnclosedCharacter{
		// ARIB STD-B24 (Unicode の Squared ideographs and kana from ARIB STD B24 と、Miscellaneous symbol from ARIB STD B24)
		ideographic('\U0001F21F', "新"),
		ideographic('\U0001F221', "終"),
		ideographic('\U0001F21E', "再"),
		ideographic('\U0001F220', "初"),
		ideographic('\U0001F222', "生"),
		ideographic('\U0001F21C', "前"),
		ideographic('\U0001F21D', "後"),
		ideographic('\U0001F211', "字"),
		ideographic('\U0001F214', "二"),
		ideographic('\U0001F215', "多"),
		ideographic('\U0001F216', "解"),
		ideographic('\U0001F212', "双"),
		ideographic('\U0001F213', "デ"),
		ideographic('\U0001F210', "手"),
		ideographic('\U0001F225', "吹"),
		ideographic('\U0001F224', "声"),
		ideographic('\U0001F226', "演"),
		ideographic('\U0001F21A', "無"),
		ideographic('\U0001F21B', "料"),
		ideographic('\U0001F219', "映"),
		ideographic('\U0001F217', "天"),
		ideographic('\U0001F218', "交"),
		ideographic('\U0001F223', "販"),
		ideographic('⚿', "鍵"),
		// ARIB STD-B24 (Unicode の Squared Latin letters のうち、放送の注記があるもの)
		alphanumeric('\U0001F142', "S"),
		alphanumeric('\U0001F14D', "SS"),
		alphanumeric('\U0001F131', "B"),
		alphanumeric('\U0001F13D', "N"),
		alphanumeric('\U0001F13F', "P"),
		alphanumeric('\U0001F146', "W"),
		alphanumeric('\U0001F14A', "HV"),
		alphanumeric('\U0001F14B', "MV"),
		alphanumeric('\U0001F14C', "SD"),
		alphanumeric('\U0001F14E', "PPV"),
		// ARIB STD-B62 (Unicode の Squared Latin letter sequences from ARIB STD B62)
		alphanumeric('\U0001F19D', "2K"),
		alphanumeric('\U0001F19E', "4K"),
		alphanumeric('\U0001F19F', "8K"),
		alphanumeric('\U0001F19B', "3D"),
		alphanumeric('\U0001F1A7', "HDR"),
		alphanumeric('\U0001F1AA', "SHV"),
		alphanumeric('\U0001F1AB', "UHD"),
		alphanumeric('\U0001F1A3', "60P"),
		alphanumeric('\U0001F1A4', "120P"),
		alphanumeric('\U0001F1A6', "HC"),
		alphanumeric('\U0001F1A0', "5.1"),
		alphanumeric('\U0001F1A1', "7.1"),
		alphanumeric('\U0001F1A2', "22.2"),
		alphanumeric('\U0001F1A8', "Hi-Res"),
		alphanumeric('\U0001F1A9', "Lossless"),
		alphanumeric('\U0001F1AC', "VOD"),
		alphanumeric('\U0001F19C', "2ndScr"),
		alphanumeric('\U0001F1A5', "d"),
	}
}

// IsEnclosedLabel は、label が表の囲み文字の中身かどうかを返す。
func IsEnclosedLabel(label string) bool {
	for _, c := range enclosedTable() {
		if c.Label == label {
			return true
		}
	}

	return false
}

// mayBeEnclosed は、r が表の囲み文字になりうるかを、表を引かずに安く判定する。
func mayBeEnclosed(r rune) bool {
	return (r >= 0x1F100 && r <= 0x1F2FF) || r == 0x26BF
}

// enclosedLabel は、r が表の囲み文字なら、その中身を返す。
func enclosedLabel(r rune) (string, bool) {
	if !mayBeEnclosed(r) {
		return "", false
	}
	for _, c := range enclosedTable() {
		if c.Character == r {
			return c.Label, true
		}
	}

	return "", false
}
