// Package history は、重複の回避に使う録画履歴 (設計書 4.4) に、録画を記録するかを決めて記録する。
package history

import (
	"context"
	"fmt"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

// MaxDrops は、重複の回避で「録画済み」とみなすドロップの数の上限 (付録 A、#1497)。
// 本番の録画 (2026-10-12〜11-12 の 1,538 件) では、ドロップは 0 件が 95% で、BS4K は
// 19 件すべてに 3〜4 件が出ていた。地上波と BS で 2〜4 件の録画は 3 件だけだった。
// BS4K の録画も録画済みとみなせるよう、上限を 4 件にする。
const MaxDrops = 4

// Eligible は、録画 recording を「録画済み」とみなして録画履歴に記録するかを返す。
// 正常に終わり (完了かエラーありの完了)、ドロップが MaxDrops 以下で、スクランブルの警告がない録画だけを記録する。
// スクランブルされたまま録れた録画を録画済みとみなすと、カードを直した後の再放送を録らなくなるからである (#1517)。
func Eligible(recording *sqlc.Recording) bool {
	return (recording.State == "completed" || recording.State == "completed_with_errors") &&
		recording.Drops <= MaxDrops && !recording.ScrambleWarning
}

// Record は、録画 recording が Eligible なら、録画履歴に記録する。
func Record(ctx context.Context, q *sqlc.Queries, recording *sqlc.Recording, now time.Time) error {
	if !Eligible(recording) {
		return nil
	}
	// 同じ回を束ねる (#1498) ときに回を読めるよう、録画の概要や、番組の記述子の話数なども写す。
	// 前の回を録らない (#1499) ときにシリーズ名で引けるよう、番組名から読んだシリーズ名も書く。
	err := q.InsertRecordingHistoryFromRecording(ctx, sqlc.InsertRecordingHistoryFromRecordingParams{
		ID: id.New(), TitleKey: rule.TitleKey(recording.Name), Series: episode.SeriesOf(recording.Name), Now: now, RecordingID: recording.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to save the recording history: %w", err)
	}

	return nil
}

// Resync は、録画の後に解析し直して、録画 recording のスクランブルの警告が変わったときに、録画履歴を合わせて直す (#1517)。
// 警告が付いたら、その録画から記録した履歴を消す。警告が外れたら、Eligible で、まだ記録していなければ記録する。
// 警告が変わっていなければ、何もしない。取り込んだ録画のように、終えたときに記録しなかった録画を、解析だけで記録し始めないためである。
func Resync(ctx context.Context, q *sqlc.Queries, recording *sqlc.Recording, wasWarned bool, now time.Time) error {
	if recording.ScrambleWarning == wasWarned {
		return nil
	}
	if recording.ScrambleWarning {
		if _, err := q.DeleteRecordingHistoryByRecording(ctx, &recording.ID); err != nil {
			return fmt.Errorf("failed to delete the recording history: %w", err)
		}

		return nil
	}
	recorded, err := q.CountRecordingHistoryByRecording(ctx, &recording.ID)
	if err != nil {
		return fmt.Errorf("failed to count the recording history: %w", err)
	}
	if recorded > 0 {
		return nil
	}

	return Record(ctx, q, recording, now)
}
