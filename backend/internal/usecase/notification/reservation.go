package notification

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// ReservationRule は、通知に載せる、予約のもとになったルール。
type ReservationRule struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ReservationContext は、予約にまつわる通知の受け取り手と、通知に載せる予約の種類とルール。
type ReservationContext struct {
	// Audience は、通知を受け取るユーザー。nil なら全員に届ける。
	Audience *Audience
	// Kind は、予約の種類 (program、time、rule)。予約が消えていれば空。
	Kind string
	// Rules は、ルール予約のもとになったルール。ルールを作った順に並べる。
	Rules []ReservationRule
}

// LoadReservationContext は、予約 reservationID の通知を受け取るユーザーを決める (設計書 6.2)。
//
// 手動の予約 (番組予約と時刻予約) は、予約したユーザーだけに届ける。ルール予約は、一致したルールを作ったユーザーに届け、
// ルール予約の録画の通知を受け取らない設定のユーザーには届けない。予約したユーザーやルールを作ったユーザーが分からなければ
// (取り込んだ予約や、ユーザーを消したとき)、全員に届ける。
func LoadReservationContext(ctx context.Context, db sqlc.DBTX, reservationID uuid.UUID) (*ReservationContext, error) {
	row, err := sqlc.New(db).GetReservationAudience(ctx, reservationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return &ReservationContext{Audience: nil, Kind: "", Rules: []ReservationRule{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the reservation audience: %w", err)
	}
	rules := make([]ReservationRule, 0, len(row.RuleIds))
	for i, ruleID := range row.RuleIds {
		rules = append(rules, ReservationRule{ID: ruleID.String(), Name: row.RuleNames[i]})
	}
	result := &ReservationContext{Audience: nil, Kind: row.Kind, Rules: rules}
	switch {
	case row.Kind == "rule":
		var users []uuid.UUID
		if len(row.RuleCreators) > 0 {
			users = row.RuleCreators
		}
		result.Audience = &Audience{UserIDs: users, ByRule: true}
	case row.CreatedBy != nil:
		result.Audience = &Audience{UserIDs: []uuid.UUID{*row.CreatedBy}, ByRule: false}
	}

	return result, nil
}

// AddTo は、予約の種類とルールを、通知の payload に載せる。Webhook でも受け取れるようにする。
func (c *ReservationContext) AddTo(payload map[string]any) {
	if c.Kind != "" {
		payload["reservationKind"] = c.Kind
	}
	payload["rules"] = c.Rules
}
