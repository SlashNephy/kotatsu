// Package player は、再生中のプレイヤーの登録と、ほかの端末からの操作を扱う (#459、設計書 7.7)。
//
// プレイヤーを開いた端末は、自分を登録し、状態を HeartbeatInterval ごとに送る。ExpireAfter の間送られてこなければ、
// 閉じたとみなす。同じユーザーのほかの端末は、一覧から選んで操作を送る。操作は、操作される端末が subscription で受け取る。
// どの tenban を経由しても届くよう、登録と操作は DB に書き、変更の通知 (設計書 1.4) で知らせる。
package player

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/notify"
)

const (
	// NotifyChannel は、プレイヤーの一覧の変化と、プレイヤーへの操作を知らせる、変更の通知のチャンネル。
	NotifyChannel = "players"
	// HeartbeatInterval は、プレイヤーが状態を送る間隔。ExpireAfter は、送られてこなければ閉じたとみなすまでの長さ。
	HeartbeatInterval = 10 * time.Second
	ExpireAfter       = 3 * HeartbeatInterval
	// commandTTL は、操作を残しておく長さ。操作される端末が、つなぎ直す間に届いた操作を読めるだけあればよい。
	commandTTL = time.Minute
)

// ErrNotFound は、プレイヤーがないか、閉じたか、ほかのユーザーかセッションのものであることを表す。
var ErrNotFound = errors.New("player not found")

// Player は、再生中のプレイヤー。
type Player struct {
	ID         uuid.UUID
	SessionID  uuid.UUID
	UserAgent  string
	State      []byte
	StartedAt  time.Time
	LastSeenAt time.Time
}

// Command は、プレイヤーへの操作。ID は、届いた順に増える。
type Command struct {
	ID      int64
	Command []byte
}

// Service は、プレイヤーを登録し、操作を届ける。
type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewService は Service を返す。
func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk}
}

// Register は、セッションのプレイヤーを登録し、その ID を返す。state は、再生の状態の JSON。
// あわせて、閉じたとみなすプレイヤーを消す。
func (s *Service) Register(ctx context.Context, userID, sessionID uuid.UUID, state []byte) (uuid.UUID, error) {
	playerID := id.New()
	now := s.clock.Now()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if _, err := q.DeleteStalePlayers(ctx, now.Add(-ExpireAfter)); err != nil {
			return fmt.Errorf("failed to delete the closed players: %w", err)
		}
		if err := q.CreatePlayer(ctx, sqlc.CreatePlayerParams{
			ID: playerID, UserID: userID, SessionID: sessionID, State: state, Now: now,
		}); err != nil {
			return fmt.Errorf("failed to register the player: %w", err)
		}

		return s.changed(ctx, tx, userID)
	})
	if err != nil {
		return uuid.Nil, err
	}

	return playerID, nil
}

// Update は、セッションのプレイヤーの状態を書き直し、閉じていないことを知らせる。
// announce なら、ほかの端末に、一覧を読み直させる。再生位置が進んだだけのときは、知らせなくてよい。
func (s *Service) Update(ctx context.Context, userID, sessionID, playerID uuid.UUID, state []byte, announce bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := sqlc.New(tx).UpdatePlayer(ctx, sqlc.UpdatePlayerParams{
			State: state, Now: s.clock.Now(), ID: playerID, SessionID: sessionID,
		})
		if err != nil {
			return fmt.Errorf("failed to update the player: %w", err)
		}
		if rows == 0 {
			return ErrNotFound
		}
		if !announce {
			return nil
		}

		return s.changed(ctx, tx, userID)
	})
}

// Unregister は、セッションのプレイヤーを取り消す。
func (s *Service) Unregister(ctx context.Context, userID, sessionID, playerID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := sqlc.New(tx).DeletePlayer(ctx, sqlc.DeletePlayerParams{ID: playerID, SessionID: sessionID})
		if err != nil {
			return fmt.Errorf("failed to unregister the player: %w", err)
		}
		if rows == 0 {
			return ErrNotFound
		}

		return s.changed(ctx, tx, userID)
	})
}

// List は、ユーザーの、閉じていないプレイヤーを、開いた順に返す。
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]*Player, error) {
	rows, err := sqlc.New(s.pool).ListPlayers(ctx, sqlc.ListPlayersParams{UserID: userID, AliveSince: s.aliveSince()})
	if err != nil {
		return nil, fmt.Errorf("failed to list the players: %w", err)
	}
	players := make([]*Player, 0, len(rows))
	for _, row := range rows {
		players = append(players, &Player{
			ID: row.ID, SessionID: row.SessionID, UserAgent: row.UserAgent, State: row.State,
			StartedAt: row.StartedAt, LastSeenAt: row.LastSeenAt,
		})
	}

	return players, nil
}

// Send は、ユーザーのプレイヤーに操作を送る。ほかのユーザーのプレイヤーと、閉じたプレイヤーには送らない。
func (s *Service) Send(ctx context.Context, userID, playerID uuid.UUID, command []byte) error {
	now := s.clock.Now()

	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		owner, err := q.GetPlayerOwner(ctx, sqlc.GetPlayerOwnerParams{ID: playerID, AliveSince: now.Add(-ExpireAfter)})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner.UserID != userID) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to get the player: %w", err)
		}
		if _, err := q.DeleteOldPlayerCommands(ctx, now.Add(-commandTTL)); err != nil {
			return fmt.Errorf("failed to delete the old commands: %w", err)
		}
		if _, err := q.CreatePlayerCommand(ctx, sqlc.CreatePlayerCommandParams{PlayerID: playerID, Command: command, Now: now}); err != nil {
			return fmt.Errorf("failed to send the command: %w", err)
		}
		if _, err := notify.Publish(ctx, tx, s.clock, NotifyChannel); err != nil {
			return fmt.Errorf("failed to notify the command: %w", err)
		}

		return nil
	})
}

// Owned は、プレイヤーが、セッションのものかどうかを確かめる。閉じたプレイヤーと、ほかのセッションのものなら ErrNotFound を返す。
func (s *Service) Owned(ctx context.Context, sessionID, playerID uuid.UUID) error {
	owner, err := sqlc.New(s.pool).GetPlayerOwner(ctx, sqlc.GetPlayerOwnerParams{ID: playerID, AliveSince: s.aliveSince()})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner.SessionID != sessionID) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get the player: %w", err)
	}

	return nil
}

// LastCommandID は、プレイヤーへの操作のうち、最も新しいものの ID を返す。操作がなければ 0。
func (s *Service) LastCommandID(ctx context.Context, playerID uuid.UUID) (int64, error) {
	last, err := sqlc.New(s.pool).LastPlayerCommandID(ctx, playerID)
	if err != nil {
		return 0, fmt.Errorf("failed to get the last command: %w", err)
	}

	return last, nil
}

// Commands は、プレイヤーへの操作のうち、after より新しいものを、届いた順に返す。
func (s *Service) Commands(ctx context.Context, playerID uuid.UUID, after int64) ([]*Command, error) {
	rows, err := sqlc.New(s.pool).ListPlayerCommands(ctx, sqlc.ListPlayerCommandsParams{PlayerID: playerID, After: after})
	if err != nil {
		return nil, fmt.Errorf("failed to list the commands: %w", err)
	}
	commands := make([]*Command, 0, len(rows))
	for _, row := range rows {
		commands = append(commands, &Command{ID: row.ID, Command: row.Command})
	}

	return commands, nil
}

// Version は、ユーザーの、再生中のプレイヤーの一覧の版を返す。
func (s *Service) Version(ctx context.Context, userID uuid.UUID) (int64, error) {
	version, err := sqlc.New(s.pool).GetPlayerVersion(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get the player version: %w", err)
	}

	return version, nil
}

// changed は、ユーザーのプレイヤーの一覧の版を上げ、ほかの端末に知らせる。
func (s *Service) changed(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	if err := sqlc.New(tx).BumpPlayerVersion(ctx, sqlc.BumpPlayerVersionParams{UserID: userID, Now: s.clock.Now()}); err != nil {
		return fmt.Errorf("failed to bump the player version: %w", err)
	}
	if _, err := notify.Publish(ctx, tx, s.clock, NotifyChannel); err != nil {
		return fmt.Errorf("failed to notify the players: %w", err)
	}

	return nil
}

func (s *Service) aliveSince() time.Time {
	return s.clock.Now().Add(-ExpireAfter)
}
