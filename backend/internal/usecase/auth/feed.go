package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

const (
	// feedBytes は、秘密の値のランダムな部分の長さ。
	feedBytes = 32
	// feedTouchInterval は、読まれた日時を書き直す間隔。アプリは何度も読みに来るので、毎回は書かない。
	feedTouchInterval = time.Minute

	feedKindCalendar = "calendar"
	feedKindIPTV     = "iptv"
)

// ErrFeedNotFound は、配信の URL を発行していないことを表す。
var ErrFeedNotFound = errors.New("feed not found")

// Feed は、発行した配信の URL の記録。秘密の値そのものは持たない。
type Feed struct {
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// Feeds は、Cookie もヘッダーも送れないアプリに配信する URL を発行し、確かめる (設計書 9.8)。
// 予約の iCalendar (#1255) と、ライブ視聴の M3U と番組表の XMLTV (#1324) で、別々の秘密の値を使う。
// アプリは Cookie もヘッダーも送れないので、URL に秘密の値を入れて認証する。
// 秘密の値は発行したときにだけ返し、DB には SHA-256 だけを保存する。ユーザーと種類ごとに 1 つだけ持ち、発行し直すと前の URL は使えなくなる。
type Feeds struct {
	pool     *pgxpool.Pool
	clock    clock.Clock
	recorder audit.Recorder
	// kind は、DB に持つ種類。mark は、秘密の値の頭に付ける印で、API トークンやほかの種類と見分け、漏れたときに見つけやすくする。
	kind string
	mark string
}

// NewCalendarFeeds は、予約を iCalendar で購読する URL (#1255) の Feeds を返す。
func NewCalendarFeeds(pool *pgxpool.Pool, clk clock.Clock, recorder audit.Recorder) *Feeds {
	return &Feeds{pool: pool, clock: clk, recorder: recorder, kind: feedKindCalendar, mark: "kotatsu_cal_"}
}

// NewIPTVFeeds は、ライブ視聴の M3U と番組表の XMLTV を配信する URL (#1324) の Feeds を返す。
func NewIPTVFeeds(pool *pgxpool.Pool, clk clock.Clock, recorder audit.Recorder) *Feeds {
	return &Feeds{pool: pool, clock: clk, recorder: recorder, kind: feedKindIPTV, mark: "kotatsu_iptv_"}
}

// Create は、ユーザーの配信の URL の秘密の値を発行し、秘密の値とその記録を返す。すでにあれば差し替える。
func (c *Feeds) Create(ctx context.Context, actor *audit.Actor, userID uuid.UUID) (string, *Feed, error) {
	raw := make([]byte, feedBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("failed to generate a feed token: %w", err)
	}
	token := c.mark + base64.RawURLEncoding.EncodeToString(raw)
	feed := &Feed{CreatedAt: c.clock.Now(), LastUsedAt: nil}
	err := pgx.BeginFunc(ctx, c.pool, func(tx pgx.Tx) error {
		err := sqlc.New(tx).UpsertCalendarFeed(ctx, sqlc.UpsertCalendarFeedParams{
			UserID: userID, Kind: c.kind, TokenHash: hashToken(token), Now: feed.CreatedAt,
		})
		if err != nil {
			return fmt.Errorf("failed to save the feed: %w", err)
		}

		return c.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: c.action("create"), TargetType: c.target(), TargetID: userID.String(),
			Summary: map[string]string{}, SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
	if err != nil {
		return "", nil, err
	}

	return token, feed, nil
}

// Get は、ユーザーの配信の URL の記録を返す。発行していなければ ErrFeedNotFound を返す。
func (c *Feeds) Get(ctx context.Context, userID uuid.UUID) (*Feed, error) {
	row, err := sqlc.New(c.pool).GetCalendarFeed(ctx, sqlc.GetCalendarFeedParams{UserID: userID, Kind: c.kind})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFeedNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the feed: %w", err)
	}

	return &Feed{CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt}, nil
}

// Delete は、ユーザーの配信の URL を取り消す。発行していなければ ErrFeedNotFound を返す。
func (c *Feeds) Delete(ctx context.Context, actor *audit.Actor, userID uuid.UUID) error {
	return pgx.BeginFunc(ctx, c.pool, func(tx pgx.Tx) error {
		deleted, err := sqlc.New(tx).DeleteCalendarFeed(ctx, sqlc.DeleteCalendarFeedParams{UserID: userID, Kind: c.kind})
		if err != nil {
			return fmt.Errorf("failed to delete the feed: %w", err)
		}
		if deleted == 0 {
			return ErrFeedNotFound
		}

		return c.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: c.action("delete"), TargetType: c.target(), TargetID: userID.String(),
			Summary: map[string]string{}, SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
}

// Authenticate は、配信の URL の秘密の値から、発行したユーザーを返す。値が違うか、ほかの種類の値か、
// ユーザーを無効にしていれば ErrUnauthenticated を返す。
// 返す Principal は、セッションを持たず、認証した日時も持たないので、配信を読むのにだけ使う。
func (c *Feeds) Authenticate(ctx context.Context, token string) (*Principal, error) {
	if !strings.HasPrefix(token, c.mark) {
		return nil, ErrUnauthenticated
	}
	q := sqlc.New(c.pool)
	row, err := q.GetCalendarFeedUser(ctx, sqlc.GetCalendarFeedUserParams{TokenHash: hashToken(token), Kind: c.kind})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up the feed: %w", err)
	}
	role := row.Role
	now := c.clock.Now()
	if row.LastUsedAt == nil || now.Sub(*row.LastUsedAt) >= feedTouchInterval {
		if err := q.TouchCalendarFeed(ctx, sqlc.TouchCalendarFeedParams{UserID: row.UserID, Kind: c.kind, Now: &now}); err != nil {
			return nil, fmt.Errorf("failed to record the feed use: %w", err)
		}
	}

	return &Principal{UserID: row.UserID, Role: role, SessionID: uuid.Nil(), AuthenticatedAt: time.Time{}, OAuthClientID: ""}, nil
}

// target は、監査ログの対象の種類。action は、操作の名前 (calendar_feed.create など)。
func (c *Feeds) target() string {
	return c.kind + "_feed"
}

func (c *Feeds) action(verb string) string {
	return c.target() + "." + verb
}
