package epg

// EnclosedGroup は、囲み文字を画面で選ぶときのまとまり。意味は Unicode の符号表の注記による。
type EnclosedGroup string

const (
	// EnclosedGroupPopular は、よく使う囲み文字 (新番組、最終回、再放送、初回放送)。
	EnclosedGroupPopular EnclosedGroup = "popular"
	// EnclosedGroupProgram は、番組の種類 (生放送、前編、映画、ニュースなど)。
	EnclosedGroupProgram EnclosedGroup = "program"
	// EnclosedGroupAccessibility は、字幕、解説、言語 (手話、吹替、二か国語など)。
	EnclosedGroupAccessibility EnclosedGroup = "accessibility"
	// EnclosedGroupVideo は、映像 (ハイビジョン、4K、HDR など)。
	EnclosedGroupVideo EnclosedGroup = "video"
	// EnclosedGroupAudio は、音声 (ステレオ、5.1 など)。
	EnclosedGroupAudio EnclosedGroup = "audio"
	// EnclosedGroupData は、データ放送や連携 (データ放送、双方向、ハイブリッドキャストなど)。
	EnclosedGroupData EnclosedGroup = "data"
	// EnclosedGroupAccess は、料金と視聴制限 (無料放送、有料放送、ペイ・パー・ビュー、視聴制限)。
	EnclosedGroupAccess EnclosedGroup = "access"
)

// EnclosedGroups は、まとまりを、画面で並べる順に返す。
func EnclosedGroups() []EnclosedGroup {
	return []EnclosedGroup{
		EnclosedGroupPopular,
		EnclosedGroupProgram,
		EnclosedGroupAccessibility,
		EnclosedGroupVideo,
		EnclosedGroupAudio,
		EnclosedGroupData,
		EnclosedGroupAccess,
	}
}

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
	c := func(r rune, label string, group EnclosedGroup) EnclosedCharacter {
		return EnclosedCharacter{Character: r, Label: label, Group: group}
	}

	// 出典は、Unicode の符号表が示す ARIB の規格 (設計書 4.2)。
	// 鍵 (⚿) は Miscellaneous symbol from ARIB STD B24、漢字と仮名は Squared ideographs and kana from ARIB STD B24、
	// 英字のうち B、N、P、S、W は各文字の注記に ARIB STD B24 とあり、HV、MV、SD、SS、PPV は放送の用語の注記がある。
	// 2K や 4K などは Squared Latin letter sequences from ARIB STD B62。
	return [enclosedCount]EnclosedCharacter{
		c('\U0001F21F', "新", EnclosedGroupPopular),
		c('\U0001F221', "終", EnclosedGroupPopular),
		c('\U0001F21E', "再", EnclosedGroupPopular),
		c('\U0001F220', "初", EnclosedGroupPopular),
		c('\U0001F222', "生", EnclosedGroupProgram),
		c('\U0001F21C', "前", EnclosedGroupProgram),
		c('\U0001F21D', "後", EnclosedGroupProgram),
		c('\U0001F219', "映", EnclosedGroupProgram),
		c('\U0001F226', "演", EnclosedGroupProgram),
		c('\U0001F217', "天", EnclosedGroupProgram),
		c('\U0001F218', "交", EnclosedGroupProgram),
		c('\U0001F223', "販", EnclosedGroupProgram),
		c('\U0001F13D', "N", EnclosedGroupProgram),
		c('\U0001F211', "字", EnclosedGroupAccessibility),
		c('\U0001F216', "解", EnclosedGroupAccessibility),
		c('\U0001F210', "手", EnclosedGroupAccessibility),
		c('\U0001F225', "吹", EnclosedGroupAccessibility),
		c('\U0001F224', "声", EnclosedGroupAccessibility),
		c('\U0001F214', "二", EnclosedGroupAccessibility),
		c('\U0001F215', "多", EnclosedGroupAccessibility),
		c('\U0001F14A', "HV", EnclosedGroupVideo),
		c('\U0001F14C', "SD", EnclosedGroupVideo),
		c('\U0001F146', "W", EnclosedGroupVideo),
		c('\U0001F13F', "P", EnclosedGroupVideo),
		c('\U0001F14B', "MV", EnclosedGroupVideo),
		c('\U0001F19D', "2K", EnclosedGroupVideo),
		c('\U0001F19E', "4K", EnclosedGroupVideo),
		c('\U0001F19F', "8K", EnclosedGroupVideo),
		c('\U0001F19B', "3D", EnclosedGroupVideo),
		c('\U0001F1A7', "HDR", EnclosedGroupVideo),
		c('\U0001F1AA', "SHV", EnclosedGroupVideo),
		c('\U0001F1AB', "UHD", EnclosedGroupVideo),
		c('\U0001F1A3', "60P", EnclosedGroupVideo),
		c('\U0001F1A4', "120P", EnclosedGroupVideo),
		c('\U0001F142', "S", EnclosedGroupAudio),
		c('\U0001F14D', "SS", EnclosedGroupAudio),
		c('\U0001F131', "B", EnclosedGroupAudio),
		c('\U0001F1A0', "5.1", EnclosedGroupAudio),
		c('\U0001F1A1', "7.1", EnclosedGroupAudio),
		c('\U0001F1A2', "22.2", EnclosedGroupAudio),
		c('\U0001F1A8', "Hi-Res", EnclosedGroupAudio),
		c('\U0001F1A9', "Lossless", EnclosedGroupAudio),
		c('\U0001F213', "デ", EnclosedGroupData),
		c('\U0001F212', "双", EnclosedGroupData),
		c('\U0001F1A6', "HC", EnclosedGroupData),
		c('\U0001F1A5', "d", EnclosedGroupData),
		c('\U0001F19C', "2ndScr", EnclosedGroupData),
		c('\U0001F1AC', "VOD", EnclosedGroupData),
		c('\U0001F21A', "無", EnclosedGroupAccess),
		c('\U0001F21B', "料", EnclosedGroupAccess),
		c('\U0001F14E', "PPV", EnclosedGroupAccess),
		c('\u26BF', "鍵", EnclosedGroupAccess),
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
