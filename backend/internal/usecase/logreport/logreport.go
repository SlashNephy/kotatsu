// Package logreport は、管理の画面で読む直近のログと、不具合の報告に添える環境を集める (#1521、設計書 11.3)。
//
// ログは、このプロセスのメモリのバッファ (logring) と、チューナーサーバーの /api/log から集める。
// 画面に送る前に、サーバーが知っている秘密の値を伏せる (1 段目)。ホスト名などを伏せる 2 段目は、ブラウザで行う。
// サーバーからは GitHub に何も送らない。
package logreport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/buildinfo"
	"github.com/SlashNephy/kotatsu/internal/lib/logring"
	"github.com/SlashNephy/kotatsu/internal/lib/redact"
)

// DefaultTimeout は、チューナーサーバーにログを問い合わせて待つ時間 (問い合わせの待ち時間、設計書 付録 A)。
const DefaultTimeout = 3 * time.Second

// Line は、画面に送る 1 行。
type Line struct {
	// Time は、epoch ミリ秒の時刻。
	Time  int64
	Level logring.Level
	// Inferred は、重さを本文の言葉から推し量ったかどうか。報告の行には、推し量った重さの印を付けない。
	Inferred bool
	Text     string
}

// SourceKind は、ログの出どころ。
type SourceKind string

const (
	SourceKotatsu     SourceKind = "KOTATSU"
	SourceTunerServer SourceKind = "TUNER_SERVER"
)

// Failure は、ログを取れなかった理由。文言にホスト名や URL を含めないよう、決まった語だけにする。
type Failure string

const (
	FailureNone        Failure = ""
	FailureTimeout     Failure = "TIMEOUT"
	FailureUnreachable Failure = "UNREACHABLE"
	// FailureUnsupported は、チューナーサーバーがログの API を持たないこと (mirakc など)。
	FailureUnsupported Failure = "UNSUPPORTED"
	FailureHTTPError   Failure = "HTTP_ERROR"
)

// Source は、1 つの出どころのログと環境。
type Source struct {
	Kind SourceKind
	// Lines は、古い順の行。
	Lines []Line
	// Total は、出どころが持っていた行の数。Lines は、このうち最後の Limit 行。
	Total   int
	Failure Failure
	// Version、OS、Arch は、出どころの版と環境。分からなければ空。
	Version string
	OS      string
	Arch    string
	// Runtime は、Kotatsu の Go の版。チューナーサーバーでは、サーバーの種類 (mahiron など、分かるときだけ)。
	Runtime string
}

// Report は、画面に送るログと、2 段目で伏せるホスト名。
type Report struct {
	Sources []*Source
	// Hosts は、2 段目で伏せるホスト名。リクエストが来たホスト名と、チューナーサーバー、DB のホスト名。
	Hosts []string
	// InstanceID と Roles は、このリクエストに答えたプロセス。ロールを分けた構成では、このプロセスのログしか出せない。
	InstanceID string
	Roles      []string
}

// TunerServer は、チューナーサーバーのログと版を読む。*mirakurun.Client が満たす。
type TunerServer interface {
	Version(ctx context.Context) (*mirakurun.Version, error)
	Status(ctx context.Context) (*mirakurun.Status, error)
	Log(ctx context.Context) ([]byte, error)
}

var _ TunerServer = new(mirakurun.Client)

// SecretSource は、1 段目で伏せる秘密の値を返す。
type SecretSource func(ctx context.Context) ([]string, error)

// Options は、Service の依存。
type Options struct {
	// Buffer は、このプロセスのログ。
	Buffer *logring.Buffer
	// Tuner は、チューナーサーバー。nil なら、チューナーサーバーのログは出さない。
	Tuner TunerServer
	// Hosts は、リクエストが来たホスト名のほかに伏せるホスト名 (チューナーサーバー、DB)。
	Hosts      []string
	InstanceID string
	Roles      []string
	Secrets    SecretSource
	// Timeout は、問い合わせの待ち時間。0 なら DefaultTimeout。
	Timeout time.Duration
	// Limit は、チューナーサーバーから受け取る行数の上限。0 なら logring.DefaultCapacity。
	Limit int
	// MaxLineLength は、チューナーサーバーの行を切る長さ。0 なら logring.DefaultMaxLineLength。
	MaxLineLength int
	// Location は、チューナーサーバーの時刻にタイムゾーンが無いときに使う。nil なら UTC。
	Location *time.Location
}

// Service は、ログの報告を作る。
type Service struct {
	opts Options
}

// New は Service を作る。
func New(opts *Options) *Service {
	o := *opts
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Limit <= 0 {
		o.Limit = logring.DefaultCapacity
	}
	if o.MaxLineLength <= 0 {
		o.MaxLineLength = logring.DefaultMaxLineLength
	}

	return &Service{opts: o}
}

// Report は、ログの報告を作る。requestHost は、リクエストが来たホスト名 (ポートを含んでもよい)。
// 秘密の値を集められなければ、伏せていないログを返さないよう、失敗にする。
func (s *Service) Report(ctx context.Context, requestHost string) (*Report, error) {
	secretValues, err := s.opts.Secrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to collect the secrets to redact: %w", err)
	}
	secrets := redact.NewSecrets(secretValues)

	build := buildinfo.Get()
	kotatsu := &Source{
		Kind: SourceKotatsu, Lines: []Line{}, Total: 0, Failure: FailureNone,
		Version: build.Version, OS: runtime.GOOS, Arch: runtime.GOARCH, Runtime: runtime.Version(),
	}
	for _, line := range s.opts.Buffer.Lines() {
		kotatsu.Lines = append(kotatsu.Lines, Line{Time: line.Time, Level: line.Level, Inferred: false, Text: secrets.Apply(line.Text)})
	}
	kotatsu.Total = len(kotatsu.Lines)

	report := &Report{
		Sources: []*Source{kotatsu}, Hosts: hostsOf(append([]string{requestHost}, s.opts.Hosts...)),
		InstanceID: s.opts.InstanceID, Roles: slices.Clone(s.opts.Roles),
	}
	if s.opts.Tuner != nil {
		report.Sources = append(report.Sources, s.tunerServer(ctx, secrets))
	}

	return report, nil
}

// tunerServer は、チューナーサーバーのログと版を、問い合わせの待ち時間まで待って集める。
func (s *Service) tunerServer(ctx context.Context, secrets *redact.Secrets) *Source {
	ctx, cancel := context.WithTimeout(ctx, s.opts.Timeout)
	defer cancel()

	source := &Source{Kind: SourceTunerServer, Lines: []Line{}, Total: 0, Failure: FailureNone, Version: "", OS: "", Arch: "", Runtime: ""}
	var (
		wg      sync.WaitGroup
		version *mirakurun.Version
		status  *mirakurun.Status
		body    []byte
		logErr  error
	)
	wg.Go(func() { version, _ = s.opts.Tuner.Version(ctx) })
	wg.Go(func() { status, _ = s.opts.Tuner.Status(ctx) })
	wg.Go(func() { body, logErr = s.opts.Tuner.Log(ctx) })
	wg.Wait()

	if version != nil {
		source.Version, source.Runtime = version.Current, version.Server
	}
	if status != nil {
		source.OS, source.Arch = status.Process.Platform, status.Process.Arch
		if source.Version == "" {
			source.Version = status.Version
		}
	}
	if logErr != nil {
		source.Failure = failureOf(ctx, logErr)

		return source
	}
	lines := ParseTunerLog(string(body), s.opts.Location)
	source.Total = len(lines)
	for _, line := range lastLines(lines, s.opts.Limit) {
		// 切り口に秘密がまたがって前半が残らないよう、伏せてから切る。
		line.Text = logring.Truncate(secrets.Apply(line.Text), s.opts.MaxLineLength)
		source.Lines = append(source.Lines, line)
	}

	return source
}

// failureOf は、ログを取れなかった理由を、決まった語にする。エラーの文言はホスト名や URL を含むので、画面に出さない。
func failureOf(ctx context.Context, err error) Failure {
	switch status := mirakurun.HTTPStatus(err); {
	case status == http.StatusNotFound || status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented:
		return FailureUnsupported
	case status != 0:
		return FailureHTTPError
	}
	if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return FailureTimeout
	}

	return FailureUnreachable
}

// hostsOf は、ホスト名 (host:port や URL でもよい) から、ポートを除いたホスト名を、重ねずに返す。
func hostsOf(values []string) []string {
	hosts := []string{}
	for _, value := range values {
		host := HostOf(value)
		if host != "" && !slices.Contains(hosts, host) {
			hosts = append(hosts, host)
		}
	}

	return hosts
}

// HostOf は、URL か host:port から、ホスト名を返す。読めなければ空を返す。
func HostOf(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		u, err := url.Parse(value)
		if err != nil {
			return ""
		}

		return strings.ToLower(u.Hostname())
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return strings.ToLower(host)
	}

	return strings.ToLower(strings.Trim(value, "[]"))
}
