package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/epgstation"
)

var (
	// errImportUserRequired は、取り込みに --user が要ることを表す。
	errImportUserRequired = errors.New("--user is required to import; run with --dry-run to only see the plan")
	// errNoServices は、Kotatsu がまだチャンネルを同期していないことを表す。
	errNoServices = errors.New("kotatsu has no services yet; start kotatsu and let it sync the channels from the tuner server before importing")
	// errInvalidPoolMapping は、--pool の値が正しくないことを表す。
	errInvalidPoolMapping = errors.New("invalid --pool")
	// errSourcePassword は、取り込み元の DB のパスワードの渡し方が正しくないことを表す。
	errSourcePassword = errors.New("invalid source password")
)

// newImportCommand は kotatsu import を作る。loadConfig は、--config と環境変数から設定を読む。
// connect は、設定を読み、DB に接続する。dry run では DB に接続しない。
func newImportCommand(
	opts *Options,
	loadConfig func(ctx context.Context) (*config.Config, error),
	connect func(ctx context.Context) (*config.Config, *pgxpool.Pool, error),
) *cobra.Command {
	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Import data from another recorder",
		Args:  cobra.NoArgs,
	}

	var (
		source   string
		pools    []string
		dryRun   bool
		username string
	)
	epgstationCmd := &cobra.Command{
		Use:   "epgstation",
		Short: "Import rules, reserves, and recordings from an EPGStation database",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !dryRun && username == "" {
				return errImportUserRequired
			}

			// dry run では DB に接続しない。
			var (
				cfg  *config.Config
				pool *pgxpool.Pool
				err  error
			)
			if dryRun {
				cfg, err = loadConfig(cmd.Context())
			} else {
				cfg, pool, err = connect(cmd.Context())
			}
			if err != nil {
				return err
			}
			defer closePool(pool)
			poolMap, err := parsePoolMappings(pools, cfg.Storage.Pools)
			if err != nil {
				return err
			}
			source, err := sourceWithPassword(source, opts.LookupEnv)
			if err != nil {
				return err
			}
			snapshot, err := readEPGStation(cmd.Context(), source)
			if err != nil {
				return err
			}
			if dryRun {
				plan := epgstation.NewPlan(snapshot, &epgstation.PlanOptions{Now: opts.Clock.Now(), Pools: poolMap})

				return printImportPlan(cmd.OutOrStdout(), plan)
			}

			result, err := importEPGStation(cmd.Context(), opts, &cfg.Database, pool, snapshot, poolMap, username)
			if err != nil {
				return err
			}

			return printImportResult(cmd.OutOrStdout(), result)
		},
	}
	epgstationCmd.Flags().StringVar(&source, "source", "",
		"EPGStation database URL (mysql://user:password@host:3306/database or sqlite:///path/to/database.db); it is opened read-only. "+
			"The password can be given by "+sourceAuthEnv+" instead")
	epgstationCmd.Flags().StringArrayVar(&pools, "pool", []string{},
		"map an EPGStation recorded directory name to a Kotatsu storage pool ID, as NAME=POOL_ID (repeatable)")
	epgstationCmd.Flags().BoolVar(&dryRun, "dry-run", false, "only show what would be imported")
	epgstationCmd.Flags().StringVar(&username, "user", "", "the Kotatsu user recorded as the creator of the imported rules and reserves")
	_ = epgstationCmd.MarkFlagRequired("source")
	importCmd.AddCommand(epgstationCmd)

	return importCmd
}

// sourceAuthEnv は、取り込み元の DB のパスワードを渡す環境変数。
// パスワードを URL に書くと、記号をエスケープする必要があり、プロセスの一覧にも見えるので、環境変数でも渡せるようにする。
const sourceAuthEnv = "KOTATSU_IMPORT_SOURCE_PASSWORD"

// sourceWithPassword は、sourceAuthEnv があれば、そのパスワードを source の URL に入れる。
// URL にもパスワードがあると、どちらを使うか分からないので拒む。
func sourceWithPassword(source string, lookupEnv config.LookupEnv) (string, error) {
	password, ok := lookupEnv(sourceAuthEnv)
	if !ok || password == "" {
		return source, nil
	}
	u, err := url.Parse(source)
	if err != nil {
		return "", fmt.Errorf("%w: failed to parse the source URL: %w", errSourcePassword, err)
	}
	if u.Scheme == "sqlite" {
		return "", fmt.Errorf("%w: %s is not used for sqlite", errSourcePassword, sourceAuthEnv)
	}
	if u.User == nil || u.User.Username() == "" {
		return "", fmt.Errorf("%w: the source URL must have a user name to use %s", errSourcePassword, sourceAuthEnv)
	}
	if _, set := u.User.Password(); set {
		return "", fmt.Errorf("%w: the source URL already has a password; remove it or unset %s", errSourcePassword, sourceAuthEnv)
	}
	u.User = url.UserPassword(u.User.Username(), password)

	return u.String(), nil
}

// readEPGStation は、EPGStation の DB を読み取り専用で開き、取り込むデータを読む。
func readEPGStation(ctx context.Context, source string) (*epgstation.Snapshot, error) {
	src, err := epgstation.Open(ctx, source)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = src.Close()
	}()

	return src.Read(ctx)
}

// importEPGStation は、snapshot を Kotatsu に取り込む。チャンネルを同期する前は、どのデータも取り込めないので拒む。
// 取り込んだ録画を解析するジョブは、nekonote が実行する。
func importEPGStation(
	ctx context.Context, opts *Options, db *config.Database, pool *pgxpool.Pool,
	snapshot *epgstation.Snapshot, poolMap map[string]string, username string,
) (*epgstation.Result, error) {
	serviceIDs, err := sqlc.New(pool).ListServiceIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	if len(serviceIDs) == 0 {
		return nil, errNoServices
	}
	userID, err := auth.UserIDByUsername(ctx, pool, username)
	if err != nil {
		return nil, err
	}

	inserter, err := jobs.NewClient(pool, opts.Logger, &jobs.Options{PollOnly: db.Pooler, Workers: nil, PeriodicJobs: nil})
	if err != nil {
		return nil, err
	}

	return epgstation.NewImporter(opts.Clock, audit.NewRecorder(opts.Clock), inserter).Apply(ctx, pool, snapshot, &epgstation.ApplyOptions{
		Pools: poolMap, UserID: userID,
	})
}

// parsePoolMappings は、--pool の NAME=POOL_ID を対応にする。POOL_ID は設定にあるプールでなければならない。
func parsePoolMappings(values []string, configured map[string]string) (map[string]string, error) {
	mappings := map[string]string{}
	for _, value := range values {
		name, poolID, ok := strings.Cut(value, "=")
		if !ok || name == "" || poolID == "" {
			return nil, fmt.Errorf("%w %q: must be NAME=POOL_ID", errInvalidPoolMapping, value)
		}
		if _, ok := configured[poolID]; !ok {
			return nil, fmt.Errorf("%w %q: pool %q is not configured (configured pools: %s)",
				errInvalidPoolMapping, value, poolID, strings.Join(slices.Sorted(maps.Keys(configured)), ", "))
		}
		if _, ok := mappings[name]; ok {
			return nil, fmt.Errorf("%w %q: directory %q is mapped more than once", errInvalidPoolMapping, value, name)
		}
		mappings[name] = poolID
	}

	return mappings, nil
}

// printImportPlan は、取り込みの計画を表にして w に書く。
func printImportPlan(w io.Writer, plan *epgstation.Plan) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	lines := []string{
		"EPGStation import plan (dry run)",
		"",
		fmt.Sprintf("Rules\t%d", plan.Rules),
		fmt.Sprintf("Manual reserves (program)\t%d", plan.ProgramReserves),
		fmt.Sprintf("Manual reserves (time)\t%d", plan.TimeReserves),
		fmt.Sprintf("Recordings\t%d", plan.Recordings),
		fmt.Sprintf("  TS files\t%d", plan.TSFiles),
		fmt.Sprintf("  Other files\t%d", plan.OtherFiles),
		fmt.Sprintf("  Drop logs\t%d", plan.DropLogs),
		fmt.Sprintf("Recorded histories\t%d", plan.Histories),
		fmt.Sprintf("Tags\t%d", plan.Tags),
		"",
		"Skipped",
		fmt.Sprintf("  Manual reserves already started\t%d", plan.StartedReserves),
		fmt.Sprintf("  Recordings in progress\t%d", plan.RecordingsInProgress),
	}
	if len(plan.UnmappedDirectories) > 0 {
		lines = append(lines,
			"",
			"Recorded directories without a storage pool (map them with --pool NAME=POOL_ID)",
			"  NAME\tFILES\tRULES\tRESERVES",
		)
		for _, dir := range plan.UnmappedDirectories {
			lines = append(lines, fmt.Sprintf("  %s\t%d\t%d\t%d", dir.Name, dir.Files, dir.Rules, dir.Reserves))
		}
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(tw, line); err != nil {
			return fmt.Errorf("failed to print the import plan: %w", err)
		}
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("failed to print the import plan: %w", err)
	}

	return nil
}

// importKinds は、取り込みの結果に出すデータの種類と、その見出し。取り込む順に並べる。
func importKinds() []struct {
	kind  epgstation.Kind
	label string
} {
	return []struct {
		kind  epgstation.Kind
		label string
	}{
		{epgstation.KindRule, "Rules"},
		{epgstation.KindReserve, "Manual reserves"},
		{epgstation.KindRecorded, "Recordings"},
		{epgstation.KindRecordedHistory, "Recorded histories"},
	}
}

// printImportResult は、取り込みの結果を表にして w に書く。
func printImportResult(w io.Writer, result *epgstation.Result) error {
	skipped := map[epgstation.Kind]int{}
	for _, s := range result.Skipped {
		skipped[s.Kind]++
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	lines := []string{
		"EPGStation import result",
		"",
		"KIND\tIMPORTED\tALREADY IMPORTED\tSKIPPED",
	}
	labels := map[epgstation.Kind]string{}
	for _, k := range importKinds() {
		labels[k.kind] = k.label
		lines = append(lines, fmt.Sprintf("%s\t%d\t%d\t%d", k.label, result.Imported[k.kind], result.AlreadyImported[k.kind], skipped[k.kind]))
	}
	if len(result.Skipped) > 0 {
		lines = append(lines, "", "Skipped", "  KIND\tID\tREASON")
		for _, s := range result.Skipped {
			lines = append(lines, fmt.Sprintf("  %s\t%d\t%s", labels[s.Kind], s.SourceID, s.Reason))
		}
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(tw, line); err != nil {
			return fmt.Errorf("failed to print the import result: %w", err)
		}
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("failed to print the import result: %w", err)
	}

	return nil
}
