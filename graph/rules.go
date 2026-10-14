package graph

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/httpserver"
	"github.com/SlashNephy/kotatsu/internal/match"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
)

const ruleType = "Rule"

type userAgentKey struct{}

// scheduler は、ルールと予約を変える操作者を返す。視聴者には許さない (設計書 9.7)。
// 監査ログに残すため、送信元の IP アドレスと User-Agent も持たせる。
func scheduler(ctx context.Context) (*audit.Actor, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if !principal.Role.Can(auth.PermissionManageSchedule) {
		return nil, NewError(CodeForbidden, "viewers cannot change rules and reservations")
	}

	return actorOf(ctx, principal), nil
}

// actorOf は、ログイン中のユーザーの操作者を返す。監査ログに残すため、送信元の IP アドレスと User-Agent も持たせる。
func actorOf(ctx context.Context, principal *auth.Principal) *audit.Actor {
	actor := audit.User(principal.UserID)
	if client := httpserver.ClientFrom(ctx); client != nil {
		actor.SourceIP = client.IP
	}
	if userAgent, ok := ctx.Value(userAgentKey{}).(string); ok {
		actor.UserAgent = userAgent
	}

	return actor
}

// parseRuleID は、Rule の ID から、ルールの ID を取り出す。
func parseRuleID(globalID string) (uuid.UUID, error) {
	typeName, raw, err := ParseGlobalID(globalID)
	if err != nil || typeName != ruleType {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid rule ID")
	}
	ruleID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid rule ID")
	}

	return ruleID, nil
}

// toRuleInput は、GraphQL のルールの内容を、ルールの入力にする。内容の検証は rule が行う。
func toRuleInput(in *model.RuleInput) (*rule.Input, error) {
	conditions, err := toConditions(in.Conditions)
	if err != nil {
		return nil, err
	}

	return &rule.Input{
		Name:                in.Name,
		Memo:                in.Memo,
		Enabled:             in.Enabled,
		Conditions:          conditions,
		Overrides:           toOverrides(in.Settings),
		ValidFrom:           in.ValidFrom,
		ValidUntil:          in.ValidUntil,
		AvoidDuplicates:     in.AvoidDuplicates,
		DuplicateWindowDays: in.DuplicateWindowDays,
		FollowEventRelay:    in.FollowEventRelay,
	}, nil
}

// toOverrides は、録画設定の上書きの入力を、予約の録画設定の上書きにする。省いたら何も上書きしない。
func toOverrides(in *model.RecordingSettingsOverridesInput) *reserve.Overrides {
	if in == nil {
		return &reserve.Overrides{PoolID: nil, FileNameTemplate: nil, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: nil, JoinSegments: nil}
	}

	return &reserve.Overrides{
		PoolID:             in.PoolID,
		FileNameTemplate:   in.FileNameTemplate,
		StartMarginSeconds: in.StartMarginSeconds,
		EndMarginSeconds:   in.EndMarginSeconds,
		Priority:           in.Priority,
		JoinSegments:       in.JoinSegments,
	}
}

func toOverridesModel(o *reserve.Overrides) *model.RecordingSettingsOverrides {
	if o == nil {
		return &model.RecordingSettingsOverrides{PoolID: nil, FileNameTemplate: nil, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: nil, JoinSegments: nil}
	}

	return &model.RecordingSettingsOverrides{
		PoolID:             o.PoolID,
		FileNameTemplate:   o.FileNameTemplate,
		StartMarginSeconds: o.StartMarginSeconds,
		EndMarginSeconds:   o.EndMarginSeconds,
		Priority:           o.Priority,
		JoinSegments:       o.JoinSegments,
	}
}

// toRules は、ルールを GraphQL の型にする。作成者と更新者は、まとめて引く。
func toRules(ctx context.Context, rules ...*rule.Rule) ([]*model.Rule, error) {
	ids := make([]*uuid.UUID, 0, 2*len(rules))
	for _, r := range rules {
		ids = append(ids, r.CreatedBy, r.UpdatedBy)
	}
	byID, err := users(ctx, ids...)
	if err != nil {
		return nil, err
	}

	models := make([]*model.Rule, 0, len(rules))
	for _, r := range rules {
		m := toRule(r)
		m.CreatedBy, m.UpdatedBy = userOf(byID, r.CreatedBy), userOf(byID, r.UpdatedBy)
		models = append(models, m)
	}

	return models, nil
}

// toRuleModel は、ルール 1 件を GraphQL の型にする。
func toRuleModel(ctx context.Context, r *rule.Rule) (*model.Rule, error) {
	models, err := toRules(ctx, r)
	if err != nil {
		return nil, err
	}

	return models[0], nil
}

func toRule(r *rule.Rule) *model.Rule {
	return &model.Rule{
		ID:                  GlobalID(ruleType, r.ID.String()),
		Name:                r.Name,
		Memo:                r.Memo,
		Enabled:             r.Enabled,
		Conditions:          toProgramConditions(r.Conditions),
		Settings:            toOverridesModel(r.Overrides),
		ValidFrom:           r.ValidFrom,
		ValidUntil:          r.ValidUntil,
		AvoidDuplicates:     r.AvoidDuplicates,
		DuplicateWindowDays: r.DuplicateWindowDays,
		FollowEventRelay:    r.FollowEventRelay,
		CreatedAt:           r.CreatedAt,
		CreatedBy:           nil,
		UpdatedAt:           r.UpdatedAt,
		UpdatedBy:           nil,
	}
}

func toProgramConditions(c *match.Conditions) *model.ProgramConditions {
	timeRanges := make([]*model.TimeRange, 0, len(c.TimeRanges))
	for _, tr := range c.TimeRanges {
		timeRanges = append(timeRanges, &model.TimeRange{Weekdays: nonNil(tr.Weekdays), StartMinute: tr.StartMinute, DurationMinutes: tr.DurationMinutes})
	}

	return &model.ProgramConditions{
		Keywords:           toKeywordModels(c.Keywords),
		KeywordMatch:       toKeywordMatchModel(c.KeywordMatch),
		ExcludeKeywords:    toKeywordModels(c.ExcludeKeywords),
		ChannelTypes:       nonNil(c.ChannelTypes),
		Genres:             toGenreConditions(c.Genres),
		ExcludeGenres:      toGenreConditions(c.ExcludeGenres),
		TimeRanges:         timeRanges,
		MinDurationSeconds: c.MinDurationSeconds,
		MaxDurationSeconds: c.MaxDurationSeconds,
		FreeOnly:           c.FreeOnly,
		ServiceKeys:        nonNil(c.ServiceIDs),
	}
}

func toKeywordModels(keywords []*match.Keyword) []*model.Keyword {
	models := make([]*model.Keyword, 0, len(keywords))
	for _, k := range keywords {
		fields := make([]model.KeywordField, 0, len(k.Fields))
		for _, field := range k.Fields {
			fields = append(fields, model.KeywordField(strings.ToUpper(string(field))))
		}
		models = append(models, &model.Keyword{Text: k.Text, Regexp: k.Regexp, CaseSensitive: k.CaseSensitive, Fields: fields})
	}

	return models
}

// rule は、ルールを引く。
func (r *Resolver) rule(ctx context.Context, globalID string) (*model.Rule, error) {
	ruleID, err := parseRuleID(globalID)
	if err != nil {
		return nil, err
	}
	found, err := r.rules.Get(ctx, r.db, ruleID)
	if err != nil {
		return nil, err
	}

	return toRuleModel(ctx, found)
}

// previewRule は、保存する前のルールに一致する番組を、開始時刻の順に返す。
// ルールの照合 (rule.Engine) と同じく、放送が始まった番組、終了時刻が未定の番組、ルールの有効期間の外の番組は含めない。
func (r *Resolver) previewRule(ctx context.Context, in *model.RuleInput, first int, after *string) (*model.ProgramConnection, error) {
	if first < 1 || first > maxSearchResults {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("first must be between 1 and %d", maxSearchResults))
	}
	input, err := toRuleInput(in)
	if err != nil {
		return nil, err
	}
	if input.ValidFrom != nil && input.ValidUntil != nil && !input.ValidFrom.Before(*input.ValidUntil) {
		return nil, NewError(CodeBadUserInput, "validFrom must be before validUntil")
	}
	matcher, err := match.Compile(input.Conditions, r.location)
	if err != nil {
		return nil, NewError(CodeBadUserInput, err.Error())
	}

	params, err := r.searchParams(input.Conditions, nil, after)
	if err != nil {
		return nil, err
	}
	// 今より後に始まる番組だけを読む。開始時刻が今と同じ番組も、サービスと eventId を最大にして除く。
	if now := r.clock.Now(); !params.AfterStartAt.After(now) {
		params.AfterStartAt, params.AfterServiceID, params.AfterEventID = now, math.MaxInt64, math.MaxInt64
	}
	accept := func(startAt time.Time, endUndecided bool) bool {
		return !endUndecided &&
			(input.ValidFrom == nil || !startAt.Before(*input.ValidFrom)) &&
			(input.ValidUntil == nil || startAt.Before(*input.ValidUntil))
	}
	edges, hasNext, err := r.collect(ctx, matcher, accept, params, first)
	if err != nil {
		return nil, err
	}

	return &model.ProgramConnection{Edges: edges, PageInfo: pageInfo(edges, hasNext, after != nil)}, nil
}
