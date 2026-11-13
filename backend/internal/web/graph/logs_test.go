package graph_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/logring"
	"github.com/SlashNephy/kotatsu/internal/usecase/logreport"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

// tunerVersion は、偽のチューナーサーバーが返す版。
const tunerVersion = "5.3.2"

const logReportQuery = `query {
  logReport {
    sources { kind total failure version os runtime lines { time level inferred text } }
    redactHosts
    instanceId
    roles
  }
}`

type logLine struct {
	Time     int64  `json:"time"`
	Level    string `json:"level"`
	Inferred bool   `json:"inferred"`
	Text     string `json:"text"`
}

type logSource struct {
	Kind    string    `json:"kind"`
	Total   int       `json:"total"`
	Failure *string   `json:"failure"`
	Version string    `json:"version"`
	OS      string    `json:"os"`
	Runtime string    `json:"runtime"`
	Lines   []logLine `json:"lines"`
}

type logReportResult struct {
	LogReport struct {
		Sources     []logSource `json:"sources"`
		RedactHosts []string    `json:"redactHosts"`
		InstanceID  string      `json:"instanceId"`
		Roles       []string    `json:"roles"`
	} `json:"logReport"`
}

// newLogTuner は、/api/log に logHandler で答える偽のチューナーサーバーの Client を作る。logHandler が nil なら、チューナーサーバーは無い。
func newLogTuner(t *testing.T, logHandler http.HandlerFunc) logreport.TunerServer {
	t.Helper()
	if logHandler == nil {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"current":"5.3.2","latest":"","server":"mahiron"}`))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"5.3.2","process":{"arch":"amd64","platform":"linux"}}`))
	})
	mux.HandleFunc("/api/log", logHandler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)

	return client
}

// tunerSource は、偽のチューナーサーバーの出どころとして返るはずのもの。
func tunerSource(total int, failure *string, lines []logLine) *logSource {
	return &logSource{
		Kind: "TUNER_SERVER", Total: total, Failure: failure, Version: tunerVersion, OS: "linux", Runtime: "mahiron", Lines: lines,
	}
}

func TestLogReport(t *testing.T) {
	t.Parallel()

	kotatsuLine := logLine{Time: 1_794_491_737_330, Level: "WARN", Inferred: false, Text: "webhook failed token=[REDACTED_SECRET]"}
	tests := []struct {
		name string
		// noLogs は、ログを読む部品を組まない (graph.Options.Logs が nil)。
		noLogs bool
		// member は、管理者でないユーザーで問い合わせる。
		member bool
		// tunerLog は、チューナーサーバーの /api/log の答え方。nil ならチューナーサーバーは無い。
		tunerLog  http.HandlerFunc
		timeout   time.Duration
		wantCodes []string
		// wantTuner は、チューナーサーバーの出どころ。nil なら、出どころは Kotatsu だけ。
		wantTuner *logSource
	}{
		{name: "管理者でなければ読めない", member: true, wantCodes: []string{codeForbidden}},
		{name: "ログを読む部品が無ければ内部のエラーにする", noLogs: true, wantCodes: []string{codeInternal}},
		{name: "チューナーサーバーが無ければ、Kotatsu だけ", wantCodes: []string{}},
		{
			name: "チューナーサーバーのログを、秘密を伏せて返す",
			tunerLog: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("2026-11-12T00:00:00.000Z error: token webhook-secret-value\n"))
			},
			wantCodes: []string{},
			wantTuner: tunerSource(1, nil, []logLine{{Time: 1_794_441_600_000, Level: "ERROR", Inferred: false, Text: "token [REDACTED_SECRET]"}}),
		},
		{
			name:      "ログの API が無ければ UNSUPPORTED",
			tunerLog:  func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) },
			wantCodes: []string{},
			wantTuner: tunerSource(0, new("UNSUPPORTED"), []logLine{}),
		},
		{
			name:      "待ち時間を過ぎたら TIMEOUT",
			tunerLog:  func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() },
			timeout:   100 * time.Millisecond,
			wantCodes: []string{},
			wantTuner: tunerSource(0, new("TIMEOUT"), []logLine{}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			buffer := logring.NewBuffer(10, 0)
			buffer.Add(logring.Line{Time: kotatsuLine.Time, Level: logring.LevelWarn, Text: "webhook failed token=webhook-secret-value"})
			opts := &graph.Options{}
			if !tt.noLogs {
				opts.Logs = logreport.New(&logreport.Options{
					Buffer: buffer, Tuner: newLogTuner(t, tt.tunerLog), Hosts: []string{"http://tuner.home.arpa:40772/"},
					InstanceID: "instance-1", Roles: []string{"tenban"},
					Secrets: func(context.Context) ([]string, error) { return []string{"webhook-secret-value"}, nil },
					Timeout: tt.timeout, Limit: 0, MaxLineLength: 0, Location: nil,
				})
			}
			f := newFixture(t, opts)
			client := f
			if !tt.member {
				client = f.as(t, adminName, domain.RoleAdmin)
			}

			var got logReportResult
			require.Equal(t, tt.wantCodes, client.query(t, logReportQuery, nil, &got))
			if len(tt.wantCodes) > 0 {
				return
			}
			report := got.LogReport
			wantSources := 1
			if tt.wantTuner != nil {
				wantSources = 2
			}
			require.Len(t, report.Sources, wantSources)
			kotatsu := report.Sources[0]
			assert.Equal(t, "KOTATSU", kotatsu.Kind)
			assert.Equal(t, 1, kotatsu.Total)
			assert.Nil(t, kotatsu.Failure)
			assert.NotEmpty(t, kotatsu.OS)
			assert.NotEmpty(t, kotatsu.Runtime)
			assert.Equal(t, []logLine{kotatsuLine}, kotatsu.Lines)
			if tt.wantTuner != nil {
				assert.Equal(t, *tt.wantTuner, report.Sources[1])
			}
			assert.Contains(t, report.RedactHosts, "tuner.home.arpa")
			assert.Equal(t, "instance-1", report.InstanceID)
			assert.Equal(t, []string{"tenban"}, report.Roles)
		})
	}
}
