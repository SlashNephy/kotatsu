// Package scramble は、スクランブルされたまま録れた録画と、スクランブルされたまま届くライブを記録し、知らせる (#1517、設計書 5.5、6.2、7.1)。
package scramble

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
)

const (
	// AlertConsecutive は、判定できた録画がこの数だけ続けてスクランブルされたままだったら、管理者に知らせる数 (設計書 6.2、付録 A)。
	// 1 本だけなら、契約のない局を録った録画のこともあるので、2 本続いたら、カードが読めていない見込みが高いとみなす。実機で測っていない仮の値。
	AlertConsecutive = 2
	// AlertInterval は、管理者へ同じ知らせを送る最短の間隔 (設計書 6.2、付録 A)。
	// カードを直すまでは、録画のたびに続くので、1 日に 1 回までにする。直した後にまた起きても、翌日には気付ける。
	AlertInterval = 24 * time.Hour
)

// Judge は、録画 recordingID のセグメントの PID ごとの集計から、スクランブルされたまま録れたかを判定し、録画に記録する (設計書 5.5)。
// 録画を終えるときと、録画の後に解析し直したときに呼ぶ。判定しなかったら、割合を NULL にし、警告の印を下ろす。
func Judge(ctx context.Context, q *sqlc.Queries, recordingID uuid.UUID, now time.Time) (ts.RecordingScramble, error) {
	rows, err := q.ListRecordingSegmentPIDStats(ctx, recordingID)
	if err != nil {
		return ts.RecordingScramble{}, fmt.Errorf("failed to list the PID stats of the segments: %w", err)
	}
	segments := make([][]ts.PIDQuality, 0, len(rows))
	for _, raw := range rows {
		if raw == nil {
			continue
		}
		var qualities []ts.PIDQuality
		if err := json.Unmarshal(raw, &qualities); err != nil {
			return ts.RecordingScramble{}, fmt.Errorf("failed to decode the PID stats: %w", err)
		}
		segments = append(segments, qualities)
	}
	result := ts.JudgeRecordingScramble(segments)
	var ratio *float64
	if result.Judged {
		ratio = new(result.Ratio)
	}
	err = q.SetRecordingScramble(ctx, sqlc.SetRecordingScrambleParams{Ratio: ratio, Warning: result.Warning, Now: now, ID: recordingID})
	if err != nil {
		return ts.RecordingScramble{}, fmt.Errorf("failed to save the scramble verdict: %w", err)
	}

	return result, nil
}

// AddToPayload は、録画の完了の通知の payload に、スクランブルの警告を添える (設計書 6.2)。警告でなければ何も足さない。
func AddToPayload(payload map[string]any, result ts.RecordingScramble) {
	if !result.Warning {
		return
	}
	payload["scrambleWarning"] = true
	payload["scrambledRatio"] = result.Ratio
}

// Alert は、録画を終えたときの判定 result を、続けてスクランブルされたままだった録画の数に足し、
// AlertConsecutive に届いたら、AlertInterval に 1 回まで、管理者に知らせる (設計書 6.2)。判定しなかった録画は数を変えない。
// 録画を終える文と同じトランザクション db で呼ぶ。payload には、知らせのきっかけになった録画の材料 (録画の ID、番組名、サービス名) を渡す。
func Alert(ctx context.Context, db sqlc.DBTX, clk clock.Clock, result ts.RecordingScramble, payload map[string]any) error {
	if !result.Judged {
		return nil
	}
	q := sqlc.New(db)
	now := clk.Now()
	state, err := q.AdvanceScrambleAlert(ctx, sqlc.AdvanceScrambleAlertParams{Scrambled: result.Warning, Now: now})
	if err != nil {
		return fmt.Errorf("failed to count the scrambled recordings: %w", err)
	}
	if state.Consecutive < AlertConsecutive || (state.NotifiedAt != nil && now.Sub(*state.NotifiedAt) < AlertInterval) {
		return nil
	}
	admins, err := q.ListActiveAdminIDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to list the administrators: %w", err)
	}
	if len(admins) == 0 {
		// 宛先を空にすると全員に届くので、管理者がいなければ知らせない。
		return nil
	}
	if err := q.MarkScrambleAlertNotified(ctx, &now); err != nil {
		return fmt.Errorf("failed to record the scramble alert: %w", err)
	}
	alert := maps.Clone(payload)
	if alert == nil {
		alert = map[string]any{}
	}
	alert["consecutive"] = state.Consecutive

	return notification.EmitTo(ctx, db, clk, domain.NotificationKindScrambledRecordings, alert, &notification.Audience{UserIDs: admins, ByRule: false})
}
