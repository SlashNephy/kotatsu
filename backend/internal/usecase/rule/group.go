package rule

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

const (
	// groupAuditTarget は、グループの監査ログの対象の種類。
	groupAuditTarget = "rule_group"
	// maxGroupNameLength は、グループの名前の長さの上限。
	maxGroupNameLength = 100
	// pgUniqueViolation と pgForeignKeyViolation は、一意制約と外部キーの制約に反したときの PostgreSQL のエラーコード。
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	// groupNameKey と groupForeignKey は、グループの名前の一意制約と、ルールからグループへの外部キーの名前。
	// ほかの制約 (作成者への外部キーなど) の違反を、グループのエラーと取り違えないよう、名前で見分ける。
	groupNameKey    = "rule_groups_name_key"
	groupForeignKey = "rules_group_id_fkey"
)

var (
	// ErrGroupNotFound は、グループがないことを表す。
	ErrGroupNotFound = errors.New("rule group not found")
	// ErrInvalidGroup は、グループの内容が正しくないことを表す。
	ErrInvalidGroup = errors.New("invalid rule group")
	// ErrGroupNameTaken は、同じ名前のグループが既にあることを表す。
	ErrGroupNameTaken = fmt.Errorf("%w: the name is already used by another group", ErrInvalidGroup)
)

// Group は、ルールのグループ (設計書 4.2、#1493)。ルールの一覧を分けて並べるための名前だけのもので、照合には関わらない。
// ルールと同じく家族で共有する。1 つのルールは、多くとも 1 つのグループに入る。
type Group struct {
	ID   uuid.UUID
	Name string
	// Position は、一覧で並べる順。小さいほど先に並べる。
	Position  int
	CreatedAt time.Time
	CreatedBy *uuid.UUID
	UpdatedAt time.Time
	UpdatedBy *uuid.UUID
}

// normalizeGroupName は、グループの名前の前後の空白を除き、長さを確認する。
func normalizeGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxGroupNameLength {
		return "", fmt.Errorf("%w: name must be 1 to %d characters", ErrInvalidGroup, maxGroupNameLength)
	}

	return name, nil
}

// groupError は、グループに関わる書き込みの、制約の違反のエラーを、グループのエラーにする。
// groupID は、ルールに入れようとしたグループ。外部キーの違反は、そのグループが消えていたことを表す。
func groupError(err error, groupID *uuid.UUID) error {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		return err
	}
	switch {
	case pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == groupNameKey:
		return ErrGroupNameTaken
	case pgErr.Code == pgForeignKeyViolation && pgErr.ConstraintName == groupForeignKey && groupID != nil:
		return fmt.Errorf("%w: %s", ErrGroupNotFound, *groupID)
	default:
		return err
	}
}

// ListGroups は、すべてのグループを、一覧で並べる順に返す。
func (r *Rules) ListGroups(ctx context.Context, db sqlc.DBTX) ([]*Group, error) {
	rows, err := sqlc.New(db).ListRuleGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list rule groups: %w", err)
	}
	groups := make([]*Group, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, groupFromRow(row))
	}

	return groups, nil
}

// ListGroupsByIDs は、ID のグループを返す。見つからない ID は結果に含めない。
func (r *Rules) ListGroupsByIDs(ctx context.Context, db sqlc.DBTX, ids []uuid.UUID) (map[uuid.UUID]*Group, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]*Group{}, nil
	}
	rows, err := sqlc.New(db).ListRuleGroupsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list rule groups: %w", err)
	}
	groups := make(map[uuid.UUID]*Group, len(rows))
	for _, row := range rows {
		groups[row.ID] = groupFromRow(row)
	}

	return groups, nil
}

// GetGroup はグループを返す。
func (r *Rules) GetGroup(ctx context.Context, db sqlc.DBTX, groupID uuid.UUID) (*Group, error) {
	return getGroup(ctx, sqlc.New(db), groupID)
}

// CreateGroup は、グループを作り、一覧の最後に並べる。
func (r *Rules) CreateGroup(ctx context.Context, db Beginner, actor *audit.Actor, name string) (*Group, error) {
	name, err := normalizeGroupName(name)
	if err != nil {
		return nil, err
	}

	var group *Group
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.LockRuleGroups(ctx); err != nil {
			return fmt.Errorf("failed to lock rule groups: %w", err)
		}
		groupID := id.New()
		err := q.InsertRuleGroup(ctx, sqlc.InsertRuleGroupParams{ID: groupID, Name: name, Now: r.clock.Now(), Actor: userOf(actor)})
		if err != nil {
			return fmt.Errorf("failed to create a rule group: %w", groupError(err, nil))
		}
		if group, err = getGroup(ctx, q, groupID); err != nil {
			return err
		}

		return r.recordGroup(ctx, tx, actor, "rule_group.create", group, map[string]any{summaryName: group.Name})
	})
	if err != nil {
		return nil, err
	}

	return group, nil
}

// RenameGroup は、グループの名前を変える。
func (r *Rules) RenameGroup(ctx context.Context, db Beginner, actor *audit.Actor, groupID uuid.UUID, name string) (*Group, error) {
	name, err := normalizeGroupName(name)
	if err != nil {
		return nil, err
	}

	var group *Group
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		before, err := getGroup(ctx, q, groupID)
		if err != nil {
			return err
		}
		if _, err := q.RenameRuleGroup(ctx, sqlc.RenameRuleGroupParams{ID: groupID, Name: name, Now: r.clock.Now(), Actor: userOf(actor)}); err != nil {
			return fmt.Errorf("failed to rename the rule group: %w", groupError(err, nil))
		}
		if group, err = getGroup(ctx, q, groupID); err != nil {
			return err
		}

		return r.recordGroup(ctx, tx, actor, "rule_group.rename", group, map[string]any{summaryName: group.Name, "previousName": before.Name})
	})
	if err != nil {
		return nil, err
	}

	return group, nil
}

// ReorderGroups は、グループを groupIDs の順に並べ直す。groupIDs は、今あるすべてのグループを 1 回ずつ含む。
// 一部だけを渡すと、渡さなかったグループの位置が決まらないので、拒む。
func (r *Rules) ReorderGroups(ctx context.Context, db Beginner, actor *audit.Actor, groupIDs []uuid.UUID) ([]*Group, error) {
	var groups []*Group
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.LockRuleGroups(ctx); err != nil {
			return fmt.Errorf("failed to lock rule groups: %w", err)
		}
		current, err := r.ListGroups(ctx, tx)
		if err != nil {
			return err
		}
		if err := samePermutation(current, groupIDs); err != nil {
			return err
		}
		if err := q.ReorderRuleGroups(ctx, groupIDs); err != nil {
			return fmt.Errorf("failed to reorder rule groups: %w", err)
		}
		if groups, err = r.ListGroups(ctx, tx); err != nil {
			return err
		}
		names := make([]string, 0, len(groups))
		for _, g := range groups {
			names = append(names, g.Name)
		}
		if _, err := notify.Publish(ctx, tx, r.clock, NotifyChannel); err != nil {
			return err
		}

		return r.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "rule_group.reorder",
			TargetType: groupAuditTarget,
			TargetID:   "",
			Summary:    map[string]any{"names": names},
			SourceIP:   netip.Addr{},
			UserAgent:  "",
		})
	})
	if err != nil {
		return nil, err
	}

	return groups, nil
}

// samePermutation は、groupIDs が、current のグループを 1 回ずつ含むかどうかを確認する。
func samePermutation(current []*Group, groupIDs []uuid.UUID) error {
	if len(groupIDs) != len(current) {
		return fmt.Errorf("%w: ids must contain every group exactly once", ErrInvalidGroup)
	}
	remaining := make(map[uuid.UUID]bool, len(current))
	for _, g := range current {
		remaining[g.ID] = true
	}
	for _, groupID := range groupIDs {
		if !remaining[groupID] {
			return fmt.Errorf("%w: ids must contain every group exactly once", ErrInvalidGroup)
		}
		delete(remaining, groupID)
	}

	return nil
}

// DeleteGroup はグループを消す。中のルールは消さず、どのグループにも入らない (一覧の「その他」) ルールに戻す。
func (r *Rules) DeleteGroup(ctx context.Context, db Beginner, actor *audit.Actor, groupID uuid.UUID) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		group, err := getGroup(ctx, q, groupID)
		if err != nil {
			return err
		}
		count, err := q.CountRulesInGroup(ctx, &groupID)
		if err != nil {
			return fmt.Errorf("failed to count rules in the group: %w", err)
		}
		n, err := q.DeleteRuleGroup(ctx, groupID)
		if err != nil {
			return fmt.Errorf("failed to delete the rule group: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", ErrGroupNotFound, groupID)
		}

		return r.recordGroup(ctx, tx, actor, "rule_group.delete", group, map[string]any{summaryName: group.Name, "rules": count})
	})
}

// SetGroupResult は、ルールをまとめてグループに移した結果。Processed は移した数、Skipped は見つからずに飛ばした数。
type SetGroupResult struct {
	Processed int
	Skipped   int
}

// SetGroup は、ルール ruleIDs を、グループ groupID に移す。groupID が nil なら、どのグループにも入れない。
// 見つからないルールは飛ばして数える。監査ログはルールごとに残す。
// グループは照合に関わらないので、ルールの変えた日は変えない。
func (r *Rules) SetGroup(ctx context.Context, db Beginner, actor *audit.Actor, ruleIDs []uuid.UUID, groupID *uuid.UUID) (*SetGroupResult, error) {
	result := &SetGroupResult{Processed: 0, Skipped: 0}
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		var groupName *string
		if groupID != nil {
			group, err := getGroup(ctx, q, *groupID)
			if err != nil {
				return err
			}
			groupName = &group.Name
		}
		*result = SetGroupResult{Processed: 0, Skipped: 0}
		for _, ruleID := range ruleIDs {
			n, err := q.SetRuleGroup(ctx, sqlc.SetRuleGroupParams{ID: ruleID, GroupID: groupID})
			if err != nil {
				return fmt.Errorf("failed to set the group of the rule: %w", groupError(err, groupID))
			}
			if n == 0 {
				result.Skipped++

				continue
			}
			rule, err := get(ctx, q, ruleID)
			if err != nil {
				return err
			}
			err = r.recorder.Record(ctx, tx, &audit.Event{
				Actor:      actor,
				Action:     "rule.set_group",
				TargetType: auditTarget,
				TargetID:   rule.ID.String(),
				Summary:    map[string]any{summaryName: rule.Name, "group": groupName},
				SourceIP:   netip.Addr{},
				UserAgent:  "",
			})
			if err != nil {
				return err
			}
			result.Processed++
		}
		if result.Processed == 0 {
			return nil
		}
		_, err := notify.Publish(ctx, tx, r.clock, NotifyChannel)

		return err
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func getGroup(ctx context.Context, q *sqlc.Queries, groupID uuid.UUID) (*Group, error) {
	row, err := q.GetRuleGroup(ctx, groupID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrGroupNotFound, groupID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the rule group: %w", err)
	}

	return groupFromRow(row), nil
}

func groupFromRow(row sqlc.RuleGroup) *Group {
	return &Group{
		ID:        row.ID,
		Name:      row.Name,
		Position:  int(row.Position),
		CreatedAt: row.CreatedAt.UTC(),
		CreatedBy: row.CreatedBy,
		UpdatedAt: row.UpdatedAt.UTC(),
		UpdatedBy: row.UpdatedBy,
	}
}

// recordGroup は、グループの変更を監査ログに残し、ルールの一覧が変わったことを通知する。
// グループは照合に関わらないが、画面のルールの一覧は、ルールの変更の通知で読み直すからである。
func (r *Rules) recordGroup(ctx context.Context, tx pgx.Tx, actor *audit.Actor, action string, group *Group, summary map[string]any) error {
	if _, err := notify.Publish(ctx, tx, r.clock, NotifyChannel); err != nil {
		return err
	}

	return r.recorder.Record(ctx, tx, &audit.Event{
		Actor:      actor,
		Action:     action,
		TargetType: groupAuditTarget,
		TargetID:   group.ID.String(),
		Summary:    summary,
		SourceIP:   netip.Addr{},
		UserAgent:  "",
	})
}
