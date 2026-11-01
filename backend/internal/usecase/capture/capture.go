// Package capture は、視聴画面で撮ったキャプチャを、サーバーに残す (設計書 7.5、#908)。
//
// キャプチャは、撮ったユーザーだけのものとする。画像はデータディレクトリの「captures/ユーザーの ID/キャプチャの ID.png」に置き、
// 撮った場面の情報 (サービス名、番組名、放送時刻、録画と再生位置、字幕の文) は captures の行に持つ。
// 枚数の上限は設けず、録画を消してもキャプチャは残す。
package capture

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/query"
)

const (
	// DefaultMaxImageBytes は、受け付ける画像の大きさの上限の既定値。画面は 1920x1080 の PNG を送り、1 枚は数百 KB から数 MB ほど。
	DefaultMaxImageBytes = 20 << 20
	// maxServiceNameBytes、maxProgramNameBytes、maxCaptionBytes は、撮った場面の情報の長さの上限。
	// どれも放送から取った文字なので、普通はこれよりずっと短い。
	maxServiceNameBytes = 1 << 10
	maxProgramNameBytes = 4 << 10
	maxCaptionBytes     = 64 << 10
)

// pngSignature は、PNG のファイルの先頭の 8 バイト。
const pngSignature = "\x89PNG\r\n\x1a\n"

var (
	// ErrUnavailable は、データディレクトリを設定していないので、キャプチャを残せないことを表す。
	ErrUnavailable = errors.New("captures are unavailable because the data directory is not configured")
	// ErrInvalidImage は、画像が PNG でないことを表す。
	ErrInvalidImage = errors.New("the capture image is not a PNG")
	// ErrTooLarge は、画像が大きすぎることを表す。
	ErrTooLarge = errors.New("the capture image is too large")
	// ErrInvalidMetadata は、撮った場面の情報が足りないか、長すぎることを表す。
	ErrInvalidMetadata = errors.New("invalid capture metadata")
	// ErrNotFound は、キャプチャがないか、ほかのユーザーのものであることを表す。
	ErrNotFound = errors.New("capture not found")
)

// Upload は、残すキャプチャ。
type Upload struct {
	ServiceName string
	// ProgramName は、番組名。番組が分からなければ nil。
	ProgramName *string
	// Caption は、撮ったときに出ていた字幕の文。出ていなければ nil。
	Caption *string
	// RecordingID と Position は、録画で撮ったときの録画と、録画の始まりからの再生位置。録画でなければ nil。
	RecordingID *uuid.UUID
	Position    *time.Duration
	// BroadcastAt は、撮った場面の放送時刻。TakenAt は、撮った時刻 (端末の時計)。
	BroadcastAt time.Time
	TakenAt     time.Time
	// Image は、PNG の画像。画面が番組情報を書き込んだもの (#458) を、そのまま残す。
	Image []byte
}

// Captures は、キャプチャを残し、消す。
type Captures struct {
	store         *kago.Store
	clock         clock.Clock
	logger        *slog.Logger
	maxImageBytes int
}

// New は Captures を返す。store は、このインスタンスにマウントしたデータディレクトリを持つもの。
// maxImageBytes が 0 以下なら DefaultMaxImageBytes にする。
func New(store *kago.Store, clk clock.Clock, logger *slog.Logger, maxImageBytes int) *Captures {
	if maxImageBytes <= 0 {
		maxImageBytes = DefaultMaxImageBytes
	}

	return &Captures{store: store, clock: clk, logger: logger, maxImageBytes: maxImageBytes}
}

// MaxImageBytes は、受け付ける画像の大きさの上限を返す。
func (c *Captures) MaxImageBytes() int {
	return c.maxImageBytes
}

// Available は、このインスタンスでキャプチャを残せるか (データディレクトリを設定してあるか) を返す。
func (c *Captures) Available() bool {
	return c.store.HasDataDir()
}

// Save は、ユーザー userID のキャプチャ upload を残し、残したものを返す。
// 画像はデータディレクトリに書いてから行を記録する。記録できなければ、書いた画像を消す。
// 録画が見つからなければ (撮った後に消されたなど)、録画の参照と再生位置を持たずに残す。
func (c *Captures) Save(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, upload *Upload) (*domain.Capture, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	if err := c.validate(upload); err != nil {
		return nil, err
	}
	captureID := id.New()
	location := kago.CaptureLocation(userID, captureID)
	if err := c.store.WriteFile(location.PoolID, location.Path, upload.Image); err != nil {
		return nil, fmt.Errorf("failed to write the capture image: %w", err)
	}
	var position *int64
	if upload.Position != nil {
		position = new(upload.Position.Milliseconds())
	}
	programName := ""
	if upload.ProgramName != nil {
		programName = *upload.ProgramName
	}
	row, err := sqlc.New(db).InsertCapture(ctx, sqlc.InsertCaptureParams{
		ID:                    captureID,
		UserID:                userID,
		PositionMs:            position,
		ServiceName:           upload.ServiceName,
		ProgramName:           upload.ProgramName,
		ServiceNameNormalized: domain.Normalize(upload.ServiceName),
		ProgramNameNormalized: domain.Normalize(programName),
		Caption:               upload.Caption,
		BroadcastAt:           upload.BroadcastAt,
		TakenAt:               upload.TakenAt,
		Size:                  int64(len(upload.Image)),
		Now:                   c.clock.Now(),
		RecordingID:           upload.RecordingID,
	})
	if err != nil {
		if removeErr := c.store.Remove(location.PoolID, location.Path, uuid.Nil); removeErr != nil {
			c.logger.WarnContext(ctx, "failed to remove the capture image after the insert failed",
				slog.String("path", location.Path), slog.Any("error", removeErr))
		}

		return nil, fmt.Errorf("failed to insert the capture: %w", err)
	}

	return query.CaptureFromRow(&row), nil
}

// Delete は、ユーザー userID のキャプチャ captureID を消す。なければ (ほかのユーザーのものも) ErrNotFound を返す。
// 行を消してから画像を消す。画像を消せなくても、行は戻さない。残った画像は、DB にないファイルとして、
// 管理者が保存先の画面から消せる (設計書 5.6)。行を残して画像だけを消すと、一覧に開けないキャプチャが残るためである。
func (c *Captures) Delete(ctx context.Context, db sqlc.DBTX, userID, captureID uuid.UUID) error {
	// データディレクトリのないインスタンスで行だけを消すと、画像が残り続けるので、消さない。
	if !c.Available() {
		return ErrUnavailable
	}
	row, err := sqlc.New(db).DeleteCapture(ctx, sqlc.DeleteCaptureParams{ID: captureID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to delete the capture: %w", err)
	}
	location := kago.CaptureLocation(row.UserID, row.ID)
	if err := c.store.Remove(location.PoolID, location.Path, uuid.Nil); err != nil {
		c.logger.WarnContext(ctx, "failed to remove the capture image", slog.String("path", location.Path), slog.Any("error", err))
	}

	return nil
}

// validate は、残すキャプチャの情報と画像を確かめる。
func (c *Captures) validate(upload *Upload) error {
	if len(upload.Image) > c.maxImageBytes {
		return fmt.Errorf("%w: %d bytes (at most %d)", ErrTooLarge, len(upload.Image), c.maxImageBytes)
	}
	if !bytes.HasPrefix(upload.Image, []byte(pngSignature)) {
		return ErrInvalidImage
	}
	if strings.TrimSpace(upload.ServiceName) == "" {
		return fmt.Errorf("%w: the service name is required", ErrInvalidMetadata)
	}
	for _, text := range []struct {
		name  string
		value *string
		limit int
	}{
		{name: "service name", value: &upload.ServiceName, limit: maxServiceNameBytes},
		{name: "program name", value: upload.ProgramName, limit: maxProgramNameBytes},
		{name: "caption", value: upload.Caption, limit: maxCaptionBytes},
	} {
		if text.value == nil {
			continue
		}
		if len(*text.value) > text.limit || !utf8.ValidString(*text.value) || strings.ContainsRune(*text.value, 0) {
			return fmt.Errorf("%w: the %s is too long or not valid text", ErrInvalidMetadata, text.name)
		}
	}
	if upload.BroadcastAt.IsZero() || upload.TakenAt.IsZero() {
		return fmt.Errorf("%w: the broadcast time and the time taken are required", ErrInvalidMetadata)
	}
	if upload.Position != nil && (*upload.Position < 0 || upload.RecordingID == nil) {
		return fmt.Errorf("%w: the position must be non-negative and comes with a recording", ErrInvalidMetadata)
	}

	return nil
}
