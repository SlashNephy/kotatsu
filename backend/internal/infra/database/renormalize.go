package database

import (
	"context"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

// dashVariantsPattern は、正規化で ~ と - にそろえる文字 (波ダッシュとダッシュの類、#2041) を含むかを確認する、PostgreSQL の正規表現。
// 今の正規化 (domain.Normalize) は、これらの文字を残さない。
const dashVariantsPattern = "[〜‐-―−]"

// renormalizeBatchSize は、番組を正規化し直すときに、1 回に読む行の数。過去の番組は多いので、まとめて読まない。
const renormalizeBatchSize = 1000

// renormalizeDashVariants は、正規化したカラムのうち、波ダッシュとダッシュの類をそろえる前の書き方の行を書き直す (#2041)。
// 今の正規化はこれらの文字を残さないので、正規化したカラムにこれらの文字が残る行は、そろえる前に書いたものか、
// ローリング更新の間に古いバイナリが書いたものである。番組表の同期は上流の番組が変わらなければ書き直さず、
// 正規化したカラムを埋める処理は空のカラムしか埋めないので、kotatsu migrate のたびに、ここで書き直す。正規化したカラムだけを確認するので、
// 書き直す行がなければ、何も変えない (開発用の DB の番組と過去の番組の 26 万件で、確認するのに 0.4 秒ほど)。
//
// NULL にできるカラム (録画の番組名と概要、キャプチャの字幕) は空に戻し、この後の、空のカラムを埋める処理に作り直させる。
// 番組の検索の gram も、正規化したカラムから作るので空に戻し、番組表の全量の照合で作り直させる。
func renormalizeDashVariants(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin normalizing again: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	steps := []func(context.Context, pgx.Tx) error{
		renormalizePrograms, renormalizeProgramArchives, renormalizeServices, renormalizeCaptures,
	}
	for _, step := range steps {
		if err := step(ctx, tx); err != nil {
			return err
		}
	}
	for _, statement := range []string{
		"UPDATE recordings SET name_normalized = NULL, description_normalized = NULL WHERE name_normalized ~ $1 OR description_normalized ~ $1",
		"UPDATE captures SET caption_normalized = NULL WHERE caption_normalized ~ $1",
	} {
		if _, err := tx.Exec(ctx, statement, dashVariantsPattern); err != nil {
			return fmt.Errorf("failed to clear the normalized columns: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit normalizing again: %w", err)
	}

	return nil
}

// programText は、番組を正規化し直すのに読むカラム。
type programText struct {
	serviceID, eventID int64
	startAt            time.Time
	name, description  string
	extended           map[string]string
}

// listProgramTexts は、table から、正規化したカラムにそろえる前の書き方が残る番組を、主キーが after より後のものから、主キーの順に読む。
func listProgramTexts(ctx context.Context, tx pgx.Tx, table string, after *programText) ([]*programText, error) {
	rows, err := tx.Query(ctx, `SELECT service_id, event_id, start_at, name, description, extended FROM `+table+`
		WHERE (name_normalized ~ $1 OR description_normalized ~ $1 OR extended_normalized ~ $1) AND (service_id, event_id, start_at) > ($2, $3, $4)
		ORDER BY service_id, event_id, start_at LIMIT $5`,
		dashVariantsPattern, after.serviceID, after.eventID, after.startAt, renormalizeBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to list the %s to normalize again: %w", table, err)
	}

	programs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*programText, error) {
		p := &programText{serviceID: 0, eventID: 0, startAt: time.Time{}, name: "", description: "", extended: map[string]string{}}
		err := row.Scan(&p.serviceID, &p.eventID, &p.startAt, &p.name, &p.description, &p.extended)

		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read the %s to normalize again: %w", table, err)
	}

	return programs, nil
}

// renormalizeProgramTable は、table (programs か program_archives) の番組を、renormalizeBatchSize 件ずつ読んで、queue の更新で書き直す。
func renormalizeProgramTable(ctx context.Context, tx pgx.Tx, table string, queue func(batch *pgx.Batch, p *programText)) error {
	after := &programText{serviceID: -1, eventID: 0, startAt: time.Time{}, name: "", description: "", extended: nil}
	for {
		programs, err := listProgramTexts(ctx, tx, table, after)
		if err != nil {
			return err
		}
		if len(programs) == 0 {
			return nil
		}
		batch := &pgx.Batch{}
		for _, p := range programs {
			queue(batch, p)
		}
		if err := sendBatch(ctx, tx, batch, table); err != nil {
			return err
		}
		after = programs[len(programs)-1]
	}
}

// renormalizePrograms は、番組表の番組の正規化したカラムを書き直し、検索の gram を空に戻す。
func renormalizePrograms(ctx context.Context, tx pgx.Tx) error {
	return renormalizeProgramTable(ctx, tx, "programs", func(batch *pgx.Batch, p *programText) {
		batch.Queue(`UPDATE programs SET name_normalized = $3, description_normalized = $4, extended_normalized = $5,
			search_grams = NULL WHERE service_id = $1 AND event_id = $2`,
			p.serviceID, p.eventID, domain.Normalize(p.name), domain.Normalize(p.description), domain.NormalizeExtended(p.extended))
	})
}

// renormalizeProgramArchives は、過去の番組の正規化したカラムを書き直し、検索の gram を空に戻す。
func renormalizeProgramArchives(ctx context.Context, tx pgx.Tx) error {
	return renormalizeProgramTable(ctx, tx, "program_archives", func(batch *pgx.Batch, p *programText) {
		batch.Queue(`UPDATE program_archives SET name_normalized = $4, description_normalized = $5, extended_normalized = $6,
			search_grams = NULL WHERE service_id = $1 AND event_id = $2 AND start_at = $3`,
			p.serviceID, p.eventID, p.startAt, domain.Normalize(p.name), domain.Normalize(p.description), domain.NormalizeExtended(p.extended))
	})
}

// renormalizeServices は、サービス名の正規化したカラムを書き直す。
func renormalizeServices(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, "SELECT id, name FROM services WHERE name_normalized ~ $1", dashVariantsPattern)
	if err != nil {
		return fmt.Errorf("failed to list the services to normalize again: %w", err)
	}
	type service struct {
		id   int64
		name string
	}
	services, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*service, error) {
		s := &service{id: 0, name: ""}
		err := row.Scan(&s.id, &s.name)

		return s, err
	})
	if err != nil {
		return fmt.Errorf("failed to read the services to normalize again: %w", err)
	}
	batch := &pgx.Batch{}
	for _, s := range services {
		batch.Queue("UPDATE services SET name_normalized = $2 WHERE id = $1", s.id, domain.Normalize(s.name))
	}

	return sendBatch(ctx, tx, batch, "services")
}

// renormalizeCaptures は、キャプチャのサービス名と番組名の正規化したカラムを書き直す。番組名がなければ、空を正規化したものにする。
func renormalizeCaptures(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT id, service_name, COALESCE(program_name, '') FROM captures
		WHERE service_name_normalized ~ $1 OR program_name_normalized ~ $1`, dashVariantsPattern)
	if err != nil {
		return fmt.Errorf("failed to list the captures to normalize again: %w", err)
	}
	type capture struct {
		id                       uuid.UUID
		serviceName, programName string
	}
	captures, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*capture, error) {
		c := &capture{id: uuid.Nil(), serviceName: "", programName: ""}
		err := row.Scan(&c.id, &c.serviceName, &c.programName)

		return c, err
	})
	if err != nil {
		return fmt.Errorf("failed to read the captures to normalize again: %w", err)
	}
	batch := &pgx.Batch{}
	for _, c := range captures {
		batch.Queue("UPDATE captures SET service_name_normalized = $2, program_name_normalized = $3 WHERE id = $1",
			c.id, domain.Normalize(c.serviceName), domain.Normalize(c.programName))
	}

	return sendBatch(ctx, tx, batch, "captures")
}

// sendBatch は、batch の更新をまとめて流す。
func sendBatch(ctx context.Context, tx pgx.Tx, batch *pgx.Batch, what string) error {
	if batch.Len() == 0 {
		return nil
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("failed to normalize the %s again: %w", what, err)
	}

	return nil
}
