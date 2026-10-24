// Package audit は監査ログ (audit_events) を扱う (設計書 8.5)。
//
// 各ドメインは、記録したい操作を書き込むトランザクションの中で Recorder.Record を呼ぶ。
// 操作が失敗してロールバックすれば、監査ログも残らない。
// 保持期間を過ぎた記録は、nekonote が River の定期ジョブ (PruneWorker) で消す。
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/notify"
)

// ActorType は操作者の種類。
type ActorType string

const (
	// ActorTypeUser はユーザーの操作。
	ActorTypeUser ActorType = "user"
	// ActorTypeSystem はシステムのロールの操作 (neko による自動予約など)。
	ActorTypeSystem ActorType = "system"
)

// Actor は操作者。User か System で作る。
type Actor struct {
	Type ActorType
	// UserID は、Type が ActorTypeUser のときの操作したユーザー。
	UserID uuid.UUID
	// Role は、Type が ActorTypeSystem のときの操作したロールの名前 (neko など)。
	Role string
	// SourceIP と UserAgent は、ユーザーの操作の送信元。分からなければ空にする。
	SourceIP  netip.Addr
	UserAgent string
	// Client は、MCP のクライアントを経由した操作で、クライアントの ID (#964)。ほかの操作では空。
	Client string
}

// User は、ユーザーの操作者を返す。
func User(userID uuid.UUID) *Actor {
	return &Actor{Type: ActorTypeUser, UserID: userID, Role: "", SourceIP: netip.Addr{}, UserAgent: "", Client: ""}
}

// UserFrom は、送信元の分かるユーザーの操作者を返す。
func UserFrom(userID uuid.UUID, sourceIP netip.Addr, userAgent string) *Actor {
	return &Actor{Type: ActorTypeUser, UserID: userID, Role: "", SourceIP: sourceIP, UserAgent: userAgent, Client: ""}
}

// System は、システムのロールの操作者を返す。
func System(role string) *Actor {
	return &Actor{Type: ActorTypeSystem, UserID: uuid.Nil, Role: role, SourceIP: netip.Addr{}, UserAgent: "", Client: ""}
}

// Event は、記録する操作。
type Event struct {
	Actor *Actor
	// Action は操作の種類 (rule.create、user.login_failed など)。
	Action string
	// TargetType と TargetID は対象の種類と ID。対象のない操作 (ログインの失敗など) では TargetID を空にする。
	TargetType string
	TargetID   string
	// Summary は変更の要約。JSON にして保存する。nil なら空のオブジェクトにする。
	Summary any
	// SourceIP と UserAgent は送信元。システムの操作では空にする。空なら Actor の送信元を使う。
	SourceIP  netip.Addr
	UserAgent string
}

// Recorder は監査ログを記録する。
type Recorder interface {
	// Record は event を db に記録する。操作を書き込むトランザクションを db に渡す。
	Record(ctx context.Context, db sqlc.DBTX, event *Event) error
}

// DBRecorder は、audit_events に記録する Recorder。
type DBRecorder struct {
	clock clock.Clock
}

var _ Recorder = new(DBRecorder)

// NewRecorder は DBRecorder を作る。記録の日時は clk から取る。
func NewRecorder(clk clock.Clock) *DBRecorder {
	return &DBRecorder{clock: clk}
}

// ErrInvalidEvent は、記録できない event を渡されたことを表す。
var ErrInvalidEvent = errors.New("invalid audit event")

func (r *DBRecorder) Record(ctx context.Context, db sqlc.DBTX, event *Event) error {
	params, err := r.params(event)
	if err != nil {
		return err
	}
	if err := sqlc.New(db).InsertAuditEvent(ctx, params); err != nil {
		return fmt.Errorf("failed to record the audit event %s: %w", event.Action, err)
	}
	// 管理の画面に、読み直しの合図を送る (#869)。
	if _, err := notify.Publish(ctx, db, r.clock, notify.AuditChannel); err != nil {
		return err
	}

	return nil
}

func (r *DBRecorder) params(event *Event) (sqlc.InsertAuditEventParams, error) {
	var params sqlc.InsertAuditEventParams
	if event == nil {
		return params, fmt.Errorf("%w: the event is required", ErrInvalidEvent)
	}
	if event.Actor == nil {
		return params, fmt.Errorf("%w: the actor is required", ErrInvalidEvent)
	}
	if event.Action == "" || event.TargetType == "" {
		return params, fmt.Errorf("%w: the action and the target type are required", ErrInvalidEvent)
	}

	params = sqlc.InsertAuditEventParams{
		ID:          id.New(),
		OccurredAt:  r.clock.Now(),
		ActorType:   string(event.Actor.Type),
		ActorUserID: nil,
		ActorRole:   nil,
		Action:      event.Action,
		TargetType:  event.TargetType,
		TargetID:    nil,
		Summary:     nil,
		SourceIP:    nil,
		UserAgent:   nil,
		ActorClient: nil,
	}
	if event.Actor.Client != "" {
		params.ActorClient = &event.Actor.Client
	}

	switch event.Actor.Type {
	case ActorTypeUser:
		if event.Actor.UserID == uuid.Nil {
			return params, fmt.Errorf("%w: the user ID of the actor is required", ErrInvalidEvent)
		}
		params.ActorUserID = &event.Actor.UserID
	case ActorTypeSystem:
		if event.Actor.Role == "" {
			return params, fmt.Errorf("%w: the role of the actor is required", ErrInvalidEvent)
		}
		params.ActorRole = &event.Actor.Role
	default:
		return params, fmt.Errorf("%w: unknown actor type %q", ErrInvalidEvent, event.Actor.Type)
	}

	summary := event.Summary
	if summary == nil {
		summary = map[string]any{}
	}
	b, err := json.Marshal(summary)
	if err != nil {
		return params, fmt.Errorf("%w: failed to encode the summary: %w", ErrInvalidEvent, err)
	}
	params.Summary = b

	// 空の値は NULL にする。
	if event.TargetID != "" {
		params.TargetID = new(event.TargetID)
	}
	sourceIP, userAgent := event.SourceIP, event.UserAgent
	if !sourceIP.IsValid() {
		sourceIP = event.Actor.SourceIP
	}
	if userAgent == "" {
		userAgent = event.Actor.UserAgent
	}
	if sourceIP.IsValid() {
		params.SourceIP = &sourceIP
	}
	if userAgent != "" {
		params.UserAgent = &userAgent
	}

	return params, nil
}
