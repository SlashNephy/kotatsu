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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

const (
	// calendarFeedMark は、予約の購読用の URL の秘密の値の頭に付ける印。API トークンと見分け、漏れたときに見つけやすくする。
	calendarFeedMark = "kotatsu_cal_"
	// calendarFeedBytes は、秘密の値のランダムな部分の長さ。
	calendarFeedBytes = 32
	// calendarFeedTouchInterval は、読まれた日時を書き直す間隔。カレンダーのアプリは何度も読みに来るので、毎回は書かない。
	calendarFeedTouchInterval = time.Minute

	actionCalendarFeedCreate = "calendar_feed.create"
	actionCalendarFeedDelete = "calendar_feed.delete"
	targetCalendarFeed       = "calendar_feed"
)

// ErrCalendarFeedNotFound は、購読用の URL を発行していないことを表す。
var ErrCalendarFeedNotFound = errors.New("calendar feed not found")

// CalendarFeed は、発行した購読用の URL の記録。秘密の値そのものは持たない。
type CalendarFeed struct {
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// CalendarFeeds は、予約を iCalendar で購読する URL (#1255、設計書 9.8) を発行し、確かめる。
// カレンダーのアプリは Cookie もヘッダーも送れないので、URL に秘密の値を入れて認証する。
// 秘密の値は発行したときにだけ返し、DB には SHA-256 だけを保存する。ユーザーごとに 1 つだけ持ち、発行し直すと前の URL は使えなくなる。
type CalendarFeeds struct {
	pool     *pgxpool.Pool
	clock    clock.Clock
	recorder audit.Recorder
}

// NewCalendarFeeds は CalendarFeeds を返す。
func NewCalendarFeeds(pool *pgxpool.Pool, clk clock.Clock, recorder audit.Recorder) *CalendarFeeds {
	return &CalendarFeeds{pool: pool, clock: clk, recorder: recorder}
}

// Create は、ユーザーの購読用の URL の秘密の値を発行し、秘密の値とその記録を返す。すでにあれば差し替える。
func (c *CalendarFeeds) Create(ctx context.Context, actor *audit.Actor, userID uuid.UUID) (string, *CalendarFeed, error) {
	raw := make([]byte, calendarFeedBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("failed to generate a calendar feed token: %w", err)
	}
	token := calendarFeedMark + base64.RawURLEncoding.EncodeToString(raw)
	feed := &CalendarFeed{CreatedAt: c.clock.Now(), LastUsedAt: nil}
	err := pgx.BeginFunc(ctx, c.pool, func(tx pgx.Tx) error {
		err := sqlc.New(tx).UpsertCalendarFeed(ctx, sqlc.UpsertCalendarFeedParams{UserID: userID, TokenHash: hashToken(token), Now: feed.CreatedAt})
		if err != nil {
			return fmt.Errorf("failed to save the calendar feed: %w", err)
		}

		return c.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionCalendarFeedCreate, TargetType: targetCalendarFeed, TargetID: userID.String(),
			Summary: map[string]string{}, SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
	if err != nil {
		return "", nil, err
	}

	return token, feed, nil
}

// Get は、ユーザーの購読用の URL の記録を返す。発行していなければ ErrCalendarFeedNotFound を返す。
func (c *CalendarFeeds) Get(ctx context.Context, userID uuid.UUID) (*CalendarFeed, error) {
	row, err := sqlc.New(c.pool).GetCalendarFeed(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCalendarFeedNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the calendar feed: %w", err)
	}

	return &CalendarFeed{CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt}, nil
}

// Delete は、ユーザーの購読用の URL を取り消す。発行していなければ ErrCalendarFeedNotFound を返す。
func (c *CalendarFeeds) Delete(ctx context.Context, actor *audit.Actor, userID uuid.UUID) error {
	return pgx.BeginFunc(ctx, c.pool, func(tx pgx.Tx) error {
		deleted, err := sqlc.New(tx).DeleteCalendarFeed(ctx, userID)
		if err != nil {
			return fmt.Errorf("failed to delete the calendar feed: %w", err)
		}
		if deleted == 0 {
			return ErrCalendarFeedNotFound
		}

		return c.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionCalendarFeedDelete, TargetType: targetCalendarFeed, TargetID: userID.String(),
			Summary: map[string]string{}, SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
}

// Authenticate は、購読用の URL の秘密の値から、発行したユーザーを返す。値が違うか、ユーザーを無効にしていれば ErrUnauthenticated を返す。
// 返す Principal は、セッションを持たず、認証した日時も持たないので、予約を読むのにだけ使う。
func (c *CalendarFeeds) Authenticate(ctx context.Context, token string) (*Principal, error) {
	if !strings.HasPrefix(token, calendarFeedMark) {
		return nil, ErrUnauthenticated
	}
	q := sqlc.New(c.pool)
	row, err := q.GetCalendarFeedUser(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up the calendar feed: %w", err)
	}
	role, err := ParseRole(row.Role)
	if err != nil {
		return nil, err
	}
	now := c.clock.Now()
	if row.LastUsedAt == nil || now.Sub(*row.LastUsedAt) >= calendarFeedTouchInterval {
		if err := q.TouchCalendarFeed(ctx, sqlc.TouchCalendarFeedParams{UserID: row.UserID, Now: &now}); err != nil {
			return nil, fmt.Errorf("failed to record the calendar feed use: %w", err)
		}
	}

	return &Principal{UserID: row.UserID, Role: role, SessionID: uuid.Nil, AuthenticatedAt: time.Time{}, OAuthClientID: ""}, nil
}
