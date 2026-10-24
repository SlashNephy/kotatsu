// Package library は、録画済みの番組 (ライブラリ) の削除、保護、タグ付けと、ユーザーごとの再生位置を扱う (設計書 5.5、8.2、9.7)。
//
// 録画は家族の共有物なので、誰の録画でも変えられる。その代わり、削除、保護、タグ付けは監査ログに残す。
// 録画を消しても、重複の回避に使う録画履歴は残す。
package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

const (
	// maxTags は、1 つの録画に付けられるタグの数の上限。
	maxTags = 20
	// maxTagLength は、タグの名前の長さの上限。
	maxTagLength = 50
	// auditTarget は、録画の監査ログの対象の種類。
	auditTarget = "recording"
	// segmentKind は、録画ファイルのうち、TS のセグメントの種類。
	// stateRecording は、録画中の録画の状態。
	stateRecording = "recording"
	segmentKind    = "segment"
	// otherKind は、録画ファイルのうち、取り込んだ MP4 のような TS 以外のファイルの種類。
	otherKind = "other"
)

var (
	// ErrNotFound は、録画が見つからないことを表す。
	ErrNotFound = errors.New("recording not found")
	// ErrProtected は、保護した録画を消そうとしたことを表す。保護を外してから消す。
	ErrProtected = errors.New("the recording is protected")
	// ErrRecording は、録画中の録画を消そうとしたことを表す。予約を取り消して、録画を終えてから消す。
	ErrRecording = errors.New("the recording is in progress")
	// ErrInvalidTags は、タグが正しくないことを表す。
	ErrInvalidTags = errors.New("invalid tags")
	// ErrInvalidPosition は、再生位置が正しくないことを表す。
	ErrInvalidPosition = errors.New("invalid watch position")
)

// Database は、録画を変えるのに使う DB。監査ログと一緒に書くため、トランザクションを始められる。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Library は、録画済みの番組を扱う。
type Library struct {
	clock    clock.Clock
	recorder audit.Recorder
	store    *kago.Store
	logger   *slog.Logger
}

// New は Library を返す。store は、録画ファイルを消すのに使う、このインスタンスにマウントされたプール。
func New(clk clock.Clock, recorder audit.Recorder, store *kago.Store, logger *slog.Logger) *Library {
	return &Library{clock: clk, recorder: recorder, store: store, logger: logger}
}

// Delete は、録画を DB から消し、監査ログに残してから、録画ファイルと生成物を消す。録画履歴は残す。
// 保護した録画は ErrProtected を、録画中の録画は ErrRecording を返して消さない。reason は、監査ログに残す消した理由。
// ファイルを先に消すと、DB を書けなかったときに、ファイルのない録画が残るので、DB を先に書く。
func (l *Library) Delete(ctx context.Context, db Database, actor *audit.Actor, recordingID uuid.UUID, reason string) error {
	files := []sqlc.RecordingFile{}
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		recording, err := lock(ctx, q, recordingID)
		if err != nil {
			return err
		}
		switch {
		case recording.Protected:
			return ErrProtected
		case recording.State == stateRecording:
			return ErrRecording
		}
		if files, err = q.ListRecordingFiles(ctx, recordingID); err != nil {
			return fmt.Errorf("failed to list the recording files: %w", err)
		}
		if _, err := q.DeleteRecording(ctx, recordingID); err != nil {
			return fmt.Errorf("failed to delete the recording: %w", err)
		}

		return l.record(ctx, tx, actor, "recording.deleted", &recording, map[string]any{"reason": reason})
	})
	if err != nil {
		return err
	}
	for i := range files {
		// 索引や画像は、セグメントのファイルの ID の生成物のディレクトリにあり、録画ファイルと一緒に消す。
		if files[i].Kind != segmentKind && files[i].Kind != otherKind {
			continue
		}
		if err := l.store.Remove(files[i].PoolID, files[i].Path, files[i].ID); err != nil {
			l.logger.WarnContext(ctx, "failed to remove a recording file", slog.String("path", files[i].Path), slog.Any("error", err))
		}
	}

	return nil
}

// SetProtected は、録画に保護の印を付けるか、外す。保護した録画は、容量の管理でも、ユーザーの操作でも消さない。
func (l *Library) SetProtected(ctx context.Context, db Database, actor *audit.Actor, recordingID uuid.UUID, protected bool) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		recording, err := lock(ctx, q, recordingID)
		if err != nil {
			return err
		}
		if recording.Protected == protected {
			return nil
		}
		if err := q.SetRecordingProtected(ctx, sqlc.SetRecordingProtectedParams{Protected: protected, Now: l.clock.Now(), ID: recordingID}); err != nil {
			return fmt.Errorf("failed to set the protection: %w", err)
		}
		action := "recording.protected"
		if !protected {
			action = "recording.unprotected"
		}

		return l.record(ctx, tx, actor, action, &recording, map[string]any{})
	})
}

// SetTags は、録画のタグを names に置き換える。名前の前後の空白は落とし、重なりは除く。ないタグは作る。
func (l *Library) SetTags(ctx context.Context, db Database, actor *audit.Actor, recordingID uuid.UUID, names []string) ([]string, error) {
	normalized, err := normalizeTags(names)
	if err != nil {
		return nil, err
	}
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		recording, err := lock(ctx, q, recordingID)
		if err != nil {
			return err
		}
		before, err := q.ListRecordingTags(ctx, []uuid.UUID{recordingID})
		if err != nil {
			return fmt.Errorf("failed to list the recording tags: %w", err)
		}
		previous := make([]string, 0, len(before))
		for _, row := range before {
			previous = append(previous, row.Name)
		}
		// 変わらなければ、書かず、監査ログにも残さない。
		if slices.Equal(previous, normalized) {
			return nil
		}
		if err := q.DeleteRecordingTags(ctx, recordingID); err != nil {
			return fmt.Errorf("failed to clear the recording tags: %w", err)
		}
		now := l.clock.Now()
		for _, name := range normalized {
			tagID, err := q.UpsertTag(ctx, sqlc.UpsertTagParams{ID: id.New(), Name: name, Now: now})
			if err != nil {
				return fmt.Errorf("failed to save the tag: %w", err)
			}
			if err := q.InsertRecordingTag(ctx, sqlc.InsertRecordingTagParams{RecordingID: recordingID, TagID: tagID}); err != nil {
				return fmt.Errorf("failed to tag the recording: %w", err)
			}
		}

		return l.record(ctx, tx, actor, "recording.tagged", &recording, map[string]any{
			"tags": map[string]any{"from": previous, "to": normalized},
		})
	})
	if err != nil {
		return nil, err
	}

	return normalized, nil
}

// SaveProgress は、ユーザーの録画の再生位置を保存する。再生位置は個人のものなので、監査ログには残さない。
func (l *Library) SaveProgress(ctx context.Context, db sqlc.DBTX, userID, recordingID uuid.UUID, position time.Duration) error {
	if position < 0 {
		return fmt.Errorf("%w: the position must not be negative", ErrInvalidPosition)
	}
	err := sqlc.New(db).UpsertWatchProgress(ctx, sqlc.UpsertWatchProgressParams{
		UserID: userID, RecordingID: recordingID, PositionMs: position.Milliseconds(), Now: l.clock.Now(),
	})
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == foreignKeyViolation {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to save the watch progress: %w", err)
	}

	return nil
}

// foreignKeyViolation は、PostgreSQL の外部キーの違反のエラーコード。録画がないときに返る。
const foreignKeyViolation = "23503"

// record は、録画の操作を監査ログに残す。要約には、録画の名前を加える。
// 録画を変えたときに必ず呼ぶので、ここで録画の変更も知らせる。
func (l *Library) record(ctx context.Context, tx pgx.Tx, actor *audit.Actor, action string, recording *sqlc.Recording, summary map[string]any) error {
	summary["name"] = recording.Name
	if _, err := notify.Publish(ctx, tx, l.clock, notify.RecordingsChannel); err != nil {
		return err
	}

	return l.recorder.Record(ctx, tx, &audit.Event{
		Actor:      actor,
		Action:     action,
		TargetType: auditTarget,
		TargetID:   recording.ID.String(),
		Summary:    summary,
		// 送信元は、操作者 (actor) に持たせる。
		SourceIP:  netip.Addr{},
		UserAgent: "",
	})
}

func lock(ctx context.Context, q *sqlc.Queries, recordingID uuid.UUID) (sqlc.Recording, error) {
	recording, err := q.LockRecording(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return recording, ErrNotFound
	}
	if err != nil {
		return recording, fmt.Errorf("failed to lock the recording: %w", err)
	}

	return recording, nil
}

// normalizeTags は、タグの名前の前後の空白を落とし、重なりを除いて、名前の順に並べる。
func normalizeTags(names []string) ([]string, error) {
	normalized := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || utf8.RuneCountInString(name) > maxTagLength {
			return nil, fmt.Errorf("%w: a tag must be 1 to %d characters", ErrInvalidTags, maxTagLength)
		}
		if !slices.Contains(normalized, name) {
			normalized = append(normalized, name)
		}
	}
	if len(normalized) > maxTags {
		return nil, fmt.Errorf("%w: a recording can have at most %d tags", ErrInvalidTags, maxTags)
	}
	slices.Sort(normalized)

	return normalized, nil
}
