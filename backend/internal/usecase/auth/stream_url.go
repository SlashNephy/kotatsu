package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

const (
	actionStreamURLCreate = "stream_url.create"
	actionStreamURLDelete = "stream_url.delete"
	targetStreamURL       = "stream_url"
	summaryTarget         = "target"
	summaryUser           = "userId"
	// liveResourcePrefix は、ライブ視聴の署名付き URL のパスの頭。一覧で、ライブか録画かを見分けるのに使う。
	liveResourcePrefix = "/api/streams/services/"
)

// ErrStreamURLNotFound は、URL の記録がないことを表す。
var ErrStreamURLNotFound = errors.New("stream URL not found")

// StreamURLActiveWindow は、期限を過ぎた URL を、まだ流れ続けているとみなす、最後に使われてからの時間 (#1757)。
// 期限は開くときにだけ確かめるので、期限の前に開いた配信は、期限を過ぎても流れ続ける。流している間は、確かめ直すたびに
// (最長で urlTouchInterval ごとに) 使われた日時を書き直すので、それより十分に長くする。この時間の内に使われた記録は、
// 期限が過ぎても消さず、一覧に出して取り消せるようにする。
const StreamURLActiveWindow = 10 * time.Minute

// IssuedStreamURL は、発行した外部プレイヤー用の署名付き URL の記録 (#1757)。URL そのものは持たない。
type IssuedStreamURL struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	UserDisplayName string
	// TargetName は、発行したときのサービス名か録画の名前。
	TargetName string
	// Live は、ライブ視聴の URL かどうか。false なら録画の URL。
	Live       bool
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastUsedAt *time.Time
}

// NewStreamURL は、記録する外部プレイヤー用の署名付き URL。
type NewStreamURL struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Resource   string
	TargetName string
	ExpiresAt  time.Time
}

// StreamURLs は、外部プレイヤー用の署名付き URL の記録を残し、管理者に一覧させ、1 本ずつ取り消させる (#1757、設計書 7.6、9.6)。
// URL の署名には記録の ID を含めるので、記録を消すと、その URL は期限の前でも使えなくなる。
type StreamURLs struct {
	db       Database
	clock    clock.Clock
	recorder audit.Recorder
	users    *URLUsers
}

// NewStreamURLs は StreamURLs を返す。users は、取り消した URL について覚えた結果を忘れさせるのに使う。nil なら忘れさせない。
func NewStreamURLs(db Database, clk clock.Clock, recorder audit.Recorder, users *URLUsers) *StreamURLs {
	return &StreamURLs{db: db, clock: clk, recorder: recorder, users: users}
}

// Create は、発行した URL を記録し、監査ログに残す。あわせて、期限が過ぎ、流れ続けてもいない記録を消す。
func (s *StreamURLs) Create(ctx context.Context, actor *audit.Actor, issued *NewStreamURL) error {
	now := s.clock.Now()

	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.DeleteExpiredStreamURLs(ctx, sqlc.DeleteExpiredStreamURLsParams{Now: now, ActiveSince: now.Add(-StreamURLActiveWindow)}); err != nil {
			return fmt.Errorf("failed to delete the expired stream URLs: %w", err)
		}
		err := q.CreateStreamURL(ctx, sqlc.CreateStreamURLParams{
			ID: issued.ID, UserID: issued.UserID, Resource: issued.Resource, TargetName: issued.TargetName, Now: now, ExpiresAt: issued.ExpiresAt,
		})
		if err != nil {
			return fmt.Errorf("failed to save the stream URL: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionStreamURLCreate, TargetType: targetStreamURL, TargetID: issued.ID.String(),
			Summary: map[string]string{summaryTarget: issued.TargetName}, SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
}

// List は、期限の過ぎていない URL と、期限を過ぎても流れ続けている URL の記録を、新しい順に返す。
func (s *StreamURLs) List(ctx context.Context) ([]*IssuedStreamURL, error) {
	now := s.clock.Now()
	rows, err := sqlc.New(s.db).ListStreamURLs(ctx, sqlc.ListStreamURLsParams{Now: now, ActiveSince: now.Add(-StreamURLActiveWindow)})
	if err != nil {
		return nil, fmt.Errorf("failed to list the stream URLs: %w", err)
	}
	urls := make([]*IssuedStreamURL, 0, len(rows))
	for _, row := range rows {
		urls = append(urls, &IssuedStreamURL{
			ID: row.ID, UserID: row.UserID, UserDisplayName: row.DisplayName, TargetName: row.TargetName,
			Live: strings.HasPrefix(row.Resource, liveResourcePrefix), CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, LastUsedAt: row.LastUsedAt,
		})
	}

	return urls, nil
}

// Delete は、URL の記録を消して取り消し、監査ログに残す。記録がなければ ErrStreamURLNotFound を返す。
// 消した URL は、次に開くときと、開いたままの配信を確かめ直すときに断られる。
func (s *StreamURLs) Delete(ctx context.Context, actor *audit.Actor, urlID uuid.UUID) error {
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		row, err := sqlc.New(tx).DeleteStreamURL(ctx, urlID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStreamURLNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to delete the stream URL: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionStreamURLDelete, TargetType: targetStreamURL, TargetID: urlID.String(),
			Summary: map[string]string{summaryTarget: row.TargetName, summaryUser: row.UserID.String()}, SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
	if err != nil {
		return err
	}
	if s.users != nil {
		s.users.ForgetURL(urlID)
	}

	return nil
}
