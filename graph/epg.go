package graph

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

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

	return &model.Service{
		ID:                 serviceGlobalID(row.ID),
		NetworkID:          int(row.NetworkID),
		ServiceID:          int(row.Sid),
		Name:               row.Name,
		Channel:            row.Channel,
		RemoteControlKeyID: key,
		Hidden:             row.Hidden,
		ChannelTypeKey:     row.ChannelType,
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

	return &model.Program{
		ID:           programGlobalID(row.ServiceID, row.EventID),
		EventID:      int(row.EventID),
		StartAt:      row.StartAt.UTC(),
		EndAt:        row.EndAt.UTC(),
		EndUndecided: row.EndUndecided,
		IsFree:       row.IsFree,
		Name:         row.Name,
		Description:  row.Description,
		Extended:     extended,
		Genres:       genres,
		Archived:     false,
		ServiceKey:   row.ServiceID,
	}, nil
}
