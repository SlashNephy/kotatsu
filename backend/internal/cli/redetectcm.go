package cli

import (
	"context"
	"fmt"
	"io"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
)

// newRedetectCMCommand は kotatsu admin redetect-cm を作る (#1814)。
// 最後に見分けた CM の区間が、管理者が選んだ見分け方の今の版より古い録画を探し、見分け直すジョブをまとめて投入する。
// 材料が残っていれば、ジョブは録画を読み直さずに判定し直す。
func newRedetectCMCommand(opts *Options, connect func(ctx context.Context) (*config.Config, *pgxpool.Pool, error)) *cobra.Command {
	var allDetectors, dryRun bool
	cmd := &cobra.Command{
		Use:   "redetect-cm",
		Short: "Queue CM detection jobs for recordings detected by an older version of the detector",
		Long: "Queue CM detection jobs for recordings whose latest CM sections were detected by an older version of the " +
			"selected detector. The jobs run one at a time on nekonote: recordings with materials are redetected from them " +
			"without reading the recordings again, and the others are read again.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, pool, err := connect(cmd.Context())
			if err != nil {
				return err
			}
			defer closePool(pool)

			detector, version, recordings, err := cmdetect.Outdated(cmd.Context(), pool, allDetectors)
			if err != nil {
				return err
			}
			fromMaterials := 0
			for _, recording := range recordings {
				if recording.HasMaterial {
					fromMaterials++
				}
			}
			out := cmd.OutOrStdout()
			if dryRun {
				return printOutdatedCM(out, detector, version, recordings, fromMaterials)
			}
			inserter, err := jobs.NewClient(pool, opts.Logger, &jobs.Options{PollOnly: cfg.Database.Pooler, Workers: nil, PeriodicJobs: nil})
			if err != nil {
				return err
			}
			ids := make([]uuid.UUID, 0, len(recordings))
			for _, recording := range recordings {
				ids = append(ids, recording.RecordingID)
			}
			queued, err := cmdetect.EnqueueMany(cmd.Context(), pool, inserter, opts.Clock, ids)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(out, "queued %d of %d recordings (%d were already queued)\n", queued, len(recordings), len(recordings)-queued)

			return err
		},
	}
	cmd.Flags().BoolVar(&allDetectors, "all-detectors", false,
		"also redetect recordings whose latest CM sections were detected by another detector")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only list the recordings to redetect")

	return cmd
}

// printOutdatedCM は、見分け直す録画の数と一覧を out に書く (#1814)。
// 1 行に 1 本ずつ、放送の開始、録画の ID、最後に見分けた見分け方と版、材料から判定し直せそうか (materials) 読み直すか (read)、番組名をタブで区切る。
func printOutdatedCM(out io.Writer, detector string, version int32, recordings []cmdetect.OutdatedRecording, fromMaterials int) error {
	_, err := fmt.Fprintf(out, "%d recordings would be redetected by %s version %d (%d from the materials, %d by reading the recordings)\n",
		len(recordings), detector, version, fromMaterials, len(recordings)-fromMaterials)
	if err != nil {
		return err
	}
	for _, recording := range recordings {
		how := "read"
		if recording.HasMaterial {
			how = "materials"
		}
		_, err = fmt.Fprintf(out, "%s\t%s\t%s v%d\t%s\t%s\n",
			recording.StartAt.Format(time.RFC3339), recording.RecordingID, recording.Detector, recording.DetectorVersion, how, recording.Name)
		if err != nil {
			return err
		}
	}

	return nil
}
