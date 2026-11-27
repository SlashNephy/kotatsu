// Command e2erecording は、録画中に mikan を入れ替える場面を、本物のプロセスで立てて確認する E2E テストを流す (#2195、設計書 11.6)。
//
// PostgreSQL とモックのチューナーサーバーを使い、kotatsu の neko と複数の mikan を別のプロセスで動かす。録画中に録っている mikan に
// SIGTERM を送り、ほかの mikan が引き継いで、録画が失敗にならず、続きが録れることを見る。場面は次のとおり。
//
//   - unreadable-settings: 引き継いだ mikan が、セグメントを作る前に録画設定を読めない。system_settings のカラムを改名して、
//     ローリング更新で新しいスキーマが古いバイナリの読むカラムを消したときと同じエラー (42703) を返させる。
//   - outdated-mikan: 古いバイナリの mikan と、新しいスキーマの DB が重なる。古いバイナリ (最新のマイグレーションを知らない) の mikan は、
//     新しいバイナリの mikan がいれば引き継ぎを受けず、新しいバイナリの mikan が引き継ぐ。
//
// 開発と検証にだけ使うエントリーポイントなので、配布するものには入れない (設計書 11.5)。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
)

const (
	// leadTime は、予約を作ってから録画の予定の開始までの時間。neko が番組表を取り込み、割り振りを書く間を見込む。
	leadTime = 20 * time.Second
	// recordFor は、予約の長さ。引き継ぎの前後に録る分と、引き継ぎを待つ時間の上限 (20 秒) を収める。
	recordFor = 2 * time.Minute
	// pollInterval は、DB の状態を読み直す間隔。
	pollInterval = 200 * time.Millisecond
	// stepTimeout は、1 つの段階が済むのを待つ時間の上限。
	stepTimeout = time.Minute
)

func main() {
	os.Exit(run())
}

type options struct {
	kotatsu     string
	oldKotatsu  string
	databaseURL string
	tsPath      string
	scenarios   []string
	work        string
}

func run() int {
	var opts options
	var scenarios string
	flag.StringVar(&opts.kotatsu, "kotatsu", "./bin/kotatsu", "kotatsu binary to test")
	flag.StringVar(&opts.oldKotatsu, "old-kotatsu", "", "kotatsu binary that does not know the latest migration (for outdated-mikan)")
	flag.StringVar(&opts.databaseURL, "database-url", os.Getenv("KOTATSU_DATABASE_URL"), "PostgreSQL URL where the test may create databases")
	flag.StringVar(&opts.tsPath, "ts", "backend/testdata/ts/test.ts", "TS file the fake tuner server streams")
	flag.StringVar(&scenarios, "scenario", "unreadable-settings,outdated-mikan", "comma-separated scenarios to run")
	flag.StringVar(&opts.work, "work", "", "directory for the logs and the recordings (a temporary directory when empty)")
	flag.Parse()
	opts.scenarios = strings.Split(scenarios, ",")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := runAll(ctx, logger, &opts); err != nil {
		logger.ErrorContext(ctx, "e2e recording test failed", slog.Any("error", err))

		return 1
	}
	logger.InfoContext(ctx, "e2e recording test passed")

	return 0
}

func runAll(ctx context.Context, logger *slog.Logger, opts *options) error {
	if opts.databaseURL == "" {
		return errors.New("-database-url (or KOTATSU_DATABASE_URL) is required")
	}
	if opts.work == "" {
		dir, err := os.MkdirTemp("", "kotatsu-e2e-recording-")
		if err != nil {
			return fmt.Errorf("failed to create the work directory: %w", err)
		}
		opts.work = dir
	}
	ts, err := os.ReadFile(opts.tsPath)
	if err != nil {
		return fmt.Errorf("failed to read the TS file: %w", err)
	}
	for _, name := range opts.scenarios {
		scenario, ok := scenarioOf(name)
		if !ok {
			return fmt.Errorf("unknown scenario %q", name)
		}
		logger := logger.With(slog.String("scenario", name))
		logger.InfoContext(ctx, "starting the scenario", slog.String("work", filepath.Join(opts.work, name)))
		env, err := newStack(ctx, logger, opts, name, ts)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		err = scenario(ctx, env)
		env.close(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w (logs in %s)", name, err, env.dir)
		}
		logger.InfoContext(ctx, "the scenario passed")
	}

	return nil
}

// scenarioOf は、名前 name の場面を返す。
func scenarioOf(name string) (func(ctx context.Context, s *stack) error, bool) {
	switch name {
	case "unreadable-settings":
		return unreadableSettings, true
	case "outdated-mikan":
		return outdatedMikan, true
	default:
		return nil, false
	}
}

// unreadableSettings は、引き継いだ mikan がセグメントを作る前に録画設定を読めなくても、録画を失敗にせず、続きを録ることを確認する。
func unreadableSettings(ctx context.Context, s *stack) error {
	if err := s.migrate(ctx, s.opts.kotatsu); err != nil {
		return err
	}
	if _, err := s.start(ctx, s.opts.kotatsu, "neko"); err != nil {
		return err
	}
	first, err := s.start(ctx, s.opts.kotatsu, "mikan")
	if err != nil {
		return err
	}
	second, err := s.start(ctx, s.opts.kotatsu, "mikan")
	if err != nil {
		return err
	}
	rec, err := s.reserveAndRecord(ctx)
	if err != nil {
		return err
	}
	holder, other := first, second
	if rec.holder == second.address {
		holder, other = second, first
	}

	// 引き継ぐ mikan が、録画設定を読めないようにする。ローリング更新で、新しいスキーマが古いバイナリの読むカラムを消した間と同じエラーを返す。
	column, err := s.anyColumn(ctx, "system_settings")
	if err != nil {
		return err
	}
	if err := s.exec(ctx, fmt.Sprintf("ALTER TABLE system_settings RENAME COLUMN %s TO %s_e2e", column, column)); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "made the recording settings unreadable; stopping the mikan that records", slog.String("mikan", holder.name))
	if err := holder.terminate(); err != nil {
		return err
	}
	if err := s.waitForTakeOver(ctx, rec, other); err != nil {
		return err
	}
	if err := s.exec(ctx, fmt.Sprintf("ALTER TABLE system_settings RENAME COLUMN %s_e2e TO %s", column, column)); err != nil {
		return err
	}
	if err := holder.wait(stepTimeout); err != nil {
		return err
	}

	return s.waitForCompletion(ctx, rec)
}

// outdatedMikan は、古いバイナリの mikan と新しいスキーマの DB が重なっても、古いバイナリの mikan は引き継ぎを受けず、
// 新しいバイナリの mikan が引き継いで、録画が失敗にならないことを確認する。
func outdatedMikan(ctx context.Context, s *stack) error {
	if s.opts.oldKotatsu == "" {
		return errors.New("-old-kotatsu is required")
	}
	if err := s.migrate(ctx, s.opts.kotatsu); err != nil {
		return err
	}
	if _, err := s.start(ctx, s.opts.kotatsu, "neko"); err != nil {
		return err
	}
	holder, err := s.start(ctx, s.opts.kotatsu, "mikan")
	if err != nil {
		return err
	}
	rec, err := s.reserveAndRecord(ctx)
	if err != nil {
		return err
	}
	// 古いバイナリの mikan は、録画を始めた後に加わる。録画中の更新では、新しいバイナリの mikan と古いバイナリの mikan が並ぶ。
	outdated, err := s.start(ctx, s.opts.oldKotatsu, "mikan")
	if err != nil {
		return err
	}
	if err := s.waitForAlive(ctx, outdated); err != nil {
		return err
	}
	// 新しいバイナリの kotatsu migrate が、古いバイナリの知らないマイグレーションを当て、動かしてよいバイナリの下限を上げたことにする。
	// 古いバイナリは、起動するときに下限を確認するので、起動した後に上げる。
	if err := s.exec(ctx, "UPDATE schema_compatibility SET min_binary_version = $1", database.LatestVersion()); err != nil {
		return err
	}

	// Kubernetes のローリング更新と同じく、新しいバイナリの mikan が立ってから、録っている mikan を止める。
	// 止まる mikan は、止まり始めたときに登録簿から外れるので、引き継ぎを任せられるのは、新しく立った mikan だけである。
	upToDate, err := s.start(ctx, s.opts.kotatsu, "mikan")
	if err != nil {
		return err
	}
	if err := s.waitForAlive(ctx, upToDate); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "stopping the mikan that records; the outdated mikan must not take over", slog.String("mikan", holder.name))
	if err := holder.terminate(); err != nil {
		return err
	}
	if err := s.waitForTakeOverBy(ctx, rec, upToDate, outdated); err != nil {
		return err
	}
	// 引き継いだ後も、古いバイナリの mikan が取り直さないことを見る。
	if err := s.ensureNotHeldBy(ctx, rec, outdated, 5*time.Second); err != nil {
		return err
	}
	if err := holder.wait(stepTimeout); err != nil {
		return err
	}

	return s.waitForCompletion(ctx, rec)
}

// stack は、1 つの場面の、DB、モックのチューナーサーバー、kotatsu のプロセス。
type stack struct {
	logger *slog.Logger
	opts   *options
	dir    string
	// root は、dir を開いたもの。ログのファイルを、dir の外に作らないようにする。
	root      *os.Root
	url       string
	pool      *pgxpool.Pool
	tuner     *httptest.Server
	processes []*process
}

func newStack(ctx context.Context, logger *slog.Logger, opts *options, name string, ts []byte) (*stack, error) {
	dir := filepath.Join(opts.work, name)
	for _, sub := range []string{"recordings", "data"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("failed to create the directory: %w", err)
		}
	}
	url, err := createDatabase(ctx, opts.databaseURL, "kotatsu_e2e_"+strings.ReplaceAll(name, "-", "_"))
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the database: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		pool.Close()

		return nil, fmt.Errorf("failed to open the work directory: %w", err)
	}
	tuner := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(time.Now(), ts)))

	return &stack{logger: logger, opts: opts, dir: dir, root: root, url: url, pool: pool, tuner: tuner, processes: []*process{}}, nil
}

// createDatabase は、admin の PostgreSQL に、名前 name の空の DB を作り直し、その DB の URL を返す。
func createDatabase(ctx context.Context, admin, name string) (string, error) {
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return "", fmt.Errorf("failed to connect to the database: %w", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
		return "", fmt.Errorf("failed to drop the database: %w", err)
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		return "", fmt.Errorf("failed to create the database: %w", err)
	}

	return replaceDatabase(admin, name)
}

// replaceDatabase は、URL admin の DB の名前を name にした URL を返す。
func replaceDatabase(admin, name string) (string, error) {
	before, query, _ := strings.Cut(admin, "?")
	slash := strings.LastIndex(before, "/")
	if slash < 0 || !strings.Contains(before, "://") {
		return "", fmt.Errorf("the database URL must be a URL: %q", admin)
	}
	url := before[:slash+1] + name
	if query != "" {
		url += "?" + query
	}

	return url, nil
}

func (s *stack) close(ctx context.Context) {
	for _, p := range s.processes {
		if err := p.kill(); err != nil {
			s.logger.WarnContext(ctx, "failed to stop a process", slog.Any("error", err))
		}
	}
	s.tuner.Close()
	s.pool.Close()
	if err := s.root.Close(); err != nil {
		s.logger.WarnContext(ctx, "failed to close the work directory", slog.Any("error", err))
	}
}

func (s *stack) exec(ctx context.Context, sql string, args ...any) error {
	if _, err := s.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("failed to run %q: %w", sql, err)
	}

	return nil
}

// migrate は、binary の kotatsu migrate を流す。
func (s *stack) migrate(ctx context.Context, binary string) error {
	cmd := exec.CommandContext(ctx, binary, "migrate")
	cmd.Env = s.env("127.0.0.1:0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kotatsu migrate failed: %w\n%s", err, out)
	}

	return nil
}

// env は、kotatsu のプロセスに渡す環境変数を返す。mikan は、同じプールとデータディレクトリを共有する。
func (s *stack) env(listen string) []string {
	return append(os.Environ(),
		"KOTATSU_DATABASE_URL="+s.url,
		"KOTATSU_AUTH_SECRET_KEY=kotatsu-e2e-only-secret-key-do-not-use",
		"KOTATSU_TUNER_URL="+s.tuner.URL+"/",
		"KOTATSU_STORAGE_POOLS=main="+filepath.Join(s.dir, "recordings"),
		"KOTATSU_STORAGE_DATA_DIR="+filepath.Join(s.dir, "data"),
		"KOTATSU_SERVER_LISTEN="+listen,
		"KOTATSU_METRICS_LISTEN=127.0.0.1:0",
	)
}

// process は、kotatsu のロールのプロセス。
type process struct {
	name    string
	address string
	cmd     *exec.Cmd
	done    chan error
}

// start は、binary の role を、空いているポートで待ち受けさせて起動する。
func (s *stack) start(ctx context.Context, binary, role string) (*process, error) {
	address, err := freeAddress(ctx)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s-%d", role, len(s.processes)+1)
	log, err := s.root.Create(name + ".log")
	if err != nil {
		return nil, fmt.Errorf("failed to create the log file: %w", err)
	}
	// 止めるのは、SIGTERM を送る terminate か、場面を終えるときの kill である。ctx が終わったら (割り込み)、止める。
	cmd := exec.CommandContext(ctx, binary, role)
	cmd.Env = s.env(address)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		_ = log.Close()

		return nil, fmt.Errorf("failed to start %s: %w", name, err)
	}
	p := &process{name: name, address: address, cmd: cmd, done: make(chan error, 1)}
	go func() {
		p.done <- cmd.Wait()
		_ = log.Close()
	}()
	s.processes = append(s.processes, p)
	s.logger.InfoContext(ctx, "started a process", slog.String("process", name), slog.String("binary", binary), slog.String("address", address))

	return p, nil
}

func freeAddress(ctx context.Context) (string, error) {
	var config net.ListenConfig
	ln, err := config.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("failed to find a free port: %w", err)
	}
	defer ln.Close()

	return ln.Addr().String(), nil
}

// terminate は、プロセスに SIGTERM を送る。Kubernetes が Pod を止めるときと同じ。
func (p *process) terminate() error {
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop %s: %w", p.name, err)
	}

	return nil
}

// wait は、プロセスが終わるまで、timeout まで待つ。
func (p *process) wait(timeout time.Duration) error {
	select {
	case err := <-p.done:
		p.done <- err
		if err != nil {
			return fmt.Errorf("%s exited with an error: %w", p.name, err)
		}

		return nil
	case <-time.After(timeout):
		return fmt.Errorf("%s did not stop in %s", p.name, timeout)
	}
}

// exited は、プロセスが終わったかどうかを返す。
func (p *process) exited() bool {
	select {
	case err := <-p.done:
		p.done <- err

		return true
	default:
		return false
	}
}

// kill は、まだ動いていれば、プロセスを止める。
func (p *process) kill() error {
	select {
	case err := <-p.done:
		p.done <- err

		return nil
	default:
	}
	if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("failed to kill %s: %w", p.name, err)
	}
	// 止めたので終わり方は問わないが、wait でまた読めるよう、戻しておく。
	err := <-p.done
	p.done <- err

	return nil
}

// recording は、録画している予約。
type recording struct {
	reservationID uuid.UUID
	endAt         time.Time
	// holder は、録画を始めた mikan の、登録簿のアドレス。
	holder string
}

// reserveAndRecord は、番組表を取り込み、mikan がそろうのを待ってから、すぐに始まる時刻予約を作り、録画が始まるまで待つ。
func (s *stack) reserveAndRecord(ctx context.Context) (*recording, error) {
	var serviceID int64
	err := s.eventually(ctx, "the services are imported", stepTimeout, func(ctx context.Context) (bool, error) {
		err := s.pool.QueryRow(ctx, "SELECT id FROM services ORDER BY id LIMIT 1").Scan(&serviceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return err == nil, err
	})
	if err != nil {
		return nil, err
	}
	clk := clock.Real{}
	startAt := time.Now().Add(leadTime).Truncate(time.Second)
	reservation, err := reserve.NewReservations(clk, audit.NewRecorder(clk)).CreateTime(ctx, s.pool, audit.System("e2e"), &reserve.TimeInput{
		ServiceID: serviceID, StartAt: startAt, EndAt: startAt.Add(recordFor), Name: "E2E の録画", Overrides: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to reserve: %w", err)
	}
	rec := &recording{reservationID: reservation.ID, endAt: startAt.Add(recordFor), holder: ""}
	err = s.eventually(ctx, "a mikan starts recording", leadTime+stepTimeout, func(ctx context.Context) (bool, error) {
		state, err := s.state(ctx, rec)
		if err != nil {
			return false, err
		}
		if state.segments == 0 || state.holder == "" {
			return false, nil
		}
		rec.holder = state.holder

		return true, nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.InfoContext(ctx, "a mikan started recording", slog.String("holder", rec.holder))
	// 引き継ぐ前に、少し録っておく。
	time.Sleep(10 * time.Second)

	return rec, nil
}

// recordingState は、録画と割り振りの状態。
type recordingState struct {
	state         string
	failureReason string
	segments      int
	// holder は、割り振りを持つ mikan の登録簿のアドレス。誰も持っていなければ空。
	holder string
}

func (s *stack) state(ctx context.Context, rec *recording) (*recordingState, error) {
	var state recordingState
	err := s.pool.QueryRow(ctx, `SELECT r.state, coalesce(r.failure_reason, ''),
		(SELECT count(*) FROM recording_files f WHERE f.recording_id = r.id AND f.kind = 'segment'),
		coalesce((SELECT i.address FROM recording_assignments a JOIN instances i ON i.id = a.holder AND i.role = 'mikan'
			WHERE a.reservation_id = r.reservation_id), '')
		FROM recordings r WHERE r.reservation_id = $1`, rec.reservationID).
		Scan(&state.state, &state.failureReason, &state.segments, &state.holder)
	if errors.Is(err, pgx.ErrNoRows) {
		return &recordingState{state: "", failureReason: "", segments: 0, holder: ""}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the recording: %w", err)
	}

	return &state, nil
}

// waitForAlive は、mikan が登録簿に載るまで待つ。
func (s *stack) waitForAlive(ctx context.Context, p *process) error {
	return s.eventually(ctx, p.name+" is registered", stepTimeout, func(ctx context.Context) (bool, error) {
		if p.exited() {
			return false, fmt.Errorf("%s exited", p.name)
		}
		var n int
		err := s.pool.QueryRow(ctx, "SELECT count(*) FROM instances WHERE role = 'mikan' AND address = $1", p.address).Scan(&n)

		return n > 0, err
	})
}

// waitForTakeOver は、mikan to が録画を引き継ぎ、新しいセグメントを書き始めるまで待つ。その間に録画が失敗になったら、失敗にする。
func (s *stack) waitForTakeOver(ctx context.Context, rec *recording, to *process) error {
	return s.waitForTakeOverBy(ctx, rec, to, nil)
}

// waitForTakeOverBy は、waitForTakeOver と同じく待つ。待つ間に、mikan not (nil でなければ) が割り振りを取ったら、失敗にする。
func (s *stack) waitForTakeOverBy(ctx context.Context, rec *recording, to, not *process) error {
	var before int
	if state, err := s.state(ctx, rec); err == nil {
		before = state.segments
	}
	err := s.eventually(ctx, to.name+" takes over the recording", stepTimeout, func(ctx context.Context) (bool, error) {
		state, err := s.state(ctx, rec)
		if err != nil {
			return false, err
		}
		if state.state == "failed" {
			return false, fmt.Errorf("the recording failed during the handover: %s", state.failureReason)
		}
		if not != nil && state.holder == not.address {
			return false, fmt.Errorf("%s took over the recording", not.name)
		}

		return state.holder == to.address && state.segments > before, nil
	})
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "the recording was taken over", slog.String("mikan", to.name))

	return nil
}

// ensureNotHeldBy は、d の間、mikan p が録画の割り振りを取らないことを確認する。
func (s *stack) ensureNotHeldBy(ctx context.Context, rec *recording, p *process, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		state, err := s.state(ctx, rec)
		if err != nil {
			return err
		}
		if state.holder == p.address {
			return fmt.Errorf("%s took over the recording", p.name)
		}
		if err := sleep(ctx, pollInterval); err != nil {
			return err
		}
	}

	return nil
}

// waitForCompletion は、予定の終わりの後に録画が終わるまで待ち、失敗になっていないこと、引き継いで分かれたセグメントがあることを確認する。
func (s *stack) waitForCompletion(ctx context.Context, rec *recording) error {
	var final *recordingState
	err := s.eventually(ctx, "the recording finishes", time.Until(rec.endAt)+stepTimeout, func(ctx context.Context) (bool, error) {
		state, err := s.state(ctx, rec)
		if err != nil {
			return false, err
		}
		final = state

		return state.state != "recording", nil
	})
	if err != nil {
		return err
	}
	if !slices.Contains([]string{"completed", "completed_with_errors"}, final.state) {
		return fmt.Errorf("the recording ended as %s (%s)", final.state, final.failureReason)
	}
	if final.segments < 2 {
		return fmt.Errorf("the recording has %d segments; the mikan that took over did not record", final.segments)
	}
	s.logger.InfoContext(ctx, "the recording finished", slog.String("state", final.state), slog.Int("segments", final.segments))

	return nil
}

// anyColumn は、テーブル table の、主キーでないカラムの 1 つを返す。
func (s *stack) anyColumn(ctx context.Context, table string) (string, error) {
	var column string
	err := s.pool.QueryRow(ctx, `SELECT column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1 AND column_name <> 'id' ORDER BY ordinal_position LIMIT 1`, table).Scan(&column)
	if err != nil {
		return "", fmt.Errorf("failed to find a column of %s: %w", table, err)
	}

	return column, nil
}

// eventually は、check が true を返すまで、timeout まで読み直す。check がエラーを返したら、待たずに返す。
func (s *stack) eventually(ctx context.Context, what string, timeout time.Duration, check func(ctx context.Context) (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for {
		ok, err := check(ctx)
		if err != nil {
			return fmt.Errorf("while waiting until %s: %w", what, err)
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting until %s", what)
		}
		if err := sleep(ctx, pollInterval); err != nil {
			return err
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return nil
	}
}
