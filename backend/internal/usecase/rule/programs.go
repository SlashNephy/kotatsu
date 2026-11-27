package rule

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

// listMatchableProgramsSQL は、ルールの対象になる番組を読む。放送が始まった番組と、終了時刻が未定の番組は対象にしない (設計書 4.3)。
// 映像か音声を流すサービス (media_service_types) の番組だけを対象にする。
// 照合には正規化したカラムを使う。詳細の正規化したカラムがない (カラムを足す前に書いた) 番組だけ、上流の詳細を返す (#530)。
//
// 結果を読みながら照合するので、sqlc で生成せずに書く。sqlc の :many は全部の行をスライスに読んでから返す (#2226)。
const listMatchableProgramsSQL = `SELECT p.service_id, p.event_id, p.start_at, p.end_at, p.is_free, p.name, p.name_normalized,
  p.description_normalized, p.extended_normalized,
  (CASE WHEN p.extended_normalized IS NULL THEN p.extended ELSE '{}' END)::jsonb AS extended,
  p.genres, p.related_items, p.series_episode, s.channel_type, s.channel
FROM programs p
JOIN services s ON s.id = p.service_id
WHERE p.start_at > $1 AND NOT p.end_undecided AND s.service_type = ANY($2::bigint[])
ORDER BY p.service_id, p.event_id`

// matchableProgram は、ルールの対象になる番組。
type matchableProgram struct {
	ServiceID             int64
	EventID               int64
	StartAt               time.Time
	EndAt                 time.Time
	IsFree                bool
	Name                  string
	NameNormalized        string
	DescriptionNormalized string
	ExtendedNormalized    *string
	Extended              []byte
	Genres                []byte
	RelatedItems          []byte
	SeriesEpisode         *int32
	ChannelType           string
	Channel               string
}

// eachMatchableProgramBatch は、ルールの対象になる番組を 1 つのクエリで読みながら、batchSize 件ずつ each に渡す。
// batchSize が 0 以下なら、defaultMatchBatchSize 件ずつ渡す。
// 全部の番組を一度に読むと、番組の数だけメモリが跳ねる (#2226)。each が受け取ったスライスは、次の呼び出しでは使わない。
// 読み終えるまで tx を使うので、each の中では tx を使わない。
func eachMatchableProgramBatch(
	ctx context.Context, tx pgx.Tx, now time.Time, batchSize int, each func(programs []matchableProgram) error,
) error {
	if batchSize <= 0 {
		batchSize = defaultMatchBatchSize
	}
	rows, err := tx.Query(ctx, listMatchableProgramsSQL, now, domain.MediaServiceTypes())
	if err != nil {
		return fmt.Errorf("failed to list programs: %w", err)
	}
	defer rows.Close()

	batch := make([]matchableProgram, 0, batchSize)
	for rows.Next() {
		var p matchableProgram
		if err := rows.Scan(
			&p.ServiceID, &p.EventID, &p.StartAt, &p.EndAt, &p.IsFree, &p.Name, &p.NameNormalized, &p.DescriptionNormalized,
			&p.ExtendedNormalized, &p.Extended, &p.Genres, &p.RelatedItems, &p.SeriesEpisode, &p.ChannelType, &p.Channel,
		); err != nil {
			return fmt.Errorf("failed to read a program: %w", err)
		}
		batch = append(batch, p)
		if len(batch) < batchSize {
			continue
		}
		if err := each(batch); err != nil {
			return err
		}
		// 一致した番組は each が値で写して持つので、スライスを使い回す。前の行の文字列などを GC に回すため、要素を消してから詰め直す。
		clear(batch)
		batch = batch[:0]
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to list programs: %w", err)
	}
	if len(batch) == 0 {
		return nil
	}

	return each(batch)
}
