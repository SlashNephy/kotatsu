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

	"github.com/vikstrous/dataloadgen"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	serviceType = "Service"
	programType = "Program"
	// maxGuideRange は、番組表で一度に取得できる時間帯の長さの上限。
	maxGuideRange = 7 * 24 * time.Hour
)

func serviceGlobalID(id int64) string {
	return GlobalID(serviceType, strconv.FormatInt(id, 10))
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

func toService(row *sqlc.ListGuideServicesRow) *model.Service {
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

func toChannelType(row *sqlc.ChannelType) *model.ChannelType {
	return &model.ChannelType{Type: row.Type, DisplayName: row.DisplayName, SortOrder: int(row.SortOrder)}
}

// programRow は、番組の列。現役の番組とアーカイブの番組で同じ形にする。
type programRow struct {
	ServiceID    int64
	EventID      int64
	StartAt      time.Time
	EndAt        time.Time
	EndUndecided bool
	IsFree       bool
	Name         string
	Description  string
	Extended     []byte
	Genres       []byte
	Audios       []byte
	RelatedItems []byte
}

func toProgram(row *programRow) (*model.Program, error) {
	extendedMap := map[string]string{}
	if err := json.Unmarshal(row.Extended, &extendedMap); err != nil {
		return nil, fmt.Errorf("failed to decode the extended of program %d/%d: %w", row.ServiceID, row.EventID, err)
	}
	extended := make([]*model.ExtendedItem, 0, len(extendedMap))
	for _, heading := range slices.Sorted(maps.Keys(extendedMap)) {
		extended = append(extended, &model.ExtendedItem{Heading: heading, Text: extendedMap[heading]})
	}
	upstreamGenres := []*mirakurun.Genre{}
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
	sharedFrom, err := sharedFromKey(row.ServiceID, row.RelatedItems)
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

// serviceKeyBase は、Mirakurun のサービス ID の、ネットワーク ID の桁の重み (network_id * 100000 + service_id)。
const serviceKeyBase = 100000

// sharedFromKey は、イベント共有の元のサービスの Mirakurun のサービス ID を返す。番組が元であるか、共有していなければ 0 を返す。
// 元は、同じネットワークで共有しているサービス (自分を含む) のうち、サービス ID が最も小さいものにする。
// 地上波では、メインチャンネルの番組は共有するサブチャンネルを全て挙げ、サブチャンネルの番組はメインチャンネルだけを挙げる。
func sharedFromKey(serviceKey int64, raw []byte) (int64, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	var related []*mirakurun.RelatedItem
	if err := json.Unmarshal(raw, &related); err != nil {
		return 0, err
	}
	networkID := serviceKey / serviceKeyBase
	from := serviceKey
	for _, item := range related {
		if item.Type != "shared" || (item.NetworkID != 0 && int64(item.NetworkID) != networkID) {
			continue
		}
		from = min(from, networkID*serviceKeyBase+int64(item.ServiceID))
	}
	if from == serviceKey {
		return 0, nil
	}

	return from, nil
}

// toProgramAudios は、上流の JSON のままの音声の構成を、主音声が先になるよう並べて返す。
// mirakc は、主音声でない音声を先に返すことがある。
func toProgramAudios(raw []byte) ([]*model.ProgramAudio, error) {
	upstream := []*mirakurun.Audio{}
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
