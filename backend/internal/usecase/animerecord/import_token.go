package animerecord

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

const (
	// importSealPurpose は、作品 ID を引くための Annict のトークンを暗号化する用途 (設計書 8.4、#1223)。
	importSealPurpose = "kotatsu/anime-import-token/v1"
	// auditTargetImportToken は、監査ログの対象の種類。
	auditTargetImportToken = "anime_import_token"
	// maxImportTokenLength は、管理者が入れる Annict のトークンの長さの上限。
	maxImportTokenLength = 256
)

// ErrInvalidImportToken は、管理者が入れた Annict のトークンが正しくないことを表す。
var ErrInvalidImportToken = errors.New("invalid Annict token")

var _ anime.TokenSource = new(Service)

// ImportTokenStatus は、作品 ID を引くための Annict のトークンの様子。トークンは返さない。
type ImportTokenStatus struct {
	Configured bool
	UpdatedAt  *time.Time
}

// ImportToken は、管理者が設定した、作品 ID を引くための Annict のトークンの様子を返す (#1223)。
func (s *Service) ImportToken(ctx context.Context, db sqlc.DBTX) (*ImportTokenStatus, error) {
	row, err := sqlc.New(db).GetAnimeImportToken(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return &ImportTokenStatus{Configured: false, UpdatedAt: nil}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the anime import token: %w", err)
	}

	return &ImportTokenStatus{Configured: true, UpdatedAt: new(row.UpdatedAt)}, nil
}

// AnnictImportToken は、管理者が設定した、作品 ID を引くための Annict のトークンを返す。設定していなければ空を返す。
func (s *Service) AnnictImportToken(ctx context.Context, db sqlc.DBTX) (string, error) {
	row, err := sqlc.New(db).GetAnimeImportToken(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get the anime import token: %w", err)
	}
	token, err := s.importBox.Open(row.AnnictTokenEncrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt the anime import token: %w", err)
	}

	return string(token), nil
}

// UpdateImportToken は、作品 ID を引くための Annict のトークンを変える (#1223)。空にすると、消す。
// サーバーが外部へ問い合わせるときに使う値なので、呼ぶ側で、直近の再認証を確かめる (設計書 9.5)。
// 監査ログには、変えたことだけを残し、値は残さない。
func (s *Service) UpdateImportToken(ctx context.Context, db Database, actor *audit.Actor, token string) error {
	token = strings.TrimSpace(token)
	if utf8.RuneCountInString(token) > maxImportTokenLength {
		return fmt.Errorf("%w: the token must be at most %d characters", ErrInvalidImportToken, maxImportTokenLength)
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if token == "" {
			deleted, err := q.DeleteAnimeImportToken(ctx)
			if err != nil {
				return fmt.Errorf("failed to delete the anime import token: %w", err)
			}
			if deleted == 0 {
				return nil
			}

			return s.recorder.Record(ctx, tx, &audit.Event{
				Actor: actor, Action: "anime_import_token.delete", TargetType: auditTargetImportToken, TargetID: "annict",
				Summary: map[string]any{}, SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
			})
		}
		encrypted, err := s.importBox.Seal([]byte(token))
		if err != nil {
			return err
		}
		if err := q.UpsertAnimeImportToken(ctx, sqlc.UpsertAnimeImportTokenParams{AnnictTokenEncrypted: encrypted, Now: s.clock.Now()}); err != nil {
			return fmt.Errorf("failed to save the anime import token: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: "anime_import_token.update", TargetType: auditTargetImportToken, TargetID: "annict",
			Summary: map[string]any{}, SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
}
