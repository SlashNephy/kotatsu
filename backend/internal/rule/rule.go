// Package rule は、ルールと、ルールから予約を作る照合を扱う (設計書 4.2、4.3、4.4)。
package rule

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/match"
	"github.com/SlashNephy/kotatsu/internal/reserve"
)

const (
	// NotifyChannel は、ルールが変わったことを知らせる変更通知のチャンネル。
	NotifyChannel = "rules"
	// auditTarget は、監査ログの対象の種類。
	auditTarget = "rule"
	// maxNameLength と maxMemoLength は、名前とメモの長さの上限。
	maxNameLength = 200
	maxMemoLength = 2000
	// maxDuplicateWindowDays は、重複を避ける期間の上限 (10 年)。
	maxDuplicateWindowDays = 3650
)

var (
	// ErrNotFound は、ルールがないことを表す。
	ErrNotFound = errors.New("rule not found")
	// ErrInvalidRule は、ルールの内容が正しくないことを表す。
	ErrInvalidRule = errors.New("invalid rule")
)

// Rule はルール。
type Rule struct {
	ID         uuid.UUID
	Name       string
	Memo       string
	Enabled    bool
	Conditions *match.Conditions
	Overrides  *reserve.Overrides
	// ValidFrom と ValidUntil は、ルール自体の有効期間。番組の開始時刻がこの範囲に入るときだけ予約する。nil なら区切らない。
	ValidFrom  *time.Time
	ValidUntil *time.Time
	// AvoidDuplicates は、同じ番組名を DuplicateWindowDays 日以内に録画済みなら予約しないかどうか (設計書 4.4)。
	AvoidDuplicates bool
	// DuplicateWindowDays は、重複を避ける期間 (日)。nil なら期間を区切らない。
	DuplicateWindowDays *int
	FollowEventRelay    bool
	CreatedAt           time.Time
	CreatedBy           *uuid.UUID
	UpdatedAt           time.Time
	UpdatedBy           *uuid.UUID
}

// Input は、ルールを作るときと変えるときの内容。
type Input struct {
	Name                string
	Memo                string
	Enabled             bool
	Conditions          *match.Conditions
	Overrides           *reserve.Overrides
	ValidFrom           *time.Time
	ValidUntil          *time.Time
	AvoidDuplicates     bool
	DuplicateWindowDays *int
	FollowEventRelay    bool
}

// encoded は、検証して保存する形にしたルール。
type encoded struct {
	conditions []byte
	settings   []byte
	window     *int32
}

func (in *Input) encode() (*encoded, error) {
	if in == nil {
		return nil, fmt.Errorf("%w: rule must not be nil", ErrInvalidRule)
	}

	errs := []error{}
	if strings.TrimSpace(in.Name) == "" || utf8.RuneCountInString(in.Name) > maxNameLength {
		errs = append(errs, fmt.Errorf("%w: name must be 1 to %d characters", ErrInvalidRule, maxNameLength))
	}
	if utf8.RuneCountInString(in.Memo) > maxMemoLength {
		errs = append(errs, fmt.Errorf("%w: memo must be at most %d characters", ErrInvalidRule, maxMemoLength))
	}
	if in.ValidFrom != nil && in.ValidUntil != nil && !in.ValidFrom.Before(*in.ValidUntil) {
		errs = append(errs, fmt.Errorf("%w: validFrom must be before validUntil", ErrInvalidRule))
	}
	var window *int32
	if in.DuplicateWindowDays != nil {
		if *in.DuplicateWindowDays < 1 || *in.DuplicateWindowDays > maxDuplicateWindowDays {
			errs = append(errs, fmt.Errorf("%w: duplicateWindowDays must be between 1 and %d", ErrInvalidRule, maxDuplicateWindowDays))
		} else {
			window = new(int32(*in.DuplicateWindowDays))
		}
	}
	conditions, err := match.Encode(in.Conditions)
	if err != nil {
		errs = append(errs, err)
	}
	settings, err := reserve.EncodeOverrides(in.Overrides)
	if err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return &encoded{conditions: conditions, settings: settings, window: window}, nil
}

// Beginner はトランザクションを始められるもの。*pgxpool.Pool と pgx.Tx が満たす。
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Rules はルールを扱う。
type Rules struct {
	clock    clock.Clock
	recorder audit.Recorder
}

// NewRules は Rules を作る。
func NewRules(clk clock.Clock, recorder audit.Recorder) *Rules {
	return &Rules{clock: clk, recorder: recorder}
}

// Create はルールを作る。条件と録画設定は、保存の前に検証する。
func (r *Rules) Create(ctx context.Context, db Beginner, actor *audit.Actor, in *Input) (*Rule, error) {
	e, err := in.encode()
	if err != nil {
		return nil, err
	}

	var rule *Rule
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		ruleID := id.New()
		err := q.InsertRule(ctx, sqlc.InsertRuleParams{
			ID:                  ruleID,
			Name:                in.Name,
			Memo:                in.Memo,
			Enabled:             in.Enabled,
			Conditions:          e.conditions,
			Settings:            e.settings,
			ValidFrom:           in.ValidFrom,
			ValidUntil:          in.ValidUntil,
			AvoidDuplicates:     in.AvoidDuplicates,
			DuplicateWindowDays: e.window,
			FollowEventRelay:    in.FollowEventRelay,
			Now:                 r.clock.Now(),
			Actor:               userOf(actor),
		})
		if err != nil {
			return fmt.Errorf("failed to create a rule: %w", err)
		}
		if rule, err = get(ctx, q, ruleID); err != nil {
			return err
		}

		return r.record(ctx, tx, actor, "rule.create", rule)
	})
	if err != nil {
		return nil, err
	}

	return rule, nil
}

// Update は、ルールの内容を in で置き換える。
func (r *Rules) Update(ctx context.Context, db Beginner, actor *audit.Actor, ruleID uuid.UUID, in *Input) (*Rule, error) {
	e, err := in.encode()
	if err != nil {
		return nil, err
	}

	var rule *Rule
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		n, err := q.UpdateRule(ctx, sqlc.UpdateRuleParams{
			ID:                  ruleID,
			Name:                in.Name,
			Memo:                in.Memo,
			Enabled:             in.Enabled,
			Conditions:          e.conditions,
			Settings:            e.settings,
			ValidFrom:           in.ValidFrom,
			ValidUntil:          in.ValidUntil,
			AvoidDuplicates:     in.AvoidDuplicates,
			DuplicateWindowDays: e.window,
			FollowEventRelay:    in.FollowEventRelay,
			Now:                 r.clock.Now(),
			Actor:               userOf(actor),
		})
		if err != nil {
			return fmt.Errorf("failed to update the rule: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", ErrNotFound, ruleID)
		}
		if rule, err = get(ctx, q, ruleID); err != nil {
			return err
		}

		return r.record(ctx, tx, actor, "rule.update", rule)
	})
	if err != nil {
		return nil, err
	}

	return rule, nil
}

// SetEnabled は、ルールを有効または無効にする。
func (r *Rules) SetEnabled(ctx context.Context, db Beginner, actor *audit.Actor, ruleID uuid.UUID, enabled bool) (*Rule, error) {
	var rule *Rule
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		n, err := q.SetRuleEnabled(ctx, sqlc.SetRuleEnabledParams{ID: ruleID, Enabled: enabled, Now: r.clock.Now(), Actor: userOf(actor)})
		if err != nil {
			return fmt.Errorf("failed to update the rule: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", ErrNotFound, ruleID)
		}
		if rule, err = get(ctx, q, ruleID); err != nil {
			return err
		}
		action := "rule.disable"
		if enabled {
			action = "rule.enable"
		}

		return r.record(ctx, tx, actor, action, rule)
	})
	if err != nil {
		return nil, err
	}

	return rule, nil
}

// Delete はルールを消す。このルールだけに一致していた予約は、次の照合で消える。
func (r *Rules) Delete(ctx context.Context, db Beginner, actor *audit.Actor, ruleID uuid.UUID) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		rule, err := get(ctx, q, ruleID)
		if err != nil {
			return err
		}
		n, err := q.DeleteRule(ctx, ruleID)
		if err != nil {
			return fmt.Errorf("failed to delete the rule: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", ErrNotFound, ruleID)
		}

		return r.record(ctx, tx, actor, "rule.delete", rule)
	})
}

// Get はルールを返す。
func (r *Rules) Get(ctx context.Context, db sqlc.DBTX, ruleID uuid.UUID) (*Rule, error) {
	return get(ctx, sqlc.New(db), ruleID)
}

// List は、すべてのルールを作った順に返す。
func (r *Rules) List(ctx context.Context, db sqlc.DBTX) ([]*Rule, error) {
	rows, err := sqlc.New(db).ListRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list rules: %w", err)
	}

	rules := make([]*Rule, 0, len(rows))
	for _, row := range rows {
		rule, err := fromRow(row)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// ListByIDs は、ID のルールを返す。見つからない ID は結果に含めない。
func (r *Rules) ListByIDs(ctx context.Context, db sqlc.DBTX, ids []uuid.UUID) (map[uuid.UUID]*Rule, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]*Rule{}, nil
	}
	rows, err := sqlc.New(db).ListRulesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list rules: %w", err)
	}

	rules := make(map[uuid.UUID]*Rule, len(rows))
	for _, row := range rows {
		rule, err := fromRow(row)
		if err != nil {
			return nil, err
		}
		rules[rule.ID] = rule
	}

	return rules, nil
}

func get(ctx context.Context, q *sqlc.Queries, ruleID uuid.UUID) (*Rule, error) {
	row, err := q.GetRule(ctx, ruleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, ruleID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the rule: %w", err)
	}

	return fromRow(row)
}

func fromRow(row sqlc.Rule) (*Rule, error) {
	conditions, err := match.Decode(row.Conditions)
	if err != nil {
		return nil, fmt.Errorf("rule %s: %w", row.ID, err)
	}
	overrides, err := reserve.DecodeOverrides(row.Settings)
	if err != nil {
		return nil, fmt.Errorf("rule %s: %w", row.ID, err)
	}
	var window *int
	if row.DuplicateWindowDays != nil {
		window = new(int(*row.DuplicateWindowDays))
	}

	return &Rule{
		ID:                  row.ID,
		Name:                row.Name,
		Memo:                row.Memo,
		Enabled:             row.Enabled,
		Conditions:          conditions,
		Overrides:           overrides,
		ValidFrom:           utc(row.ValidFrom),
		ValidUntil:          utc(row.ValidUntil),
		AvoidDuplicates:     row.AvoidDuplicates,
		DuplicateWindowDays: window,
		FollowEventRelay:    row.FollowEventRelay,
		CreatedAt:           row.CreatedAt.UTC(),
		CreatedBy:           row.CreatedBy,
		UpdatedAt:           row.UpdatedAt.UTC(),
		UpdatedBy:           row.UpdatedBy,
	}, nil
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}

	return new(t.UTC())
}

// userOf は、操作者がユーザーならその ID を返す。システムの操作なら nil を返す。
func userOf(actor *audit.Actor) *uuid.UUID {
	if actor == nil || actor.Type != audit.ActorTypeUser {
		return nil
	}

	return &actor.UserID
}

// record は、ルールの変更を監査ログに残し、ルールが変わったことを通知する。ルールの照合を走らせるきっかけになる。
func (r *Rules) record(ctx context.Context, tx pgx.Tx, actor *audit.Actor, action string, rule *Rule) error {
	if _, err := notify.Publish(ctx, tx, r.clock, NotifyChannel); err != nil {
		return err
	}

	return r.recorder.Record(ctx, tx, &audit.Event{
		Actor:      actor,
		Action:     action,
		TargetType: auditTarget,
		TargetID:   rule.ID.String(),
		Summary:    map[string]any{"name": rule.Name, "enabled": rule.Enabled},
		SourceIP:   netip.Addr{},
		UserAgent:  "",
	})
}
