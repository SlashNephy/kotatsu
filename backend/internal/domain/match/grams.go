package match

import (
	"slices"
)

// nameGramMark は、番組名から作った gram の先頭に付けるマーク。番組名と概要の両方から作る gram と見分ける。
// 番組の文字列は NFKC で正規化してあり、制御文字は出てこないので、本文の gram と重ならない。
const nameGramMark = "\x01"

// SearchGrams は、番組の検索のインデックスに入れる gram を返す (#966)。番組名だけの gram (マークを付ける) と、番組名と概要と詳細の gram を入れる。
// nameNormalized、descriptionNormalized、extendedNormalized は、正規化した番組名、概要、詳細 (domain.Normalize、domain.NormalizeExtended)。
// 照合関数と同じく、大文字と小文字を畳んでから作る。
// gram は、1 文字と、続く 2 文字。キーワードが番組の項目に含まれれば、キーワードの gram (KeywordGrams) は、その項目の gram にすべて含まれる。
// 項目をつないだ文字列からは作らない。つなぎ目をまたぐ gram は、どのキーワードの照合にも使わないからである。
func SearchGrams(nameNormalized, descriptionNormalized, extendedNormalized string) []string {
	set := map[string]struct{}{}
	for _, gram := range textGrams(foldCase(nameNormalized)) {
		set[nameGramMark+gram] = struct{}{}
		set[gram] = struct{}{}
	}
	for _, text := range []string{descriptionNormalized, extendedNormalized} {
		for _, gram := range textGrams(foldCase(text)) {
			set[gram] = struct{}{}
		}
	}
	grams := make([]string, 0, len(set))
	for gram := range set {
		grams = append(grams, gram)
	}
	slices.Sort(grams)

	return grams
}

// textGrams は、文字列の 1 文字と、続く 2 文字の gram を返す。重なりは除かない。
func textGrams(text string) []string {
	runes := []rune(text)
	grams := make([]string, 0, 2*len(runes))
	for i, r := range runes {
		grams = append(grams, string(r))
		if i+1 < len(runes) {
			grams = append(grams, string(runes[i:i+2]))
		}
	}

	return grams
}

// RequiredGrams は、番組が条件のキーワードに一致するなら、番組の gram (SearchGrams) が必ず含む gram を返す (#966)。
// 絞り込めるのは、すべてのキーワードに一致する条件 (KeywordMatchAll) のキーワードである。番組名だけを見るキーワードは、番組名の gram で絞る。
// 正規表現は、必ず含む文字列が 1 つに決まるときだけ使う。
// 絞り込める gram がなければ、空を返す。
func RequiredGrams(c *Conditions) []string {
	if c.KeywordMatch == KeywordMatchAny {
		return []string{}
	}
	set := map[string]struct{}{}
	for _, keyword := range c.Keywords {
		literal, ok := requiredLiteral(keyword)
		if !ok {
			continue
		}
		mark := ""
		if len(keyword.Fields) == 1 && keyword.Fields[0] == FieldName {
			mark = nameGramMark
		}
		for _, gram := range KeywordGrams(literal) {
			set[mark+gram] = struct{}{}
		}
	}
	grams := make([]string, 0, len(set))
	for gram := range set {
		grams = append(grams, gram)
	}
	slices.Sort(grams)

	return grams
}

// requiredLiteral は、キーワードに一致する項目の文字列が必ず含む文字列を、畳んで返す。
// 項目のないキーワードと、必ず含む文字列が 1 つに決まらない正規表現では、ok を false にする。
func requiredLiteral(keyword *Keyword) (string, bool) {
	if len(keyword.Fields) == 0 {
		return "", false
	}
	compiled, err := compileKeyword(keyword)
	if err != nil {
		return "", false
	}

	return compiledLiteral(compiled)
}

// KeywordGrams は、畳んだ文字列 literal を含む文字列が必ず持つ gram を返す。1 文字ならその 1 文字、2 文字以上なら続く 2 文字のすべて。
func KeywordGrams(literal string) []string {
	runes := []rune(literal)
	switch len(runes) {
	case 0:
		return []string{}
	case 1:
		return []string{literal}
	}
	grams := make([]string, 0, len(runes)-1)
	for i := range len(runes) - 1 {
		grams = append(grams, string(runes[i:i+2]))
	}

	return grams
}
