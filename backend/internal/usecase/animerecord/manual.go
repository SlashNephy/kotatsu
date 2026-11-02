package animerecord

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// manualServiceID は、番組によらず手で記録した話 (視聴管理の画面の作品の次の話。#1232) の、anime_records の service_id。
// サービスの ID は 0 にならない。start_at には、記録を頼んだ日時を入れる。
const manualServiceID = 0

var (
	// ErrNothingToRecord は、記録できる話がないこと (話に結びついていない、話数がない) を表す。
	ErrNothingToRecord = errors.New("there is no episode to record")
	// ErrAlreadyRecorded は、どの記録先にも、同じ話を記録したか、記録しようとしていることを表す。
	ErrAlreadyRecorded = errors.New("the episode is already recorded")
)

// ManualTarget は、手で記録する話 (#1232)。どれか 1 つを指す。
type ManualTarget struct {
	// Program は、番組 (サービスと開始時刻、番組名)。番組に結びついた話を記録する。番組の詳細から使う。
	Program *ManualProgram
	// RecordingID は、録画。録画に写した結びつきの話を記録する。録画の詳細から使う。
	RecordingID *uuid.UUID
	// Work は、作品 (TID) の 1 話。番組によらず記録する。視聴管理の画面のカードから使う。
	Work *ManualWork
}

// ManualProgram は、手で記録する番組。
type ManualProgram struct {
	ServiceID int64
	StartAt   time.Time
	Name      string
}

// ManualWork は、手で記録する作品の 1 話。
type ManualWork struct {
	TID    int32
	Number float64
	// Title は、通知に出す作品名。
	Title string
}

// RecordManually は、ユーザー userID が手で選んだ話を、連携した記録先ごとに記録するジョブを入れる (#1232)。
// 送るのは、プレイヤーで見た割合から記録するとき (Report) と同じジョブで、結果も同じく、そのユーザーだけに通知する。
// 番組と録画は、番組 (サービスと開始時刻) と話と記録先の組で 1 回だけ記録するので、後からプレイヤーで見ても二重に記録しない。
// 作品の 1 話は、同じ話の記録がどの番組にもなければ記録し、後からその話の番組を見ても記録しない。
// 管理者が連携を無効にしていれば ErrUnavailable を、どの記録先とも連携していなければ ErrNotConnected を、
// 記録できる話がなければ ErrNothingToRecord を、すべて記録済みなら ErrAlreadyRecorded を返す。
func (s *Service) RecordManually(ctx context.Context, db Database, inserter Inserter, userID uuid.UUID, target *ManualTarget) error {
	system, err := settings.Load(ctx, db)
	if err != nil {
		return err
	}
	if !system.AnimeLinkingEnabled {
		return ErrUnavailable
	}
	connections, err := s.Connections(ctx, db, userID)
	if err != nil {
		return err
	}
	if len(connections) == 0 {
		return ErrNotConnected
	}
	p, err := s.manualProgram(ctx, db, target)
	if err != nil {
		return err
	}
	if !hasRecordableEpisode(p) {
		return ErrNothingToRecord
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		// 二度押しやほかのタブから同時に頼まれても、同じ話の記録を二重に作らない。
		if err := sqlc.New(tx).LockManualAnimeRecords(ctx, userID.String()); err != nil {
			return fmt.Errorf("failed to lock the manual anime records: %w", err)
		}
		inserted, err := s.insertRecords(ctx, tx, inserter, userID, p, connections)
		if err != nil {
			return err
		}
		if !inserted {
			return ErrAlreadyRecorded
		}

		return nil
	})
}

// manualProgram は、手で記録する話を、記録の行を作る番組の形にする。
func (s *Service) manualProgram(ctx context.Context, db sqlc.DBTX, target *ManualTarget) (*program, error) {
	switch {
	case target.Program != nil:
		episodes, err := anime.ProgramEpisodes(ctx, db, target.Program.ServiceID, target.Program.StartAt)
		if err != nil {
			return nil, err
		}

		return &program{
			serviceID: target.Program.ServiceID, startAt: target.Program.StartAt, endAt: target.Program.StartAt, name: target.Program.Name,
			episodes: episodes,
		}, nil
	case target.RecordingID != nil:
		row, err := sqlc.New(db).GetRecordingProgram(ctx, *target.RecordingID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNothingToRecord
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get the recording: %w", err)
		}
		episodes, err := anime.RecordingEpisodes(ctx, db, row.ID)
		if err != nil {
			return nil, err
		}

		return &program{serviceID: row.ServiceID, startAt: row.StartAt, endAt: row.EndAt, name: row.Name, episodes: episodes}, nil
	case target.Work != nil:
		work := target.Work
		if work.Number < 0 || math.IsNaN(work.Number) || math.IsInf(work.Number, 0) {
			return nil, ErrNothingToRecord
		}
		number := work.Number
		episode := &anime.Episode{
			Source: anime.SourceTitle, TID: new(work.TID), PID: nil, CountStart: &number, CountEnd: &number, Subtitle: nil,
			First: false, Last: false, Rerun: false,
		}
		// 記録を頼んだ日時を、番組の開始時刻の代わりにする。マイクロ秒に丸め、DB に書いた値とジョブの引数をそろえる。
		requested := s.clock.Now().Truncate(time.Microsecond)

		return &program{
			serviceID: manualServiceID, startAt: requested, endAt: requested, name: work.Title,
			episodes: []*anime.LinkedEpisode{{Episode: episode, Works: nil}},
		}, nil
	default:
		return nil, ErrNothingToRecord
	}
}

// hasRecordableEpisode は、番組に、記録できる話 (TID に結びついた話) があるかどうかを返す。
func hasRecordableEpisode(p *program) bool {
	for _, episode := range p.episodes {
		if episode.TID != nil {
			return true
		}
	}

	return false
}
