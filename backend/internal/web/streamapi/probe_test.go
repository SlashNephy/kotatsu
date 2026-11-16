package streamapi_test

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/streamapi"
)

const (
	// probeUser と probeOther は、試しのデータを求めるユーザーの ID。
	probeUser  = "0199d000-0000-7000-8000-0000000000c1"
	probeOther = "0199d000-0000-7000-8000-0000000000c2"
)

// probeRequest は、user (空ならログインしていない) で、試しのデータを query で求める。
func probeRequest(t *testing.T, handler http.Handler, user, query string) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, streamapi.ProbePath+query, nil)
	if user != "" {
		r = r.WithContext(auth.WithPrincipal(r.Context(), &auth.Principal{UserID: uuid.MustParse(user)}))
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	return w
}

// TestProbeHandlerSize は、求めた量だけ、キャッシュさせない乱数を返し、範囲の外の量は断ることを確かめる (設計書 7.3、#1513)。
func TestProbeHandlerSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantBytes  int
	}{
		{name: "下限", query: "?bytes=" + strconv.Itoa(streamapi.ProbeMinBytes), wantStatus: http.StatusOK, wantBytes: streamapi.ProbeMinBytes},
		{name: "使い回す乱数より多い", query: "?bytes=1000000", wantStatus: http.StatusOK, wantBytes: 1_000_000},
		{name: "上限", query: "?bytes=" + strconv.Itoa(streamapi.ProbeMaxBytes), wantStatus: http.StatusOK, wantBytes: streamapi.ProbeMaxBytes},
		{name: "下限より少ない", query: "?bytes=" + strconv.Itoa(streamapi.ProbeMinBytes-1), wantStatus: http.StatusBadRequest, wantBytes: 0},
		{name: "上限より多い", query: "?bytes=" + strconv.Itoa(streamapi.ProbeMaxBytes+1), wantStatus: http.StatusBadRequest, wantBytes: 0},
		{name: "量が無い", query: "", wantStatus: http.StatusBadRequest, wantBytes: 0},
		{name: "数でない", query: "?bytes=abc", wantStatus: http.StatusBadRequest, wantBytes: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := streamapi.NewProbeHandler(clocktest.NewFake(time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)))
			w := probeRequest(t, handler, probeUser, tt.query)
			require.Equal(t, tt.wantStatus, w.Code, w.Body.String())
			if tt.wantStatus != http.StatusOK {
				return
			}
			assert.Equal(t, tt.wantBytes, w.Body.Len())
			assert.Equal(t, strconv.Itoa(tt.wantBytes), w.Header().Get("Content-Length"))
			assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
			// 前段で圧縮されて速さを測り違えないよう、圧縮しても縮まない中身にする。
			var compressed bytes.Buffer
			gz := gzip.NewWriter(&compressed)
			_, err := gz.Write(w.Body.Bytes())
			require.NoError(t, err)
			require.NoError(t, gz.Close())
			assert.Greater(t, compressed.Len(), tt.wantBytes*99/100)
		})
	}
}

// TestProbeHandlerInterval は、同じユーザーには試しの間隔の制限に 1 回までしか返さず、超えたら 429 と Retry-After を返すことを確かめる。
func TestProbeHandlerInterval(t *testing.T) {
	t.Parallel()

	// step は、時計を advance だけ進めてから、user で求めること。query が空なら、下限の量を求める。
	type step struct {
		advance        time.Duration
		user           string
		query          string
		wantStatus     int
		wantRetryAfter string
	}
	query := "?bytes=" + strconv.Itoa(streamapi.ProbeMinBytes)
	tests := []struct {
		name  string
		steps []step
	}{
		{
			name: "間隔の内は断り、残りの秒を切り上げて返す",
			steps: []step{
				{advance: 0, user: probeUser, wantStatus: http.StatusOK, wantRetryAfter: ""},
				{advance: 1500 * time.Millisecond, user: probeUser, wantStatus: http.StatusTooManyRequests, wantRetryAfter: "4"},
				{advance: 3400 * time.Millisecond, user: probeUser, wantStatus: http.StatusTooManyRequests, wantRetryAfter: "1"},
			},
		},
		{
			name: "間隔がたてば、また返す",
			steps: []step{
				{advance: 0, user: probeUser, wantStatus: http.StatusOK, wantRetryAfter: ""},
				{advance: streamapi.ProbeInterval, user: probeUser, wantStatus: http.StatusOK, wantRetryAfter: ""},
			},
		},
		{
			name: "断ったリクエストは、間隔を延ばさない",
			steps: []step{
				{advance: 0, user: probeUser, wantStatus: http.StatusOK, wantRetryAfter: ""},
				{advance: 4 * time.Second, user: probeUser, wantStatus: http.StatusTooManyRequests, wantRetryAfter: "1"},
				{advance: time.Second, user: probeUser, wantStatus: http.StatusOK, wantRetryAfter: ""},
			},
		},
		{
			name: "ユーザーごとに数える",
			steps: []step{
				{advance: 0, user: probeUser, wantStatus: http.StatusOK, wantRetryAfter: ""},
				{advance: 0, user: probeOther, wantStatus: http.StatusOK, wantRetryAfter: ""},
				{advance: time.Second, user: probeOther, wantStatus: http.StatusTooManyRequests, wantRetryAfter: "4"},
			},
		},
		{
			name: "量が範囲の外のリクエストは数えない",
			steps: []step{
				{advance: 0, user: probeUser, query: "?bytes=1", wantStatus: http.StatusBadRequest, wantRetryAfter: ""},
				{advance: 0, user: probeUser, wantStatus: http.StatusOK, wantRetryAfter: ""},
			},
		},
		{
			name: "ログインしていなければ断る",
			steps: []step{
				{advance: 0, user: "", wantStatus: http.StatusUnauthorized, wantRetryAfter: ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clk := clocktest.NewFake(time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC))
			handler := streamapi.NewProbeHandler(clk)
			for i, s := range tt.steps {
				clk.Advance(s.advance)
				q := query
				if s.query != "" {
					q = s.query
				}
				w := probeRequest(t, handler, s.user, q)
				assert.Equal(t, s.wantStatus, w.Code, "step %d", i)
				assert.Equal(t, s.wantRetryAfter, w.Header().Get("Retry-After"), "step %d", i)
				if s.wantStatus == http.StatusTooManyRequests {
					assert.Equal(t, "no-store", w.Header().Get("Cache-Control"), "step %d", i)
				}
			}
		})
	}
}
