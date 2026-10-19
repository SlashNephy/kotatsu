package match

import "regexp/syntax"

// PrefilterLiterals は、正規表現 pattern の前段の絞り込みに使う文字列と、大文字と小文字を区別せずに探すかどうかを返す。
// 絞り込まない正規表現なら、ok が false になる。
func PrefilterLiterals(pattern string) ([]string, bool, bool, error) {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, false, false, err
	}
	f := newPrefilter(re)
	if f == nil {
		return nil, false, false, nil
	}

	return f.literals, f.fold, true, nil
}
