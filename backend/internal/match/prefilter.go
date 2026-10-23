package match

import (
	"regexp/syntax"
	"slices"
	"strings"
	"unicode/utf8"
)

// maxPrefilterLiterals は、前段の絞り込みに使う文字列の数の上限。これより多い選択肢を持つ正規表現は、絞り込まずに照合する。
const maxPrefilterLiterals = 32

// prefilter は、正規表現に一致する文字列が必ず含む文字列の候補。候補のどれも含まない文字列は、正規表現に一致しない。
// Go の regexp は入力の長さに比例する時間で動くが、1 バイトあたりが遅いので、長い詳細を多くのルールで照合すると重い。
// 一致しない番組がほとんどなので、文字列の検索で先に落とす (#852)。
type prefilter struct {
	// literals は、どれか 1 つを必ず含む文字列。fold なら畳んで持つ。
	literals []string
	// fold は、大文字と小文字を区別せずに探すかどうか。
	fold bool
}

// newPrefilter は、正規表現の構文から前段の絞り込みを作る。絞り込めない正規表現なら nil を返す。
func newPrefilter(re *syntax.Regexp) *prefilter {
	f := requiredLiterals(re.Simplify())
	if f == nil || len(f.literals) == 0 {
		return nil
	}
	if f.fold {
		for i, literal := range f.literals {
			f.literals[i] = foldCase(literal)
		}
	}
	slices.Sort(f.literals)
	f.literals = slices.Compact(f.literals)

	return f
}

// requiredLiterals は、re に一致する文字列が必ずどれか 1 つを含む文字列の集合を返す。そのような集合がなければ nil を返す。
// fold は、集合のどれかが大文字と小文字を区別しない (?i) の下にあるかどうか。区別する文字列も畳んで探せば、必要な条件は保たれる。
func requiredLiterals(re *syntax.Regexp) *prefilter {
	switch re.Op {
	case syntax.OpLiteral:
		return &prefilter{literals: []string{string(re.Rune)}, fold: re.Flags&syntax.FoldCase != 0}
	case syntax.OpCapture, syntax.OpPlus:
		return requiredLiterals(re.Sub[0])
	case syntax.OpRepeat:
		if re.Min < 1 {
			return nil
		}

		return requiredLiterals(re.Sub[0])
	case syntax.OpConcat:
		// 連なりは、どの部分の集合も必要な条件になる。最も短い文字列が最も長い部分を選び、多くの番組を落とせるようにする。
		var best *prefilter
		bestLength := 0
		for _, sub := range re.Sub {
			if f := requiredLiterals(sub); f != nil {
				if length := shortestLength(f.literals); length > bestLength {
					best, bestLength = f, length
				}
			}
		}

		return best
	case syntax.OpAlternate:
		// 選択は、どの選択肢も集合を持つときだけ、その和が必要な条件になる。
		union := &prefilter{literals: []string{}, fold: false}
		for _, sub := range re.Sub {
			f := requiredLiterals(sub)
			if f == nil || len(union.literals)+len(f.literals) > maxPrefilterLiterals {
				return nil
			}
			union.literals, union.fold = append(union.literals, f.literals...), union.fold || f.fold
		}

		return union
	case syntax.OpNoMatch, syntax.OpEmptyMatch, syntax.OpCharClass, syntax.OpAnyCharNotNL, syntax.OpAnyChar,
		syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary,
		syntax.OpStar, syntax.OpQuest:
		// 文字クラス、任意の 1 文字、0 回を許す繰り返し、位置の指定などは、含む文字列を決めない。
		return nil
	}

	return nil
}

// shortestLength は、文字列の集合のうち、最も短いものの文字数を返す。
func shortestLength(literals []string) int {
	shortest := 0
	for i, literal := range literals {
		if length := utf8.RuneCountInString(literal); i == 0 || length < shortest {
			shortest = length
		}
	}

	return shortest
}

// mayMatch は、項目 field の文字列が、正規表現に一致しうるかどうかを返す。
func (f *prefilter) mayMatch(p *Program, field Field) bool {
	text := p.text(field)
	if f.fold {
		text = p.foldedText(field)
	}

	return slices.ContainsFunc(f.literals, func(literal string) bool { return strings.Contains(text, literal) })
}
