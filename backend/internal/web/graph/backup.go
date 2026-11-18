package graph

import (
	"context"
	"errors"
	"net/netip"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/backup"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	// targetBackup は、バックアップの操作の、監査ログの対象の種類 (#1926)。
	targetBackup = "backup"
	// 監査ログの種類。設定を変えたことと、今すぐ書き出すよう頼んだこと。
	actionBackupSettingsUpdate = "backup.settings.update"
	actionBackupRun            = "backup.run"
	// onlyAdminsBackup は、管理者でないときの誤りの文。
	onlyAdminsBackup = "only administrators can manage backups"
)

// backupStatus は、定期のバックアップの設定と、最後の書き出しの結果を返す (#1926)。管理者だけが使える。
func (r *Resolver) backupStatus(ctx context.Context) (*model.BackupStatus, error) {
	if _, _, err := r.administratorActor(ctx, onlyAdminsBackup); err != nil {
		return nil, err
	}

	return r.loadBackupStatus(ctx)
}

func (r *Resolver) loadBackupStatus(ctx context.Context) (*model.BackupStatus, error) {
	settings, err := backup.LoadSettings(ctx, r.db)
	if err != nil {
		return nil, err
	}
	status := &model.BackupStatus{
		Settings: &model.BackupSettings{
			Recipients: settings.Recipients, IntervalHours: settings.IntervalHours, Generations: settings.Generations, UpdatedAt: settings.UpdatedAt,
		},
		LatestRun: nil,
	}
	run, err := backup.LatestRun(ctx, r.db)
	if errors.Is(err, backup.ErrNoRuns) {
		return status, nil
	}
	if err != nil {
		return nil, err
	}
	status.LatestRun = &model.BackupRun{
		Manual: run.Manual, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, FileName: run.FileName, Size: run.SizeBytes, Error: run.Error,
	}

	return status, nil
}

// updateBackupSettings は、定期のバックアップの設定を変え、監査ログに残す (#1926)。省いた項目は変えない。
func (r *Resolver) updateBackupSettings(ctx context.Context, input model.BackupSettingsInput) (*model.BackupStatus, error) {
	_, actor, err := r.administratorActor(ctx, onlyAdminsBackup)
	if err != nil {
		return nil, err
	}
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		settings, err := backup.LoadSettings(ctx, tx)
		if err != nil {
			return err
		}
		if input.Recipients != nil {
			settings.Recipients = input.Recipients
		}
		if input.IntervalHours != nil {
			settings.IntervalHours = *input.IntervalHours
		}
		if input.Generations != nil {
			settings.Generations = *input.Generations
		}
		if err := backup.SaveSettings(ctx, tx, r.clock.Now(), settings); err != nil {
			if errors.Is(err, backup.ErrInvalidSettings) {
				return NewError(CodeBadUserInput, err.Error())
			}

			return err
		}
		saved, err := backup.LoadSettings(ctx, tx)
		if err != nil {
			return err
		}

		// 公開鍵は秘密ではないが、監査ログには数だけを残す。
		return r.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionBackupSettingsUpdate, TargetType: targetBackup, TargetID: "settings",
			Summary: map[string]string{
				"recipients":    strconv.Itoa(len(saved.Recipients)),
				"intervalHours": strconv.Itoa(saved.IntervalHours),
				"generations":   strconv.Itoa(saved.Generations),
			},
			SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
	if err != nil {
		return nil, err
	}

	return r.loadBackupStatus(ctx)
}

// runBackup は、今すぐバックアップを書き出すジョブを入れ、監査ログに残す (#1926)。
func (r *Resolver) runBackup(ctx context.Context) (*model.BackupStatus, error) {
	_, actor, err := r.administratorActor(ctx, onlyAdminsBackup)
	if err != nil {
		return nil, err
	}
	if r.jobs == nil {
		return nil, errJobsUnavailable
	}
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		settings, err := backup.LoadSettings(ctx, tx)
		if err != nil {
			return err
		}
		if len(settings.Recipients) == 0 {
			return NewError(CodeBadUserInput, "register an age public key before making a backup")
		}
		inserted, err := backup.EnqueueManualTx(ctx, tx, r.jobs)
		if err != nil {
			return err
		}
		if !inserted {
			return NewError(CodeConflict, "a backup is already requested")
		}

		return r.recorder.Record(ctx, tx, &audit.Event{
			Actor: actor, Action: actionBackupRun, TargetType: targetBackup, TargetID: "run",
			Summary: map[string]string{}, SourceIP: netip.Addr{}, UserAgent: "",
		})
	})
	if err != nil {
		return nil, err
	}

	return r.loadBackupStatus(ctx)
}
