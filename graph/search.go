package graph

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/match"
)

const (
	archiveType = "ProgramArchive"
	// defaultArchiveRange は、過去の番組の検索の期間の既定値 (設計書 3.4)。
	defaultArchiveRange = 30 * 24 * time.Hour
	// maxSearchResults は、一度に返す件数の上限。
	maxSearchResults = 100
	// searchBatchSize は、候補を DB から読む 1 回の件数。照合で落ちる候補が多いので、返す件数より多めに読む。
	searchBatchSize = 500
)

// searchCursor は、番組の検索のカーソルの位置。並べ替えのキーと同じ。
type searchCursor struct {
	StartAt   time.Time `json:"startAt"`
	ServiceID int64     `json:"serviceId"`
	EventID   int64     `json:"eventId"`
}

func archiveGlobalID(serviceID, eventID int64, startAt time.Time) string {
	return GlobalID(archiveType, strconv.FormatInt(serviceID, 10)+"."+strconv.FormatInt(eventID, 10)+"."+strconv.FormatInt(startAt.Unix(), 10))
}

// searchPrograms は、条件に一致する番組を、開始時刻の順に first 件まで返す (設計書 3.5)。
// 期間、チャンネル種別、サービスでの絞り込みは SQL で行い、キーワードなどの照合はルールの照合と同じ関数で行う。
func (r *Resolver) searchPrograms(
	ctx context.Context,
	in *model.ProgramConditionsInput,
	archive *model.ArchiveRangeInput,
	first int,
	after *string,
) (*model.ProgramConnection, error) {
	if first < 1 || first > maxSearchResults {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("first must be between 1 and %d", maxSearchResults))
	}
	conditions, err := toConditions(in)
	if err != nil {
		return nil, err
	}
	matcher, err := match.Compile(conditions, r.location)
	if err != nil {
		return nil, NewError(CodeBadUserInput, err.Error())
	}

	params, err := r.searchParams(conditions, archive, after)
	if err != nil {
		return nil, err
	}
	edges := []*model.ProgramEdge{}
	hasNext := false
	for {
		rows, err := sqlc.New(r.db).SearchProgramCandidates(ctx, *params)
		if err != nil {
			return nil, fmt.Errorf("failed to search programs: %w", err)
		}
		for i := range rows {
			row := &rows[i]
			params.AfterStartAt, params.AfterServiceID, params.AfterEventID = row.StartAt, row.ServiceID, row.EventID
			edge, matched, err := r.matchCandidate(matcher, row)
			if err != nil {
				return nil, err
			}
			if !matched {
				continue
			}
			if len(edges) == first {
				hasNext = true

				break
			}
			edges = append(edges, edge)
		}
		if hasNext || len(rows) < int(params.BatchSize) {
			break
		}
	}

	return &model.ProgramConnection{Edges: edges, PageInfo: pageInfo(edges, hasNext, after != nil)}, nil
}

// archivedProgram は、ProgramArchive の ID の中身 (サービス ID.eventId.開始時刻の UNIX 時間) から、アーカイブの番組を引く。
func (r *Resolver) archivedProgram(ctx context.Context, raw string) (*model.Program, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, NewError(CodeBadUserInput, "invalid archived program ID")
	}
	serviceID, eventID, err := parseProgramID(parts[0] + "." + parts[1])
	if err != nil {
		return nil, err
	}
	unix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, NewError(CodeBadUserInput, "invalid archived program ID")
	}
	row, err := sqlc.New(r.db).GetArchivedProgram(ctx, sqlc.GetArchivedProgramParams{ServiceID: serviceID, EventID: eventID, StartAt: time.Unix(unix, 0)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, NewError(CodeNotFound, "program not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the archived program: %w", err)
	}
	program, err := toProgram((*programRow)(&row))
	if err != nil {
		return nil, err
	}
	program.ID = archiveGlobalID(serviceID, eventID, row.StartAt)
	program.Archived = true

	return program, nil
}

// searchParams は、候補を引くクエリの引数を作る。
func (r *Resolver) searchParams(conditions *match.Conditions, archive *model.ArchiveRangeInput, after *string) (*sqlc.SearchProgramCandidatesParams, error) {
	params := &sqlc.SearchProgramCandidatesParams{
		IncludeArchive: archive != nil,
		ArchiveFrom:    time.Time{},
		ArchiveTo:      time.Time{},
		ChannelTypes:   conditions.ChannelTypes,
		ServiceIds:     conditions.ServiceIDs,
		// カーソルが無ければ、最も古い位置から読む。
		AfterStartAt:   time.Unix(0, 0).UTC(),
		AfterServiceID: 0,
		AfterEventID:   0,
		BatchSize:      searchBatchSize,
	}
	if archive != nil {
		now := r.clock.Now()
		params.ArchiveTo = now
		if archive.To != nil {
			params.ArchiveTo = *archive.To
		}
		params.ArchiveFrom = params.ArchiveTo.Add(-defaultArchiveRange)
		if archive.From != nil {
			params.ArchiveFrom = *archive.From
		}
		if !params.ArchiveFrom.Before(params.ArchiveTo) {
			return nil, NewError(CodeBadUserInput, "archive.from must be before archive.to")
		}
	}
	if after != nil {
		var cursor searchCursor
		if err := DecodeCursor(*after, &cursor); err != nil {
			return nil, NewError(CodeBadUserInput, "invalid cursor")
		}
		params.AfterStartAt, params.AfterServiceID, params.AfterEventID = cursor.StartAt, cursor.ServiceID, cursor.EventID
	}

	return params, nil
}

// matchCandidate は、候補が条件に一致するかを照合し、一致すれば一覧の要素にする。
func (r *Resolver) matchCandidate(matcher *match.Matcher, row *sqlc.SearchProgramCandidatesRow) (*model.ProgramEdge, bool, error) {
	program, err := toProgram(&programRow{
		ServiceID: row.ServiceID, EventID: row.EventID, StartAt: row.StartAt, EndAt: row.EndAt,
		EndUndecided: row.EndUndecided, IsFree: row.IsFree, Name: row.Name, Description: row.Description,
		Extended: row.Extended, Genres: row.Genres,
	})
	if err != nil {
		return nil, false, err
	}
	extended := map[string]string{}
	for _, item := range program.Extended {
		extended[item.Heading] = item.Text
	}
	genres := make([]*match.Genre, 0, len(program.Genres))
	for _, g := range program.Genres {
		genres = append(genres, &match.Genre{Lv1: g.Lv1, Lv2: new(g.Lv2)})
	}
	if !matcher.Match(match.NewProgram(&match.ProgramInput{
		ServiceID:   row.ServiceID,
		ChannelType: row.ChannelType,
		Name:        row.Name,
		Description: row.Description,
		Extended:    extended,
		Genres:      genres,
		StartAt:     row.StartAt,
		Duration:    row.EndAt.Sub(row.StartAt),
		IsFree:      row.IsFree,
	})) {
		return nil, false, nil
	}

	if row.Archived {
		program.ID = archiveGlobalID(row.ServiceID, row.EventID, row.StartAt)
		program.Archived = true
	}
	cursor, err := EncodeCursor(searchCursor{StartAt: row.StartAt, ServiceID: row.ServiceID, EventID: row.EventID})
	if err != nil {
		return nil, false, err
	}

	return &model.ProgramEdge{Cursor: cursor, Node: program}, true, nil
}

// pageInfo は、一覧のページの情報を作る。前のページは、カーソルを渡されたかどうかで判断する。
func pageInfo(edges []*model.ProgramEdge, hasNext, hasPrevious bool) *model.PageInfo {
	info := &model.PageInfo{HasNextPage: hasNext, HasPreviousPage: hasPrevious, StartCursor: nil, EndCursor: nil}
	if len(edges) > 0 {
		info.StartCursor = &edges[0].Cursor
		info.EndCursor = &edges[len(edges)-1].Cursor
	}

	return info
}
