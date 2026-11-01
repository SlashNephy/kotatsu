package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/vikstrous/dataloadgen"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	serviceType = "Service"
	programType = "Program"
)

func serviceGlobalID(id int64) string {
	return GlobalID(serviceType, strconv.FormatInt(id, 10))
}

// ServiceLiveURL は、サービスのライブ視聴の画面のパスを返す。視聴予約の通知を押したときに開く (設計書 4.8)。
func ServiceLiveURL(serviceID int64) string {
	return "/live/" + serviceGlobalID(serviceID)
}

func programGlobalID(serviceID, eventID int64) string {
	return GlobalID(programType, strconv.FormatInt(serviceID, 10)+"."+strconv.FormatInt(eventID, 10))
}

// parseServiceGlobalID は、Service の ID から、Mirakurun のサービス ID を取り出す。
func parseServiceGlobalID(globalID string) (int64, error) {
	typeName, raw, err := ParseGlobalID(globalID)
	if err != nil || typeName != serviceType {
		return 0, NewError(CodeBadUserInput, "invalid service ID")
	}

	return parseServiceID(raw)
}

// parseServiceID は、Service の ID の中身から、Mirakurun のサービス ID を取り出す。
func parseServiceID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, NewError(CodeBadUserInput, "invalid service ID")
	}

	return id, nil
}

// parseProgramID は、Program の ID から、サービス ID と eventId を取り出す。
func parseProgramID(raw string) (int64, int64, error) {
	service, event, ok := strings.Cut(raw, ".")
	if !ok {
		return 0, 0, NewError(CodeBadUserInput, "invalid program ID")
	}
	serviceID, err := strconv.ParseInt(service, 10, 64)
	if err != nil {
		return 0, 0, NewError(CodeBadUserInput, "invalid program ID")
	}
	eventID, err := strconv.ParseInt(event, 10, 64)
	if err != nil {
		return 0, 0, NewError(CodeBadUserInput, "invalid program ID")
	}

	return serviceID, eventID, nil
}

func toService(row *domain.Service) *model.Service {
	var key *int
	if row.RemoteControlKeyID != nil {
		key = new(int(*row.RemoteControlKeyID))
	}

	var order *int
	if row.SortOrder != nil {
		order = new(int(*row.SortOrder))
	}

	return &model.Service{
		ID:                 serviceGlobalID(row.ID),
		NetworkID:          int(row.NetworkID),
		ServiceID:          int(row.Sid),
		Name:               row.Name,
		Channel:            row.Channel,
		RemoteControlKeyID: key,
		Hidden:             row.Hidden,
		SortOrder:          order,
		// ロゴの番号は 0 もある (NHK など) ので、nil かどうかで見る。
		HasLogo:        row.LogoID != nil,
		ChannelTypeKey: row.ChannelType,
	}
}

func toChannelType(row *domain.ChannelType) *model.ChannelType {
	return &model.ChannelType{Type: row.Type, DisplayName: row.DisplayName, SortOrder: int(row.SortOrder)}
}

func toProgram(row *domain.Program) (*model.Program, error) {
	extendedMap := map[string]string{}
	if err := json.Unmarshal(row.Extended, &extendedMap); err != nil {
		return nil, fmt.Errorf("failed to decode the extended of program %d/%d: %w", row.ServiceID, row.EventID, err)
	}
	extended := make([]*model.ExtendedItem, 0, len(extendedMap))
	for _, heading := range slices.Sorted(maps.Keys(extendedMap)) {
		extended = append(extended, &model.ExtendedItem{Heading: heading, Text: extendedMap[heading]})
	}
	upstreamGenres := []*domain.Genre{}
	if err := json.Unmarshal(row.Genres, &upstreamGenres); err != nil {
		return nil, fmt.Errorf("failed to decode the genres of program %d/%d: %w", row.ServiceID, row.EventID, err)
	}
	genres := make([]*model.Genre, 0, len(upstreamGenres))
	for _, g := range upstreamGenres {
		genres = append(genres, &model.Genre{Lv1: g.Lv1, Lv2: g.Lv2})
	}
	audios, err := toProgramAudios(row.Audios)
	if err != nil {
		return nil, fmt.Errorf("failed to decode the audios of program %d/%d: %w", row.ServiceID, row.EventID, err)
	}
	sharedFrom, err := domain.SharedFrom(row.ServiceID, row.RelatedItems)
	if err != nil {
		return nil, fmt.Errorf("failed to decode the related items of program %d/%d: %w", row.ServiceID, row.EventID, err)
	}

	return &model.Program{
		ID:            programGlobalID(row.ServiceID, row.EventID),
		EventID:       int(row.EventID),
		StartAt:       row.StartAt.UTC(),
		EndAt:         row.EndAt.UTC(),
		EndUndecided:  row.EndUndecided,
		IsFree:        row.IsFree,
		Name:          row.Name,
		Description:   row.Description,
		Extended:      extended,
		Genres:        genres,
		Audios:        audios,
		Archived:      false,
		ServiceKey:    row.ServiceID,
		SharedFromKey: sharedFrom,
	}, nil
}

// toProgramAudios は、上流の JSON のままの音声の構成を、主音声が先になるよう並べて返す。
// mirakc は、主音声でない音声を先に返すことがある。
func toProgramAudios(raw []byte) ([]*model.ProgramAudio, error) {
	upstream := []*domain.Audio{}
	if err := json.Unmarshal(raw, &upstream); err != nil {
		return nil, err
	}
	audios := make([]*model.ProgramAudio, 0, len(upstream))
	for _, a := range upstream {
		languages := a.Langs
		if languages == nil {
			languages = []string{}
		}
		audios = append(audios, &model.ProgramAudio{
			ComponentType: a.ComponentType, IsMain: a.IsMain, Languages: languages, SamplingRate: a.SamplingRate,
		})
	}
	slices.SortStableFunc(audios, func(a, b *model.ProgramAudio) int {
		switch {
		case a.IsMain == b.IsMain:
			return 0
		case a.IsMain:
			return -1
		default:
			return 1
		}
	})

	return audios, nil
}

// loadServices は、サービスを keys の順に引く。消えたサービスは飛ばす。
func (r *Resolver) loadServices(ctx context.Context, keys []int64) ([]*model.Service, error) {
	rows, err := loadersFrom(ctx).Services.LoadAll(ctx, keys)
	var errs dataloadgen.ErrorSlice
	if err != nil && !errors.As(err, &errs) {
		return nil, err
	}
	services := make([]*model.Service, 0, len(rows))
	for i, row := range rows {
		if row == nil || (i < len(errs) && errs[i] != nil) {
			continue
		}
		services = append(services, toService(row))
	}

	return services, nil
}

// addArchivedPrograms は、時間帯 [from, to) に重なる過去の番組を、サービスごとの番組に足す (#1121、設計書 3.4)。
// 放送が終わってから猶予の間は、同じ番組が programs にも残っているので、programs にある番組 (サービス, eventId, 開始時刻) は足さない。
// 足した後は、開始時刻の順に並べ直す。
func (r *Resolver) addArchivedPrograms(ctx context.Context, byService map[int64]*model.GuideRow, serviceIDs []int64, from, to time.Time) error {
	archives, err := r.queries.ListArchivedProgramsInRange(ctx, query.ListProgramsInRangeParams{ServiceIDs: serviceIDs, FromAt: from, ToAt: to})
	if err != nil {
		return fmt.Errorf("failed to list archived programs: %w", err)
	}
	if len(archives) == 0 {
		return nil
	}
	type key struct {
		eventID int64
		startAt int64
	}
	current := map[int64]map[key]bool{}
	for serviceID, row := range byService {
		current[serviceID] = map[key]bool{}
		for _, program := range row.Programs {
			current[serviceID][key{eventID: int64(program.EventID), startAt: program.StartAt.Unix()}] = true
		}
	}
	for i := range archives {
		archive := &archives[i]
		if current[archive.ServiceID][key{eventID: archive.EventID, startAt: archive.StartAt.Unix()}] {
			continue
		}
		program, err := toProgram(archive)
		if err != nil {
			return err
		}
		program.ID = archiveGlobalID(archive.ServiceID, archive.EventID, archive.StartAt)
		program.Archived = true
		byService[archive.ServiceID].Programs = append(byService[archive.ServiceID].Programs, program)
	}
	for _, row := range byService {
		slices.SortStableFunc(row.Programs, func(a, b *model.Program) int { return a.StartAt.Compare(b.StartAt) })
	}

	return nil
}

// oldestArchivedProgramAt は、最も古い過去の番組の開始時刻を返す。過去の番組がなければ ok が false になる。
func (r *Resolver) oldestArchivedProgramAt(ctx context.Context) (time.Time, bool, error) {
	startAt, err := r.queries.GetOldestArchivedProgramStartAt(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to get the oldest archived program: %w", err)
	}

	return startAt, true, nil
}

// programRecordings は、番組の放送時間に重なる、番組の録画を返す (#1121)。
func (r *Resolver) programRecordings(ctx context.Context, program *model.Program) ([]*model.Recording, error) {
	rows, err := r.queries.ListProgramRecordings(ctx, program.ServiceKey, int64(program.EventID), program.StartAt, program.EndAt)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recordings of the program: %w", err)
	}

	return r.recordingModels(ctx, rows)
}
