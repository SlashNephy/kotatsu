package graph

import (
	"strings"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/match"
)

// toConditions は、GraphQL の番組の条件を、照合の条件にする。条件の検証は match が行う。
func toConditions(in *model.ProgramConditionsInput) (*match.Conditions, error) {
	services, err := parseServiceGlobalIDs(in.Services)
	if err != nil {
		return nil, err
	}
	excludeServices, err := parseServiceGlobalIDs(in.ExcludeServices)
	if err != nil {
		return nil, err
	}

	conditions := &match.Conditions{
		Keywords:           toKeywords(in.Keywords),
		KeywordMatch:       toKeywordMatch(in.KeywordMatch),
		ExcludeKeywords:    toKeywords(in.ExcludeKeywords),
		ChannelTypes:       nonNil(in.ChannelTypes),
		ServiceIDs:         services,
		ExcludeServiceIDs:  excludeServices,
		Genres:             toGenres(in.Genres),
		ExcludeGenres:      toGenres(in.ExcludeGenres),
		TimeRanges:         toTimeRanges(in.TimeRanges),
		MinDurationSeconds: in.MinDurationSeconds,
		MaxDurationSeconds: in.MaxDurationSeconds,
		FreeOnly:           in.FreeOnly != nil && *in.FreeOnly,
		Marks:              in.Marks,
		ExcludeMarks:       in.ExcludeMarks,
	}
	if err := conditions.Validate(); err != nil {
		return nil, NewError(CodeBadUserInput, err.Error())
	}

	return conditions, nil
}

// parseServiceGlobalIDs は、サービスのグローバル ID の並びを、Mirakurun のサービス ID の並びにする。
func parseServiceGlobalIDs(globalIDs []string) ([]int64, error) {
	ids := make([]int64, 0, len(globalIDs))
	for _, globalID := range globalIDs {
		id, err := parseServiceGlobalID(globalID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// toKeywordMatch と toKeywordMatchModel は、含めるキーワードの照合のしかたを、GraphQL と照合の条件で行き来させる。
func toKeywordMatch(in model.KeywordMatch) match.KeywordMatch {
	if in == model.KeywordMatchAny {
		return match.KeywordMatchAny
	}

	return match.KeywordMatchAll
}

func toKeywordMatchModel(in match.KeywordMatch) model.KeywordMatch {
	if in == match.KeywordMatchAny {
		return model.KeywordMatchAny
	}

	return model.KeywordMatchAll
}

func toKeywords(in []*model.KeywordInput) []*match.Keyword {
	keywords := make([]*match.Keyword, 0, len(in))
	for _, k := range in {
		fields := make([]match.Field, 0, len(k.Fields))
		for _, field := range k.Fields {
			fields = append(fields, match.Field(strings.ToLower(string(field))))
		}
		keywords = append(keywords, &match.Keyword{Text: k.Text, Regexp: k.Regexp, CaseSensitive: k.CaseSensitive, Fields: fields})
	}

	return keywords
}

func toGenres(in []*model.GenreInput) []*match.Genre {
	genres := make([]*match.Genre, 0, len(in))
	for _, g := range in {
		genres = append(genres, &match.Genre{Lv1: g.Lv1, Lv2: g.Lv2})
	}

	return genres
}

func toTimeRanges(in []*model.TimeRangeInput) []*match.TimeRange {
	ranges := make([]*match.TimeRange, 0, len(in))
	for _, r := range in {
		ranges = append(ranges, &match.TimeRange{Weekdays: nonNil(r.Weekdays), StartMinute: r.StartMinute, DurationMinutes: r.DurationMinutes})
	}

	return ranges
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}

	return s
}
