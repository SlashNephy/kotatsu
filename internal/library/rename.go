package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/settings"
)

// joiningSuffix は、セグメントをつないでいる途中のファイルの名前に付く接尾辞 (segjoin パッケージの tempSuffix と同じ)。
const joiningSuffix = ".joining"

// ErrBusy は、セグメントをつないでいる途中なので、ファイルを移せないことを表す。
var ErrBusy = errors.New("the recording files are being joined")

// RenameFile は、付け直すセグメントのファイル。
type RenameFile struct {
	FileID uuid.UUID
	PoolID string
	From   string
	To     string
}

// RenamePlan は、録画のファイルの付け直し。名前が変わるセグメントだけを持つ。
type RenamePlan struct {
	RecordingID uuid.UUID
	Files       []*RenameFile
}

// PlanRenames は、録画 ids (nil なら、録画中でないすべての録画) のセグメントのファイルを、今の録画設定のファイル名のテンプレートで
// 付け直したときの名前を求め、名前が変わる録画だけを返す (設計書 4.7)。録画設定は、録画のときと同じく、全体の既定値に、
// ルール予約ならルールの上書き、さらに予約自体の上書きを重ねて求める。予約が消えていれば、全体の既定値を使う。
// 分かれた録画では、2 つ目以降のセグメントに " (2)" のような連番を付ける。location は、ファイル名に使う表示のタイムゾーン。
func (l *Library) PlanRenames(ctx context.Context, db sqlc.DBTX, ids []uuid.UUID, location *time.Location) ([]*RenamePlan, error) {
	q := sqlc.New(db)
	system, err := settings.Load(ctx, db)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListRecordingsForRename(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recordings: %w", err)
	}
	ruleSettings, err := firstRuleSettings(ctx, q, rows)
	if err != nil {
		return nil, err
	}
	plans := []*RenamePlan{}
	for i := range rows {
		plan, err := l.plan(ctx, q, &rows[i], system.Recording, ruleSettings, location)
		if err != nil {
			return nil, err
		}
		if len(plan.Files) > 0 {
			plans = append(plans, plan)
		}
	}

	return plans, nil
}

func firstRuleSettings(ctx context.Context, q *sqlc.Queries, rows []sqlc.ListRecordingsForRenameRow) (map[uuid.UUID][]byte, error) {
	ruleReservations := []uuid.UUID{}
	for i := range rows {
		if rows[i].ReservationKind != nil && reserve.Kind(*rows[i].ReservationKind) == reserve.KindRule {
			ruleReservations = append(ruleReservations, *rows[i].ReservationID)
		}
	}
	result := map[uuid.UUID][]byte{}
	if len(ruleReservations) == 0 {
		return result, nil
	}
	settingsRows, err := q.ListFirstRuleSettings(ctx, ruleReservations)
	if err != nil {
		return nil, fmt.Errorf("failed to list rule settings: %w", err)
	}
	for _, row := range settingsRows {
		result[row.ReservationID] = row.Settings
	}

	return result, nil
}

func (l *Library) plan(
	ctx context.Context, q *sqlc.Queries, row *sqlc.ListRecordingsForRenameRow, defaults *reserve.Settings,
	ruleSettings map[uuid.UUID][]byte, location *time.Location,
) (*RenamePlan, error) {
	resolved := defaults
	if row.ReservationID != nil {
		for _, raw := range [][]byte{ruleSettings[*row.ReservationID], row.ReservationSettings} {
			if raw == nil {
				continue
			}
			overrides, err := reserve.DecodeOverrides(raw)
			if err != nil {
				return nil, err
			}
			resolved = resolved.Resolve(overrides)
		}
	}
	tmpl, err := reserve.ParseFileNameTemplate(resolved.FileNameTemplate)
	if err != nil {
		return nil, err
	}
	eventID := int64(0)
	if row.EventID != nil {
		eventID = *row.EventID
	}
	data := &reserve.FileNameData{
		StartAt: row.StartAt.In(location), EndAt: row.EndAt.In(location), ServiceName: row.ServiceName,
		ChannelType: row.ChannelType, Channel: row.Channel, Name: row.Name, EventID: eventID,
	}
	files, err := q.ListRecordingFiles(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recording files: %w", err)
	}
	plan := &RenamePlan{RecordingID: row.ID, Files: []*RenameFile{}}
	n := 0
	for i := range files {
		if files[i].Kind != segmentKind {
			continue
		}
		n++
		// 取り込んだ録画の .ts のような拡張子は、そのまま残す。
		ext := filepath.Ext(files[i].Path)
		if ext == "" || len(ext) > 16 {
			ext = ".m2ts"
		}
		rendered, err := reserve.RenderFileName(tmpl, data, ext)
		if err != nil {
			return nil, err
		}
		dir, name := path.Split(rendered)
		to := path.Join(dir, kago.SequencedName(name, n))
		if to != files[i].Path {
			plan.Files = append(plan.Files, &RenameFile{FileID: files[i].ID, PoolID: files[i].PoolID, From: files[i].Path, To: to})
		}
	}

	return plan, nil
}

// Rename は、計画 plan のとおりに、録画のセグメントのファイルと付属ファイルのディレクトリを移し、記録のパスを書き換える。
// 移す先に同じ名前があれば連番を付ける。録画中と、セグメントをつないでいる途中の録画は移さない。監査ログに残す。
// 記録を書き換えられなければ、移したファイルを元に戻す。同じプールの中だけで移す。
func (l *Library) Rename(ctx context.Context, db Database, actor *audit.Actor, plan *RenamePlan) error {
	moved := []*RenameFile{}
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		recording, err := lock(ctx, q, plan.RecordingID)
		if err != nil {
			return err
		}
		if recording.State == stateRecording {
			return ErrRecording
		}
		for _, file := range plan.Files {
			if joining, _ := l.store.Exists(file.PoolID, file.From+joiningSuffix); joining {
				return ErrBusy
			}
		}
		renamed := []map[string]string{}
		for _, file := range plan.Files {
			dir, name := path.Split(file.To)
			to, err := l.store.Rename(file.PoolID, file.From, strings.TrimSuffix(dir, "/"), name)
			if err != nil {
				return err
			}
			moved = append(moved, &RenameFile{FileID: file.FileID, PoolID: file.PoolID, From: file.From, To: to})
			if err := q.SetRecordingFilePath(ctx, sqlc.SetRecordingFilePathParams{Path: to, ID: file.FileID}); err != nil {
				return fmt.Errorf("failed to update the recording file path: %w", err)
			}
			err = q.MoveRecordingSidecarPaths(ctx, sqlc.MoveRecordingSidecarPathsParams{
				NewPrefix: to + ".kotatsu/", OldPrefix: file.From + ".kotatsu/", RecordingID: plan.RecordingID, PoolID: file.PoolID,
			})
			if err != nil {
				return fmt.Errorf("failed to update the sidecar paths: %w", err)
			}
			renamed = append(renamed, map[string]string{"from": file.From, "to": to})
		}

		return l.record(ctx, tx, actor, "recording.renamed", &recording, map[string]any{"files": renamed})
	})
	if err != nil {
		// 記録を書き換えられなかったので、移したファイルを元に戻す。
		for _, file := range slices.Backward(moved) {
			dir, name := path.Split(file.From)
			if _, undoErr := l.store.Rename(file.PoolID, file.To, strings.TrimSuffix(dir, "/"), name); undoErr != nil {
				l.logger.ErrorContext(ctx, "failed to undo a renamed recording file",
					slog.String("from", file.To), slog.String("to", file.From), slog.Any("error", undoErr))
			}
		}

		return err
	}

	return nil
}
