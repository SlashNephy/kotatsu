// Package anime は、アニメの番組と作品の結びつき (しょぼいカレンダー、Annict、AniList との連携) を扱う (設計書 3.8)。
//
// 外部へ問い合わせるので、管理者が全体の設定で有効にしたときだけ動く (設計書 0.3)。
// 問い合わせるのは nekonote のジョブだけで、EPG の同期 (neko) には入れない。しょぼいカレンダーが遅いときや落ちているときに、
// 同期とチューナーの割り当てまで遅れないようにするためである。
package anime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/animedata"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// 取り込み元の名前 (anime_imports.source)。
const (
	SourceSayaDefinitions = "saya_definitions"
	SourceArm             = "arm"
)

const (
	// ImportInterval は、連携先の情報を取り込み直す間隔 (付録 A)。
	ImportInterval = 24 * time.Hour
	// importRetryInterval は、取り込めなかったときに、次に問い合わせるまでの間隔。
	importRetryInterval = time.Hour
	// channelTypeTerrestrial は、地上波の種別。saya-definitions は地上波のネットワーク ID を持たない。
	channelTypeTerrestrial = "GR"
	// maxErrorLength は、記録する取り込めなかった理由の長さの上限 (バイト)。
	maxErrorLength = 1000
)

// Fetcher は、連携先の情報を取得する。*animedata.Client が満たす。
type Fetcher interface {
	SayaDefinitions(ctx context.Context, etag string) ([]*animedata.Channel, string, error)
	Arm(ctx context.Context, etag string) ([]*animedata.Work, string, error)
}

var _ Fetcher = new(animedata.Client)

// Database は、読み出しと、トランザクションでの書き込みに使う DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Importer は、saya-definitions と arm、しょぼいカレンダーのタイトルの一覧を取り込む。
type Importer struct {
	db      Database
	clock   clock.Clock
	fetcher Fetcher
	// titles は、しょぼいカレンダーのタイトルの一覧を引く。番組名の解析で、作品名から TID を引くのに使う。
	titles TitleFetcher
	// inserter は、チャンネルの対応が変わったときに、番組を照合し直すジョブを入れる。nil なら入れない。
	inserter Inserter
	logger   *slog.Logger
}

// NewImporter は Importer を返す。
func NewImporter(db Database, clk clock.Clock, fetcher Fetcher, titles TitleFetcher, inserter Inserter, logger *slog.Logger) *Importer {
	return &Importer{db: db, clock: clk, fetcher: fetcher, titles: titles, inserter: inserter, logger: logger}
}

// ImportResult は、1 回の取り込みの結果。取り込み元ごとに、取り込んだ件数を持つ。
type ImportResult struct {
	// Disabled は、連携が無効なので何もしなかったかどうか。
	Disabled bool `json:"disabled"`
	// Imported は、内容が変わって取り込み直した取り込み元と、その件数。
	Imported map[string]int `json:"imported"`
	// Unchanged は、内容が変わっていなかった取り込み元。
	Unchanged []string `json:"unchanged"`
	// Failed は、取り込めなかった取り込み元と、その理由。
	Failed map[string]string `json:"failed"`
}

// Import は、取り込み直す時期の来た取り込み元を取り込む。force なら、時期を待たずにすべて問い合わせる。
// 1 つの取り込み元が取り込めなくても、ほかの取り込み元は取り込む。取り込めなかった理由は anime_imports に残し、1 時間後にまた問い合わせる。
func (i *Importer) Import(ctx context.Context, force bool) (*ImportResult, error) {
	result := &ImportResult{Disabled: false, Imported: map[string]int{}, Unchanged: []string{}, Failed: map[string]string{}}
	system, err := settings.Load(ctx, i.db)
	if err != nil {
		return nil, err
	}
	if !system.AnimeLinkingEnabled {
		result.Disabled = true

		return result, nil
	}

	for _, source := range []string{SourceSayaDefinitions, SourceArm, SourceSyobocalTitles} {
		previous, err := sqlc.New(i.db).GetAnimeImport(ctx, source)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("failed to read the import status of %s: %w", source, err)
		}
		found := err == nil
		if found && !force && !i.due(&previous) {
			continue
		}
		etag := ""
		if found && previous.Etag != nil {
			etag = *previous.Etag
		}
		count, err := i.importSource(ctx, source, etag)
		switch {
		case errors.Is(err, animedata.ErrNotModified):
			result.Unchanged = append(result.Unchanged, source)
			err = sqlc.New(i.db).RecordAnimeImport(ctx, sqlc.RecordAnimeImportParams{
				Source: source, Etag: previous.Etag, Now: i.clock.Now(), ItemCount: previous.ItemCount,
			})
		case err != nil:
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			i.logger.WarnContext(ctx, "failed to import anime data", slog.String("source", source), slog.Any("error", err))
			reason := truncate(err.Error(), maxErrorLength)
			result.Failed[source] = reason
			err = sqlc.New(i.db).RecordAnimeImportFailure(ctx, sqlc.RecordAnimeImportFailureParams{Source: source, Now: i.clock.Now(), Error: &reason})
		default:
			result.Imported[source] = count
		}
		if err != nil {
			return nil, fmt.Errorf("failed to record the import status of %s: %w", source, err)
		}
	}

	return result, nil
}

// due は、取り込み元を問い合わせる時期が来たかを返す。取り込めなかったときは、間隔を短くして問い合わせ直す。
func (i *Importer) due(status *sqlc.AnimeImport) bool {
	interval := ImportInterval
	if status.Error != nil {
		interval = importRetryInterval
	}

	return !i.clock.Now().Before(status.CheckedAt.Add(interval))
}

// importSource は、取り込み元 source を取得して入れ替え、取り込んだ件数を返す。変わっていなければ animedata.ErrNotModified を返す。
func (i *Importer) importSource(ctx context.Context, source, etag string) (int, error) {
	switch source {
	case SourceSyobocalTitles:
		return i.importTitles(ctx)
	case SourceSayaDefinitions:
		channels, newETag, err := i.fetcher.SayaDefinitions(ctx, etag)
		if err != nil {
			return 0, err
		}

		return i.replaceChannels(ctx, channelDefinitionsOf(channels), newETag)
	default:
		works, newETag, err := i.fetcher.Arm(ctx, etag)
		if err != nil {
			return 0, err
		}

		return i.replaceWorks(ctx, source, worksOf(source, works), newETag)
	}
}

// replaceChannels は、取り込んだチャンネルの対応を入れ替える。管理者の上書き (anime_channel_overrides) は残す。
func (i *Importer) replaceChannels(ctx context.Context, params *sqlc.InsertAnimeChannelDefinitionsParams, etag string) (int, error) {
	count, err := itemCount(len(params.Sids))
	if err != nil {
		return 0, err
	}
	err = pgx.BeginFunc(ctx, i.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.LockAnimeImport(ctx, SourceSayaDefinitions); err != nil {
			return fmt.Errorf("failed to lock the import of %s: %w", SourceSayaDefinitions, err)
		}
		if err := q.DeleteAnimeChannelDefinitions(ctx); err != nil {
			return fmt.Errorf("failed to delete the channel definitions: %w", err)
		}
		if err := q.InsertAnimeChannelDefinitions(ctx, *params); err != nil {
			return fmt.Errorf("failed to insert the channel definitions: %w", err)
		}
		// 対応が変わったので、これから放送される番組を照合し直す。
		if i.inserter != nil {
			if err := EnqueueMatchTx(ctx, tx, i.inserter); err != nil {
				return err
			}
		}

		return q.RecordAnimeImport(ctx, sqlc.RecordAnimeImportParams{
			Source: SourceSayaDefinitions, Etag: optional(etag), Now: i.clock.Now(), ItemCount: count,
		})
	})
	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// replaceWorks は、取り込み元 source の作品 ID の対応を入れ替える。
func (i *Importer) replaceWorks(ctx context.Context, source string, params *sqlc.InsertAnimeWorksParams, etag string) (int, error) {
	count, err := itemCount(len(params.SyobocalTids))
	if err != nil {
		return 0, err
	}
	err = pgx.BeginFunc(ctx, i.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.LockAnimeImport(ctx, source); err != nil {
			return fmt.Errorf("failed to lock the import of %s: %w", source, err)
		}
		if err := q.DeleteAnimeWorks(ctx, source); err != nil {
			return fmt.Errorf("failed to delete the works of %s: %w", source, err)
		}
		if err := q.InsertAnimeWorks(ctx, *params); err != nil {
			return fmt.Errorf("failed to insert the works of %s: %w", source, err)
		}

		return q.RecordAnimeImport(ctx, sqlc.RecordAnimeImportParams{Source: source, Etag: optional(etag), Now: i.clock.Now(), ItemCount: count})
	})
	if err != nil {
		return 0, err
	}

	return int(count), nil
}

// itemCount は、取り込んだ件数を、DB に保存する形にする。
func itemCount(n int) (int32, error) {
	if n < 0 || n > math.MaxInt32 {
		return 0, fmt.Errorf("too many items: %d", n)
	}

	return int32(n), nil
}

// channelKey は、取り込んだチャンネルの対応の主キー。
type channelKey struct {
	channelType string
	networkID   int64
	sid         int64
}

// channelDefinitionsOf は、saya-definitions のチャンネルを、サービスごとの行にする。
// 地上波はネットワーク ID を 0 にする。同じサービスが 2 度出てきたら、先に出てきたものを使う (TVTestAnnictRecorder と同じ)。
func channelDefinitionsOf(channels []*animedata.Channel) *sqlc.InsertAnimeChannelDefinitionsParams {
	params := &sqlc.InsertAnimeChannelDefinitionsParams{
		ChannelTypes: []string{}, NetworkIds: []int64{}, Sids: []int64{}, Names: []string{},
		SyobocalChannelIds: []int32{}, AnnictChannelIds: []int32{},
	}
	seen := map[channelKey]bool{}
	for _, channel := range channels {
		if channel.SyobocalID == nil && channel.AnnictID == nil {
			continue
		}
		networkID := channel.NetworkID
		if channel.Type == channelTypeTerrestrial {
			networkID = 0
		}
		for _, sid := range channel.ServiceIDs {
			key := channelKey{channelType: channel.Type, networkID: networkID, sid: sid}
			if seen[key] {
				continue
			}
			seen[key] = true
			params.ChannelTypes = append(params.ChannelTypes, channel.Type)
			params.NetworkIds = append(params.NetworkIds, networkID)
			params.Sids = append(params.Sids, sid)
			params.Names = append(params.Names, channel.Name)
			params.SyobocalChannelIds = append(params.SyobocalChannelIds, positiveOrZero(channel.SyobocalID))
			params.AnnictChannelIds = append(params.AnnictChannelIds, positiveOrZero(channel.AnnictID))
		}
	}

	return params
}

// workKey は、同じ作品の対応を二重に持たないための鍵。
type workKey struct {
	tid, annict, anilist, mal int32
}

// worksOf は、arm の作品を、TID ごとに並びの番号を振った行にする。TID のない作品と、同じ対応の重複は除く。
func worksOf(source string, works []*animedata.Work) *sqlc.InsertAnimeWorksParams {
	params := &sqlc.InsertAnimeWorksParams{
		Source: source, SyobocalTids: []int32{}, Positions: []int16{}, AnnictWorkIds: []int32{}, AnilistIds: []int32{}, MalIds: []int32{},
	}
	positions := map[int32]int16{}
	seen := map[workKey]bool{}
	for _, work := range works {
		tid := positiveOrZero(work.SyobocalTID)
		if tid == 0 {
			continue
		}
		key := workKey{tid: tid, annict: positiveOrZero(work.AnnictID), anilist: positiveOrZero(work.AniListID), mal: positiveOrZero(work.MALID)}
		if seen[key] || (key.annict == 0 && key.anilist == 0 && key.mal == 0) {
			continue
		}
		seen[key] = true
		params.SyobocalTids = append(params.SyobocalTids, tid)
		params.Positions = append(params.Positions, positions[tid])
		positions[tid]++
		params.AnnictWorkIds = append(params.AnnictWorkIds, key.annict)
		params.AnilistIds = append(params.AnilistIds, key.anilist)
		params.MalIds = append(params.MalIds, key.mal)
	}

	return params
}

// positiveOrZero は、正の ID ならその値を、なければ 0 を返す。
func positiveOrZero(id *int32) int32 {
	if id == nil || *id <= 0 {
		return 0
	}

	return *id
}

// optional は、空でない文字列なら、そのポインタを返す。
func optional(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

// truncate は、s を最大 n バイトに切り詰める。UTF-8 の文字の途中では切らない。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}

	return s[:n]
}
