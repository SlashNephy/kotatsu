package timeshift

import (
	"context"
	"fmt"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// Gap は、バッファの途切れた区間。チューナーを予約に渡したり、録り直すのを待ったりして、チャンクのない時間。
type Gap struct {
	From time.Time
	To   time.Time
}

// PlayableProgram は、バッファから再生できる番組 (設計書 7.2)。
type PlayableProgram struct {
	EventID int64
	// Name は、番組名。mikan が番組の始まりに気づいたときに番組表になければ空。
	Name string
	// StartedAt は、mikan が番組の始まりに気づいた時刻。EndedAt は、終わりに気づいた時刻で、放送中なら nil。
	StartedAt time.Time
	EndedAt   *time.Time
	// PlayableFrom は、再生できる範囲の始まり。番組の頭のチャンクを破棄したか、録れていなかったら、StartedAt より後になる。
	PlayableFrom time.Time
	// Gaps は、PlayableFrom から番組の終わりまでの、途切れた区間。始まりの順に並ぶ。
	Gaps []Gap
}

// PlayablePrograms は、サービスのバッファから再生できる番組を、始まりの順に返す。
// サービスをタイムシフトで録っていなければ、バッファは破棄を待つだけなので、空を返す。
func PlayablePrograms(ctx context.Context, db sqlc.DBTX, serviceID int64, now time.Time) ([]*PlayableProgram, error) {
	q := sqlc.New(db)
	exists, err := q.TimeshiftChannelExists(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to check the timeshift channel: %w", err)
	}
	if !exists {
		return []*PlayableProgram{}, nil
	}
	programs, err := q.ListTimeshiftPrograms(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the timeshift programs: %w", err)
	}
	chunks, err := q.ListTimeshiftChunksSince(ctx, sqlc.ListTimeshiftChunksSinceParams{ServiceID: serviceID, Since: time.Time{}})
	if err != nil {
		return nil, fmt.Errorf("failed to list the timeshift chunks: %w", err)
	}
	spans := make([]span, 0, len(chunks))
	for i := range chunks {
		end := now
		if chunks[i].EndedAt != nil {
			end = *chunks[i].EndedAt
		}
		spans = append(spans, span{from: chunks[i].StartedAt, to: end})
	}

	return playable(programs, spans, now), nil
}

// span は、チャンクが録った時間。書いている途中のチャンクは、今まで録れているものとする。
type span struct {
	from, to time.Time
}

// playable は、番組ごとに、番組の時間と重なるチャンクから、再生できる範囲の始まりと途切れた区間を求める。
// spans は始まりの順に並ぶ。重なるチャンクがない番組は、再生できないので除く。
func playable(programs []sqlc.TimeshiftProgram, spans []span, now time.Time) []*PlayableProgram {
	result := []*PlayableProgram{}
	for i := range programs {
		program := &programs[i]
		end := now
		if program.EndedAt != nil {
			end = *program.EndedAt
		}
		// covered は、番組の時間のうち、チャンクで録れた最も遅い時刻。
		var covered time.Time
		p := &PlayableProgram{
			EventID: program.EventID, Name: program.Name, StartedAt: program.StartedAt, EndedAt: program.EndedAt,
			PlayableFrom: time.Time{}, Gaps: []Gap{},
		}
		for _, s := range spans {
			if !s.to.After(program.StartedAt) || !s.from.Before(end) {
				continue
			}
			from := maxTime(s.from, program.StartedAt)
			if p.PlayableFrom.IsZero() {
				p.PlayableFrom, covered = from, from
			}
			// 引き継ぎで重なったチャンクは、途切れていない。
			if from.After(covered) {
				p.Gaps = append(p.Gaps, Gap{From: covered, To: from})
			}
			covered = maxTime(covered, minTime(s.to, end))
		}
		if p.PlayableFrom.IsZero() {
			continue
		}
		result = append(result, p)
	}

	return result
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}

	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}

	return b
}
