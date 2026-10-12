// Package settings は、画面から変える全体の設定を扱う (設計書 11.2)。
//
// 起動に要るものや秘密情報は YAML と環境変数に置き、家族が日常的に変えたいものを DB に置く。
// ここでは、録画設定の全体の既定値、過去の番組の保持期間、チャンネル種別の表示を扱う。
package settings

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/reserve"
)

const (
	// DefaultArchiveRetentionDays は、過去の番組の保持期間の既定値 (付録 A)。
	DefaultArchiveRetentionDays = 90
	// maxArchiveRetentionDays は、保持期間の上限。無期限にするなら 0 にする。
	maxArchiveRetentionDays = 3650
	// auditTarget は、設定の監査ログの対象の種類。
	auditTarget = "system_settings"
)

// ErrInvalidSettings は、設定の値が正しくないことを表す。
var ErrInvalidSettings = errors.New("invalid system settings")

// System は、画面から変える全体の設定。
type System struct {
	// Recording は、録画設定の全体の既定値 (設計書 4.7)。Go の既定値に、画面で変えた項目を重ねたもの。
	Recording *reserve.Settings
	// RecordingOverrides は、画面で変えた項目。変えていない項目は nil。
	RecordingOverrides *reserve.Overrides
	// ArchiveRetentionDays は、過去の番組の保持期間 (日)。0 なら無期限。
	ArchiveRetentionDays int
	// UpdatedAt は、最後に変えた日時。一度も変えていなければ nil。
	UpdatedAt *time.Time
}

// ArchiveRetention は、過去の番組の保持期間を返す。無期限なら負の値を返す。
func (s *System) ArchiveRetention() time.Duration {
	if s.ArchiveRetentionDays == 0 {
		return -1
	}

	return time.Duration(s.ArchiveRetentionDays) * 24 * time.Hour
}

// Changes は、変える項目。nil の項目は変えない。
type Changes struct {
	// Recording は、録画設定の既定値の変える項目。項目を既定値に戻すときは、既定値と同じ値を渡す。
	Recording            *reserve.Overrides
	ArchiveRetentionDays *int
}

// Load は、全体の設定を返す。一度も変えていなければ、既定値を返す。
func Load(ctx context.Context, db sqlc.DBTX) (*System, error) {
	row, err := sqlc.New(db).GetSystemSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaults(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the system settings: %w", err)
	}

	return systemOf(&row)
}

// Store は、全体の設定を変える。変えたことは監査ログに残す。
type Store struct {
	clock    clock.Clock
	recorder audit.Recorder
}

// NewStore は Store を返す。
func NewStore(clk clock.Clock, recorder audit.Recorder) *Store {
	return &Store{clock: clk, recorder: recorder}
}

// Database は、読み出しと、トランザクションでの書き込みに使う DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Update は、changes の項目を変え、変えた後の設定を返す。
func (s *Store) Update(ctx context.Context, db Database, actor *audit.Actor, changes *Changes) (*System, error) {
	if changes == nil {
		return nil, fmt.Errorf("%w: changes is nil", ErrInvalidSettings)
	}
	if days := changes.ArchiveRetentionDays; days != nil {
		if _, err := retentionDays(*days); err != nil {
			return nil, err
		}
	}

	var updated *System
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		now := s.clock.Now()
		row, err := q.LockSystemSettings(ctx, sqlc.LockSystemSettingsParams{ArchiveRetentionDays: DefaultArchiveRetentionDays, Now: now})
		if err != nil {
			return fmt.Errorf("failed to lock the system settings: %w", err)
		}
		current, err := systemOf(&row)
		if err != nil {
			return err
		}
		overrides := merge(current.RecordingOverrides, changes.Recording)
		encoded, err := reserve.EncodeOverrides(overrides)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidSettings, err)
		}
		days := current.ArchiveRetentionDays
		if changes.ArchiveRetentionDays != nil {
			days = *changes.ArchiveRetentionDays
		}
		stored, err := retentionDays(days)
		if err != nil {
			return err
		}
		saved, err := q.UpdateSystemSettings(ctx, sqlc.UpdateSystemSettingsParams{RecordingDefaults: encoded, ArchiveRetentionDays: stored, Now: now})
		if err != nil {
			return fmt.Errorf("failed to update the system settings: %w", err)
		}
		if updated, err = systemOf(&saved); err != nil {
			return err
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "settings.updated",
			TargetType: auditTarget,
			TargetID:   "system",
			Summary: map[string]any{
				"recording":            change(current.RecordingOverrides, updated.RecordingOverrides),
				"archiveRetentionDays": change(current.ArchiveRetentionDays, updated.ArchiveRetentionDays),
			},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP:  netip.Addr{},
			UserAgent: "",
		})
	})
	if err != nil {
		return nil, err
	}

	return updated, nil
}

// change は、監査ログに残す、変える前と後の値。
func change(from, to any) map[string]any {
	return map[string]any{"from": from, "to": to}
}

// retentionDays は、保持期間の日数が範囲にあるかを確かめ、DB に保存する形にする。
func retentionDays(days int) (int32, error) {
	if days < 0 || days > maxArchiveRetentionDays {
		return 0, fmt.Errorf("%w: archiveRetentionDays must be between 0 and %d", ErrInvalidSettings, maxArchiveRetentionDays)
	}

	return int32(days), nil
}

// merge は、base に changes の nil でない項目を重ねる。
func merge(base, changes *reserve.Overrides) *reserve.Overrides {
	merged := *base
	if changes == nil {
		return &merged
	}
	if changes.PoolID != nil {
		merged.PoolID = changes.PoolID
	}
	if changes.FileNameTemplate != nil {
		merged.FileNameTemplate = changes.FileNameTemplate
	}
	if changes.StartMarginSeconds != nil {
		merged.StartMarginSeconds = changes.StartMarginSeconds
	}
	if changes.EndMarginSeconds != nil {
		merged.EndMarginSeconds = changes.EndMarginSeconds
	}
	if changes.Priority != nil {
		merged.Priority = changes.Priority
	}
	if changes.JoinSegments != nil {
		merged.JoinSegments = changes.JoinSegments
	}

	return &merged
}

func defaults() *System {
	return &System{
		Recording:            reserve.DefaultSettings(),
		RecordingOverrides:   new(reserve.Overrides),
		ArchiveRetentionDays: DefaultArchiveRetentionDays,
		UpdatedAt:            nil,
	}
}

func systemOf(row *sqlc.SystemSetting) (*System, error) {
	overrides, err := reserve.DecodeOverrides(row.RecordingDefaults)
	if err != nil {
		return nil, err
	}

	return &System{
		Recording:            reserve.DefaultSettings().Resolve(overrides),
		RecordingOverrides:   overrides,
		ArchiveRetentionDays: int(row.ArchiveRetentionDays),
		UpdatedAt:            &row.UpdatedAt,
	}, nil
}
