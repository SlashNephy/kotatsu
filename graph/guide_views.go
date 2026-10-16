package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/match"
	"github.com/SlashNephy/kotatsu/internal/usersettings"
)

const (
	guideViewType = "GuideView"
	// maxGuideViewName は、番組表ビューの名前の長さの上限。
	maxGuideViewName = 100
	// maxGuideViewServices は、番組表ビューに並べられるサービスの数の上限。
	maxGuideViewServices = 1000
	// pgUniqueViolation は、一意制約の違反を表す PostgreSQL のエラーコード。
	pgUniqueViolation = "23505"
)

// guideViewSettings は、番組表ビューの条件の、JSONB に保存する形。
type guideViewSettings struct {
	Genres             []*match.Genre `json:"genres"`
	ExcludeGenres      []*match.Genre `json:"excludeGenres"`
	MinDurationSeconds *int           `json:"minDurationSeconds,omitempty"`
	// Services は、チャンネルの並び (Mirakurun のサービス ID)。
	Services []int64 `json:"services"`
	// FreeOnly は、無料放送の番組だけを出すかどうか。項目を足す前に保存したビューは、区切らない (false) として読む。
	FreeOnly bool `json:"freeOnly,omitempty"`
}

// conditions は、番組の絞り込みに使う照合の条件を返す。
func (s *guideViewSettings) conditions() *match.Conditions {
	return &match.Conditions{
		Keywords: []*match.Keyword{}, KeywordMatch: match.KeywordMatchAll, ExcludeKeywords: []*match.Keyword{}, ChannelTypes: []string{}, ServiceIDs: []int64{},
		Genres: s.Genres, ExcludeGenres: s.ExcludeGenres, TimeRanges: []*match.TimeRange{},
		MinDurationSeconds: s.MinDurationSeconds, MaxDurationSeconds: nil, FreeOnly: s.FreeOnly,
	}
}

// guideViewFromInput は、入力を検証して、保存する形にする。
func guideViewFromInput(in *model.GuideViewInput) (*guideViewSettings, error) {
	if strings.TrimSpace(in.Name) == "" || utf8.RuneCountInString(in.Name) > maxGuideViewName {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("name must be 1 to %d characters", maxGuideViewName))
	}
	if len(in.Services) > maxGuideViewServices {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("services must have at most %d items", maxGuideViewServices))
	}
	services := make([]int64, 0, len(in.Services))
	for _, globalID := range in.Services {
		serviceID, err := parseServiceGlobalID(globalID)
		if err != nil {
			return nil, err
		}
		if slices.Contains(services, serviceID) {
			return nil, NewError(CodeBadUserInput, "services must not contain duplicates")
		}
		services = append(services, serviceID)
	}

	settings := &guideViewSettings{
		Genres:             toGenres(in.Genres),
		ExcludeGenres:      toGenres(in.ExcludeGenres),
		MinDurationSeconds: in.MinDurationSeconds,
		Services:           services,
		FreeOnly:           in.FreeOnly,
	}
	if err := settings.conditions().Validate(); err != nil {
		return nil, NewError(CodeBadUserInput, err.Error())
	}

	return settings, nil
}

func toGuideView(row *sqlc.GuideView) (*model.GuideView, error) {
	var settings guideViewSettings
	if err := json.Unmarshal(row.Settings, &settings); err != nil {
		return nil, fmt.Errorf("failed to decode guide view %s: %w", row.ID, err)
	}

	return &model.GuideView{
		ID:                 GlobalID(guideViewType, row.ID.String()),
		Name:               row.Name,
		Genres:             toGenreConditions(settings.Genres),
		ExcludeGenres:      toGenreConditions(settings.ExcludeGenres),
		MinDurationSeconds: settings.MinDurationSeconds,
		HideEmptyServices:  row.HideEmptyServices,
		FreeOnly:           settings.FreeOnly,
		ServiceKeys:        nonNil(settings.Services),
	}, nil
}

func toGenreConditions(genres []*match.Genre) []*model.GenreCondition {
	conditions := make([]*model.GenreCondition, 0, len(genres))
	for _, g := range genres {
		conditions = append(conditions, &model.GenreCondition{Lv1: g.Lv1, Lv2: g.Lv2})
	}

	return conditions
}

// parseGuideViewID は、GuideView の ID から、番組表ビューの ID を取り出す。
func parseGuideViewID(globalID string) (uuid.UUID, error) {
	typeName, raw, err := ParseGlobalID(globalID)
	if err != nil || typeName != guideViewType {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid guide view ID")
	}
	viewID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid guide view ID")
	}

	return viewID, nil
}

// viewer は、ログイン中のユーザーを返す。番組表ビューは自分の設定なので、視聴者も含めて誰でも扱える (設計書 9.7)。
func viewer(ctx context.Context) (*auth.Principal, error) {
	principal := auth.PrincipalFrom(ctx)
	if principal == nil {
		return nil, NewError(CodeUnauthenticated, "login required")
	}

	return principal, nil
}

// guideView は、ログイン中のユーザーの番組表ビューを引く。他のユーザーのものは見つからないことにする。
func (r *Resolver) guideView(ctx context.Context, globalID string) (*sqlc.GuideView, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	viewID, err := parseGuideViewID(globalID)
	if err != nil {
		return nil, err
	}
	row, err := sqlc.New(r.db).GetGuideView(ctx, sqlc.GetGuideViewParams{ID: viewID, UserID: principal.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, NewError(CodeNotFound, "guide view not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the guide view: %w", err)
	}

	return &row, nil
}

// saveGuideView は、番組表ビューを作るか、viewID のものを置き換える。
func (r *Resolver) saveGuideView(ctx context.Context, viewID *uuid.UUID, in *model.GuideViewInput) (*model.GuideView, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := guideViewFromInput(in)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the guide view: %w", err)
	}

	// 番組表ビューは個人の設定なので、書いたら、同じユーザーのほかの端末に知らせる。
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		now := r.clock.Now()
		if viewID == nil {
			viewID = new(id.New())
			err := q.InsertGuideView(ctx, sqlc.InsertGuideViewParams{
				ID: *viewID, UserID: principal.UserID, Name: in.Name, Settings: encoded, HideEmptyServices: in.HideEmptyServices, Now: now,
			})
			if err != nil {
				return err
			}
		} else {
			n, err := q.UpdateGuideView(ctx, sqlc.UpdateGuideViewParams{
				ID: *viewID, UserID: principal.UserID, Name: in.Name, Settings: encoded, HideEmptyServices: in.HideEmptyServices, Now: now,
			})
			if err != nil {
				return err
			}
			if n == 0 {
				return NewError(CodeNotFound, "guide view not found")
			}
		}

		return usersettings.Touch(ctx, tx, r.clock, principal.UserID)
	})
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgUniqueViolation {
		return nil, NewError(CodeBadUserInput, "a guide view with the same name already exists")
	}
	if _, ok := errors.AsType[*Error](err); ok {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("failed to save the guide view: %w", err)
	}

	row, err := sqlc.New(r.db).GetGuideView(ctx, sqlc.GetGuideViewParams{ID: *viewID, UserID: principal.UserID})
	if err != nil {
		return nil, fmt.Errorf("failed to get the guide view: %w", err)
	}

	return toGuideView(&row)
}

// applyGuideView は、番組表に番組表ビューの条件を当てる。番組を絞り込み、サービスを並べ替え、番組のないサービスを隠す。
func (r *Resolver) applyGuideView(rows []*model.GuideRow, view *sqlc.GuideView) ([]*model.GuideRow, error) {
	var settings guideViewSettings
	if err := json.Unmarshal(view.Settings, &settings); err != nil {
		return nil, fmt.Errorf("failed to decode guide view %s: %w", view.ID, err)
	}
	matcher, err := match.Compile(settings.conditions(), r.location)
	if err != nil {
		return nil, fmt.Errorf("invalid guide view %s: %w", view.ID, err)
	}

	filtered := make([]*model.GuideRow, 0, len(rows))
	for _, row := range rows {
		programs := slices.DeleteFunc(row.Programs, func(p *model.Program) bool {
			return !matcher.Match(matchProgram(row.Service, p))
		})
		if view.HideEmptyServices && len(programs) == 0 {
			continue
		}
		filtered = append(filtered, &model.GuideRow{Service: row.Service, Programs: programs})
	}

	// ビューに並べたサービスを先に、その順で置く。残りは既定の並びのまま後ろに置く。
	order := map[string]int{}
	for i, serviceID := range settings.Services {
		order[serviceGlobalID(serviceID)] = i
	}
	slices.SortStableFunc(filtered, func(a, b *model.GuideRow) int {
		ia, okA := order[a.Service.ID]
		ib, okB := order[b.Service.ID]
		switch {
		case okA && okB:
			return ia - ib
		case okA:
			return -1
		case okB:
			return 1
		default:
			return 0
		}
	})

	return filtered, nil
}

// matchProgram は、番組表の番組を、照合する形にする。
func matchProgram(service *model.Service, p *model.Program) *match.Program {
	genres := make([]*match.Genre, 0, len(p.Genres))
	for _, g := range p.Genres {
		genres = append(genres, &match.Genre{Lv1: g.Lv1, Lv2: new(g.Lv2)})
	}

	return match.NewProgram(&match.ProgramInput{
		ServiceID:   p.ServiceKey,
		ChannelType: service.ChannelTypeKey,
		Name:        p.Name,
		Description: p.Description,
		Extended:    map[string]string{},
		Genres:      genres,
		StartAt:     p.StartAt,
		Duration:    p.EndAt.Sub(p.StartAt),
		IsFree:      p.IsFree,
	})
}

// deleteGuideView は、ユーザーの番組表ビューを消し、同じユーザーのほかの端末に知らせる。
func (r *Resolver) deleteGuideView(ctx context.Context, userID, viewID uuid.UUID) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		n, err := sqlc.New(tx).DeleteGuideView(ctx, sqlc.DeleteGuideViewParams{ID: viewID, UserID: userID})
		if err != nil {
			return fmt.Errorf("failed to delete the guide view: %w", err)
		}
		if n == 0 {
			return NewError(CodeNotFound, "guide view not found")
		}

		return usersettings.Touch(ctx, tx, r.clock, userID)
	})
}
