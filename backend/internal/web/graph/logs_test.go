package graph_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/logring"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/logreport"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const logReportQuery = `query {
  logReport {
    sources { kind total failure version os runtime lines { time level inferred text } }
    redactHosts
    instanceId
    roles
  }
}`

func TestLogReport(t *testing.T) {
	t.Parallel()

	buffer := logring.NewBuffer(10, 0)
	buffer.Add(logring.Line{Time: 1_794_491_737_330, Level: logring.LevelWarn, Text: "webhook failed token=webhook-secret-value"})
	logs := logreport.New(&logreport.Options{
		Buffer: buffer, Tuner: nil, Hosts: []string{"http://tuner.home.arpa:40772/"}, InstanceID: "instance-1", Roles: []string{"tenban"},
		Secrets: func(context.Context) ([]string, error) { return []string{"webhook-secret-value"}, nil },
		Timeout: 0, Limit: 0, MaxLineLength: 0, Location: nil,
	})
	f := newFixture(t, &graph.Options{Logs: logs})
	admin := f.as(t, adminName, auth.RoleAdmin)

	assert.Equal(t, []string{codeForbidden}, f.query(t, logReportQuery, nil, nil), "members cannot read the logs")

	type line struct {
		Time     int64  `json:"time"`
		Level    string `json:"level"`
		Inferred bool   `json:"inferred"`
		Text     string `json:"text"`
	}
	var got struct {
		LogReport struct {
			Sources []struct {
				Kind    string  `json:"kind"`
				Total   int     `json:"total"`
				Failure *string `json:"failure"`
				Version string  `json:"version"`
				OS      string  `json:"os"`
				Runtime string  `json:"runtime"`
				Lines   []line  `json:"lines"`
			} `json:"sources"`
			RedactHosts []string `json:"redactHosts"`
			InstanceID  string   `json:"instanceId"`
			Roles       []string `json:"roles"`
		} `json:"logReport"`
	}
	require.Empty(t, admin.query(t, logReportQuery, nil, &got))
	report := got.LogReport
	require.Len(t, report.Sources, 1, "no tuner server is configured")
	source := report.Sources[0]
	assert.Equal(t, "KOTATSU", source.Kind)
	assert.Equal(t, 1, source.Total)
	assert.Nil(t, source.Failure)
	assert.NotEmpty(t, source.OS)
	assert.NotEmpty(t, source.Runtime)
	assert.Equal(t, []line{{Time: 1_794_491_737_330, Level: "WARN", Inferred: false, Text: "webhook failed token=[REDACTED_SECRET]"}}, source.Lines)
	assert.Contains(t, report.RedactHosts, "tuner.home.arpa")
	assert.Equal(t, "instance-1", report.InstanceID)
	assert.Equal(t, []string{"tenban"}, report.Roles)
}
