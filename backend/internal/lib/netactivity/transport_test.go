package netactivity_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
)

// fakeStore は、書かれた集計を持つ Store。
type fakeStore struct {
	mu      sync.Mutex
	enabled bool
	err     error
	saved   []netactivity.Bucket
}

func (s *fakeStore) Enabled(context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.enabled, nil
}

func (s *fakeStore) Save(_ context.Context, buckets []netactivity.Bucket) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.saved = append(s.saved, buckets...)

	return nil
}

func enabledRecorder(t *testing.T, store *fakeStore, now time.Time) (*netactivity.Recorder, func()) {
	t.Helper()

	recorder := netactivity.NewRecorder(clocktest.NewFake(now))
	flush := func() { netactivity.Flush(t.Context(), recorder, store, slog.New(slog.DiscardHandler)) }
	flush() // 記録の有効を読ませる。

	return recorder, flush
}

// TestClientRecords は、通信の回数、送受信量、結果を、URL のパスとクエリを持たずに集計することを確かめる。
func TestClientRecords(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 9, 12, 34, 56, 0, time.UTC)
	tests := []struct {
		name        string
		status      int
		wantSuccess bool
	}{
		{name: "success", status: http.StatusOK, wantSuccess: true},
		{name: "client error", status: http.StatusNotFound, wantSuccess: false},
		{name: "server error", status: http.StatusBadGateway, wantSuccess: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte("0123456789"))
			}))
			t.Cleanup(server.Close)

			store := &fakeStore{mu: sync.Mutex{}, enabled: true, err: nil, saved: nil}
			recorder, flush := enabledRecorder(t, store, now)
			client := netactivity.NewClient(recorder, netactivity.PurposeWebPush, netactivity.ClientOptions{Transport: nil, Timeout: time.Second, CheckRedirect: nil})

			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/secret/path?token=abc", strings.NewReader("hello"))
			require.NoError(t, err)
			res, err := client.Do(req)
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, res.Body)
			require.NoError(t, err)
			require.NoError(t, res.Body.Close())
			flush()

			require.Len(t, store.saved, 1)
			got := store.saved[0]
			assert.Equal(t, now.Truncate(time.Hour), got.Hour)
			assert.Equal(t, netactivity.PurposeWebPush, got.Purpose)
			assert.Equal(t, "127.0.0.1", got.Host)
			assert.Equal(t, netactivity.InitiatorScheduled, got.Initiator)
			assert.Equal(t, tt.wantSuccess, got.Success)
			assert.Equal(t, netactivity.Totals{Count: 1, SentBytes: 5, ReceivedBytes: 10}, got.Totals)
		})
	}
}

// TestHostVisibility は、決まった接続先だけホスト名を残し、設定の中身が分かるものは残さないことを確かめる。
func TestHostVisibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		purpose netactivity.Purpose
		host    string
		want    string
	}{
		{name: "known host", purpose: netactivity.PurposeUpdateCheck, host: "api.github.com", want: "api.github.com"},
		{name: "redirected to another host", purpose: netactivity.PurposeHIBP, host: "hibp.internal.example", want: ""},
		{name: "broadcaster", purpose: netactivity.PurposeDataBroadcast, host: "bml.broadcaster.example", want: ""},
		{name: "webhook", purpose: netactivity.PurposeWebhook, host: "discord.com", want: ""},
		{name: "artwork reveals the work", purpose: netactivity.PurposeAnimeImage, host: "official-site.example", want: ""},
		{name: "push service", purpose: netactivity.PurposeWebPush, host: "fcm.googleapis.com", want: "fcm.googleapis.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeStore{mu: sync.Mutex{}, enabled: true, err: nil, saved: nil}
			recorder, flush := enabledRecorder(t, store, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC))
			recorder.Add(tt.purpose, tt.host, true, netactivity.Totals{Count: 1, SentBytes: 0, ReceivedBytes: 0})
			flush()

			require.Len(t, store.saved, 1)
			assert.Equal(t, tt.want, store.saved[0].Host)
		})
	}
}

// TestRecorderIsDisabledByDefault は、記録が無効の間は数えず、無効に戻したら残りを捨てることを確かめる。
func TestRecorderIsDisabledByDefault(t *testing.T) {
	t.Parallel()

	store := &fakeStore{mu: sync.Mutex{}, enabled: false, err: nil, saved: nil}
	recorder, flush := enabledRecorder(t, store, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC))
	recorder.Add(netactivity.PurposeHIBP, "api.pwnedpasswords.com", true, netactivity.Totals{Count: 1, SentBytes: 0, ReceivedBytes: 0})
	flush()
	assert.Empty(t, store.saved)

	store.enabled = true
	flush()
	recorder.Add(netactivity.PurposeHIBP, "api.pwnedpasswords.com", true, netactivity.Totals{Count: 1, SentBytes: 0, ReceivedBytes: 0})
	store.enabled = false
	flush()
	assert.Empty(t, store.saved, "無効に戻したら、それまでの集計を捨てる")
}

// TestFlushRetriesAfterFailure は、書けなかった集計を失わず、次に足し合わせて書くことを確かめる。
func TestFlushRetriesAfterFailure(t *testing.T) {
	t.Parallel()

	store := &fakeStore{mu: sync.Mutex{}, enabled: true, err: nil, saved: nil}
	recorder, flush := enabledRecorder(t, store, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC))
	add := func() {
		recorder.Add(netactivity.PurposeHIBP, "api.pwnedpasswords.com", true, netactivity.Totals{Count: 1, SentBytes: 1, ReceivedBytes: 2})
	}

	add()
	store.err = errors.New("database is down")
	flush()
	assert.Empty(t, store.saved)

	store.err = nil
	add()
	flush()
	require.Len(t, store.saved, 1)
	assert.Equal(t, netactivity.Totals{Count: 2, SentBytes: 2, ReceivedBytes: 4}, store.saved[0].Totals)
}

// TestWithPurposeOverrides は、1 つのクライアントで目的を切り替えられることを確かめる。
func TestWithPurposeOverrides(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(server.Close)

	store := &fakeStore{mu: sync.Mutex{}, enabled: true, err: nil, saved: nil}
	recorder, flush := enabledRecorder(t, store, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC))
	client := netactivity.NewClient(recorder, netactivity.PurposeJikkyoChannels, netactivity.ClientOptions{Transport: nil, Timeout: time.Second, CheckRedirect: nil})

	req, err := http.NewRequestWithContext(netactivity.WithPurpose(t.Context(), netactivity.PurposeJikkyoKakolog), http.MethodGet, server.URL, http.NoBody)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	flush()

	require.Len(t, store.saved, 1)
	assert.Equal(t, netactivity.PurposeJikkyoKakolog, store.saved[0].Purpose)
}
