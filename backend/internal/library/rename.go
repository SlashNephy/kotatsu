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
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/settings"
)

// joiningSuffix は、セグメントをつないでいる途中のファイルの名前に付く接尾辞 (segjoin パッケージの tempSuffix と同じ)。
const joiningSuffix = ".joining"

// undoTimeout は、付け直しに失敗した後、記録をファイルに合わせるのを待つ時間の上限。
const undoTimeout = 10 * time.Second

// ErrBusy は、セグメントをつないでいる途中なので、ファイルを移せないことを表す。
var ErrBusy = errors.New("the recording files are being joined")

// RenameFile は、付け直すセグメントのファイル。
type RenameFile struct {
	FileID uuid.UUID
	PoolID string
	From   string
	// To は、付け直した後のプール内の相対パス。Dir、Name、Sequence を合わせたもの。
	To string
	// Dir と Name は、移す先のディレクトリと、連番を付ける前のファイル名。Sequence は、付ける連番 (1 なら連番なし)。
	// 移す先がふさがっていれば、Sequence の次から試す。
	Dir      string
	Name     string
	Sequence int
}

// RenamePlan は、録画のファイルの付け直し。名前が変わるセグメントだけを持つ。
type RenamePlan struct {
	RecordingID uuid.UUID
	Files       []*RenameFile
}

// PlanRenames は、録画 ids (nil なら、録画中でないすべての録画。空なら、どの録画も選ばない) のセグメントのファイルを、今の録画設定のファイル名のテンプレートで
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
	checker := l.store.NewChecker()
	defer checker.Close()
	plans := []*RenamePlan{}
	for i := range rows {
		plan, err := l.plan(ctx, q, checker, &rows[i], system.Recording, ruleSettings, location)
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
	ctx context.Context, q *sqlc.Queries, checker *kago.Checker, row *sqlc.ListRecordingsForRenameRow, defaults *reserve.Settings,
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
	// taken は、この録画のほかのセグメントが移る先。同じ録画のセグメントはまとめて移すので、ふさがっているとみなす。
	taken := map[string]bool{}
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
		dir = strings.TrimSuffix(dir, "/")
		to, moves := destination(checker, taken, files[i].PoolID, files[i].Path, dir, name, n)
		taken[to] = true
		if moves {
			plan.Files = append(plan.Files, &RenameFile{
				FileID: files[i].ID, PoolID: files[i].PoolID, From: files[i].Path, To: to,
				Dir: dir, Name: name, Sequence: n,
			})
		}
	}

	return plan, nil
}

// destination は、セグメントのファイル from を dir/name の start 番目の連番から付け直したときに移る先と、名前が変わるかを返す。
// kago.Store.Rename と同じく、ふさがっている名前を飛ばして次の連番を試す。ほかの録画が名前をふさいでいて連番が付いた
// ファイルは、付け直しても名前が変わらないので、計画に出さない。taken の名前も、ふさがっているとみなす。
// プールを調べられなければ、start 番目の連番に移るとみなす。
func destination(checker *kago.Checker, taken map[string]bool, poolID, from, dir, name string, start int) (string, bool) {
	for n := start; ; n++ {
		to := path.Join(dir, kago.SequencedName(name, n))
		if to == from {
			return to, false
		}
		if taken[to] {
			continue
		}
		exists, err := checker.Exists(poolID, to)
		if err != nil {
			return path.Join(dir, kago.SequencedName(name, start)), true
		}
		if !exists {
			return to, true
		}
	}
}

// recordUnrestoredPath は、元に戻せなかったセグメントのファイル file の記録を、移した先のパスに合わせる。
func (l *Library) recordUnrestoredPath(ctx context.Context, db Database, file *RenameFile) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), undoTimeout)
	defer cancel()
	if err := sqlc.New(db).SetRecordingFilePath(ctx, sqlc.SetRecordingFilePathParams{Path: file.To, ID: file.FileID}); err != nil {
		l.logger.ErrorContext(ctx, "failed to record the path of a recording file that could not be restored",
			slog.String("path", file.To), slog.Any("error", err))
	}
}

// Rename は、計画 plan のとおりに、録画のセグメントのファイルを移し、記録のパスを書き換える。
// 生成物は、セグメントのファイルの ID で置くので移さない。
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
			to, err := l.store.Rename(file.PoolID, file.From, file.Dir, file.Name, file.Sequence)
			if err != nil {
				return err
			}
			moved = append(moved, &RenameFile{
				FileID: file.FileID, PoolID: file.PoolID, From: file.From, To: to, Dir: file.Dir, Name: file.Name, Sequence: file.Sequence,
			})
			if err := q.SetRecordingFilePath(ctx, sqlc.SetRecordingFilePathParams{Path: to, ID: file.FileID}); err != nil {
				return fmt.Errorf("failed to update the recording file path: %w", err)
			}
			renamed = append(renamed, map[string]string{"from": file.From, "to": to})
		}

		return l.record(ctx, tx, actor, "recording.renamed", &recording, map[string]any{"files": renamed})
	})
	if err != nil {
		// 記録を書き換えられなかったので、移したファイルを元に戻す。
		// 元の名前は、今の整え方で使えない文字を含むことがあるので (#917)、整え直さずに戻す。
		for _, file := range slices.Backward(moved) {
			undoErr := l.store.Restore(file.PoolID, file.To, file.From)
			if undoErr == nil {
				continue
			}
			l.logger.ErrorContext(ctx, "failed to undo a renamed recording file",
				slog.String("from", file.To), slog.String("to", file.From), slog.Any("error", undoErr))
			// 戻せなければ、ファイルは移した先にあるので、記録をそこに合わせる。
			// 合わせないと、記録が元のパスを指したまま、ファイルが見つからなくなる。
			// 要求が取り消されて失敗したときも合わせるよう、取り消しを引き継がない ctx で書く。
			l.recordUnrestoredPath(ctx, db, file)
		}

		return err
	}

	return nil
}
