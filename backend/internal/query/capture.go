package query

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// ErrInvalidCaptureLimit は、キャプチャの一覧の件数の上限が 1 未満であることを表す。
var ErrInvalidCaptureLimit = errors.New("the capture limit must be at least 1")

// CaptureListQuery は、キャプチャの一覧の条件。
type CaptureListQuery struct {
	// Keyword は、探す言葉。空白で区切ると、すべての語を、番組名かサービス名に含むものだけを返す。空なら絞らない。
	Keyword string
	// Before は、これより前 (古い) のキャプチャだけを返すためのカーソル。nil なら最も新しいものから返す。
	Before *domain.CapturePosition
	// Limit は、返す件数の上限。1 以上にする。
	Limit int32
}

// GetCapture は、ユーザー userID のキャプチャ captureID を返す。ほかのユーザーのものは、ないものとして pgx.ErrNoRows を返す。
func (q *Queries) GetCapture(ctx context.Context, userID, captureID uuid.UUID) (*domain.Capture, error) {
	row, err := sqlc.New(q.pool).GetCapture(ctx, sqlc.GetCaptureParams{ID: captureID, UserID: userID})
	if err != nil {
		return nil, err
	}

	return CaptureFromRow(&row), nil
}

// ListCaptures は、ユーザー userID のキャプチャを、記録した新しい順に返す (設計書 7.5)。
// 探す言葉は、録画の検索と同じく NFKC で正規化し、大文字と小文字を区別せずに当てる (設計書 3.5)。
func (q *Queries) ListCaptures(ctx context.Context, userID uuid.UUID, query *CaptureListQuery) ([]*domain.Capture, error) {
	if query.Limit < 1 {
		return nil, ErrInvalidCaptureLimit
	}
	words := []string{}
	for word := range strings.FieldsSeq(domain.Normalize(query.Keyword)) {
		words = append(words, escapeLike(word))
	}
	params := sqlc.ListCapturesParams{UserID: userID, Words: words, BeforeAt: nil, BeforeID: nil, MaxCount: query.Limit}
	if query.Before != nil {
		params.BeforeAt, params.BeforeID = &query.Before.CreatedAt, &query.Before.ID
	}
	rows, err := sqlc.New(q.pool).ListCaptures(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list captures: %w", err)
	}
	captures := make([]*domain.Capture, 0, len(rows))
	for i := range rows {
		captures = append(captures, CaptureFromRow(&rows[i]))
	}

	return captures, nil
}

// CaptureFromRow は、captures の行を domain の型にする。
func CaptureFromRow(row *sqlc.Capture) *domain.Capture {
	var position *time.Duration
	if row.PositionMs != nil {
		position = new(time.Duration(*row.PositionMs) * time.Millisecond)
	}

	return &domain.Capture{
		ID:          row.ID,
		UserID:      row.UserID,
		RecordingID: row.RecordingID,
		Position:    position,
		ServiceName: row.ServiceName,
		ProgramName: row.ProgramName,
		Caption:     row.Caption,
		BroadcastAt: row.BroadcastAt,
		TakenAt:     row.TakenAt,
		Size:        row.Size,
		CreatedAt:   row.CreatedAt,
	}
}

// escapeLike は、LIKE の特別な文字 (%、_、\) を、文字そのものとして探すようにエスケープする。
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
