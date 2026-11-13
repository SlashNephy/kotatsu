package edcbimport

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

const (
	// freeCAOnlyFree と freeCAOnlyPaid は、EDCB の無料と有料の条件。0 はどちらでも。
	freeCAOnlyFree = 1
	freeCAOnlyPaid = 2
)

// ruleInput は、EDCB のキーワード予約を Kotatsu のルールの内容にする。services は、Kotatsu にあるサービスの ID の集合。
// 写せない条件があれば、取り込まずに飛ばす理由を返す。写せたが捨てた項目は、メモに残す。
func ruleInput(a *edcb.AutoAdd, services map[int64]bool) (*rule.Input, error) {
	notes := []string{}
	and := parseAndKey(a.Search.AndKey)
	fields := []match.Field{match.FieldName}
	if !a.Search.TitleOnly {
		fields = append(fields, match.FieldDescription, match.FieldExtended)
	}
	keywords, keywordMatch, err := keywordsOf(and.text, a.Search.Regexp, and.caseSensitive, fields)
	if err != nil {
		return nil, err
	}
	// EDCB の NOT キーワードは、語のどれか 1 つを含む番組を除く (EpgDBManager.cpp の FindKeyword を andFlag=false で呼ぶ)。
	// Kotatsu の除外のキーワードと同じ意味なので、語ごとのキーワードにする。「|」で組に分けることはしない。
	excludeKeywords, err := excludeKeywordsOf(parseNotKey(a.Search.NotKey), a.Search.Regexp, and.caseSensitive, fields)
	if err != nil {
		return nil, err
	}

	conditions := &match.Conditions{
		Keywords: keywords, KeywordMatch: keywordMatch, ExcludeKeywords: excludeKeywords,
		ChannelTypes: []string{}, ServiceIDs: []int64{}, ExcludeServiceIDs: []int64{},
		Genres: []*match.Genre{}, ExcludeGenres: []*match.Genre{}, TimeRanges: []*match.TimeRange{},
		MinDurationSeconds: nil, MaxDurationSeconds: nil, FreeOnly: false, Marks: []string{}, ExcludeMarks: []string{},
	}
	if err := applyServices(conditions, a.Search.Services, services, &notes); err != nil {
		return nil, err
	}
	if err := applyGenres(conditions, a.Search, &notes); err != nil {
		return nil, err
	}
	if len(a.Search.Dates) > 0 {
		if a.Search.NotDates {
			conditions.TimeRanges = complementTimeRanges(a.Search.Dates)
		} else {
			for _, d := range a.Search.Dates {
				conditions.TimeRanges = append(conditions.TimeRanges, timeRangeOf(d))
			}
		}
	}
	if and.minMinutes > 0 {
		conditions.MinDurationSeconds = new(and.minMinutes * 60)
	}
	if and.maxMinutes > 0 {
		conditions.MaxDurationSeconds = new(and.maxMinutes * 60)
	}
	switch a.Search.FreeCA {
	case freeCAOnlyFree:
		conditions.FreeOnly = true
	case freeCAOnlyPaid:
		return nil, unsupported("paid programs only")
	}
	if a.Search.Fuzzy {
		notes = append(notes, "あいまい検索 (普通の検索として取り込んだ)")
	}
	enabled := !and.disabled && a.RecSetting.Records()
	if !enabled {
		notes = append(notes, "無効 (無効のルールとして取り込んだ)")
	}

	return &rule.Input{
		Name:       ruleName(and.text, a.ID),
		Memo:       memo("キーワード予約", int64(a.ID), notes),
		Enabled:    enabled,
		Conditions: conditions,
		Overrides:  new(domain.RecordingOverrides),
		ValidFrom:  nil, ValidUntil: nil,
		// 録画済みかのチェックは、録画履歴による重複の回避と、同じ回の束ねにする (#1498)。期間は写さない。
		AvoidDuplicates:     a.Search.CheckRecorded,
		DuplicateWindowDays: nil,
		FollowEventRelay:    false,
		BundleSameEpisodes:  a.Search.CheckRecorded,
		SkipEarlierEpisodes: false,
	}, nil
}

// applyServices は、EDCB の局を Kotatsu のサービスにする。Kotatsu にない局は外してメモに残し、1 つも残らなければ飛ばす。
// EDCB では、局が空のルールは何にも当たらない。Kotatsu の「空なら全局」とは逆なので、全局にはせずに飛ばす。
func applyServices(conditions *match.Conditions, packed []uint64, services map[int64]bool, notes *[]string) error {
	if len(packed) == 0 {
		return unsupported("no services (it matches nothing in EDCB)")
	}
	missing := []string{}
	for _, p := range packed {
		id := serviceIDOf(p)
		if !services[id] {
			missing = append(missing, strconv.FormatInt(id, 10))

			continue
		}
		if !slices.Contains(conditions.ServiceIDs, id) {
			conditions.ServiceIDs = append(conditions.ServiceIDs, id)
		}
	}
	if len(conditions.ServiceIDs) == 0 {
		return unsupported("none of the services are in Kotatsu")
	}
	if len(missing) > 0 {
		*notes = append(*notes, "Kotatsu にない局 "+strings.Join(missing, "、"))
	}

	return nil
}

// applyGenres は、EDCB のジャンルを写す。写せないジャンルは外してメモに残し、ジャンルを指定していたのに 1 つも写せなければ飛ばす。
func applyGenres(conditions *match.Conditions, search edcb.SearchKey, notes *[]string) error {
	if len(search.Contents) == 0 {
		return nil
	}
	genres := []*match.Genre{}
	dropped := 0
	for _, c := range search.Contents {
		genre, ok := genreOf(c)
		if !ok {
			dropped++

			continue
		}
		genres = append(genres, genre)
	}
	if len(genres) == 0 {
		return unsupported("none of the genres can be represented")
	}
	if dropped > 0 {
		*notes = append(*notes, fmt.Sprintf("ジャンル情報なしか拡張ジャンルの条件 %d 件", dropped))
	}
	if search.NotContents {
		conditions.ExcludeGenres = genres
	} else {
		conditions.Genres = genres
	}

	return nil
}

// memo は、取り込み元と、取り込めなかった項目をメモにする。
func memo(kind string, sourceID int64, notes []string) string {
	text := fmt.Sprintf("EDCB の%s #%d から取り込みました。", kind, sourceID)
	if len(notes) > 0 {
		text += "\n取り込めなかった項目: " + strings.Join(notes, "、")
	}

	return text
}

// isUnsupported は、err が Kotatsu の条件で表せないことを表すかどうかを返す。
func isUnsupported(err error) bool {
	_, ok := errors.AsType[*unsupportedError](err)

	return ok
}
