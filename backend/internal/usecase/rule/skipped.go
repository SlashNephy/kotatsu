package rule

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// skippedProgram は、ルールに一致したが、重複の回避か、同じ回を束ねたか、前の回を録らない設定で予約しなかった番組。
type skippedProgram struct {
	row    sqlc.ListMatchableProgramsRow
	rules  []*compiledRule
	reason SkipReason
	// lastRecorded は、前に録った録画の終わり。別の放送で録るときは nil。
	lastRecorded *time.Time
	// recorded は、同じ回を録画済みのときの、その録画。
	recorded *recordedEpisode
	// instead は、同じ回を別の放送で録るときの、その放送。
	instead *programKey
	// laterEpisode は、前の回として外したときの、比べた相手の一番大きい話数。ほかの理由では 0。
	laterEpisode int
	// byRecording は、前の回として外したときに、録画 (録画履歴か録画中の録画) が理由に含まれるかどうか。
	byRecording bool
}

// immediate は、外れた予約を、放送の直前の猶予の内でもすぐ消すかどうかを返す (#1501)。
// 録画済みで外れた予約は、番組表の書き換えでなく録画の完了で外れたので、すぐ消す。
// 別の放送で録るので外れた予約と、予約や番組表の放送だけを理由に前の回として外れた予約は、番組表の変化で外れたものなので、
// 猶予の内なら残す (#1498、#1499)。
func (s *skippedProgram) immediate() bool {
	switch s.reason {
	case SkipReasonOtherBroadcast:
		return false
	case SkipReasonEarlierEpisode:
		return s.byRecording
	case SkipReasonDuplicateTitle, SkipReasonRecordedEpisode:
		return true
	}

	return true
}

// SkippedDuplicate は、ルールに一致したが、同じ番組名か同じ回を録画済みか、同じ回を別の放送で録るので予約しなかった番組
// (設計書 4.4、#469、#1498)。
type SkippedDuplicate struct {
	ServiceID int64
	EventID   int64
	// Rules は、一致したが外したルールの ID と名前。
	Rules []*SkippedRule
	// Reason は、外した理由。
	Reason SkipReason
	// LastRecordedAt は、前に録った録画の終わり。同じ番組名で外したときは、同じサービスで最後に録った録画の終わり。
	// 別の放送で録るので外したときは nil。
	LastRecordedAt *time.Time
	// Recorded は、同じ回を録画済みなので外したときの、その録画。ほかの理由では nil。
	Recorded *SkippedRecording
	// Instead は、同じ回を別の放送で録るので外したときの、その放送。ほかの理由では nil。
	Instead *SkippedInstead
	// LaterEpisode は、前の回として外したときの、録った回、予約してある回、候補の放送のうち、一番大きい話数。ほかの理由では nil。
	LaterEpisode *int
}

// SkippedRecording は、同じ回を録った録画 (録画履歴か、録画中の録画)。
type SkippedRecording struct {
	ServiceID int64
	Name      string
	EndAt     time.Time
}

// SkippedInstead は、同じ回を代わりに録る放送。
type SkippedInstead struct {
	ServiceID int64
	EventID   int64
}

// SkippedRule は、重複の回避で外したルール。
type SkippedRule struct {
	ID   uuid.UUID
	Name string
}

// FindSkippedDuplicates は、照合と同じ計算で、重複の回避と同じ回の束ねで予約しなかった番組を、放送の早い順に返す。DB には書かない。
// すでに予約のある番組 (利用者が手動で予約したものなど) は返さない。
func FindSkippedDuplicates(ctx context.Context, db Beginner, clk clock.Clock, location *time.Location) ([]*SkippedDuplicate, error) {
	e := &Engine{pool: nil, clock: clk, location: location, guard: nil, reservations: nil}
	result := []*SkippedDuplicate{}
	// 番組、ルール、録画履歴、予約を同じ時点で読むよう、REPEATABLE READ の 1 つのトランザクションで読む。
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY"); err != nil {
			return fmt.Errorf("failed to start a read-only snapshot: %w", err)
		}
		skipped := map[programKey]*skippedProgram{}
		if _, err := e.desired(ctx, tx, clk.Now(), newResult(), skipped); err != nil {
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
			result = append(result, s.public(key, rules))
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find the programs skipped as duplicates: %w", err)
	}

	return result, nil
}

// public は、見送った番組を、外に返す形にする。
func (s *skippedProgram) public(key programKey, rules []*SkippedRule) *SkippedDuplicate {
	skipped := &SkippedDuplicate{
		ServiceID: key.serviceID, EventID: key.eventID, Rules: rules, Reason: s.reason, LastRecordedAt: s.lastRecorded,
		Recorded: nil, Instead: nil, LaterEpisode: nil,
	}
	if s.reason == SkipReasonEarlierEpisode {
		skipped.LaterEpisode = new(s.laterEpisode)
	}
	if s.recorded != nil {
		skipped.Recorded = &SkippedRecording{ServiceID: s.recorded.serviceID, Name: s.recorded.name, EndAt: s.recorded.endAt}
	}
	if s.instead != nil {
		skipped.Instead = &SkippedInstead{ServiceID: s.instead.serviceID, EventID: s.instead.eventID}
	}

	return skipped
}

// reservedPrograms は、skipped の番組のうち、予約のあるものを返す。
func reservedPrograms(ctx context.Context, tx pgx.Tx, skipped map[programKey]*skippedProgram) (map[programKey]bool, error) {
	if len(skipped) == 0 {
		return map[programKey]bool{}, nil
	}
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
