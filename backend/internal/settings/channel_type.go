package settings

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

const (
	// maxDisplayNameLength は、チャンネル種別の表示名の長さの上限。
	maxDisplayNameLength = 32
	// maxSortOrder は、チャンネル種別の並び順の上限。既定値のない種別の並び順 (1000) より後ろにも置ける。
	maxSortOrder = 10000
	// channelTypeAuditTarget は、チャンネル種別の監査ログの対象の種類。
	channelTypeAuditTarget = "channel_type"
)

// ErrChannelTypeNotFound は、チャンネル種別が見つからないことを表す。
var ErrChannelTypeNotFound = errors.New("channel type not found")

// ChannelTypeChanges は、チャンネル種別の表示の変える項目。nil の項目は変えない。
type ChannelTypeChanges struct {
	DisplayName *string
	SortOrder   *int
}

// UpdateChannelType は、チャンネル種別 channelType の表示名と並び順を変え、変えた後の種別を返す (設計書 3.1、11.2)。
// 種別は、チューナーサーバーから EPG を取り込むときに足すので、ここでは足さない。
func (s *Store) UpdateChannelType(
	ctx context.Context, db Database, actor *audit.Actor, channelType string, changes *ChannelTypeChanges,
) (*sqlc.ChannelType, error) {
	if changes == nil {
		return nil, fmt.Errorf("%w: changes is nil", ErrInvalidSettings)
	}
	// 表示名の前後の空白は落とす。
	var name *string
	if changes.DisplayName != nil {
		trimmed := strings.TrimSpace(*changes.DisplayName)
		if trimmed == "" || utf8.RuneCountInString(trimmed) > maxDisplayNameLength {
			return nil, fmt.Errorf("%w: displayName must be 1 to %d characters", ErrInvalidSettings, maxDisplayNameLength)
		}
		name = &trimmed
	}
	var order *int32
	if changes.SortOrder != nil {
		stored, err := sortOrder(*changes.SortOrder)
		if err != nil {
			return nil, err
		}
		order = &stored
	}

	var updated sqlc.ChannelType
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		current, err := q.LockChannelType(ctx, channelType)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrChannelTypeNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to lock the channel type: %w", err)
		}
		params := sqlc.UpdateChannelTypeParams{Type: channelType, DisplayName: current.DisplayName, SortOrder: current.SortOrder}
		if name != nil {
			params.DisplayName = *name
		}
		if order != nil {
			params.SortOrder = *order
		}
		if updated, err = q.UpdateChannelType(ctx, params); err != nil {
			return fmt.Errorf("failed to update the channel type: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "channel_type.updated",
			TargetType: channelTypeAuditTarget,
			TargetID:   channelType,
			Summary: map[string]any{
				"displayName": change(current.DisplayName, updated.DisplayName),
				"sortOrder":   change(current.SortOrder, updated.SortOrder),
			},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP:  netip.Addr{},
			UserAgent: "",
		})
	})
	if err != nil {
		return nil, err
	}

	return &updated, nil
}

// sortOrder は、並び順が範囲にあるかを確かめ、DB に保存する形にする。
func sortOrder(order int) (int32, error) {
	if order < 0 || order > maxSortOrder {
		return 0, fmt.Errorf("%w: sortOrder must be between 0 and %d", ErrInvalidSettings, maxSortOrder)
	}

	return int32(order), nil
}
