package drama

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

var (
	// noisePattern は、番組名のどこにあっても作品名の一部でないもの。【心底ムカついています】のような隅付き括弧の煽り文句、
	// ★のような飾り、「第7週」のような週の数え方 (連続テレビ小説)。NFKC で正規化した番組名から除く。
	noisePattern = regexp.MustCompile(`【[^】]*】|[★☆◆◇■□●○▲△]|第[0-9]+週`)
	// excludedPattern は、照合しない、宣伝や総集編の番組のマーク (PR、ダイジェスト、見どころ、予告、メイキング)。
	excludedPattern = regexp.MustCompile(`(?:^|[^A-Za-z])PR(?:[^A-Za-z]|$)|ダイジェスト|見どころ|予告|メイキング`)
	// episodePattern は、番組名に書いた話数 (#7、第7話、(4))。(1971) のような 4 桁の年は、話数とみなさない。
	episodePattern = regexp.MustCompile(`#\s*([0-9]{1,4})(?:[^0-9.]|$)|第([0-9]{1,4})話|\(([0-9]{1,3})\)`)
	// wavePattern は、作品名の後ろの「〜副題〜」。TMDB の作品名にないサブタイトルを除いて探し直すのに使う。
	wavePattern = regexp.MustCompile(`\s*[~〜～].*$`)
	// sequelPattern は、作品名の後ろの続編の番号 (晩酌の流儀5、はぐれ刑事純情派2、スケバン刑事II)。
	// TMDB は続編を 1 つの作品のシーズンにまとめることが多いので、番号を除いた作品名でも探す。番号は TMDB のシーズンの番号と合わない。
	sequelPattern = regexp.MustCompile(`\s*(?:[0-9]{1,2}|I{1,3}|IV|V|VI{1,3}|IX|X)$`)
)

// minQueryRunes は、TMDB に問い合わせる作品名の長さの下限 (文字)。短すぎる名前は、別の作品ばかりに当たる。
const minQueryRunes = 2

// WorkTitle は、番組名から、TMDB で作品を探す作品名を読む (#1651)。番組名をNFKC で正規化し、囲み文字と、煽り文句の隅付き括弧、
// 週の数え方を除いてから、ドラマの放送枠の名前 (土曜ドラマ、中国ドラマ、懐ドラなど)、話数、サブタイトルを domain.WorkTitle で除く。
func WorkTitle(name string) string {
	cleaned := noisePattern.ReplaceAllString(domain.Normalize(name), " ")
	cleaned = strings.Join(strings.Fields(cleaned), " ")

	return domain.WorkTitle(cleaned)
}

// TitleKey は、作品名を比較するキーを返す。NFKC で正規化し、小文字にし、空白と、表記の揺れやすい記号 (〜、・、!、? など) を除く。
// TMDB の作品名 (「TOKYO MER 〜走る緊急救命室〜」) と番組名 (「TOKYO MER〜走る緊急救命室〜」) の、空白や波ダッシュの違いを吸収する。
func TitleKey(title string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || strings.ContainsRune("~〜～・:：!！?？-－―‐'’\"”“.,、。", r) {
			return -1
		}

		return unicode.ToLower(r)
	}, domain.Normalize(title))
}

// SearchQueries は、作品名 title で TMDB を探す名前を、試す順に返す。作品名そのもの、後ろの「〜副題〜」を除いたもの、
// 後ろの続編の番号を除いたもの (晩酌の流儀5 → 晩酌の流儀) の順。短すぎる名前と、重なる名前は除く。
func SearchQueries(title string) []string {
	queries := []string{}
	add := func(query string) {
		query = strings.TrimSpace(query)
		if len([]rune(query)) < minQueryRunes || slices.Contains(queries, query) {
			return
		}
		queries = append(queries, query)
	}
	add(title)
	base := wavePattern.ReplaceAllString(title, "")
	add(base)
	add(sequelPattern.ReplaceAllString(base, ""))

	return queries
}

// HeadQuery は、作品名 title の最初の空白より前を返す。空白の後ろにサブタイトルが付いた番組名 (南田南弁護士は天才肌 嘘をつくのは悪いことです!) は、
// そのままでは TMDB で見つからないので、頭だけで探し直すのに使う。頭だけの名前は、別の作品の名前と同じことがあるので、
// 見つかった作品は、名前の先頭が一致する候補として、放送日の話があるかで確認してから結びつける。空白がないか、頭が短すぎれば空を返す。
func HeadQuery(title string) string {
	fields := strings.Fields(title)
	if len(fields) < 2 || len([]rune(fields[0])) < minPrefixRunes {
		return ""
	}

	return fields[0]
}

// Excluded は、番組名 name の番組を照合しないかどうかを返す。宣伝の番組 (PR、見どころ、予告) と、総集編 (ダイジェスト)、メイキングは、
// ドラマのジャンルを持っていても、作品の話ではないので照合しない。
func Excluded(name string) bool {
	return excludedPattern.MatchString(domain.Normalize(name))
}

// EpisodeNumber は、番組名に書いた話数 (#7、第7話、(4)) を返す。書いていなければ 0 を返す。いくつも書いてあれば、最初のものを使う。
func EpisodeNumber(name string) int32 {
	for _, match := range episodePattern.FindAllStringSubmatch(domain.Normalize(name), -1) {
		for _, group := range match[1:] {
			if group == "" {
				continue
			}
			if number, err := strconv.ParseInt(group, 10, 32); err == nil && number > 0 {
				return int32(number)
			}
		}
	}

	return 0
}

// Rerun は、番組名の [再] のマーク (囲み文字の 🈞 を含む) か、シリーズの記述子の再放送のマークから、再放送かどうかを返す。
func Rerun(name string, seriesRepeat bool) bool {
	return seriesRepeat || slices.Contains(domain.Marks(name), "再")
}
