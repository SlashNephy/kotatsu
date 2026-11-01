package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/capture"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	// captureType は、Capture の ID に付ける型の名前。
	captureType = "Capture"
	// maxCaptures は、キャプチャを 1 回に返す件数の上限。
	maxCaptures = 200
)

// CaptureID は、キャプチャ captureID の GraphQL の ID を返す。キャプチャを受け取る API (captureapi) が、応答に使う。
func CaptureID(captureID uuid.UUID) string {
	return GlobalID(captureType, captureID.String())
}

// ParseRecordingID は、録画の GraphQL の ID を、録画の ID にする。録画の ID でなければエラーを返す。
// 画面は録画を GraphQL の ID で持つので、キャプチャを受け取る API (captureapi) が、撮った録画を引くのに使う。
func ParseRecordingID(globalID string) (uuid.UUID, error) {
	return parseTypedID(globalID, recordingType, "invalid recording ID")
}

// captureCursor は、キャプチャのページを送るカーソル。最後に返したキャプチャを残した日時と ID。
type captureCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        uuid.UUID `json:"id"`
}

// listCaptures は、ログイン中のユーザーのキャプチャを、残した新しい順に first 件まで返す。
func (r *Resolver) listCaptures(ctx context.Context, first int, after, keyword *string) (*model.CaptureConnection, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if first < 1 || first > maxCaptures {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("first must be between 1 and %d", maxCaptures))
	}
	// 次のページがあるかを知るため、1 件多く読む。
	listQuery := &query.CaptureListQuery{Keyword: "", Before: nil, Limit: int32(first + 1)}
	if keyword != nil {
		listQuery.Keyword = *keyword
	}
	if after != nil {
		var cursor captureCursor
		if err := DecodeCursor(*after, &cursor); err != nil {
			return nil, NewError(CodeBadUserInput, "invalid cursor")
		}
		listQuery.Before = &domain.CapturePosition{CreatedAt: cursor.CreatedAt, ID: cursor.ID}
	}
	captures, err := r.queries.ListCaptures(ctx, principal.UserID, listQuery)
	if err != nil {
		return nil, err
	}
	hasNext := len(captures) > first
	captures = captures[:min(len(captures), first)]
	edges := make([]*model.CaptureEdge, 0, len(captures))
	for _, c := range captures {
		cursor, err := EncodeCursor(captureCursor{CreatedAt: c.CreatedAt, ID: c.ID})
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.CaptureEdge{Cursor: cursor, Node: captureOf(c)})
	}
	info := &model.PageInfo{HasNextPage: hasNext, HasPreviousPage: after != nil, StartCursor: nil, EndCursor: nil}
	if len(edges) > 0 {
		info.StartCursor, info.EndCursor = &edges[0].Cursor, &edges[len(edges)-1].Cursor
	}

	return &model.CaptureConnection{Edges: edges, PageInfo: info}, nil
}

// capturesAvailable は、このサーバーでキャプチャを残せるかを返す。
func (r *Resolver) capturesAvailable(ctx context.Context) (bool, error) {
	if _, err := viewer(ctx); err != nil {
		return false, err
	}

	return r.captures.Available(), nil
}

// deleteCapture は、ログイン中のユーザーのキャプチャを、画像ごと消す。
func (r *Resolver) deleteCapture(ctx context.Context, globalID string) (string, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return "", err
	}
	captureID, err := parseTypedID(globalID, captureType, "invalid capture ID")
	if err != nil {
		return "", err
	}
	err = r.captures.Delete(ctx, r.db, principal.UserID, captureID)
	switch {
	case errors.Is(err, capture.ErrNotFound):
		return "", NewError(CodeNotFound, "capture not found")
	case errors.Is(err, capture.ErrUnavailable):
		return "", NewError(CodeConflict, "captures are unavailable because the data directory is not configured")
	case err != nil:
		return "", err
	}

	return globalID, nil
}

// captureOf は、キャプチャを GraphQL の型にする。
func captureOf(c *domain.Capture) *model.Capture {
	var recordingID *string
	if c.RecordingID != nil {
		recordingID = new(GlobalID(recordingType, c.RecordingID.String()))
	}
	var position *float64
	if c.Position != nil {
		position = new(c.Position.Seconds())
	}

	return &model.Capture{
		ID:          CaptureID(c.ID),
		ImageURL:    "/media/captures/" + c.ID.String(),
		ServiceName: c.ServiceName,
		ProgramName: c.ProgramName,
		Caption:     c.Caption,
		RecordingID: recordingID,
		Position:    position,
		BroadcastAt: c.BroadcastAt,
		TakenAt:     c.TakenAt,
		Size:        c.Size,
	}
}
