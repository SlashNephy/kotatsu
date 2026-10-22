package oauth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

const actionGrantDelete = "oauth_grant.delete"

// ErrGrantNotFound は、連携がないことを表す。
var ErrGrantNotFound = errors.New("OAuth grant not found")

// GrantRecord は、ユーザーの連携の記録。トークンそのものは持たない。
type GrantRecord struct {
	ID         uuid.UUID
	ClientID   string
	ClientName string
	Scopes     []string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// ListGrants は、ユーザーの連携を、新しい順に返す。MCP を無効にしていても返す。取り消せるようにするためである。
func (s *Server) ListGrants(ctx context.Context, userID uuid.UUID) ([]*GrantRecord, error) {
	rows, err := sqlc.New(s.db).ListOAuthGrants(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the OAuth grants: %w", err)
	}
	grants := make([]*GrantRecord, 0, len(rows))
	for _, row := range rows {
		grants = append(grants, &GrantRecord{
			ID: row.ID, ClientID: row.ClientID, ClientName: row.ClientName, Scopes: row.Scopes,
			CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt,
		})
	}

	return grants, nil
}

// DeleteGrant は、ユーザーの連携を取り消す。連携のトークンもまとめて使えなくなる。
// ほかのユーザーの連携なら ErrGrantNotFound を返す。取り消したことは監査ログに残す。
func (s *Server) DeleteGrant(ctx context.Context, actor *audit.Actor, userID, grantID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		row, err := sqlc.New(tx).DeleteUserOAuthGrant(ctx, sqlc.DeleteUserOAuthGrantParams{ID: grantID, UserID: userID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGrantNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to delete the OAuth grant: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionGrantDelete, TargetType: targetOAuthClient, TargetID: row.ClientID,
			Summary: map[string]any{"client_name": row.ClientName}, SourceIP: actor.SourceIP, UserAgent: actor.UserAgent,
		})
	})
}
