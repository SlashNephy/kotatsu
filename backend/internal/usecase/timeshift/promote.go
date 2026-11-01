package timeshift

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/assign"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
)

// promoteDelay は、書き出す範囲の終わりから、書き出し始めるまで待つ時間。mikan はチャンクを 1 秒ごとに書き出すので、
// 書いている途中のチャンクが、書き出す範囲の終わりを確かに越えるようにする。
const promoteDelay = 5 * time.Second

// promoteAction は、バッファの番組を録画に昇格したことを残す、監査ログの操作の種類。
const promoteAction = "timeshift.promote"

var (
	// ErrProgramNotFound は、バッファに、昇格する番組がないことを表す。
	ErrProgramNotFound = errors.New("timeshift program not found")
	// ErrAlreadyPromoting は、同じ番組を、録画に書き出している途中であることを表す。
	ErrAlreadyPromoting = errors.New("the timeshift program is already being promoted")
)

// Inserter は、ジョブをトランザクションの中で投入する。*jobs.Client が満たす。
type Inserter interface {
	InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error)
}

// Promotion は、バッファの番組を昇格した結果。放送の終わった番組は Recording、放送中の番組は Reservation のどちらかを持つ。
type Promotion struct {
	// RecordingID は、バッファから書き出す録画。nekonote のジョブが書き出し終えるまでは、録画中にする。
	RecordingID *uuid.UUID
	// Reservation は、放送中の番組の続きを録る番組予約。
	Reservation *domain.Reservation
}

// Promoter は、バッファにある番組を、通常の録画に昇格する (#1057、設計書 5.7)。
type Promoter struct {
	clock        clock.Clock
	recorder     audit.Recorder
	reservations *reserve.Reservations
	defaults     assign.DefaultsFunc
	inserter     Inserter
}

// NewPromoter は Promoter を返す。defaults は録画設定の全体の既定値。
func NewPromoter(clk clock.Clock, recorder audit.Recorder, reservations *reserve.Reservations, defaults assign.DefaultsFunc, inserter Inserter) *Promoter {
	return &Promoter{clock: clk, recorder: recorder, reservations: reservations, defaults: defaults, inserter: inserter}
}

// Promote は、サービス serviceID のバッファの、startAt に始まった番組 (PlayablePrograms の StartedAt) を、録画に昇格する。
//
// 放送中の番組は、番組予約を作って、続きを通常の録画として録る。録画の開始が遅れるので、mikan が番組の頭からの分を
// バッファから写し、重なった区間を切り詰める (#826)。放送の終わった番組は、予約のない録画を録画中として記録し、
// nekonote のジョブに、バッファの番組の始まりから終わり (前後のマージンを含む) までを書き出させる。
// バッファにその番組がなければ ErrProgramNotFound、同じ番組を書き出している途中なら ErrAlreadyPromoting を返す。
func (p *Promoter) Promote(ctx context.Context, db Database, actor *audit.Actor, serviceID int64, startAt time.Time) (*Promotion, error) {
	now := p.clock.Now()
	programs, err := PlayablePrograms(ctx, db, serviceID, now)
	if err != nil {
		return nil, err
	}
	var program *PlayableProgram
	for _, candidate := range programs {
		if candidate.StartedAt.Equal(startAt) {
			program = candidate
		}
	}
	if program == nil {
		return nil, ErrProgramNotFound
	}
	if program.EndedAt == nil {
		return p.reserve(ctx, db, actor, serviceID, program)
	}

	return p.enqueue(ctx, db, actor, serviceID, program, now)
}

// reserve は、放送中の番組の番組予約を作り、昇格したことを監査ログに残す。
func (p *Promoter) reserve(ctx context.Context, db Database, actor *audit.Actor, serviceID int64, program *PlayableProgram) (*Promotion, error) {
	reservation, _, err := p.reservations.CreateProgram(ctx, db, actor, &reserve.ProgramInput{
		ServiceID: serviceID, EventID: program.EventID, FollowEventRelay: true, Overrides: nil,
	})
	if errors.Is(err, reserve.ErrProgramNotFound) {
		return nil, ErrProgramNotFound
	}
	if err != nil {
		return nil, err
	}
	err = p.recorder.Record(ctx, db, &audit.Event{
		Actor: actor, Action: promoteAction, TargetType: "reservation", TargetID: reservation.ID.String(),
		Summary:  map[string]any{"name": reservation.Name, "serviceId": serviceID, "startAt": program.StartedAt},
		SourceIP: netip.Addr{}, UserAgent: "",
	})
	if err != nil {
		return nil, err
	}

	return &Promotion{RecordingID: nil, Reservation: reservation}, nil
}

// enqueue は、放送の終わった番組の録画を録画中として記録し、書き出すジョブを入れる。
// 番組の情報と時間は番組表から写し、番組表になければ (古い番組を消した後など)、mikan が記録した番組の名前と時間を使う。
// 書き出す範囲は、番組表の時間と mikan が気づいた時間を合わせた範囲に、録画設定の前後のマージンを足したもの。
// mikan は番組の替わりに少し遅れて気づき、番組表は延長を反映しないことがあるので、合わせた範囲で欠けないようにする。
func (p *Promoter) enqueue(
	ctx context.Context, db Database, actor *audit.Actor, serviceID int64, program *PlayableProgram, now time.Time,
) (*Promotion, error) {
	settings, err := p.defaults(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("failed to get the recording settings: %w", err)
	}
	q := sqlc.New(db)
	insert := sqlc.InsertPromotedRecordingParams{
		ID: id.New(), ServiceID: serviceID, EventID: &program.EventID, Name: program.Name, Description: "", NameNormalized: nil, DescriptionNormalized: nil,
		StartAt: program.StartedAt, EndAt: *program.EndedAt, Audios: []byte("[]"), VideoResolution: nil, Genres: []byte("[]"),
		RecordedFrom: time.Time{}, Now: now,
	}
	epg, err := q.GetTimeshiftPromotionProgram(ctx, sqlc.GetTimeshiftPromotionProgramParams{ServiceID: serviceID, EventID: program.EventID, At: program.StartedAt})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, fmt.Errorf("failed to look up the program: %w", err)
	default:
		insert.Name, insert.Description, insert.StartAt = epg.Name, epg.Description, epg.StartAt
		insert.Audios, insert.VideoResolution, insert.Genres = epg.Audios, epg.VideoResolution, epg.Genres
		if !epg.EndUndecided {
			insert.EndAt = epg.EndAt
		}
	}
	from := minTime(insert.StartAt, program.StartedAt).Add(-time.Duration(settings.StartMarginSeconds) * time.Second)
	to := maxTime(insert.EndAt, *program.EndedAt).Add(time.Duration(settings.EndMarginSeconds) * time.Second)
	insert.RecordedFrom = from
	insert.NameNormalized, insert.DescriptionNormalized = new(domain.Normalize(insert.Name)), new(domain.Normalize(insert.Description))

	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		// 同じ番組を書き出している途中なら、重ねない。書き出す範囲はマージンで変わるので、イベント ID と番組の始まりでも確かめる。
		promotions, err := q.ListTimeshiftPromotions(ctx, serviceID)
		if err != nil {
			return fmt.Errorf("failed to list the timeshift promotions: %w", err)
		}
		if promoting(promotions, program) {
			return ErrAlreadyPromoting
		}
		row, err := q.InsertPromotedRecording(ctx, insert)
		if err != nil {
			return fmt.Errorf("failed to save the recording: %w", err)
		}
		// 予約の録画と同じく、番組とアニメの作品の結びつきを写す (設計書 3.8)。
		if err := anime.CopyToRecording(ctx, tx, row.ID, row.ServiceID, row.StartAt, now); err != nil {
			return err
		}
		inserted, err := q.InsertTimeshiftPromotion(ctx, sqlc.InsertTimeshiftPromotionParams{
			RecordingID: row.ID, ServiceID: serviceID, FromAt: from, ToAt: to, CreatedBy: userOf(actor), Now: now,
		})
		if err != nil {
			return fmt.Errorf("failed to save the timeshift promotion: %w", err)
		}
		if inserted == 0 {
			return ErrAlreadyPromoting
		}
		// 終わりのマージンを過ぎて、書いている途中のチャンクが書き出す範囲の終わりを越えるまで待ってから書き出す。
		args := recording.PromoteArgs{RecordingID: row.ID}
		opts := args.InsertOpts()
		if ready := to.Add(promoteDelay); ready.After(now) {
			opts.ScheduledAt = ready
		}
		if _, err := p.inserter.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: args, InsertOpts: &opts}}); err != nil {
			return fmt.Errorf("failed to enqueue the timeshift promotion: %w", err)
		}
		if _, err := notify.Publish(ctx, tx, p.clock, notify.RecordingsChannel); err != nil {
			return err
		}

		return p.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: promoteAction, TargetType: "recording", TargetID: row.ID.String(),
			Summary:  map[string]any{"name": row.Name, "serviceId": serviceID, "from": from, "to": to},
			SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
	if err != nil {
		return nil, err
	}

	return &Promotion{RecordingID: &insert.ID, Reservation: nil}, nil
}

// promoting は、番組 program を、promotions のどれかで書き出している途中かどうかを返す。
// 書き出す範囲は、番組の始まりより前のマージンから始まり、番組の始まりを含む。終わりのマージンが次の番組の始まりに
// かかるので、イベント ID でも確かめる。
func promoting(promotions []sqlc.ListTimeshiftPromotionsRow, program *PlayableProgram) bool {
	return slices.ContainsFunc(promotions, func(promotion sqlc.ListTimeshiftPromotionsRow) bool {
		return promotion.EventID != nil && *promotion.EventID == program.EventID &&
			!promotion.FromAt.After(program.StartedAt) && promotion.ToAt.After(program.StartedAt)
	})
}
