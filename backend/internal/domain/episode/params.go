// Package episode は、番組名と概要から放送の「回」を読み、局をまたいで同じ回を束ねる (設計書 4.4、#1498)。
//
// 回の読み方と同じ回の判定は、限られた局の書き方を基に作った規則で、DB や外部サービスに触れない純粋な関数にする。
// 取り違えて録り逃すより 1 本多く録るほうを選ぶので、迷う組は同じ回とみなさない。
package episode

// 次の値は、Kotatsu の番組表で分布を測る前に置いた初期値である (付録 A)。測って直すまでは、推測の値として扱う。
const (
	// MaxFrameNameLength は、番組名の先頭の <…> を枠の名前として除く、中身の最長 (文字数)。
	// 「<アニメイズム>」「<ノイタミナ>」のような枠の名前を除き、<…> で始まる作品名そのものは消さない長さにする。
	MaxFrameNameLength = 10
	// MaxNumberDigits は、期と話数で、数字として読む最大の桁数。長寿番組の 4 桁の回を読み、5 桁以上の品番などを読まない。
	// 4 桁の年 (2026 など) は、話数の印 (#、第…話) の後ろにしか読まないので、取り違えにくい。
	MaxNumberDigits = 4
	// MaxSeasonKanjiLength は、期として読む漢数字の最長。「第十二期」までを読む。
	MaxSeasonKanjiLength = 3
	// MaxEpisodeKanjiLength は、概要で話数として読む漢数字の最長。十と百の位取りで 999 を書く「九百九十九」の長さに合わせる。
	MaxEpisodeKanjiLength = 5
	// MinArcWordLength と MaxArcWordLength は、番組名の末尾の「…編」を編として読む語の長さ (「編」を含む文字数) の範囲。
	// 「前編」「後編」(2 文字) から「刀鍛冶の里編」程度までを読み、作品名の一部まで取り込まない。
	MinArcWordLength = 2
	MaxArcWordLength = 10
	// MaxQuotedSummarySubtitleLength は、概要で、話数の後ろの鍵括弧を副題として読む最長。台詞の引用のような地の文を読まない。
	MaxQuotedSummarySubtitleLength = 40
	// MaxPlainSummarySubtitleLength は、概要の 2 回目の読み方で、行の残りを副題とする最長。鍵括弧の副題より短くする。
	MaxPlainSummarySubtitleLength = 25
	// MinFingerprintLength は、概要の指紋に使う、揃えた概要の最短の長さ。短い概要はどの回にも当たりやすいので使わない。
	MinFingerprintLength = 30
	// MinCoreLength は、芯で緩めるときの、芯の最短の長さ。「最終回」(3 文字) のような短い語で別の番組に紛れないよう、それより長くする。
	MinCoreLength = 4
)
