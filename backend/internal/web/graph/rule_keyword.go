package graph

import (
	"context"
	"fmt"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// ruleKeyword は、番組からルールを作るときに、番組名のキーワードにする作品名を返す (#1240、#2041)。
// 番組名から読んだ作品名 (domain.RuleKeyword) が「作品名 副題」のように空白で区切った語を持ち、番組に話数があれば、
// 同じサービスの番組表のほかの回と照合し、回ごとに変わるサブタイトルを除く。ほかの回のキーワードが、語の並びの先頭の部分で一致し、
// その後ろだけが違えば (作品名 霧の朝 #6、作品名: 霧の朝 #6、作品名 #7)、一致した部分を作品名とする。長い部分から順に探す。
// 話数のない番組とは照合しない。頭の語が同じだけの別の番組 (MUSIC STATION と MUSIC BLISS #26) で、作品名を削りすぎないためである。
func (r *Resolver) ruleKeyword(ctx context.Context, p *model.Program) (string, error) {
	keyword := domain.RuleKeyword(p.Name)
	prefixes := domain.RuleKeywordPrefixes(keyword)
	if len(prefixes) == 0 || !domain.HasEpisodeNumber(p.Name) {
		return keyword, nil
	}
	shortest := prefixes[len(prefixes)-1]
	// 正規化した番組名は頭の飾りや枠の名前 ([新]、連続ドラマW) を残しているので、頭からではなく部分文字列で探す。
	// LIKE のパターンで特別な意味を持つ文字は、文字どおりに一致させる。
	likeEscaper := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)
	names, err := r.queries.ListServiceProgramNamesLike(ctx, p.ServiceKey, int64(p.EventID), "%"+likeEscaper.Replace(shortest)+"%")
	if err != nil {
		return "", fmt.Errorf("failed to list the programs of service %d: %w", p.ServiceKey, err)
	}
	others := make([]string, 0, len(names))
	for _, name := range names {
		if other := domain.RuleKeyword(name); domain.HasEpisodeNumber(name) && other != keyword {
			others = append(others, other)
		}
	}
	for _, prefix := range prefixes {
		for _, other := range others {
			if domain.HasKeywordPrefix(other, prefix) {
				return prefix, nil
			}
		}
	}

	return keyword, nil
}
