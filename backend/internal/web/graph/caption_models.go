package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/captionmodel"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	// targetCaptionModel は、字幕の自動生成のモデルの操作の、監査ログの対象の種類。対象の ID は、モデルの種類 (WHISPER か VAD)。
	targetCaptionModel = "caption_model"
	// 字幕の自動生成のモデルの操作の、監査ログの種類 (#1682)。
	actionCaptionModelDownload = "caption_model.download"
	actionCaptionModelCancel   = "caption_model.cancel"
	actionCaptionModelDelete   = "caption_model.delete"
	// summaryModel は、監査ログの要約の、モデルの ID のキー。
	summaryModel = "model"
)

// captionsConfig は、字幕の自動生成の設定を返す。渡されていなければ、場所を書いていない設定とみなす。
func (r *Resolver) captionsConfig() *config.Captions {
	if r.captions == nil {
		return &config.Captions{WhisperCommand: "", WhisperModel: "", Threads: 0, VADCommand: "", VADModel: ""}
	}

	return r.captions
}

// captionModelSlots は、字幕の自動生成のモデルの置き場所を、種類ごとに返す (#1682)。管理者だけが使える。
func (r *Resolver) captionModelSlots(ctx context.Context) ([]*model.CaptionModelSlot, error) {
	if _, _, err := r.administratorActor(ctx, "only administrators can manage caption models"); err != nil {
		return nil, err
	}

	return r.loadCaptionModelSlots(ctx)
}

func (r *Resolver) loadCaptionModelSlots(ctx context.Context) ([]*model.CaptionModelSlot, error) {
	statuses, err := captionmodel.Statuses(ctx, r.db, r.dataDir, r.captionsConfig())
	if err != nil {
		return nil, err
	}
	slots := make([]*model.CaptionModelSlot, 0, len(statuses))
	for _, s := range statuses {
		slots = append(slots, captionModelSlot(s))
	}

	return slots, nil
}

// captionModelSlot は、種類 kind の置き場所を返す。
func (r *Resolver) captionModelSlot(ctx context.Context, kind captionmodel.Kind) (*model.CaptionModelSlot, error) {
	slots, err := r.loadCaptionModelSlots(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range slots {
		if s.Kind == kind {
			return s, nil
		}
	}

	return nil, fmt.Errorf("unknown caption model kind: %s", kind)
}

// downloadCaptionModel は、一覧のモデル id をダウンロードするジョブを入れ、監査ログに残す (#1682)。
func (r *Resolver) downloadCaptionModel(ctx context.Context, id string) (*model.CaptionModelSlot, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can manage caption models")
	if err != nil {
		return nil, err
	}
	m, ok := captionmodel.Lookup(id)
	if !ok {
		return nil, NewError(CodeBadUserInput, "unknown caption model")
	}
	if r.jobs == nil {
		return nil, errors.New("the job client is not configured")
	}
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		inserted, err := captionmodel.EnqueueTx(ctx, tx, r.jobs, m)
		if err != nil {
			return err
		}
		if !inserted {
			return NewError(CodeConflict, "a caption model of this kind is already being downloaded")
		}

		return r.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionCaptionModelDownload, TargetType: targetCaptionModel, TargetID: string(m.Kind),
			Summary: map[string]string{summaryModel: m.ID}, SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
	if err != nil {
		return nil, err
	}

	return r.captionModelSlot(ctx, m.Kind)
}

// cancelCaptionModelDownload は、種類 kind の終えていないダウンロードを取り消し、監査ログに残す (#1682)。
func (r *Resolver) cancelCaptionModelDownload(ctx context.Context, kind captionmodel.Kind) (*model.CaptionModelSlot, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can manage caption models")
	if err != nil {
		return nil, err
	}
	if r.jobAdmin == nil {
		return nil, errJobsUnavailable
	}
	jobID, err := captionmodel.ActiveJob(ctx, r.db, kind)
	if errors.Is(err, captionmodel.ErrNotFound) {
		return nil, NewError(CodeNotFound, "no caption model is being downloaded")
	}
	if err != nil {
		return nil, err
	}
	job, err := r.jobAdmin.JobCancel(ctx, jobID)
	if errors.Is(err, river.ErrNotFound) {
		return nil, NewError(CodeNotFound, "no caption model is being downloaded")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to cancel the caption model download: %w", err)
	}
	// 監査ログには、取り消したモデルを残す。引数を読めなくても、取り消したことは残す。
	var args captionmodel.DownloadArgs
	_ = json.Unmarshal(job.EncodedArgs, &args)
	if err := r.recorder.Record(ctx, r.db, &audit.Event{
		Actor: actor, Action: actionCaptionModelCancel, TargetType: targetCaptionModel, TargetID: string(kind),
		Summary: map[string]string{summaryModel: args.ModelID}, SourceIP: netip.Addr{}, UserAgent: "",
	}); err != nil {
		return nil, err
	}

	return r.captionModelSlot(ctx, kind)
}

// deleteCaptionModel は、データディレクトリの既定の場所から、種類 kind のモデルを消し、監査ログに残す (#1682)。
// データディレクトリは、nekonote と同じものを tenban にもマウントする (設計書 5.6)。
func (r *Resolver) deleteCaptionModel(ctx context.Context, kind captionmodel.Kind) (*model.CaptionModelSlot, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can manage caption models")
	if err != nil {
		return nil, err
	}
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		removed, err := captionmodel.Remove(ctx, tx, r.dataDir, kind)
		switch {
		case errors.Is(err, captionmodel.ErrNoDataDir):
			return NewError(CodeConflict, "the data directory is not configured")
		case errors.Is(err, captionmodel.ErrDownloading):
			return NewError(CodeConflict, "the caption model is being downloaded")
		case errors.Is(err, captionmodel.ErrNotFound):
			return NewError(CodeNotFound, "no caption model is placed")
		case err != nil:
			return err
		}
		summary := map[string]string{summaryModel: ""}
		if removed != nil {
			summary[summaryModel] = removed.ModelID
		}

		return r.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionCaptionModelDelete, TargetType: targetCaptionModel, TargetID: string(kind),
			Summary: summary, SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
	if err != nil {
		return nil, err
	}

	return r.captionModelSlot(ctx, kind)
}

// captionModelSlot は、種類ごとの状態を GraphQL の形にする。
func captionModelSlot(s *captionmodel.Status) *model.CaptionModelSlot {
	slot := &model.CaptionModelSlot{
		Kind: s.Kind, State: s.State, Installed: nil, Download: nil, Overridden: s.Overridden, Models: []*model.CaptionModel{},
	}
	for _, m := range captionmodel.Catalog() {
		if m.Kind == s.Kind {
			slot.Models = append(slot.Models, captionModel(&m))
		}
	}
	if i := s.Installed; i != nil {
		slot.Installed = &model.InstalledCaptionModel{Model: nil, Size: float64(i.Size), InstalledAt: i.InstalledAt}
		if i.Model != nil {
			slot.Installed.Model = captionModel(i.Model)
		}
	}
	if d := s.Download; d != nil {
		slot.Download = &model.CaptionModelDownload{Model: nil, Running: d.Running, Failed: d.Failed, Progress: nil}
		if d.Model != nil {
			slot.Download.Model = captionModel(d.Model)
		}
		if p := d.Progress; p != nil {
			slot.Download.Progress = &model.JobProgress{Ratio: p.Ratio, RemainingSeconds: nil, UpdatedAt: p.UpdatedAt}
			if p.Remaining != nil {
				slot.Download.Progress.RemainingSeconds = new(int(p.Remaining.Round(time.Second).Seconds()))
			}
		}
	}

	return slot
}

func captionModel(m *captionmodel.Model) *model.CaptionModel {
	return &model.CaptionModel{
		ID: m.ID, Kind: m.Kind, Name: m.Name, Size: float64(m.Size), Sha256: m.SHA256, License: m.License, URL: m.URL, Recommended: m.Recommended,
	}
}
