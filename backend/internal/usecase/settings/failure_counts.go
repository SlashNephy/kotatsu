package settings

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

// failureCountAuditTarget は、録画の失敗の累計の監査ログの対象の種類。
const failureCountAuditTarget = "recording_failure_count"

// ResetRecordingFailureCounts は、理由 reasons の、録画の失敗の累計を 0 に、最後に失敗した日時を空に戻す (#1035、設計書 5.5)。
// 検証で失敗させた分や、直した障害の分を消すのに使う。戻したことは、理由ごとに、戻す前の値とともに監査ログに残す。
// 戻した日時は、/metrics の累計の作成の時刻にし、Prometheus や OTLP の受け手がカウンターのリセットとして扱えるようにする。
func (s *Store) ResetRecordingFailureCounts(ctx context.Context, db Database, actor *audit.Actor, reasons []string) error {
	if len(reasons) == 0 {
		return fmt.Errorf("%w: reasons must not be empty", ErrInvalidSettings)
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		rows, err := sqlc.New(tx).ResetRecordingFailureCounts(ctx, sqlc.ResetRecordingFailureCountsParams{Reasons: reasons, Now: s.clock.Now()})
		if err != nil {
			return fmt.Errorf("failed to reset the recording failure counts: %w", err)
		}
		for _, row := range rows {
			if err := s.recorder.Record(ctx, tx, &audit.Event{
				Actor:      actor,
				Action:     "recording_failure_count.reset",
				TargetType: failureCountAuditTarget,
				TargetID:   row.Reason,
				Summary:    map[string]any{"count": row.Count, "lastFailedAt": row.LastFailedAt},
				// 送信元は、操作者 (actor) に持たせる。
				SourceIP:  netip.Addr{},
				UserAgent: "",
			}); err != nil {
				return err
			}
		}

		return nil
	})
}
