package drama

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

// overrideAuditTarget は、上書きを変えたことを監査ログに残すときの対象の種類。
const overrideAuditTarget = "drama_title"

// ErrInvalidOverride は、上書きの値が正しくないことを表す。
var ErrInvalidOverride = errors.New("invalid drama title override")

// Override は、管理者が作品名ごとに決めた TMDb の作品 (設計書 3.9)。SeriesID が nil なら、その作品名の番組を結びつけない。
type Override struct {
	TitleKey  string
	SeriesID  *int32
	UpdatedAt time.Time
}

// OverrideChange は、上書きの変更。Reset なら上書きを消し、検索で作品を決めるのに戻す。Exclude なら、どの作品にも結びつけない。
// どちらでもなければ、SeriesID (1 以上) の作品に結びつける。
type OverrideChange struct {
	SeriesID *int32
	Exclude  bool
	Reset    bool
}

// Overrides は、作品名ごとの上書きを変える。変えたことは監査ログに残す。TMDb には問い合わせず、照合し直すジョブを入れる。
type Overrides struct {
	clock    clock.Clock
	recorder audit.Recorder
	// inserter は、上書きを変えたときに、番組を照合し直すジョブを入れる。nil なら入れない。
	inserter Inserter
}

// NewOverrides は Overrides を返す。
func NewOverrides(clk clock.Clock, recorder audit.Recorder, inserter Inserter) *Overrides {
	return &Overrides{clock: clk, recorder: recorder, inserter: inserter}
}

// ListOverrides は、すべての上書きを、作品名の鍵の順に返す。
func ListOverrides(ctx context.Context, db sqlc.DBTX) ([]*Override, error) {
	rows, err := sqlc.New(db).ListDramaTitleOverrides(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the drama title overrides: %w", err)
	}
	overrides := make([]*Override, 0, len(rows))
	for _, row := range rows {
		overrides = append(overrides, &Override{TitleKey: row.TitleKey, SeriesID: row.TmdbSeriesID, UpdatedAt: row.UpdatedAt})
	}

	return overrides, nil
}

// TitleKeyOf は、番組名 name から、上書きを引く作品名の鍵を返す。照合と同じ読み方をする。
func TitleKeyOf(name string) string {
	return TitleKey(WorkTitle(name))
}

// Update は、作品名の鍵 titleKey の上書きを change で変える。作品名の番組表にある番組は、確定を外して照合し直す。
// 上書きを消したときは、照合し直すまで、前の上書きで結びつけた作品のままになる。
func (o *Overrides) Update(ctx context.Context, db Database, actor *audit.Actor, titleKey string, change *OverrideChange) error {
	if titleKey == "" || (change.Reset && (change.Exclude || change.SeriesID != nil)) ||
		(!change.Reset && change.Exclude == (change.SeriesID != nil)) || (change.SeriesID != nil && *change.SeriesID <= 0) {
		return ErrInvalidOverride
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		var before any
		current, err := q.GetDramaTitleOverride(ctx, titleKey)
		switch {
		case err == nil:
			before = overrideSummary(current.TmdbSeriesID)
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("failed to read the drama title override: %w", err)
		}
		var after any
		if change.Reset {
			if _, err := q.DeleteDramaTitleOverride(ctx, titleKey); err != nil {
				return fmt.Errorf("failed to delete the drama title override: %w", err)
			}
		} else {
			if err := q.UpsertDramaTitleOverride(ctx, sqlc.UpsertDramaTitleOverrideParams{
				TitleKey: titleKey, TmdbSeriesID: change.SeriesID, Now: o.clock.Now(),
			}); err != nil {
				return fmt.Errorf("failed to update the drama title override: %w", err)
			}
			after = overrideSummary(change.SeriesID)
		}
		if _, err := q.UnfinalizeDramaMatches(ctx, titleKey); err != nil {
			return fmt.Errorf("failed to unfinalize the drama matches: %w", err)
		}
		if o.inserter != nil {
			if err := EnqueueMatchTx(ctx, tx, o.inserter); err != nil {
				return err
			}
		}

		return o.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "drama.title_override_updated",
			TargetType: overrideAuditTarget,
			TargetID:   titleKey,
			Summary:    map[string]any{"override": map[string]any{"from": before, "to": after}},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP:  netip.Addr{},
			UserAgent: "",
		})
	})
}

// overrideSummary は、監査ログに残す上書きの値。nil なら「結びつけない」。
func overrideSummary(seriesID *int32) any {
	if seriesID == nil {
		return "excluded"
	}

	return *seriesID
}
