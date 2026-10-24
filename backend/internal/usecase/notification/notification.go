// Package notification は、ユーザーに届ける通知を扱う (設計書 6.2)。
//
// 各ロールは、出来事を書くトランザクションの中で Emit を呼び、通知するイベントを記録する。
// nekonote の配信のジョブ (Dispatcher) が、イベントを、受け取るユーザーごとの画面内の通知に配る。
// ユーザーは、イベントの種類ごとに、どの通知先で受け取るかを設定できる。
//
// ロール間の変更の通知 (notify パッケージ) とは別のもので、こちらは人に届ける。
// 画面内の通知が増えたことや既読にしたことは、notify の NotifyChannel で tenban に知らせ、subscription で届ける。
package notification

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
)

// NotifyChannel は、ユーザーの画面内の通知が増えたか、既読にしたときに、変更を通知するチャンネル (設計書 1.4)。
const NotifyChannel = "notifications"

// Audience は、イベントを受け取るユーザーの絞り込み。
type Audience struct {
	// UserIDs は、受け取るユーザー。空なら、有効なユーザー全員に届ける。
	UserIDs []uuid.UUID
	// ByRule は、ルール予約の録画の通知かどうか。ルール予約の録画の通知を受け取らない設定のユーザーには届けない。
	ByRule bool
}

// Emit は、有効なユーザー全員に届ける通知するイベントを記録する。出来事を書くトランザクションの中で呼ぶ。
// payload は、画面に出す文の材料 (番組名や理由など) で、JSON にする。
func Emit(ctx context.Context, db sqlc.DBTX, clk clock.Clock, kind domain.NotificationKind, payload map[string]any) error {
	return EmitTo(ctx, db, clk, kind, payload, nil)
}

// EmitTo は、audience に届ける通知するイベントを記録する。audience が nil なら、Emit と同じく全員に届ける。
func EmitTo(ctx context.Context, db sqlc.DBTX, clk clock.Clock, kind domain.NotificationKind, payload map[string]any, audience *Audience) error {
	if _, err := domain.ParseNotificationKind(string(kind)); err != nil {
		return err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode the notification payload: %w", err)
	}
	params := sqlc.InsertNotificationEventParams{
		ID: id.New(), Kind: string(kind), OccurredAt: clk.Now(), Payload: encoded, Audience: nil, ByRule: false,
	}
	if audience != nil {
		// 空のスライスは全員に届けるのと同じにする。そのまま渡すと、空の配列として誰にも届かない。
		if len(audience.UserIDs) > 0 {
			params.Audience = audience.UserIDs
		}
		params.ByRule = audience.ByRule
	}
	if err := sqlc.New(db).InsertNotificationEvent(ctx, params); err != nil {
		return fmt.Errorf("failed to record the notification event: %w", err)
	}

	return nil
}
