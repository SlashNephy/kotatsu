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
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/usecase/segjoin"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// joiningSuffix は、セグメントをつないでいる途中のファイルの名前に付く接尾辞 (segjoin パッケージの tempSuffix と同じ)。
const joiningSuffix = ".joining"

// undoTimeout は、付け直しに失敗した後、記録をファイルに合わせるのを待つ時間の上限。
const undoTimeout = 10 * time.Second

// ErrBusy は、セグメントをつないでいる途中なので、ファイルを移せないことを表す。
var ErrBusy = errors.New("the recording files are being joined")

// RenameFile は、付け直すセグメントのファイルか、変換した MP4。
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
	// Preset は、変換した MP4 (#1253) のときの画質。セグメントでは空。
	// 変換した MP4 は、最初のセグメントの名前から作った名前で探すので、最初のセグメント (Segment) を移した先に合わせて移す (#1720)。
	// 連番は付けず、移す先がふさがっていれば移さない。
	Preset  string
	Segment uuid.UUID
}

// RenamePlan は、録画のファイルの付け直し。名前が変わるセグメントと、それに合わせて移す変換した MP4 だけを持つ。
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
		// 予約が消えていれば、外部結合の ReservationKind は空になる。
		if rows[i].ReservationKind == domain.ReservationKindRule {
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
	ctx context.Context, q *sqlc.Queries, checker *kago.Checker, row *sqlc.ListRecordingsForRenameRow, defaults *domain.RecordingSettings,
	ruleSettings map[uuid.UUID][]byte, location *time.Location,
) (*RenamePlan, error) {
	resolved := defaults
	if row.ReservationID != nil {
		for _, raw := range [][]byte{ruleSettings[*row.ReservationID], row.ReservationSettings} {
			if raw == nil {
				continue
			}
			overrides, err := domain.DecodeOverrides(raw)
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
				Dir: dir, Name: name, Sequence: n, Preset: "", Segment: uuid.Nil(),
			})
		}
	}
	plan.Files = append(plan.Files, transcodedRenames(files, plan.Files)...)

	return plan, nil
}

// transcodedRenames は、最初のセグメントを移すとき、その名前から作った名前の変換した MP4 を、移した先の名前に合わせて移す計画を返す (#1720)。
// 変換した MP4 の記録は最初のセグメントの名前で探すので (transcode.ChaptersWorker)、移さないと、CM のチャプターを書けなくなる。
// 取り込んだ MP4 のような、ほかの名前のファイルは移さない。
func transcodedRenames(files []sqlc.RecordingFile, segments []*RenameFile) []*RenameFile {
	first := slices.IndexFunc(files, func(f sqlc.RecordingFile) bool { return f.Kind == segmentKind })
	if first < 0 {
		return []*RenameFile{}
	}
	i := slices.IndexFunc(segments, func(f *RenameFile) bool { return f.FileID == files[first].ID })
	if i < 0 {
		return []*RenameFile{}
	}
	moved := segments[i]
	renames := []*RenameFile{}
	for _, preset := range domain.TranscodePresets() {
		from := domain.TranscodedPath(moved.From, preset)
		j := slices.IndexFunc(files, func(f sqlc.RecordingFile) bool {
			return f.Kind == otherKind && f.PoolID == moved.PoolID && f.Path == from
		})
		if j < 0 {
			continue
		}
		to := domain.TranscodedPath(moved.To, preset)
		dir, name := path.Split(to)
		renames = append(renames, &RenameFile{
			FileID: files[j].ID, PoolID: moved.PoolID, From: from, To: to,
			Dir: strings.TrimSuffix(dir, "/"), Name: name, Sequence: 1, Preset: preset, Segment: moved.FileID,
		})
	}

	return renames
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
		// 大文字と小文字や正規化だけを変えるときは、移す先が自分自身として見つかる。ふさがっているとはみなさない。
		if same, err := checker.SameFile(poolID, from, to); err == nil && same {
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

// checkNotJoining は、録画 recordingID のセグメントを、つないでいる途中でないことを確かめる。途中なら ErrBusy を返す (#1721)。
// つなぐジョブと同じ録画ごとのロックを、トランザクションの終わりまで取る。つないで DB の記録を 1 本にしてから、
// つないだファイルに置き換えるまでも、つないだ元のファイルを消すのを待っているので、途中とみなす。
// 最初のセグメントの名前が変わらず、2 つ目以降だけを移すときも、置き換えのジョブが消すはずの元のファイルを動かさないようにする。
func checkNotJoining(ctx context.Context, tx pgx.Tx, recordingID uuid.UUID) error {
	locked, err := sqlc.New(tx).TryLockRecordingJoinXact(ctx, recordingID.String())
	if err != nil {
		return fmt.Errorf("failed to lock the recording: %w", err)
	}
	if !locked {
		return ErrBusy
	}
	pending, err := segjoin.FinalizePending(ctx, tx, recordingID)
	if err != nil {
		return err
	}
	if pending {
		return ErrBusy
	}

	return nil
}

// move は、計画のファイル file を移し、移した先を返す。セグメントは連番を付けて移し、移した先を movedTo に残す。
// 変換した MP4 は、最初のセグメントを実際に移した先 (movedTo) から作った名前に移す。最初のセグメントを移していないときと、
// 移す先がふさがっているときは、移さずに false を返す。連番を付けると、最初のセグメントの名前から作った名前と合わなくなる (#1720)。
func (l *Library) move(ctx context.Context, file *RenameFile, movedTo map[uuid.UUID]string) (string, bool, error) {
	if file.Preset == "" {
		to, err := l.store.Rename(file.PoolID, file.From, file.Dir, file.Name, file.Sequence)
		if err != nil {
			return "", false, err
		}
		movedTo[file.FileID] = to

		return to, true, nil
	}
	segment, ok := movedTo[file.Segment]
	if !ok {
		return "", false, nil
	}
	to := domain.TranscodedPath(segment, file.Preset)
	if to == file.From {
		return "", false, nil
	}
	if taken, err := l.taken(file.PoolID, file.From, to); err != nil || taken {
		l.logger.WarnContext(ctx, "kept the transcoded file; its new name is taken",
			slog.String("from", file.From), slog.String("to", to), slog.Any("error", err))

		return "", false, nil
	}
	if err := l.store.Restore(file.PoolID, file.From, to); err != nil {
		return "", false, err
	}

	return to, true, nil
}

// taken は、プール poolID の to を、from とは別のファイルがふさいでいるかを返す。
// 大文字と小文字や正規化だけを変えるときは、to が from 自身として見つかるが、ふさがっているとはみなさない (#1095)。
func (l *Library) taken(poolID, from, to string) (bool, error) {
	exists, err := l.store.Exists(poolID, to)
	if err != nil || !exists {
		return exists, err
	}
	checker := l.store.NewChecker()
	defer checker.Close()
	same, err := checker.SameFile(poolID, from, to)
	if err != nil {
		return true, err
	}

	return !same, nil
}

// Rename は、計画 plan のとおりに、録画のセグメントのファイルを移し、記録のパスを書き換える。
// 生成物は、セグメントのファイルの ID で置くので移さない。
// 移す先に同じ名前があれば連番を付ける。録画中と、セグメントをつないでいる途中の録画は移さない。監査ログに残す。
// 記録を書き換えられなければ、移したファイルを元に戻す。同じプールの中だけで移す。
// RenamedHook は、名前を付け直した記録を書いたトランザクションの中で、付け直したことを受け取る。
// セグメントの名前で探されるもの (TvtPlay のチャプターのファイル、#1737) を、付け直した名前で作り直させるのに使う。
type RenamedHook interface {
	OnRenamed(ctx context.Context, tx pgx.Tx, recordingID uuid.UUID) error
}

// notifyRenamed は、録画 recordingID の名前を付け直したことを、hooks に並べた順に渡す。moved が false (移したファイルがない) なら渡さない。
func notifyRenamed(ctx context.Context, tx pgx.Tx, recordingID uuid.UUID, moved bool, hooks []RenamedHook) error {
	if !moved {
		return nil
	}
	for _, hook := range hooks {
		if err := hook.OnRenamed(ctx, tx, recordingID); err != nil {
			return err
		}
	}

	return nil
}

func (l *Library) Rename(ctx context.Context, db Database, actor *audit.Actor, plan *RenamePlan, hooks ...RenamedHook) error {
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
		if err := checkNotJoining(ctx, tx, plan.RecordingID); err != nil {
			return err
		}
		for _, file := range plan.Files {
			if joining, _ := l.store.Exists(file.PoolID, file.From+joiningSuffix); joining {
				return ErrBusy
			}
		}
		renamed := []map[string]string{}
		// movedTo は、移したセグメントの、実際に移した先。変換した MP4 は、これに合わせて移す。
		movedTo := map[uuid.UUID]string{}
		for _, file := range plan.Files {
			to, ok, err := l.move(ctx, file, movedTo)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			moved = append(moved, &RenameFile{
				FileID: file.FileID, PoolID: file.PoolID, From: file.From, To: to, Dir: file.Dir, Name: file.Name, Sequence: file.Sequence,
				Preset: file.Preset, Segment: file.Segment,
			})
			if err := q.SetRecordingFilePath(ctx, sqlc.SetRecordingFilePathParams{Path: to, ID: file.FileID}); err != nil {
				return fmt.Errorf("failed to update the recording file path: %w", err)
			}
			renamed = append(renamed, map[string]string{"from": file.From, "to": to})
		}

		if err := notifyRenamed(ctx, tx, plan.RecordingID, len(moved) > 0, hooks); err != nil {
			return err
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
