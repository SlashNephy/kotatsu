package password_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/password"
)

// hibpServer は、Pwned Passwords の範囲検索を真似る。breached の SHA-1 の後ろ 35 文字を返し、水増しの行も混ぜる。
func hibpServer(t *testing.T, status int, breachedSuffix string) (*httptest.Server, *[]*http.Request) {
	t.Helper()

	requests := []*http.Request{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r)
		w.WriteHeader(status)
		_, _ = w.Write([]byte("0018A45C4D1DEF81644B54AB7F969B88D65:0\r\n" + breachedSuffix + ":42\r\n"))
	}))
	t.Cleanup(server.Close)

	return server, &requests
}

func TestPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		phrase     string
		userInputs []string
		hibpStatus int
		want       password.Reason
	}{
		{name: "strong password", phrase: strongPhrase, hibpStatus: 0},
		{name: "too short", phrase: "mikan77", hibpStatus: 0, want: password.ReasonTooShort},
		{name: "8 characters are enough when strong", phrase: "Ko7!q#Zx", hibpStatus: 0},
		{name: "too long", phrase: strings.Repeat("こ", password.MaxLength+1), hibpStatus: 0, want: password.ReasonTooLong},
		{name: "blocklisted regardless of case", phrase: "PassWord1", hibpStatus: 0, want: password.ReasonBlocklisted},
		{name: "weak", phrase: "kotatsukotatsu", hibpStatus: 0, want: password.ReasonTooWeak},
		{name: "contains the user name", phrase: "spica1234", userInputs: []string{"spica"}, hibpStatus: 0, want: password.ReasonTooWeak},
		{name: "breached", phrase: strongPhrase, hibpStatus: http.StatusOK, want: password.ReasonBreached},
		{name: "unavailable breach check does not block", phrase: strongPhrase, hibpStatus: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var hibp *password.HIBP
			if tt.hibpStatus != 0 {
				server, _ := hibpServer(t, tt.hibpStatus, sha1Suffix(strongPhrase))
				hibp = password.NewHIBP(server.URL)
			}
			policy, err := password.NewPolicy(&password.PolicyOptions{HIBP: hibp, StrengthTimeout: testStrengthTimeout, Logger: slog.New(slog.DiscardHandler)})
			require.NoError(t, err)

			err = policy.Check(t.Context(), tt.phrase, tt.userInputs)
			if tt.want == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorIs(t, err, password.ErrWeakPassword)
			rejected, ok := errors.AsType[*password.RejectedError](err)
			require.True(t, ok)
			assert.Equal(t, tt.want, rejected.Reason)
			if tt.want == password.ReasonTooWeak {
				assert.Less(t, rejected.Strength.Score, password.MinScore)
			}
		})
	}
}

func TestHIBPSendsOnlyThePrefix(t *testing.T) {
	t.Parallel()

	server, requests := hibpServer(t, http.StatusOK, "0000000000000000000000000000000000A")
	breached, err := password.NewHIBP(server.URL).Breached(context.Background(), strongPhrase)
	require.NoError(t, err)
	assert.False(t, breached)
	require.Len(t, *requests, 1)
	r := (*requests)[0]
	assert.Equal(t, "/range/"+sha1Prefix(strongPhrase), r.URL.Path)
	assert.Equal(t, "true", r.Header.Get("Add-Padding"))
}
