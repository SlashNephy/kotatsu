package anime

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

var (
	// ErrInvalidChannel は、チャンネルの対応の上書きの値が正しくないことを表す。
	ErrInvalidChannel = errors.New("invalid anime channel")
	// ErrServiceNotFound は、上書きするサービスがないことを表す。
	ErrServiceNotFound = errors.New("service not found")
)

// serviceAuditTarget は、チャンネルの対応の上書きの監査ログの対象の種類。サービスの表示の変更と同じ。
const serviceAuditTarget = "service"

// Channel は、サービスと、しょぼいカレンダーの ChID、Annict のチャンネルの対応 (設計書 3.8)。
type Channel struct {
	ServiceID int64
	// DefinitionName は、対応を取り込んだ saya-definitions のチャンネルの名前。取り込んだ対応がなければ nil。
	DefinitionName *string
	// ImportedSyobocalChannelID と ImportedAnnictChannelID は、saya-definitions から取り込んだ対応。
	ImportedSyobocalChannelID *int32
	ImportedAnnictChannelID   *int32
	// OverrideSyobocalChannelID と OverrideAnnictChannelID は、管理者の上書き。上書きしていなければ nil。
	OverrideSyobocalChannelID *int32
	OverrideAnnictChannelID   *int32
	// SyobocalChannelID と AnnictChannelID は、使う値。上書きがあれば上書きを、なければ取り込んだ対応を使う。
	SyobocalChannelID *int32
	AnnictChannelID   *int32
}

// ListChannels は、すべてのサービスについて、チャンネルの対応を返す。
func ListChannels(ctx context.Context, db sqlc.DBTX) ([]*Channel, error) {
	rows, err := sqlc.New(db).ListAnimeChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime channels: %w", err)
	}
	channels := make([]*Channel, 0, len(rows))
	for _, row := range rows {
		channels = append(channels, &Channel{
			ServiceID: row.ServiceID, DefinitionName: row.DefinitionName,
			ImportedSyobocalChannelID: row.ImportedSyobocalChannelID, ImportedAnnictChannelID: row.ImportedAnnictChannelID,
			OverrideSyobocalChannelID: row.OverrideSyobocalChannelID, OverrideAnnictChannelID: row.OverrideAnnictChannelID,
			SyobocalChannelID: row.SyobocalChannelID, AnnictChannelID: row.AnnictChannelID,
		})
	}

	return channels, nil
}

// Import は、取り込み元 1 つの、取り込んだ様子。
type Import struct {
	Source     string
	CheckedAt  time.Time
	ImportedAt *time.Time
	ItemCount  int
	Error      *string
}

// ListImports は、取り込み元ごとの、取り込んだ様子を返す。1 度も問い合わせていない取り込み元は返さない。
func ListImports(ctx context.Context, db sqlc.DBTX) ([]*Import, error) {
	rows, err := sqlc.New(db).ListAnimeImports(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime imports: %w", err)
	}
	imports := make([]*Import, 0, len(rows))
	for _, row := range rows {
		imports = append(imports, &Import{
			Source: row.Source, CheckedAt: row.CheckedAt, ImportedAt: row.ImportedAt, ItemCount: int(row.ItemCount), Error: row.Error,
		})
	}

	return imports, nil
}

// ChannelChanges は、チャンネルの対応の上書きの変える項目。nil の項目は変えない。
type ChannelChanges struct {
	// SyobocalChannelID は、しょぼいカレンダーの ChID の上書き (1 以上)。ResetSyobocalChannelID なら、上書きを消して取り込んだ対応に戻す。
	SyobocalChannelID      *int32
	ResetSyobocalChannelID bool
	// AnnictChannelID は、Annict のチャンネルの上書き (1 以上)。ResetAnnictChannelID なら、上書きを消して取り込んだ対応に戻す。
	AnnictChannelID      *int32
	ResetAnnictChannelID bool
}

// Channels は、チャンネルの対応の上書きを変える。変えたことは監査ログに残す。
type Channels struct {
	clock    clock.Clock
	recorder audit.Recorder
	// inserter は、上書きを変えたときに、番組を照合し直すジョブを入れる。nil なら入れない。
	inserter Inserter
}

// NewChannels は Channels を返す。
func NewChannels(clk clock.Clock, recorder audit.Recorder, inserter Inserter) *Channels {
	return &Channels{clock: clk, recorder: recorder, inserter: inserter}
}

// Update は、サービス serviceID のチャンネルの対応の上書きを変える。どちらの上書きもなくなれば、行を消す。
func (c *Channels) Update(ctx context.Context, db Database, actor *audit.Actor, serviceID int64, changes *ChannelChanges) error {
	if err := validateChannelChanges(changes); err != nil {
		return err
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		current, err := q.LockAnimeChannelOverride(ctx, serviceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrServiceNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to lock the anime channel override: %w", err)
		}
		syobocal := overrideOf(current.SyobocalChannelID, changes.SyobocalChannelID, changes.ResetSyobocalChannelID)
		annict := overrideOf(current.AnnictChannelID, changes.AnnictChannelID, changes.ResetAnnictChannelID)
		if syobocal == nil && annict == nil {
			err = q.DeleteAnimeChannelOverride(ctx, serviceID)
		} else {
			err = q.UpsertAnimeChannelOverride(ctx, sqlc.UpsertAnimeChannelOverrideParams{
				ServiceID: serviceID, SyobocalChannelID: syobocal, AnnictChannelID: annict, Now: c.clock.Now(),
			})
		}
		if err != nil {
			return fmt.Errorf("failed to update the anime channel override: %w", err)
		}
		// 対応が変わったので、これから放送される番組を照合し直す。
		if c.inserter != nil {
			if err := EnqueueMatchTx(ctx, tx, c.inserter); err != nil {
				return err
			}
		}

		return c.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "service.anime_channel_updated",
			TargetType: serviceAuditTarget,
			TargetID:   strconv.FormatInt(serviceID, 10),
			Summary: map[string]any{
				"syobocalChannelId": map[string]any{"from": current.SyobocalChannelID, "to": syobocal},
				"annictChannelId":   map[string]any{"from": current.AnnictChannelID, "to": annict},
			},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP:  netip.Addr{},
			UserAgent: "",
		})
	})
}

// validateChannelChanges は、上書きの変える項目が正しいかを確かめる。
func validateChannelChanges(changes *ChannelChanges) error {
	if changes == nil {
		return fmt.Errorf("%w: changes is nil", ErrInvalidChannel)
	}
	if changes.ResetSyobocalChannelID && changes.SyobocalChannelID != nil {
		return fmt.Errorf("%w: syobocalChannelId and resetSyobocalChannelId cannot be used together", ErrInvalidChannel)
	}
	if changes.ResetAnnictChannelID && changes.AnnictChannelID != nil {
		return fmt.Errorf("%w: annictChannelId and resetAnnictChannelId cannot be used together", ErrInvalidChannel)
	}
	for name, id := range map[string]*int32{"syobocalChannelId": changes.SyobocalChannelID, "annictChannelId": changes.AnnictChannelID} {
		if id != nil && *id <= 0 {
			return fmt.Errorf("%w: %s must be positive", ErrInvalidChannel, name)
		}
	}

	return nil
}

// overrideOf は、今の上書き current に、変える値 change か、消す印 reset を重ねる。
func overrideOf(current, change *int32, reset bool) *int32 {
	switch {
	case reset:
		return nil
	case change != nil:
		return change
	default:
		return current
	}
}

// Work は、しょぼいカレンダーの TID に対応する作品の ID (設計書 3.8)。どの ID も、対応がなければ nil。
type Work struct {
	AnnictWorkID *int32
	AniListID    *int32
	MALID        *int32
}

// ListWorks は、TID ごとに、対応する作品を返す。
// 対応のない TID は、返す表に含めない。
func ListWorks(ctx context.Context, db sqlc.DBTX, tids []int32) (map[int32][]*Work, error) {
	works := map[int32][]*Work{}
	if len(tids) == 0 {
		return works, nil
	}
	rows, err := sqlc.New(db).ListAnimeWorks(ctx, tids)
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime works: %w", err)
	}
	for _, row := range rows {
		works[row.SyobocalTid] = append(works[row.SyobocalTid], &Work{AnnictWorkID: row.AnnictWorkID, AniListID: row.AnilistID, MALID: row.MalID})
	}

	return works, nil
}
