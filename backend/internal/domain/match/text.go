package match

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"unicode/utf8"
)

// TextMatcher は、項目を持たない文の集まり (録画の字幕の文) と照合するキーワード (#2214、設計書 3.5)。
// キーワードの正規化、正規表現、大文字と小文字の区別は、番組の照合 (Matcher) と同じにする。
// キーワードは文ごとに照らすので、文をまたぐ語には一致しない。複数の goroutine から使ってよい。
type TextMatcher struct {
	keywords     []*compiledKeyword
	keywordMatch KeywordMatch
	required     []string
}

// TextKeyword は、文と照合するキーワード。番組の照合のキーワード (Keyword) から、照合する項目を除いたもの。
type TextKeyword struct {
	Text string
	// Regexp は、Text を正規表現 (RE2) として扱うかどうか。
	Regexp bool
	// CaseSensitive は、大文字と小文字を区別するかどうか。
	CaseSensitive bool
}

// CompileText は、キーワードを検証し、文と照合できる形にする。keywordMatch は、すべてといずれかのどちらに一致を求めるか。
func CompileText(keywords []*TextKeyword, keywordMatch KeywordMatch) (*TextMatcher, error) {
	if len(keywords) == 0 || len(keywords) > maxKeywords {
		return nil, fmt.Errorf("%w: the number of keywords must be 1 to %d", ErrInvalidConditions, maxKeywords)
	}
	if !keywordMatch.IsValid() {
		return nil, fmt.Errorf("%w: unknown keyword match %q", ErrInvalidConditions, keywordMatch)
	}
	m := &TextMatcher{keywords: make([]*compiledKeyword, 0, len(keywords)), keywordMatch: keywordMatch, required: []string{}}
	required := map[string]struct{}{}
	for i, k := range keywords {
		if err := validateTextKeyword(k); err != nil {
			return nil, fmt.Errorf("%w: keywords[%d]: %w", ErrInvalidConditions, i, err)
		}
		compiled, err := compileKeyword(&Keyword{Text: k.Text, Regexp: k.Regexp, CaseSensitive: k.CaseSensitive, Fields: []Field{}})
		if err != nil {
			return nil, err
		}
		m.keywords = append(m.keywords, compiled)
		// いずれかに一致を求めるときも、キーワードが 1 つなら、そのキーワードは必ず含む。
		if keywordMatch == KeywordMatchAll || len(keywords) == 1 {
			if literal, ok := compiledLiteral(compiled); ok {
				for _, gram := range KeywordGrams(literal) {
					required[gram] = struct{}{}
				}
			}
		}
	}
	for gram := range required {
		m.required = append(m.required, gram)
	}
	slices.Sort(m.required)

	return m, nil
}

func validateTextKeyword(k *TextKeyword) error {
	if k == nil {
		return errors.New("must not be null")
	}
	if k.Text == "" || utf8.RuneCountInString(k.Text) > maxKeywordLength {
		return fmt.Errorf("text must be 1 to %d characters", maxKeywordLength)
	}
	if k.Regexp {
		if _, err := regexp.Compile(k.Text); err != nil {
			return fmt.Errorf("invalid regular expression: %w", err)
		}
	}

	return nil
}

// Match は、正規化した文 texts (domain.Normalize) が、キーワードに一致するかどうかを返す。
// すべてに一致を求めるときは、どのキーワードも、どれかの文が含めばよい。同じ文でなくてよい。
func (m *TextMatcher) Match(texts []string) bool {
	found := make([]bool, len(m.keywords))
	count := 0
	for _, text := range texts {
		folded := lazyFold(text)
		for i, k := range m.keywords {
			if found[i] || !k.matchText(text, folded) {
				continue
			}
			if m.keywordMatch == KeywordMatchAny {
				return true
			}
			found[i] = true
			count++
			if count == len(m.keywords) {
				return true
			}
		}
	}

	return false
}

// Contains は、正規化した文 text が、どれかのキーワードを含むかどうかを返す。一致した録画の、当たった文を選ぶのに使う。
func (m *TextMatcher) Contains(text string) bool {
	folded := lazyFold(text)

	return slices.ContainsFunc(m.keywords, func(k *compiledKeyword) bool { return k.matchText(text, folded) })
}

// RequiredGrams は、文の集まりがキーワードに一致するなら、その gram (TextGrams) が必ず含む gram を返す。
// 絞り込める gram がなければ、空 (nil でない) を返す。
func (m *TextMatcher) RequiredGrams() []string {
	return append([]string{}, m.required...)
}

// TextGrams は、文の集まりの検索のインデックスに入れる gram を返す (#2214)。texts は、正規化した文 (domain.Normalize)。
// 番組の gram (SearchGrams) と同じく、大文字と小文字を畳んでから、1 文字と、続く 2 文字の gram を作る。
// キーワードは文ごとに照らすので、文をまたぐ gram は作らない。
func TextGrams(texts []string) []string {
	set := map[string]struct{}{}
	for _, text := range texts {
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

// compiledLiteral は、キーワードに一致する文が必ず含む文字列を、畳んで返す。必ず含む文字列が 1 つに決まらない正規表現では、ok を false にする。
func compiledLiteral(compiled *compiledKeyword) (string, bool) {
	if compiled.pattern == nil {
		return foldCase(compiled.literal), true
	}
	if compiled.prefilter == nil || len(compiled.prefilter.literals) != 1 {
		return "", false
	}

	return foldCase(compiled.prefilter.literals[0]), true
}

// lazyFold は、text を畳んだ文字列を、初めて要るときに作って返す関数を返す。
func lazyFold(text string) func() string {
	var folded *string

	return func() string {
		if folded == nil {
			folded = new(foldCase(text))
		}

		return *folded
	}
}
