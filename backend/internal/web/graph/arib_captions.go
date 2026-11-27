package graph

import (
	"context"
	"errors"
	"net/netip"
	"strconv"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/usecase/aribcaption"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	// actionARIBCaptionsExtract は、既にある録画の ARIB 字幕の文を取り出すジョブを入れた操作 (#2218)。
	actionARIBCaptionsExtract = "arib_captions.extract"
	// targetARIBCaptions は、ARIB 字幕の文の取り出しの操作の対象の種類。録画をまとめて扱うので、対象の ID は持たない。
	targetARIBCaptions = "arib_captions"
	// summaryRecordings と summaryQueued は、監査ログの要約の、対象の録画の数と、入れたジョブの数のキー。
	summaryRecordings = "recordings"
	summaryQueued     = "queued"
	// messageARIBCaptionsAdmin は、管理者でない人が ARIB 字幕の文の取り出しを扱おうとしたときのエラーの文。
	messageARIBCaptionsAdmin = "only administrators can extract the ARIB captions of recordings"
)

// aribCaptionBackfill は、既にある録画の ARIB 字幕の文を取り出す状況を返す (#2218)。
func (r *Resolver) aribCaptionBackfill(ctx context.Context) (*model.ARIBCaptionBackfill, error) {
	if _, _, err := r.administratorActor(ctx, messageARIBCaptionsAdmin); err != nil {
		return nil, err
	}
	recordings, err := aribcaption.Pending(ctx, r.db, false)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(recordings))
	for _, recording := range recordings {
		ids = append(ids, recording.RecordingID)
	}
	queued, err := aribcaption.Queued(ctx, r.db, ids)
	if err != nil {
		return nil, err
	}

	return &model.ARIBCaptionBackfill{Pending: len(recordings), Queued: queued, ExtractorVersion: aribcaption.ExtractorVersion}, nil
}

// extractARIBCaptions は、字幕の文を取り出していないか、取り出し方の古いバージョンで取り出した録画の、取り出すジョブをまとめて入れ、
// 監査ログに残す (#2218)。入れた数を返す。
func (r *Resolver) extractARIBCaptions(ctx context.Context) (int, error) {
	_, actor, err := r.administratorActor(ctx, messageARIBCaptionsAdmin)
	if err != nil {
		return 0, err
	}
	if r.jobs == nil {
		return 0, errors.New("the job client is not configured")
	}
	recordings, err := aribcaption.Pending(ctx, r.db, false)
	if err != nil {
		return 0, err
	}
	ids := make([]uuid.UUID, 0, len(recordings))
	for _, recording := range recordings {
		ids = append(ids, recording.RecordingID)
	}
	queued, enqueueErr := aribcaption.EnqueueMany(ctx, r.db, r.jobs, ids)
	// 途中で失敗しても、入れた分は動くので、入れた数を監査ログに残す。
	if queued > 0 || enqueueErr == nil {
		err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
			return r.recorder.Record(ctx, tx, &audit.Event{
				Actor: actor, Action: actionARIBCaptionsExtract, TargetType: targetARIBCaptions, TargetID: "",
				Summary:  map[string]string{summaryRecordings: strconv.Itoa(len(ids)), summaryQueued: strconv.Itoa(queued)},
				SourceIP: netip.Addr{}, UserAgent: "",
			})
		})
		if err != nil {
			return 0, errors.Join(enqueueErr, err)
		}
	}
	if enqueueErr != nil {
		return 0, enqueueErr
	}

	return queued, nil
}
