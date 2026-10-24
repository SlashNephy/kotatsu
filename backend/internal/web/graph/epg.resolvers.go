package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/web/graph/generated"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// Service is the resolver for the service field.
func (r *programResolver) Service(ctx context.Context, obj *model.Program) (*model.Service, error) {
	row, err := loadersFrom(ctx).Services.Load(ctx, obj.ServiceKey)
	if err != nil {
		return nil, err
	}

	return toService(row), nil
}

// SharedFrom is the resolver for the sharedFrom field.
func (r *programResolver) SharedFrom(ctx context.Context, obj *model.Program) (*model.Service, error) {
	var row *sqlc.ListGuideServicesRow
	var err error
	if obj.SharedFromKey != 0 {
		row, err = loadersFrom(ctx).Services.Load(ctx, obj.SharedFromKey)
	}
	// 元のサービスがまだ同期されていなければ、共有していないものとして扱う。
	if e, ok := errors.AsType[*Error](err); ok && e.Code == CodeNotFound {
		row, err = nil, nil
	}
	if err != nil || row == nil {
		return nil, err
	}

	return toService(row), nil
}

// Node is the resolver for the node field.
func (r *queryResolver) Node(ctx context.Context, id string) (model.Node, error) {
	typeName, raw, err := ParseGlobalID(id)
	if err != nil {
		return nil, NewError(CodeBadUserInput, "invalid ID")
	}

	switch typeName {
	case userType:
		userID, err := uuid.Parse(raw)
		if err != nil {
			return nil, NewError(CodeBadUserInput, "invalid user ID")
		}
		user, err := loadersFrom(ctx).Users.Load(ctx, userID)
		if err != nil {
			return nil, err
		}

		return toUser(user), nil
	case recordingType:
		recordingID, err := uuid.Parse(raw)
		if err != nil {
			return nil, NewError(CodeBadUserInput, "invalid recording ID")
		}

		return r.recording(ctx, recordingID)
	case serviceType:
		serviceID, err := parseServiceID(raw)
		if err != nil {
			return nil, err
		}
		row, err := loadersFrom(ctx).Services.Load(ctx, serviceID)
		if err != nil {
			return nil, err
		}

		return toService(row), nil
	case programType:
		serviceID, eventID, err := parseProgramID(raw)
		if err != nil {
			return nil, err
		}
		row, err := sqlc.New(r.db).GetGuideProgram(ctx, sqlc.GetGuideProgramParams{ServiceID: serviceID, EventID: eventID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, NewError(CodeNotFound, "program not found")
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get the program: %w", err)
		}

		return toProgram((*programRow)(&row))
	case archiveType:
		return r.archivedProgram(ctx, raw)
	case reservationType:
		return r.reservation(ctx, id)
	case ruleType:
		return r.rule(ctx, id)
	case guideViewType:
		view, err := r.guideView(ctx, id)
		if err != nil {
			return nil, err
		}

		return toGuideView(view)
	default:
		return nil, NewError(CodeBadUserInput, "unknown type "+typeName)
	}
}

// ChannelTypes is the resolver for the channelTypes field.
func (r *queryResolver) ChannelTypes(ctx context.Context) ([]*model.ChannelType, error) {
	rows, err := sqlc.New(r.db).ListChannelTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list channel types: %w", err)
	}
	types := make([]*model.ChannelType, 0, len(rows))
	for i := range rows {
		types = append(types, toChannelType(&rows[i]))
	}

	return types, nil
}

// Services is the resolver for the services field.
func (r *queryResolver) Services(ctx context.Context, includeHidden bool) ([]*model.Service, error) {
	// 番組表やライブ視聴に出す一覧では、ワンセグやデータ放送のサービスを外す。非表示を選ぶ管理の画面では、すべて返す。
	mediaTypes := epg.MediaServiceTypes()
	if includeHidden {
		mediaTypes = []int64{}
	}
	rows, err := sqlc.New(r.db).ListGuideServices(ctx, sqlc.ListGuideServicesParams{
		IncludeHidden:     includeHidden,
		MediaServiceTypes: mediaTypes,
		Ids:               []int64{},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	services := make([]*model.Service, 0, len(rows))
	for i := range rows {
		services = append(services, toService(&rows[i]))
	}

	return services, nil
}

// Guide is the resolver for the guide field.
func (r *queryResolver) Guide(ctx context.Context, from time.Time, to time.Time, services []string, view *string) ([]*model.GuideRow, error) {
	var guideView *sqlc.GuideView
	if view != nil {
		var err error
		if guideView, err = r.guideView(ctx, *view); err != nil {
			return nil, err
		}
	}
	if !from.Before(to) || to.Sub(from) > epg.MaxGuideRange {
		return nil, NewError(CodeBadUserInput, "from must be before to, and the range must be at most 7 days")
	}
	// 空の一覧を指定したときは、どのサービスも選ばれていないので、何も返さない。省いたときとは区別する。
	if services != nil && len(services) == 0 {
		return []*model.GuideRow{}, nil
	}
	ids := make([]int64, 0, len(services))
	for _, globalID := range services {
		id, err := parseServiceGlobalID(globalID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	q := sqlc.New(r.db)
	// サービスを指定したときは、非表示のサービスも、ワンセグやデータ放送のサービスも返す。
	mediaTypes := epg.MediaServiceTypes()
	if len(ids) > 0 {
		mediaTypes = []int64{}
	}
	serviceRows, err := q.ListGuideServices(ctx, sqlc.ListGuideServicesParams{
		IncludeHidden:     len(ids) > 0,
		MediaServiceTypes: mediaTypes,
		Ids:               ids,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	rows := make([]*model.GuideRow, 0, len(serviceRows))
	byService := make(map[int64]*model.GuideRow, len(serviceRows))
	keys := make([]int64, 0, len(serviceRows))
	for i := range serviceRows {
		row := &model.GuideRow{Service: toService(&serviceRows[i]), Programs: []*model.Program{}}
		rows = append(rows, row)
		byService[serviceRows[i].ID] = row
		keys = append(keys, serviceRows[i].ID)
	}

	programs, err := q.ListProgramsInRange(ctx, sqlc.ListProgramsInRangeParams{ServiceIds: keys, FromAt: from, ToAt: to})
	if err != nil {
		return nil, fmt.Errorf("failed to list programs: %w", err)
	}
	for i := range programs {
		program, err := toProgram((*programRow)(&programs[i]))
		if err != nil {
			return nil, err
		}
		byService[programs[i].ServiceID].Programs = append(byService[programs[i].ServiceID].Programs, program)
	}
	if guideView != nil {
		return r.applyGuideView(rows, guideView)
	}

	return rows, nil
}

// ChannelType is the resolver for the channelType field.
func (r *serviceResolver) ChannelType(ctx context.Context, obj *model.Service) (*model.ChannelType, error) {
	row, err := loadersFrom(ctx).ChannelTypes.Load(ctx, obj.ChannelTypeKey)
	if err != nil {
		return nil, err
	}

	return toChannelType(row), nil
}

// Jikkyo is the resolver for the jikkyo field.
func (r *serviceResolver) Jikkyo(ctx context.Context, obj *model.Service) (*model.JikkyoChannel, error) {
	return r.serviceJikkyo(ctx, obj), nil
}

// VideoFormat is the resolver for the videoFormat field.
func (r *serviceResolver) VideoFormat(ctx context.Context, obj *model.Service) (model.VideoFormat, error) {
	return r.serviceVideoFormat(ctx, obj)
}

// Program returns generated.ProgramResolver implementation.
func (r *Resolver) Program() generated.ProgramResolver { return &programResolver{r} }

// Service returns generated.ServiceResolver implementation.
func (r *Resolver) Service() generated.ServiceResolver { return &serviceResolver{r} }

type (
	programResolver struct{ *Resolver }
	serviceResolver struct{ *Resolver }
)
