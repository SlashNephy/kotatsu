package anime

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
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
	// DefinitionName は、対応を取り込んだ、チャンネルの定義の名前。取り込んだ対応がなければ nil。
	DefinitionName *string
	// ImportedSyobocalChannelID と ImportedAnnictChannelID は、チャンネルの定義から取り込んだ対応。
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
	q := sqlc.New(db)
	definitions, err := q.ListAnimeChannelDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime channel definitions: %w", err)
	}
	rows, err := q.ListAnimeChannelServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime channels: %w", err)
	}
	resolver := newChannelResolver(definitions)
	channels := make([]*Channel, 0, len(rows))
	for _, row := range rows {
		channel := &Channel{
			ServiceID: row.ServiceID, DefinitionName: nil, ImportedSyobocalChannelID: nil, ImportedAnnictChannelID: nil,
			OverrideSyobocalChannelID: row.OverrideSyobocalChannelID, OverrideAnnictChannelID: row.OverrideAnnictChannelID,
			SyobocalChannelID: nil, AnnictChannelID: nil,
		}
		if imported := resolver.resolve(row.ChannelType, row.NetworkID, row.Sid); imported != nil {
			channel.DefinitionName = new(imported.name)
			channel.ImportedSyobocalChannelID, channel.ImportedAnnictChannelID = imported.syobocalChannelID, imported.annictChannelID
		}
		channel.SyobocalChannelID = cmp.Or(channel.OverrideSyobocalChannelID, channel.ImportedSyobocalChannelID)
		channel.AnnictChannelID = cmp.Or(channel.OverrideAnnictChannelID, channel.ImportedAnnictChannelID)
		channels = append(channels, channel)
	}

	return channels, nil
}

// しょぼいカレンダーのチャンネルの ChID。
const (
	syobocalNHKETele  = 2
	syobocalTokyoMX   = 19
	syobocalNHKETele2 = 64
	syobocalNHKETele3 = 65
	syobocalTokyoMX2  = 187
)

// subchannelSyobocalChannel は、しょぼいカレンダーで、メインと別のチャンネルを持つサブチャンネルの ChID を返す。
// メインの ChID mainID と、サブチャンネルのサービス ID がメインからいくつ後か offset で引く。Eテレ2 と Eテレ3、TOKYO MX2。
// サブチャンネルで放送する番組は、しょぼいカレンダーではこれらのチャンネルに登録されるので、メインの ChID で引くと、メインの別の番組に結びついてしまう。
func subchannelSyobocalChannel(mainID int32, offset int64) (int32, bool) {
	switch {
	case mainID == syobocalNHKETele && offset == 1:
		return syobocalNHKETele2, true
	case mainID == syobocalNHKETele && offset == 2:
		return syobocalNHKETele3, true
	case mainID == syobocalTokyoMX && offset == 2:
		return syobocalTokyoMX2, true
	default:
		return 0, false
	}
}

// maxSubchannelOffset は、地上波のサブチャンネルのサービス ID が、メインのサービス ID からいくつ後まであるか。
const maxSubchannelOffset = 2

// definitionKey は、取り込んだチャンネルの対応を引く鍵。地上波はネットワーク ID を 0 にする。
type definitionKey struct {
	channelType string
	networkID   int64
	sid         int64
}

// importedChannel は、サービスに対応させた、取り込んだチャンネルの対応。
type importedChannel struct {
	name              string
	syobocalChannelID *int32
	annictChannelID   *int32
}

// channelResolver は、サービスに、取り込んだチャンネルの対応を当てる (設計書 3.8)。
type channelResolver struct {
	definitions map[definitionKey]*importedChannel
}

func newChannelResolver(definitions []sqlc.ListAnimeChannelDefinitionsRow) *channelResolver {
	r := &channelResolver{definitions: map[definitionKey]*importedChannel{}}
	for _, d := range definitions {
		r.definitions[definitionKey{channelType: d.ChannelType, networkID: d.NetworkID, sid: d.Sid}] = &importedChannel{
			name: d.Name, syobocalChannelID: d.SyobocalChannelID, annictChannelID: d.AnnictChannelID,
		}
	}

	return r
}

// resolve は、サービスに対応させる、取り込んだチャンネルの対応を返す。対応がなければ nil を返す。
//   - 種別が地上波なら種別とサービス ID で、BS と CS は種別とネットワーク ID とサービス ID で照らし合わせる。
//   - ネットワーク ID が地上波でも、Mirakurun の種別が地上波でないサービス (他の地域を受ける EXT などの種別) は、
//     地上波の対応を使う。定義は系列局を局ごとに持つので (#1589)、どの地域で受けても、その局のチャンネルに当たる。
//   - 地上波のサブチャンネル (サービス ID がメインの 1 つ後か 2 つ後) は、メインの対応を使う。メインで野球中継をし、
//     サブチャンネルでアニメを放送することがあるためである。しょぼいカレンダーでサブチャンネルが別のチャンネルを持てば、そちらを使う。
func (r *channelResolver) resolve(channelType string, networkID, sid int64) *importedChannel {
	key := definitionKey{channelType: channelType, networkID: networkID, sid: sid}
	if channelType == channelTypeTerrestrial {
		key.networkID = 0
	}
	if d, ok := r.definitions[key]; ok {
		return d
	}
	if !domain.IsTerrestrialNetwork(networkID) {
		return nil
	}
	for offset := range int64(maxSubchannelOffset + 1) {
		d := r.terrestrial(sid - offset)
		if d == nil {
			continue
		}
		if offset == 0 || d.syobocalChannelID == nil {
			return d
		}
		if id, ok := subchannelSyobocalChannel(*d.syobocalChannelID, offset); ok {
			// Annict のチャンネルは、サブチャンネルのものが分からないので付けない。
			return &importedChannel{name: d.name, syobocalChannelID: new(id), annictChannelID: nil}
		}

		return d
	}

	return nil
}

// terrestrial は、地上波のサービス ID sid の対応を返す。
func (r *channelResolver) terrestrial(sid int64) *importedChannel {
	return r.definitions[definitionKey{channelType: channelTypeTerrestrial, networkID: 0, sid: sid}]
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
