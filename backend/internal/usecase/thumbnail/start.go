package thumbnail

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
)

// StartEnqueuer は、録画の開始のイベントを配信するときに、番組の始まりの代表画像を作るジョブを予約する。
// 録画中の代表画像は、録り始めてから LiveInterval たつまで作らない (LiveWorker) ので、それまでのつなぎにする。
// 番組の放送予定の始まりから StartThumbnailDelay 後の I フレームから作る。番組の途中から録り始めたら、録り始めから数える。
// I フレームの GOP を書き終えるよう、その LiveThumbnailLag 後に動かす。
type StartEnqueuer struct {
	inserter Inserter
	clock    clock.Clock
}

var _ notification.Hook = new(StartEnqueuer)

// NewStartEnqueuer は StartEnqueuer を返す。
func NewStartEnqueuer(inserter Inserter, clk clock.Clock) *StartEnqueuer {
	return &StartEnqueuer{inserter: inserter, clock: clk}
}

func (e *StartEnqueuer) OnEvent(ctx context.Context, tx pgx.Tx, event *notification.Event) error {
	if event.Kind != domain.NotificationKindRecordingStarted {
		return nil
	}
	recordingID, ok := recordingIDOf(event.Payload)
	if !ok {
		// 録画の ID のないイベントでは、何もできない。ほかの通知は配信したいので、失敗にはしない。
		return nil
	}
	recording, err := sqlc.New(tx).GetRecording(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get the recording: %w", err)
	}
	// 開始の通知が遅れ (#1500)、録り始めてから LiveInterval たっていれば、LiveWorker が今の様子を作るので、古い場面で上書きしない。
	if recording.State != stateRecording || !e.clock.Now().Before(recording.RecordedFrom.Add(LiveInterval)) {
		return nil
	}
	at := maxTime(recording.StartAt, recording.RecordedFrom).Add(StartThumbnailDelay)
	args := GenerateArgs{RecordingID: recordingID, Live: true, ThumbnailOnly: false, At: &at}
	opts := &river.InsertOpts{ScheduledAt: at.Add(LiveThumbnailLag)}
	if _, err := e.inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: args, InsertOpts: opts}}); err != nil {
		return fmt.Errorf("failed to enqueue the thumbnail job at the start of the recording: %w", err)
	}

	return nil
}

// recordingIDOf は、録画の開始のイベントの材料から、録画の ID を読む。
func recordingIDOf(payload map[string]any) (uuid.UUID, bool) {
	raw, _ := payload["recordingId"].(string)
	recordingID, err := uuid.Parse(raw)

	return recordingID, err == nil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}

	return b
}
