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
	"github.com/SlashNephy/kotatsu/internal/usecase/aribcaption"
)

// newExtractARIBCaptionsCommand は kotatsu admin extract-arib-captions を作る (#2218)。
// ARIB 字幕の文を取り出していない録画 (取り出すジョブを足す前に録った録画) と、取り出し方の古いバージョンで取り出した録画を探し、
// 取り出すジョブをまとめて投入する。ジョブは、録画を終えたときと同じく nekonote が動かす。
func newExtractARIBCaptionsCommand(opts *Options, connect func(ctx context.Context) (*config.Config, *pgxpool.Pool, error)) *cobra.Command {
	var all, dryRun bool
	cmd := &cobra.Command{
		Use:   "extract-arib-captions",
		Short: "Queue jobs to extract the ARIB captions of recordings that have none",
		Long: "Queue jobs to extract the ARIB captions of recordings whose captions were not extracted yet or were extracted by " +
			"an older version of the extractor. The jobs run one at a time on nekonote, after the jobs for new recordings.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, pool, err := connect(cmd.Context())
			if err != nil {
				return err
			}
			defer closePool(pool)

			recordings, err := aribcaption.Pending(cmd.Context(), pool, all)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if dryRun {
				return printPendingARIBCaptions(out, recordings)
			}
			inserter, err := jobs.NewClient(pool, opts.Logger, &jobs.Options{PollOnly: cfg.Database.Pooler, Workers: nil, PeriodicJobs: nil})
			if err != nil {
				return err
			}
			ids := make([]uuid.UUID, 0, len(recordings))
			for _, recording := range recordings {
				ids = append(ids, recording.RecordingID)
			}
			queued, err := aribcaption.EnqueueMany(cmd.Context(), pool, inserter, ids)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(out, "queued %d of %d recordings (%d were already queued)\n", queued, len(recordings), len(recordings)-queued)

			return err
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "also extract the captions of recordings extracted by the current version")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only list the recordings to extract")

	return cmd
}

// printPendingARIBCaptions は、字幕の文を取り出す録画の数と一覧を out に書く (#2218)。
// 1 行に 1 本ずつ、放送の開始、録画の ID、前に取り出したときの取り出し方のバージョン (取り出していなければ none)、番組名をタブで区切る。
func printPendingARIBCaptions(out io.Writer, recordings []aribcaption.PendingRecording) error {
	_, err := fmt.Fprintf(out, "%d recordings would be extracted by version %d\n", len(recordings), aribcaption.ExtractorVersion)
	if err != nil {
		return err
	}
	for _, recording := range recordings {
		previous := "none"
		if recording.ExtractorVersion != nil {
			previous = fmt.Sprintf("v%d", *recording.ExtractorVersion)
		}
		_, err = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", recording.StartAt.Format(time.RFC3339), recording.RecordingID, previous, recording.Name)
		if err != nil {
			return err
		}
	}

	return nil
}
