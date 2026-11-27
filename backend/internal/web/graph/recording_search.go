package graph

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/autocaption"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// recordingSearchBatchSize は、キーワードの条件で録画を探すときに、候補を DB から読む 1 回の件数。
const recordingSearchBatchSize = 200

// recordingKeywordConditions は、録画の絞り込みのキーワードの条件 (#966) を、照合の条件にする。キーワードの条件がなければ、ok に false を返す。
// 録画は詳細を正規化したカラムを持たず、詳細では探さないので (設計書 3.5)、詳細を見るキーワードは拒む。
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

// coarseRecordingKeyword は、SQL で粗く絞るのに使うキーワードを、LIKE のパターンの内容にして返す。使えるものがなければ nil を返す。
// 録画の検索の SQL は、番組名か概要の正規化したカラムの部分一致で絞る (大文字と小文字を区別しない)。照合関数がすべてに一致を求めるとき、
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

// recordingCaptionMatchLimit は、録画 1 つについて返す、字幕の文で探したときに当たった字幕の数の上限 (#2214)。
const recordingCaptionMatchLimit = 20

// recordingSearch は、録画を候補から 1 件ずつ照らす条件。programs は番組名と概要のキーワード (#966)、
// captions は字幕の文のキーワード (#2214)。使わない条件は nil にする。
type recordingSearch struct {
	programs *match.Matcher
	captions *match.TextMatcher
}

// newRecordingSearch は、録画の絞り込みのキーワード (#966) と字幕の文のキーワード (#2214) を、録画を 1 件ずつ照らす条件にする。
// SQL で粗く絞る条件は params に入れる。どちらのキーワードもなければ、ok に false を返す。
func (r *Resolver) newRecordingSearch(filter *model.RecordingFilter, params *query.SearchRecordingsParams) (*recordingSearch, bool, error) {
	search := &recordingSearch{programs: nil, captions: nil}
	conditions, ok, err := recordingKeywordConditions(filter)
	if err != nil {
		return nil, false, err
	}
	if ok {
		search.programs, err = match.Compile(conditions, r.location)
		if err != nil {
			return nil, false, NewError(CodeBadUserInput, err.Error())
		}
		if params.Keyword == nil {
			params.Keyword = coarseRecordingKeyword(conditions)
		}
	}
	if filter != nil && len(filter.CaptionKeywords) > 0 {
		search.captions, err = recordingCaptionMatcher(filter)
		if err != nil {
			return nil, false, err
		}
		params.CaptionGrams = search.captions.RequiredGrams()
	}

	return search, search.programs != nil || search.captions != nil, nil
}

// recordingCaptionMatcher は、録画の絞り込みの字幕の文のキーワード (#2214) を、照合できる形にする。
func recordingCaptionMatcher(filter *model.RecordingFilter) (*match.TextMatcher, error) {
	keywordMatch := match.KeywordMatchAll
	if filter.CaptionKeywordMatch != nil {
		keywordMatch = *filter.CaptionKeywordMatch
	}
	keywords := make([]*match.TextKeyword, 0, len(filter.CaptionKeywords))
	for _, k := range filter.CaptionKeywords {
		keywords = append(keywords, &match.TextKeyword{Text: k.Text, Regexp: k.Regexp, CaseSensitive: k.CaseSensitive})
	}
	matcher, err := match.CompileText(keywords, keywordMatch)
	if err != nil {
		return nil, NewError(CodeBadUserInput, "invalid captionKeywords: "+err.Error())
	}

	return matcher, nil
}

// collectRecordings は、キーワードの条件に一致する録画を、order の順に first 件まで集める。続きがあるかと、当たった字幕も返す (#966、#2214)。
// 番組の検索 (collect) と同じく、候補を順に読んで照合し、first + 1 件目が一致するかで続きの有無を決める。
// searchTimeout を過ぎたら、条件を絞ってもらうエラーにする。照合は、番組の検索と同じ枠 (searchSlots) で数を抑える。
func (r *Resolver) collectRecordings(
	ctx context.Context, search *recordingSearch, params *query.SearchRecordingsParams, order model.RecordingOrder, first int,
) ([]domain.Recording, map[uuid.UUID][]*model.RecordingCaptionMatch, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()
	select {
	case r.searchSlots <- struct{}{}:
		defer func() { <-r.searchSlots }()
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, nil, false, NewError(CodeBadUserInput, "too many searches are running; try again later")
		}

		return nil, nil, false, ctx.Err()
	}
	tooLong := func(err error) error {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return NewError(CodeBadUserInput, "the search took too long; narrow down the conditions")
		}

		return err
	}
	params.MaxCount = recordingSearchBatchSize
	matched := []domain.Recording{}
	captionMatches := map[uuid.UUID][]*model.RecordingCaptionMatch{}
	for {
		rows, err := r.searchRecordingRows(ctx, params, order)
		if err != nil {
			return nil, nil, false, tooLong(err)
		}
		batch, batchCaptions, err := r.matchRecordingBatch(ctx, search, rows)
		if err != nil {
			return nil, nil, false, tooLong(err)
		}
		for i := range batch {
			if len(matched) == first {
				return matched, captionMatches, true, nil
			}
			matched = append(matched, batch[i])
			if found, ok := batchCaptions[batch[i].ID]; ok {
				captionMatches[batch[i].ID] = found
			}
		}
		if len(rows) < recordingSearchBatchSize {
			return matched, captionMatches, false, nil
		}
		last := rows[len(rows)-1]
		params.BeforeAt, params.BeforeID = &last.RecordedFrom, &last.ID
	}
}

// matchRecordingBatch は、候補の録画 rows のうち、条件に一致する録画を rows の順に返す。字幕の文で探したときは、当たった字幕も返す。
func (r *Resolver) matchRecordingBatch(
	ctx context.Context, search *recordingSearch, rows []domain.Recording,
) ([]domain.Recording, map[uuid.UUID][]*model.RecordingCaptionMatch, error) {
	candidates := make([]domain.Recording, 0, len(rows))
	for i := range rows {
		if search.programs == nil || search.programs.Match(recordingMatchProgram(&rows[i])) {
			candidates = append(candidates, rows[i])
		}
	}
	if search.captions == nil {
		return candidates, map[uuid.UUID][]*model.RecordingCaptionMatch{}, nil
	}
	captions, err := r.matchRecordingCaptions(ctx, search.captions, candidates)
	if err != nil {
		return nil, nil, err
	}
	matched := make([]domain.Recording, 0, len(captions))
	for i := range candidates {
		if _, ok := captions[candidates[i].ID]; ok {
			matched = append(matched, candidates[i])
		}
	}

	return matched, captions, nil
}

// matchRecordingCaptions は、録画 recordings のうち、自動生成の字幕が字幕の文のキーワードに一致する録画の、当たった字幕を返す (#2214)。
// 一致しない録画は返す map に含めない。
func (r *Resolver) matchRecordingCaptions(
	ctx context.Context, matcher *match.TextMatcher, recordings []domain.Recording,
) (map[uuid.UUID][]*model.RecordingCaptionMatch, error) {
	out := map[uuid.UUID][]*model.RecordingCaptionMatch{}
	if len(recordings) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, 0, len(recordings))
	for i := range recordings {
		ids = append(ids, recordings[i].ID)
	}
	cuesByRecording, err := autocaption.RecordingsCues(ctx, r.db, ids)
	if err != nil {
		return nil, err
	}
	for i := range recordings {
		cues, ok := cuesByRecording[recordings[i].ID]
		if !ok {
			continue
		}
		texts := make([]string, 0, len(cues))
		for _, c := range cues {
			texts = append(texts, domain.Normalize(c.Text))
		}
		if !matcher.Match(texts) {
			continue
		}
		found := []*model.RecordingCaptionMatch{}
		for j, text := range texts {
			if len(found) == recordingCaptionMatchLimit {
				break
			}
			if matcher.Contains(text) {
				found = append(found, &model.RecordingCaptionMatch{
					Source: model.CaptionSourceAuto, From: recordings[i].RecordedFrom.Add(cues[j].Start),
					To: recordings[i].RecordedFrom.Add(cues[j].End), Text: cues[j].Text,
				})
			}
		}
		out[recordings[i].ID] = found
	}

	return out, nil
}

// searchRecordingRows は、params の録画を、order の順に読む。
func (r *Resolver) searchRecordingRows(ctx context.Context, params *query.SearchRecordingsParams, order model.RecordingOrder) ([]domain.Recording, error) {
	var rows []domain.Recording
	var err error
	if order == model.RecordingOrderOldestFirst {
		rows, err = r.queries.SearchRecordingsOldest(ctx, query.SearchRecordingsOldestParams{
			States: params.States, ServiceIDs: params.ServiceIDs, TagIDs: params.TagIDs, Keyword: params.Keyword,
			CaptionGrams: params.CaptionGrams, Since: params.Since, Until: params.Until, RuleID: params.RuleID, Genres: params.Genres,
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

// recordingMatchProgram は、録画を、キーワードの照合に渡す形にする。番組名と概要は正規化したカラムを使い、
// カラムのない録画 (カラムを知らない古いバイナリが書いたもの) は、ここで正規化する。キーワードのほかの条件は照合しないので、ほかの項目は空にする。
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
