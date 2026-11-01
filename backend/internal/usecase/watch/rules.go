package watch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/usersettings"
)

const (
	// RulesNotifyChannel は、視聴ルールが変わったことを知らせる変更通知のチャンネル。ルールの照合を走らせるきっかけになる。
	RulesNotifyChannel = "watch_rules"
	// MaxRulesPerUser は、1 ユーザーが持てる視聴ルールの数の上限 (設計書 4.8)。照合は、ルールの数に比例して重くなる。
	MaxRulesPerUser = 50
	// maxRuleNameLength は、視聴ルールの名前の長さの上限。録画のルールとそろえる。
	maxRuleNameLength = 200
)

var (
	// ErrRuleNotFound は、視聴ルールがないか、ほかのユーザーのものであることを表す。
	ErrRuleNotFound = errors.New("watch rule not found")
	// ErrInvalidRule は、視聴ルールの内容が正しくないことを表す。
	ErrInvalidRule = errors.New("invalid watch rule")
	// ErrTooManyRules は、視聴ルールの数が上限に達していることを表す。
	ErrTooManyRules = errors.New("too many watch rules")
)

// Rule は、視聴ルール (設計書 4.8)。条件に一致する番組に、持ち主の視聴予約を自動で付ける。
type Rule struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Name       string
	Enabled    bool
	Conditions *match.Conditions
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// RuleInput は、視聴ルールを作るときと変えるときの内容。
type RuleInput struct {
	Name       string
	Enabled    bool
	Conditions *match.Conditions
}

// encode は、内容を検証し、条件を保存する形にする。条件の検証は、録画のルールと同じ。
func (in *RuleInput) encode() ([]byte, error) {
	if in == nil {
		return nil, fmt.Errorf("%w: rule must not be nil", ErrInvalidRule)
	}
	errs := []error{}
	if strings.TrimSpace(in.Name) == "" || utf8.RuneCountInString(in.Name) > maxRuleNameLength {
		errs = append(errs, fmt.Errorf("%w: name must be 1 to %d characters", ErrInvalidRule, maxRuleNameLength))
	}
	conditions, err := match.Encode(in.Conditions)
	if err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return conditions, nil
}

// CreateRule は、ユーザー userID の視聴ルールを作る。MaxRulesPerUser 個あれば ErrTooManyRules を返す。
func CreateRule(ctx context.Context, db usersettings.Beginner, clk clock.Clock, userID uuid.UUID, in *RuleInput) (*Rule, error) {
	conditions, err := in.encode()
	if err != nil {
		return nil, err
	}

	var rule *Rule
	err = changeRules(ctx, db, clk, userID, func(q *sqlc.Queries) error {
		// 数えてから作るまでの間に、同じユーザーがほかの端末で作って上限を超えないよう、ロックを取ってから数える。
		if err := q.LockWatchRules(ctx, userID); err != nil {
			return fmt.Errorf("failed to lock the watch rules: %w", err)
		}
		count, err := q.CountWatchRules(ctx, userID)
		if err != nil {
			return fmt.Errorf("failed to count the watch rules: %w", err)
		}
		if count >= MaxRulesPerUser {
			return fmt.Errorf("%w: at most %d", ErrTooManyRules, MaxRulesPerUser)
		}
		ruleID := id.New()
		if err := q.InsertWatchRule(ctx, sqlc.InsertWatchRuleParams{
			ID: ruleID, UserID: userID, Name: in.Name, Enabled: in.Enabled, Conditions: conditions, Now: clk.Now(),
		}); err != nil {
			return fmt.Errorf("failed to create a watch rule: %w", err)
		}
		rule, err = getRule(ctx, q, userID, ruleID)

		return err
	})
	if err != nil {
		return nil, err
	}

	return rule, nil
}

// UpdateRule は、ユーザー userID の視聴ルールの内容を in で置き換える。
func UpdateRule(ctx context.Context, db usersettings.Beginner, clk clock.Clock, userID, ruleID uuid.UUID, in *RuleInput) (*Rule, error) {
	conditions, err := in.encode()
	if err != nil {
		return nil, err
	}

	var rule *Rule
	err = changeRules(ctx, db, clk, userID, func(q *sqlc.Queries) error {
		n, err := q.UpdateWatchRule(ctx, sqlc.UpdateWatchRuleParams{
			ID: ruleID, UserID: userID, Name: in.Name, Enabled: in.Enabled, Conditions: conditions, Now: clk.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to update the watch rule: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", ErrRuleNotFound, ruleID)
		}
		rule, err = getRule(ctx, q, userID, ruleID)

		return err
	})
	if err != nil {
		return nil, err
	}

	return rule, nil
}

// SetRuleEnabled は、ユーザー userID の視聴ルールを有効または無効にする。
func SetRuleEnabled(ctx context.Context, db usersettings.Beginner, clk clock.Clock, userID, ruleID uuid.UUID, enabled bool) (*Rule, error) {
	var rule *Rule
	err := changeRules(ctx, db, clk, userID, func(q *sqlc.Queries) error {
		n, err := q.SetWatchRuleEnabled(ctx, sqlc.SetWatchRuleEnabledParams{ID: ruleID, UserID: userID, Enabled: enabled, Now: clk.Now()})
		if err != nil {
			return fmt.Errorf("failed to update the watch rule: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", ErrRuleNotFound, ruleID)
		}
		rule, err = getRule(ctx, q, userID, ruleID)

		return err
	})
	if err != nil {
		return nil, err
	}

	return rule, nil
}

// DeleteRule は、ユーザー userID の視聴ルールを消す。視聴ルールが作った視聴予約は、手で付けたものとして残す。
func DeleteRule(ctx context.Context, db usersettings.Beginner, clk clock.Clock, userID, ruleID uuid.UUID) error {
	return changeRules(ctx, db, clk, userID, func(q *sqlc.Queries) error {
		n, err := q.DeleteWatchRule(ctx, sqlc.DeleteWatchRuleParams{ID: ruleID, UserID: userID})
		if err != nil {
			return fmt.Errorf("failed to delete the watch rule: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", ErrRuleNotFound, ruleID)
		}

		return nil
	})
}

// GetRule は、ユーザー userID の視聴ルールを返す。ほかのユーザーのものは ErrRuleNotFound を返す。
func GetRule(ctx context.Context, db sqlc.DBTX, userID, ruleID uuid.UUID) (*Rule, error) {
	return getRule(ctx, sqlc.New(db), userID, ruleID)
}

// ListRules は、ユーザー userID の視聴ルールを、作った順に返す。
func ListRules(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) ([]*Rule, error) {
	rows, err := sqlc.New(db).ListWatchRules(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the watch rules: %w", err)
	}
	rules := make([]*Rule, 0, len(rows))
	for _, row := range rows {
		rule, err := DecodeRule(row)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// DecodeRule は、DB の視聴ルールを読む。条件を読めなければ (ローリング更新の途中で、新しい版が足した条件など) エラーを返す。
func DecodeRule(row sqlc.WatchRule) (*Rule, error) {
	conditions, err := match.Decode(row.Conditions)
	if err != nil {
		return nil, fmt.Errorf("watch rule %s: %w", row.ID, err)
	}

	return &Rule{
		ID: row.ID, UserID: row.UserID, Name: row.Name, Enabled: row.Enabled, Conditions: conditions,
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}, nil
}

func getRule(ctx context.Context, q *sqlc.Queries, userID, ruleID uuid.UUID) (*Rule, error) {
	row, err := q.GetWatchRule(ctx, sqlc.GetWatchRuleParams{ID: ruleID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrRuleNotFound, ruleID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the watch rule: %w", err)
	}

	return DecodeRule(row)
}

// changeRules は、1 つのトランザクションで視聴ルールを change で変え、ルールの照合を走らせ、同じユーザーのほかの端末に知らせる。
// 視聴ルールは個人のものなので、録画のルールと違って監査ログには残さない。
func changeRules(ctx context.Context, db usersettings.Beginner, clk clock.Clock, userID uuid.UUID, change func(q *sqlc.Queries) error) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if err := change(sqlc.New(tx)); err != nil {
			return err
		}
		if _, err := notify.Publish(ctx, tx, clk, RulesNotifyChannel); err != nil {
			return err
		}

		return usersettings.Touch(ctx, tx, clk, userID)
	})
}
