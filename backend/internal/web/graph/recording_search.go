package graph

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// recordingSearchBatchSize は、キーワードの条件で録画を探すときに、候補を DB から読む 1 回の件数。
const recordingSearchBatchSize = 200

// recordingKeywordConditions は、録画の絞り込みのキーワードの条件 (#966) を、照合の条件にする。キーワードの条件がなければ、ok に false を返す。
// 録画は詳細を正規化した列を持たず、詳細では探さないので (設計書 3.5)、詳細を見るキーワードは拒む。
func recordingKeywordConditions(filter *model.RecordingFilter) (*match.Conditions, bool, error) {
	if filter == nil || (len(filter.Keywords) == 0 && len(filter.ExcludeKeywords) == 0) {
		return nil, false, nil
	}
	for _, keyword := range slices.Concat(filter.Keywords, filter.ExcludeKeywords) {
		if slices.Contains(keyword.Fields, match.FieldExtended) {
			return nil, false, NewError(CodeBadUserInput, "recordings cannot be searched by the extended description")
		}
	}
	keywordMatch := match.KeywordMatchAll
	if filter.KeywordMatch != nil {
		keywordMatch = *filter.KeywordMatch
	}
	conditions := &match.Conditions{
		Keywords: toKeywords(filter.Keywords), KeywordMatch: keywordMatch, ExcludeKeywords: toKeywords(filter.ExcludeKeywords),
		ChannelTypes: []string{}, ServiceIDs: []int64{}, ExcludeServiceIDs: []int64{}, Genres: []*match.Genre{}, ExcludeGenres: []*match.Genre{},
		TimeRanges: []*match.TimeRange{}, MinDurationSeconds: nil, MaxDurationSeconds: nil, FreeOnly: false, Marks: []string{}, ExcludeMarks: []string{},
	}
	if err := conditions.Validate(); err != nil {
		return nil, false, NewError(CodeBadUserInput, err.Error())
	}

	return conditions, true, nil
}

// coarseRecordingKeyword は、SQL で粗く絞るのに使うキーワードを、LIKE のパターンの中身にして返す。使えるものがなければ nil を返す。
// 録画の検索の SQL は、番組名か概要の正規化した列の部分一致で絞る (大文字と小文字を区別しない)。照合関数がすべてに一致を求めるとき、
// 正規表現でないキーワードは、そのどちらかに必ず含まれるので、これで絞っても結果は変わらない。
func coarseRecordingKeyword(conditions *match.Conditions) *string {
	if conditions.KeywordMatch == match.KeywordMatchAny {
		return nil
	}
	for _, keyword := range conditions.Keywords {
		if !keyword.Regexp {
			return new(escapeLike(domain.Normalize(keyword.Text)))
		}
	}

	return nil
}

// collectRecordings は、キーワードの条件に一致する録画を、order の順に first 件まで集める。続きがあるかも返す (#966)。
// 番組の検索 (collect) と同じく、候補を順に読んで照合し、first + 1 件目が一致するかで続きの有無を決める。
// searchTimeout を過ぎたら、条件を絞ってもらうエラーにする。照合は、番組の検索と同じ枠 (searchSlots) で数を抑える。
func (r *Resolver) collectRecordings(
	ctx context.Context, matcher *match.Matcher, params *query.SearchRecordingsParams, order model.RecordingOrder, first int,
) ([]domain.Recording, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	select {
	case r.searchSlots <- struct{}{}:
		defer func() { <-r.searchSlots }()
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, false, NewError(CodeBadUserInput, "too many searches are running; try again later")
		}

		return nil, false, ctx.Err()
	}
	params.MaxCount = recordingSearchBatchSize
	matched := []domain.Recording{}
	for {
		rows, err := r.searchRecordingRows(ctx, params, order)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, false, NewError(CodeBadUserInput, "the search took too long; narrow down the conditions")
			}

			return nil, false, err
		}
		for i := range rows {
			if !matcher.Match(recordingMatchProgram(&rows[i])) {
				continue
			}
			if len(matched) == first {
				return matched, true, nil
			}
			matched = append(matched, rows[i])
		}
		if len(rows) < recordingSearchBatchSize {
			return matched, false, nil
		}
		last := rows[len(rows)-1]
		params.BeforeAt, params.BeforeID = &last.RecordedFrom, &last.ID
	}
}

// searchRecordingRows は、params の録画を、order の順に読む。
func (r *Resolver) searchRecordingRows(ctx context.Context, params *query.SearchRecordingsParams, order model.RecordingOrder) ([]domain.Recording, error) {
	var rows []domain.Recording
	var err error
	if order == model.RecordingOrderOldestFirst {
		rows, err = r.queries.SearchRecordingsOldest(ctx, query.SearchRecordingsOldestParams{
			States: params.States, ServiceIDs: params.ServiceIDs, TagIDs: params.TagIDs, Keyword: params.Keyword,
			Since: params.Since, Until: params.Until, RuleID: params.RuleID, Genres: params.Genres,
			AfterAt: params.BeforeAt, AfterID: params.BeforeID, MaxCount: params.MaxCount,
		})
	} else {
		rows, err = r.queries.SearchRecordings(ctx, *params)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to search recordings: %w", err)
	}

	return rows, nil
}

// recordingMatchProgram は、録画を、キーワードの照合に渡す形にする。番組名と概要は正規化した列を使い、
// 列のない録画 (列を知らない古いバイナリが書いたもの) は、ここで正規化する。キーワードのほかの条件は照合しないので、ほかの項目は空にする。
func recordingMatchProgram(recording *domain.Recording) *match.Program {
	normalized := func(column *string, raw string) string {
		if column != nil {
			return *column
		}

		return domain.Normalize(raw)
	}

	return &match.Program{
		ServiceID: recording.ServiceID, SharedFrom: 0, ChannelType: "",
		Name: normalized(recording.NameNormalized, recording.Name), Description: normalized(recording.DescriptionNormalized, recording.Description),
		Extended: "", Genres: []*match.Genre{}, StartAt: recording.StartAt, Duration: recording.EndAt.Sub(recording.StartAt), IsFree: true,
		Marks: []string{},
	}
}
