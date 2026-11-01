package timeshift

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

const (
	// DefaultRetention は、チャンネルごとの保持時間の既定値 (付録 A)。
	DefaultRetention = 2 * time.Hour
	// MinRetention と MaxRetention は、保持時間の下限と上限。DB の制約と同じ。
	MinRetention = 10 * time.Minute
	MaxRetention = 24 * time.Hour
	// minRetentionSeconds と maxRetentionSeconds は、保持時間の下限と上限 (秒)。
	minRetentionSeconds = int(MinRetention / time.Second)
	maxRetentionSeconds = int(MaxRetention / time.Second)
	// channelAuditTarget は、タイムシフトのチャンネルの監査ログの対象の種類。ID はサービス ID。
	channelAuditTarget = "timeshift_channel"
)

var (
	// ErrInvalidRetention は、保持時間が範囲外であることを表す。
	ErrInvalidRetention = errors.New("invalid timeshift retention")
	// ErrServiceNotFound は、サービスが見つからないことを表す。
	ErrServiceNotFound = errors.New("service not found")
	// ErrChannelNotFound は、タイムシフトで録るチャンネルに、そのサービスがないことを表す。
	ErrChannelNotFound = errors.New("timeshift channel not found")
)

// Database は、トランザクションでの書き込みに使う DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Channels は、管理者が決める、タイムシフトで録るチャンネルと保持時間を変える。
// neko は、変えたチャンネルに、次の見直し (DefaultInterval) で気づく。
type Channels struct {
	clock    clock.Clock
	recorder audit.Recorder
}

// NewChannels は Channels を返す。
func NewChannels(clk clock.Clock, recorder audit.Recorder) *Channels {
	return &Channels{clock: clk, recorder: recorder}
}

// ValidateRetention は、保持時間 (秒) が下限と上限の間にあるかを確かめ、DB に書く値にする。範囲外なら ErrInvalidRetention を返す。
func ValidateRetention(retentionSeconds int) (int32, error) {
	if retentionSeconds < minRetentionSeconds || retentionSeconds > maxRetentionSeconds {
		return 0, fmt.Errorf("%w: retentionSeconds must be between %d and %d", ErrInvalidRetention, minRetentionSeconds, maxRetentionSeconds)
	}

	return int32(retentionSeconds), nil
}

// Set は、サービス serviceID をタイムシフトで録るチャンネルにし、保持時間を retentionSeconds にする。既に録っていれば、保持時間だけを変える。
func (c *Channels) Set(ctx context.Context, db Database, actor *audit.Actor, serviceID int64, retentionSeconds int) error {
	retention, err := ValidateRetention(retentionSeconds)
	if err != nil {
		return err
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		exists, err := q.ServiceExists(ctx, serviceID)
		if err != nil {
			return fmt.Errorf("failed to check the service: %w", err)
		}
		if !exists {
			return ErrServiceNotFound
		}
		var before *int32
		current, err := q.LockTimeshiftChannel(ctx, serviceID)
		switch {
		case err == nil:
			before = &current
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("failed to lock the timeshift channel: %w", err)
		}
		if err := q.UpsertTimeshiftChannel(ctx, sqlc.UpsertTimeshiftChannelParams{
			ServiceID: serviceID, RetentionSeconds: retention, Now: c.clock.Now(), UpdatedBy: userOf(actor),
		}); err != nil {
			return fmt.Errorf("failed to set the timeshift channel: %w", err)
		}

		return c.record(ctx, tx, actor, "timeshift_channel.updated", serviceID, map[string]any{
			"retentionSeconds": map[string]any{"from": before, "to": retention},
		})
	})
}

// Remove は、サービス serviceID を、タイムシフトで録るチャンネルから外す。録ったチャンクは、破棄のジョブが消す。
func (c *Channels) Remove(ctx context.Context, db Database, actor *audit.Actor, serviceID int64) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		current, err := q.LockTimeshiftChannel(ctx, serviceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrChannelNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to lock the timeshift channel: %w", err)
		}
		if _, err := q.DeleteTimeshiftChannel(ctx, serviceID); err != nil {
			return fmt.Errorf("failed to remove the timeshift channel: %w", err)
		}

		return c.record(ctx, tx, actor, "timeshift_channel.deleted", serviceID, map[string]any{"retentionSeconds": current})
	})
}

func (c *Channels) record(ctx context.Context, tx pgx.Tx, actor *audit.Actor, action string, serviceID int64, summary map[string]any) error {
	return c.recorder.Record(ctx, tx, &audit.Event{
		Actor:      actor,
		Action:     action,
		TargetType: channelAuditTarget,
		TargetID:   strconv.FormatInt(serviceID, 10),
		Summary:    summary,
		// 送信元は、操作者 (actor) に持たせる。
		SourceIP:  netip.Addr{},
		UserAgent: "",
	})
}

// userOf は、操作者がユーザーならその ID を返す。システムの操作なら nil を返す。
func userOf(actor *audit.Actor) *uuid.UUID {
	if actor == nil || actor.Type != audit.ActorTypeUser {
		return nil
	}

	return &actor.UserID
}
