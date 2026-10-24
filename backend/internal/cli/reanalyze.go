package cli

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/SlashNephy/kotatsu/internal/analysis"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
)

// newReanalyzeCommand は kotatsu admin reanalyze を作る (#897)。
// 索引か代表画像のない録画を探し、解析し直すジョブをまとめて投入する。解析が終われば、画像も作り直す。
func newReanalyzeCommand(opts *Options, connect func(ctx context.Context) (*config.Config, *pgxpool.Pool, error)) *cobra.Command {
	var all, dryRun bool
	cmd := &cobra.Command{
		Use:   "reanalyze",
		Short: "Queue analysis jobs for recordings without an index or a thumbnail",
		Long: "Queue analysis jobs for finished recordings whose index or thumbnail is missing in the data directory. " +
			"The analysis also regenerates the images. Run this where the pools and the data directory are mounted, " +
			"for example with kubectl exec. The jobs run after those of new recordings.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, pool, err := connect(cmd.Context())
			if err != nil {
				return err
			}
			defer closePool(pool)

			candidates, err := analysis.Candidates(cmd.Context(), pool, storeOf(cfg))
			if err != nil {
				return err
			}
			ids := []uuid.UUID{}
			for _, candidate := range candidates {
				if all || candidate.Missing {
					ids = append(ids, candidate.RecordingID)
				}
			}
			out := cmd.OutOrStdout()
			if dryRun {
				_, err = fmt.Fprintf(out, "%d of %d recordings would be reanalyzed\n", len(ids), len(candidates))

				return err
			}
			inserter, err := jobs.NewClient(pool, opts.Logger, &jobs.Options{PollOnly: cfg.Database.Pooler, Workers: nil, PeriodicJobs: nil})
			if err != nil {
				return err
			}
			queued, err := analysis.Enqueue(cmd.Context(), pool, inserter, ids)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(out, "queued %d of %d recordings (%d were already queued)\n", queued, len(ids), len(ids)-queued)

			return err
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "reanalyze all finished recordings, not only those without an index or a thumbnail")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only count the recordings to reanalyze")

	return cmd
}
