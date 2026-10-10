// Package notification は、ユーザーに届ける通知を扱う (設計書 6.2)。
//
// 各ロールは、出来事を書くトランザクションの中で Emit を呼び、通知するイベントを記録する。
// nekonote の配信のジョブ (Dispatcher) が、イベントを、受け取るユーザーごとの画面内の通知に配る。
// ユーザーは、イベントの種類ごとに、どの通知先で受け取るかを設定できる。
//
// ロール間の変更の通知 (notify パッケージ) とは別のもので、こちらは人に届ける。
package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
)

// Kind は、通知するイベントの種類 (設計書 6.2)。
type Kind string

const (
	KindRecordingStarted       Kind = "recording_started"
	KindRecordingCompleted     Kind = "recording_completed"
	KindRecordingFailed        Kind = "recording_failed"
	KindReservationConflict    Kind = "reservation_conflict"
	KindStorageLow             Kind = "storage_low"
	KindTunerServerUnreachable Kind = "tuner_server_unreachable"
)

// ErrUnknownKind は、知らない種類のイベントであることを表す。
var ErrUnknownKind = errors.New("unknown notification kind")

// Kinds は、通知するイベントの種類を、画面に並べる順に返す。
func Kinds() []Kind {
	return []Kind{
		KindRecordingStarted, KindRecordingCompleted, KindRecordingFailed,
		KindReservationConflict, KindStorageLow, KindTunerServerUnreachable,
	}
}

// ParseKind は、文字列をイベントの種類にする。
func ParseKind(s string) (Kind, error) {
	kind := Kind(s)
	if !slices.Contains(Kinds(), kind) {
		return "", fmt.Errorf("%w: %q", ErrUnknownKind, s)
	}

	return kind, nil
}

// Emit は、通知するイベントを記録する。出来事を書くトランザクションの中で呼ぶ。
// payload は、画面に出す文の材料 (番組名や理由など) で、JSON にする。
func Emit(ctx context.Context, db sqlc.DBTX, clk clock.Clock, kind Kind, payload map[string]any) error {
	if _, err := ParseKind(string(kind)); err != nil {
		return err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode the notification payload: %w", err)
	}
	err = sqlc.New(db).InsertNotificationEvent(ctx, sqlc.InsertNotificationEventParams{
		ID: id.New(), Kind: string(kind), OccurredAt: clk.Now(), Payload: encoded,
	})
	if err != nil {
		return fmt.Errorf("failed to record the notification event: %w", err)
	}

	return nil
}
