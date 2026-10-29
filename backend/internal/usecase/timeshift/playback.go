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
	// FromStart は、番組の頭から録れていて、頭から再生できるかどうか。
	FromStart bool
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

	return playable(mergeRecorded(programs), spans, now), nil
}

// mergeRecorded は、同じ番組 (イベント ID) が続けて記録された行を、1 つの番組にまとめる。
// mikan を録り直すと、放送中の番組を新しい行に記録し直すので、同じ番組が何行にも分かれる。
// まとめた番組は、最初の行に始まり、最後の行に終わる。番組名は、最後の行の名前を使い、空なら前の行の名前を使う。
func mergeRecorded(programs []sqlc.TimeshiftProgram) []sqlc.TimeshiftProgram {
	merged := make([]sqlc.TimeshiftProgram, 0, len(programs))
	for _, program := range programs {
		if n := len(merged); n > 0 && merged[n-1].EventID == program.EventID {
			last := &merged[n-1]
			last.EndedAt = program.EndedAt
			if program.Name != "" {
				last.Name = program.Name
			}

			continue
		}
		merged = append(merged, program)
	}

	return merged
}

// span は、チャンクが録った時間。書いている途中のチャンクは、今まで録れているものとする。
type span struct {
	from, to time.Time
}

// playable は、番組ごとに、番組の時間と重なるチャンクから、再生できる範囲の始まりと途切れた区間を求める。
// programs と spans は始まりの順に並ぶ。重なるチャンクがない番組は、再生できないので除く。
//
// 番組の始まりは、mikan が番組の替わりに気づいた時刻である。録り始めたときと、落ちた mikan の後を録り直したときは、
// 放送中の番組を、その時刻から始まる行に記録するので、行の始まりは番組の頭ではない。番組の替わりに気づいたときは、
// 前の番組を閉じた時刻と同じ時刻で次の番組を始めるので、前の行の終わりと同じ時刻に始まった行だけを、頭から録れたものとする。
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
			PlayableFrom: time.Time{}, Gaps: []Gap{}, FromStart: false,
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
		p.FromStart = i > 0 && programs[i-1].EndedAt != nil && programs[i-1].EndedAt.Equal(program.StartedAt) &&
			p.PlayableFrom.Equal(program.StartedAt)
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
