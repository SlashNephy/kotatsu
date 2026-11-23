package database

import (
	"context"
	"fmt"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

// renormalizeDashVariantsVersion は、renormalizeDashVariants の版。足したときに上げたスキーマの版。
const renormalizeDashVariantsVersion int64 = 20261123030402

// dashVariantsPattern は、正規化で ~ と - にそろえるようにした文字 (波ダッシュとダッシュの類、#2041) を含むかを調べる、
// PostgreSQL の正規表現。
const dashVariantsPattern = "[〜‐-―−]"

// renormalizeDashVariants は、正規化で波ダッシュとダッシュの類をそろえる前に書いた、正規化した列を書き直す (#2041)。
// そろえる文字を含む行だけを読む。番組表の同期は、上流の番組が変わらなければ書き直さず、正規化した列を埋める処理は空の列しか埋めないので、
// ここで書き直す。
//
// NULL にできる列 (録画の番組名と概要、キャプチャの字幕、番組の検索の gram、シリーズ名) は空に戻し、
// kotatsu migrate と番組表の全量の突き合わせの、空の列を埋める処理に任せる。
func renormalizeDashVariants(ctx context.Context, tx pgx.Tx) error {
	steps := []func(context.Context, pgx.Tx) error{
		renormalizePrograms, renormalizeProgramArchives, renormalizeServices, renormalizeCaptures,
	}
	for _, step := range steps {
		if err := step(ctx, tx); err != nil {
			return err
		}
	}
	for _, statement := range []string{
		"UPDATE recordings SET name_normalized = NULL, description_normalized = NULL WHERE name ~ $1 OR description ~ $1",
		"UPDATE recording_history SET series = NULL WHERE name ~ $1",
		"UPDATE captures SET caption_normalized = NULL WHERE caption ~ $1",
	} {
		if _, err := tx.Exec(ctx, statement, dashVariantsPattern); err != nil {
			return fmt.Errorf("failed to clear the normalized columns: %w", err)
		}
	}

	return nil
}

// programText は、番組を正規化し直すのに読む列。
type programText struct {
	serviceID, eventID int64
	startAt            time.Time
	name, description  string
	extended           map[string]string
}

// listProgramTexts は、query で、そろえる文字を含む番組を読む。query は service_id、event_id、start_at、name、description、extended を返す。
func listProgramTexts(ctx context.Context, tx pgx.Tx, query string) ([]*programText, error) {
	rows, err := tx.Query(ctx, query, dashVariantsPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list the programs to normalize again: %w", err)
	}

	programs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*programText, error) {
		p := &programText{serviceID: 0, eventID: 0, startAt: time.Time{}, name: "", description: "", extended: map[string]string{}}
		err := row.Scan(&p.serviceID, &p.eventID, &p.startAt, &p.name, &p.description, &p.extended)

		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read the programs to normalize again: %w", err)
	}

	return programs, nil
}

// renormalizePrograms は、番組表の番組の正規化した列を書き直す。検索の gram とシリーズ名は空に戻し、全量の突き合わせで作り直させる。
func renormalizePrograms(ctx context.Context, tx pgx.Tx) error {
	programs, err := listProgramTexts(ctx, tx, `SELECT service_id, event_id, start_at, name, description, extended FROM programs
		WHERE name ~ $1 OR description ~ $1 OR extended::text ~ $1`)
	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, p := range programs {
		batch.Queue(`UPDATE programs SET name_normalized = $3, description_normalized = $4, extended_normalized = $5,
			search_grams = NULL, series = NULL, episode_number = NULL WHERE service_id = $1 AND event_id = $2`,
			p.serviceID, p.eventID, domain.Normalize(p.name), domain.Normalize(p.description), domain.NormalizeExtended(p.extended))
	}

	return sendBatch(ctx, tx, batch, "programs")
}

// renormalizeProgramArchives は、過去の番組の正規化した列を書き直す。検索の gram は空に戻し、全量の突き合わせで作り直させる。
func renormalizeProgramArchives(ctx context.Context, tx pgx.Tx) error {
	programs, err := listProgramTexts(ctx, tx, `SELECT service_id, event_id, start_at, name, description, extended FROM program_archives
		WHERE name ~ $1 OR description ~ $1 OR extended::text ~ $1`)
	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, p := range programs {
		batch.Queue(`UPDATE program_archives SET name_normalized = $4, description_normalized = $5, extended_normalized = $6,
			search_grams = NULL WHERE service_id = $1 AND event_id = $2 AND start_at = $3`,
			p.serviceID, p.eventID, p.startAt, domain.Normalize(p.name), domain.Normalize(p.description), domain.NormalizeExtended(p.extended))
	}

	return sendBatch(ctx, tx, batch, "program archives")
}

// renormalizeServices は、サービス名の正規化した列を書き直す。
func renormalizeServices(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, "SELECT id, name FROM services WHERE name ~ $1", dashVariantsPattern)
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

// renormalizeCaptures は、キャプチャのサービス名と番組名の正規化した列を書き直す。番組名がなければ、空を正規化したものにする。
func renormalizeCaptures(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, "SELECT id, service_name, COALESCE(program_name, '') FROM captures WHERE service_name ~ $1 OR program_name ~ $1", dashVariantsPattern)
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
