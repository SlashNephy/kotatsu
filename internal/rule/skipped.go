package rule

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// skippedProgram は、ルールに一致したが、重複の回避で予約しなかった番組。
type skippedProgram struct {
	row          sqlc.ListMatchableProgramsRow
	rules        []*compiledRule
	lastRecorded time.Time
}

// SkippedDuplicate は、ルールに一致したが、同じ番組名を録画済みなので、重複の回避で予約しなかった番組 (設計書 4.4、#469)。
type SkippedDuplicate struct {
	ServiceID int64
	EventID   int64
	// Rules は、一致したが重複の回避で外したルールの ID と名前。
	Rules []*SkippedRule
	// LastRecordedAt は、同じ番組名を同じサービスで最後に録った録画の終わり。
	LastRecordedAt time.Time
}

// SkippedRule は、重複の回避で外したルール。
type SkippedRule struct {
	ID   uuid.UUID
	Name string
}

// FindSkippedDuplicates は、照合と同じ計算で、重複の回避で予約しなかった番組を、放送の早い順に返す。DB には書かない。
// すでに予約のある番組 (利用者が手動で予約したものなど) は返さない。
func FindSkippedDuplicates(ctx context.Context, db Beginner, clk clock.Clock, location *time.Location) ([]*SkippedDuplicate, error) {
	e := &Engine{pool: nil, clock: clk, location: location, guard: nil, reservations: nil}
	result := []*SkippedDuplicate{}
	// 照合と同じ時点の番組、ルール、録画履歴で計算するよう、1 つのトランザクションで読む。
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET TRANSACTION READ ONLY"); err != nil {
			return fmt.Errorf("failed to make the transaction read-only: %w", err)
		}
		skipped := map[programKey]*skippedProgram{}
		if _, err := e.desired(ctx, tx, clk.Now(), &Result{Created: 0, Deleted: 0, Linked: 0, Unlinked: 0, Skipped: map[uuid.UUID]error{}}, skipped); err != nil {
			return err
		}
		reserved, err := reservedPrograms(ctx, tx, skipped)
		if err != nil {
			return err
		}
		keys := slices.SortedFunc(maps.Keys(skipped), func(a, b programKey) int {
			if c := skipped[a].row.StartAt.Compare(skipped[b].row.StartAt); c != 0 {
				return c
			}

			return compareKeys(a, b)
		})
		for _, key := range keys {
			if reserved[key] {
				continue
			}
			s := skipped[key]
			rules := make([]*SkippedRule, 0, len(s.rules))
			for _, r := range s.rules {
				rules = append(rules, &SkippedRule{ID: r.ID, Name: r.Name})
			}
			result = append(result, &SkippedDuplicate{ServiceID: key.serviceID, EventID: key.eventID, Rules: rules, LastRecordedAt: s.lastRecorded})
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find the programs skipped as duplicates: %w", err)
	}

	return result, nil
}

// reservedPrograms は、skipped の番組のうち、予約のあるものを返す。
func reservedPrograms(ctx context.Context, tx pgx.Tx, skipped map[programKey]*skippedProgram) (map[programKey]bool, error) {
	serviceIDs := make([]int64, 0, len(skipped))
	eventIDs := make([]int64, 0, len(skipped))
	for key := range skipped {
		serviceIDs = append(serviceIDs, key.serviceID)
		eventIDs = append(eventIDs, key.eventID)
	}
	rows, err := sqlc.New(tx).ListReservationsByPrograms(ctx, sqlc.ListReservationsByProgramsParams{ServiceIds: serviceIDs, EventIds: eventIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to list the reservations of the skipped programs: %w", err)
	}
	reserved := map[programKey]bool{}
	for i := range rows {
		if rows[i].EventID != nil {
			reserved[programKey{serviceID: rows[i].ServiceID, eventID: *rows[i].EventID}] = true
		}
	}

	return reserved, nil
}
