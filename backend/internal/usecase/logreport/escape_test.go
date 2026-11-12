package logreport_test

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/logring"
	"github.com/SlashNephy/kotatsu/internal/usecase/logreport"
)

// leakedParts は、伏せ損ねたときに残る、秘密の一部 (そのまま、HTML のエスケープ、JSON のエスケープ)。
func leakedParts() []string { return []string{"ss<wo", `ss\u003cwo`, `pa\"ss`} }

// TestReportRedactsFormattedSecrets は、slog の JSON で整形 (エスケープ) された秘密と、
// 1 行の上限の切り口にまたがった秘密が、画面に送る行に残らないことを、本物の Handler と Buffer で確かめる。
func TestReportRedactsFormattedSecrets(t *testing.T) {
	t.Parallel()

	const tricky = `pa"ss<wo>&rd\x`
	const long = "s3cr3t-webhook-token-0123456789"
	tests := []struct {
		name          string
		maxLineLength int
		log           func(logger *slog.Logger)
		leaked        []string
	}{
		{
			name:          "JSON の文字列にエスケープされた秘密",
			maxLineLength: 0,
			log:           func(logger *slog.Logger) { logger.Info("login", slog.String("value", tricky)) },
			leaked:        leakedParts(),
		},
		{
			name:          "encoding/json で HTML もエスケープされた秘密",
			maxLineLength: 0,
			log: func(logger *slog.Logger) {
				logger.Info("login", slog.Any("value", struct{ V string }{V: tricky}))
			},
			leaked: leakedParts(),
		},
		{
			name:          "エラーの文言に入った秘密",
			maxLineLength: 0,
			log:           func(logger *slog.Logger) { logger.Error("failed", slog.Any("error", errors.New("bad token "+tricky))) },
			leaked:        leakedParts()[:1:1],
		},
		{
			name:          "1 行の上限の切り口にまたがった秘密",
			maxLineLength: 40,
			log:           func(logger *slog.Logger) { logger.Info("webhook", slog.String("url", "https://x.test/"+long)) },
			leaked:        []string{"s3cr3t-w"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			buffer := logring.NewBuffer(10, tt.maxLineLength)
			tt.log(slog.New(logring.New(slog.NewJSONHandler(&bytes.Buffer{}, nil), buffer)))
			service := logreport.New(&logreport.Options{
				Buffer: buffer, Tuner: nil, Hosts: nil, InstanceID: "", Roles: nil,
				Secrets: staticSecrets(tricky, long), Timeout: 0, Limit: 0, MaxLineLength: 0, Location: nil,
			})
			report, err := service.Report(t.Context(), "")
			require.NoError(t, err)
			require.Len(t, report.Sources[0].Lines, 1)
			text := report.Sources[0].Lines[0].Text
			assert.Contains(t, text, "[REDACTED_SECRET]")
			for _, part := range tt.leaked {
				assert.NotContains(t, text, part)
			}
		})
	}
}

// TestReportRedactsTunerLinesBeforeTruncating は、チューナーサーバーの行を、伏せてから切ることを確かめる。
func TestReportRedactsTunerLinesBeforeTruncating(t *testing.T) {
	t.Parallel()

	const long = "s3cr3t-webhook-token-0123456789"
	tuner := newTuner(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("2026-11-12T00:00:00.000Z info: url https://x.test/" + long + strings.Repeat(" tail", 20) + "\n"))
	})
	service := logreport.New(&logreport.Options{
		Buffer: logring.NewBuffer(1, 0), Tuner: tuner, Hosts: nil, InstanceID: "", Roles: nil,
		Secrets: staticSecrets(long), Timeout: 0, Limit: 0, MaxLineLength: 30, Location: nil,
	})
	report, err := service.Report(t.Context(), "")
	require.NoError(t, err)
	require.Len(t, report.Sources[1].Lines, 1)
	assert.NotContains(t, report.Sources[1].Lines[0].Text, "s3cr3t")
}
