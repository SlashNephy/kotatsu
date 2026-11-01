package anime

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/syobocal"
)

const (
	// SourceTitle は、番組名を解析して結びつけた話の出どころ。
	SourceTitle = "title"
	// SourceSyobocalTitles は、しょぼいカレンダーのタイトルの一覧の取り込み元の名前 (anime_imports.source)。
	SourceSyobocalTitles = "syobocal_titles"
	// titleOverlap は、タイトルの一覧を前に取り込んだときより、どれだけ前から更新されたタイトルを取り込み直すか。
	// しょぼいカレンダーの更新日時と、問い合わせた時刻のずれで、更新を取りこぼさないようにする。
	titleOverlap = 24 * time.Hour
)

var (
	// leadingMarks と trailingMarks は、番組名の前後に付く、[新] や [字] のような印。
	leadingMarks  = regexp.MustCompile(`^(?:\[[^\[\]]{1,3}\]\s*)+`)
	trailingMarks = regexp.MustCompile(`(?:\s*\[[^\[\]]{1,3}\])+$`)
	// 番組名の話数の書き方。TVTestAnnictRecorder と同じく、作品名の後に「#1」や「#1-#2」と書くものに加え、「第1話」も読む。
	multipleHashEpisodes  = regexp.MustCompile(`^(.+?)\s*#([0-9]+(?:\.[0-9]+)?)\s*[-・~～〜]\s*#?([0-9]+(?:\.[0-9]+)?)(.*)$`)
	multipleKanjiEpisodes = regexp.MustCompile(`^(.+?)\s*第([0-9]+(?:\.[0-9]+)?)話\s*[-・~～〜]\s*第([0-9]+(?:\.[0-9]+)?)話(.*)$`)
	singleHashEpisode     = regexp.MustCompile(`^(.+?)\s*#([0-9]+(?:\.[0-9]+)?)()(.*)$`)
	singleKanjiEpisode    = regexp.MustCompile(`^(.+?)\s*第([0-9]+(?:\.[0-9]+)?)話()(.*)$`)
	// subtitlePattern は、話数の後に「」で書いたサブタイトル。
	subtitlePattern = regexp.MustCompile(`^\s*「([^」]+)」`)
)

// ParsedTitle は、番組名から読み取った作品名と話数。
type ParsedTitle struct {
	// Title は、作品名。
	Title string
	// CountStart と CountEnd は、話数の範囲。1 話だけなら同じ値。
	CountStart float64
	CountEnd   float64
	// Subtitle は、話数の後に「」で書いたサブタイトル。なければ空。
	Subtitle string
}

// ParseTitle は、しょぼいカレンダーで引けない番組 (アニメ専門チャンネルのリピート放送など) の番組名から、作品名と話数を読み取る (設計書 3.8)。
// 話数が書かれていなければ、ok は false。
func ParseTitle(name string) (*ParsedTitle, bool) {
	normalized := strings.TrimSpace(trailingMarks.ReplaceAllString(leadingMarks.ReplaceAllString(domain.Normalize(name), ""), ""))
	// 先に、複数の話をまとめた書き方を試す。
	for _, pattern := range []*regexp.Regexp{multipleHashEpisodes, multipleKanjiEpisodes, singleHashEpisode, singleKanjiEpisode} {
		match := pattern.FindStringSubmatch(normalized)
		if match == nil {
			continue
		}
		title := strings.TrimSpace(trailingMarks.ReplaceAllString(match[1], ""))
		start, end := parseCount(match[2]), parseCount(match[3])
		if title == "" || start == nil {
			continue
		}
		if end == nil || *end < *start {
			end = start
		}
		parsed := &ParsedTitle{Title: title, CountStart: *start, CountEnd: *end, Subtitle: ""}
		if sub := subtitlePattern.FindStringSubmatch(match[4]); sub != nil {
			parsed.Subtitle = strings.TrimSpace(sub[1])
		}

		return parsed, true
	}

	return nil, false
}

// TitleKey は、作品名を引く鍵を返す。NFKC で正規化し、空白を除き、小文字にする。
func TitleKey(title string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}

		return unicode.ToLower(r)
	}, domain.Normalize(title))
}

// titleEpisodes は、番組名 name を解析した話を返す。作品名がしょぼいカレンダーのタイトル 1 つだけに一致すれば、その TID を付ける。
// 話数が書かれていなければ、空の一覧を返す。
func titleEpisodes(ctx context.Context, db sqlc.DBTX, name string) ([]*Episode, error) {
	parsed, ok := ParseTitle(name)
	if !ok {
		return []*Episode{}, nil
	}
	episode := &Episode{
		Source: SourceTitle, TID: nil, PID: nil, CountStart: new(parsed.CountStart), CountEnd: new(parsed.CountEnd),
		Subtitle: nil, First: false, Last: false,
	}
	if parsed.Subtitle != "" {
		episode.Subtitle = new(parsed.Subtitle)
	}
	titles, err := sqlc.New(db).ListSyobocalTitlesByKey(ctx, TitleKey(parsed.Title))
	if err != nil {
		return nil, fmt.Errorf("failed to look up the syobocal title: %w", err)
	}
	// 同じ作品名のタイトルが複数あれば、どれか決められないので、TID は付けない。
	if len(titles) == 1 {
		episode.TID = new(titles[0].Tid)
	}

	return []*Episode{episode}, nil
}

// TitleFetcher は、しょぼいカレンダーのタイトルの一覧を引く。*syobocal.Client が満たす。
type TitleFetcher interface {
	TitleLookup(ctx context.Context, since time.Time) ([]*syobocal.Title, error)
}

var _ TitleFetcher = new(syobocal.Client)

// importTitles は、しょぼいカレンダーのタイトルの一覧を取り込み、取り込んだタイトルの数を返す。
// 前に取り込んだときより後に更新されたタイトルだけを引く。初めてなら、すべてのタイトルを 1 回の問い合わせで引く。
func (i *Importer) importTitles(ctx context.Context) (int, error) {
	q := sqlc.New(i.db)
	since := time.Time{}
	last, err := q.GetSyobocalTitlesLastUpdate(ctx)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return 0, fmt.Errorf("failed to read the syobocal titles: %w", err)
	case last != nil:
		since = last.Add(-titleOverlap)
	}
	titles, err := i.titles.TitleLookup(ctx, since)
	if err != nil {
		return 0, err
	}
	var count int32
	err = pgx.BeginFunc(ctx, i.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		for _, title := range titles {
			if title.TID <= 0 || title.Title == "" {
				continue
			}
			var lastUpdate *time.Time
			if !title.LastUpdate.IsZero() {
				lastUpdate = new(title.LastUpdate)
			}
			if err := q.UpsertSyobocalTitle(ctx, sqlc.UpsertSyobocalTitleParams{
				Tid: title.TID, Title: title.Title, TitleKey: TitleKey(title.Title), LastUpdate: lastUpdate,
			}); err != nil {
				return fmt.Errorf("failed to save a syobocal title: %w", err)
			}
		}
		if count, err = q.CountSyobocalTitles(ctx); err != nil {
			return fmt.Errorf("failed to count the syobocal titles: %w", err)
		}

		return q.RecordAnimeImport(ctx, sqlc.RecordAnimeImportParams{Source: SourceSyobocalTitles, Etag: nil, Now: i.clock.Now(), ItemCount: count})
	})
	if err != nil {
		return 0, err
	}

	return int(count), nil
}
