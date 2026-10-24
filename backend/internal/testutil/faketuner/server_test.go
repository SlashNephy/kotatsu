package faketuner_test

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
)

// testNow は、テストで使う現在時刻。
func testNow() time.Time {
	return time.Date(2026, 10, 9, 21, 10, 0, 0, time.UTC)
}

func TestEndpoints(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(testNow(), nil)))
	t.Cleanup(server.Close)

	t.Run("version", func(t *testing.T) {
		t.Parallel()

		var version faketuner.Version
		getJSON(t, server.URL+"/api/version", &version)
		assert.NotEmpty(t, version.Current)
	})

	t.Run("channels", func(t *testing.T) {
		t.Parallel()

		var channels []*faketuner.Channel
		getJSON(t, server.URL+"/api/channels", &channels)
		require.Len(t, channels, 4)
		assert.Equal(t, "GR", channels[0].Type)
		assert.Equal(t, "27", channels[0].Channel)
		assert.Len(t, channels[0].Services, 2)
	})

	t.Run("services", func(t *testing.T) {
		t.Parallel()

		var services []*faketuner.Service
		getJSON(t, server.URL+"/api/services", &services)
		require.Len(t, services, 5)
		assert.Equal(t, int64(3273601024), services[0].ID, "Mirakurun service ID is networkId * 100000 + serviceId")
		assert.Equal(t, &faketuner.ChannelRef{Type: "GR", Channel: "27"}, services[0].Channel)
	})

	t.Run("programs", func(t *testing.T) {
		t.Parallel()

		var programs []*faketuner.Program
		getJSON(t, server.URL+"/api/programs?networkId=32736&serviceId=1024", &programs)
		require.NotEmpty(t, programs)
		for _, p := range programs {
			assert.Equal(t, 1024, p.ServiceID)
			assert.Equal(t, faketuner.ProgramID(p.NetworkID, p.ServiceID, p.EventID), p.ID)
		}
		// 今の時刻を含む番組がある。
		assert.True(t, slices.ContainsFunc(programs, func(p *faketuner.Program) bool {
			return p.StartAt <= testNow().UnixMilli() && testNow().UnixMilli() < p.StartAt+p.Duration
		}))

		var one []*faketuner.Program
		getJSON(t, server.URL+"/api/programs?networkId=32736&serviceId=1024&eventId=3", &one)
		require.Len(t, one, 1)
		assert.Equal(t, int64(3273601024_00003), one[0].ID)
	})

	t.Run("tuners", func(t *testing.T) {
		t.Parallel()

		var tuners []*faketuner.Tuner
		getJSON(t, server.URL+"/api/tuners", &tuners)
		require.Len(t, tuners, 3)
		assert.Equal(t, []string{"GR"}, tuners[0].Types)
		assert.True(t, tuners[0].IsFree)
	})
}

func TestEventsStream(t *testing.T) {
	t.Parallel()

	fake := faketuner.New(faketuner.DefaultOptions(testNow(), nil))
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	// tuner のイベントを除き、番組のイベントだけを受け取る。
	events := subscribe(t, server.URL+"/api/events/stream?resource=program")

	added := &faketuner.Program{
		ID:        faketuner.ProgramID(32736, 1024, 100),
		EventID:   100,
		ServiceID: 1024,
		NetworkID: 32736,
		StartAt:   testNow().Add(time.Hour).UnixMilli(),
		Duration:  (30 * time.Minute).Milliseconds(),
		IsFree:    true,
		Name:      "追加した番組",
	}
	fake.AddProgram(added)
	require.NoError(t, fake.RescheduleProgram(added.ID, testNow().Add(2*time.Hour), time.Hour))
	require.NoError(t, fake.ExtendProgram(added.ID, 10*time.Minute))
	require.NoError(t, fake.RemoveProgram(added.ID))
	require.Error(t, fake.RemoveProgram(added.ID), "removing an unknown program must fail")

	tests := []struct {
		name      string
		eventType string
		check     func(t *testing.T, data json.RawMessage)
	}{
		{
			name:      "add",
			eventType: "create",
			check: func(t *testing.T, data json.RawMessage) {
				t.Helper()
				assert.Equal(t, added, decode[faketuner.Program](t, data))
			},
		},
		{
			name:      "reschedule",
			eventType: "update",
			check: func(t *testing.T, data json.RawMessage) {
				t.Helper()
				p := decode[faketuner.Program](t, data)
				assert.Equal(t, testNow().Add(2*time.Hour).UnixMilli(), p.StartAt)
				assert.Equal(t, time.Hour.Milliseconds(), p.Duration)
			},
		},
		{
			name:      "extend",
			eventType: "update",
			check: func(t *testing.T, data json.RawMessage) {
				t.Helper()
				assert.Equal(t, (70 * time.Minute).Milliseconds(), decode[faketuner.Program](t, data).Duration)
			},
		},
		{
			name:      "remove",
			eventType: "remove",
			check: func(t *testing.T, data json.RawMessage) {
				t.Helper()
				assert.JSONEq(t, `{"id":`+strconv.FormatInt(added.ID, 10)+`}`, string(data))
			},
		},
	}
	// イベントは送った順に届くので、サブテストは順に実行する。
	for _, tt := range tests {
		event := nextEvent(t, events)
		assert.Equal(t, "program", event.Resource, tt.name)
		assert.Equal(t, tt.eventType, event.Type, tt.name)
		assert.Positive(t, event.Time, tt.name)
		tt.check(t, event.Data)
	}
}

func TestEventsStreamFiltersByType(t *testing.T) {
	t.Parallel()

	fake := faketuner.New(faketuner.DefaultOptions(testNow(), nil))
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	events := subscribe(t, server.URL+"/api/events/stream?type=remove")
	id := faketuner.ProgramID(32736, 1024, 1)
	require.NoError(t, fake.ExtendProgram(id, time.Minute))
	require.NoError(t, fake.RemoveProgram(id))

	// update は届かず、remove だけが届く。
	event := nextEvent(t, events)
	assert.Equal(t, "remove", event.Type)
}

func TestBadRequests(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(testNow(), nil)))
	t.Cleanup(server.Close)

	tests := []struct {
		name     string
		path     string
		priority string
	}{
		{name: "invalid program filter", path: "/api/programs?serviceId=invalid"},
		{name: "invalid service id", path: "/api/services/invalid/stream"},
		{name: "invalid priority", path: "/api/services/3273601024/stream", priority: "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+tt.path, nil)
			require.NoError(t, err)
			if tt.priority != "" {
				req.Header.Set("X-Mirakurun-Priority", tt.priority)
			}
			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			assert.Equal(t, http.StatusBadRequest, res.StatusCode)
			var body struct {
				Code   int    `json:"code"`
				Reason string `json:"reason"`
			}
			require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
			assert.Equal(t, http.StatusBadRequest, body.Code)
			assert.NotEmpty(t, body.Reason)
		})
	}
}

func TestTunerContention(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// holder が先にチューナーを使い、requester が後から別のチャンネルを要求する。
		holderPriority    int
		requesterPriority int
		wantTakeover      bool
	}{
		{name: "recording is not taken over by viewing", holderPriority: 2, requesterPriority: 0, wantTakeover: false},
		{name: "viewing is taken over by recording", holderPriority: 0, requesterPriority: 2, wantTakeover: true},
		{name: "conflicting recording is taken over by recording", holderPriority: 1, requesterPriority: 2, wantTakeover: true},
		{name: "equal priority does not take over", holderPriority: 2, requesterPriority: 2, wantTakeover: false},
		{name: "negative priority never takes over", holderPriority: -2, requesterPriority: -1, wantTakeover: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake, server := singleTunerServer(t)
			holder := openStream(t, server.URL, serviceA, tt.holderPriority)
			require.Equal(t, http.StatusOK, holder.StatusCode)
			readTS(t, holder.Body)

			requester := openStream(t, server.URL, serviceB, tt.requesterPriority)
			if !tt.wantTakeover {
				assert.Equal(t, http.StatusServiceUnavailable, requester.StatusCode)
				readTS(t, holder.Body)
				assertTunerUsers(t, fake, tt.holderPriority)

				return
			}

			require.Equal(t, http.StatusOK, requester.StatusCode)
			readTS(t, requester.Body)
			// 奪われた側のストリームは終わる。
			_, err := io.Copy(io.Discard, holder.Body)
			require.NoError(t, err)
			assertTunerUsers(t, fake, tt.requesterPriority)
		})
	}
}

func TestSameChannelSharesTuner(t *testing.T) {
	t.Parallel()

	fake, server := singleTunerServer(t)
	first := openStream(t, server.URL, serviceA, 2)
	require.Equal(t, http.StatusOK, first.StatusCode)
	// 同じチャンネルの別のサービスは、優先度が低くても同じチューナーに相乗りする。
	second := openStream(t, server.URL, serviceA2, 0)
	require.Equal(t, http.StatusOK, second.StatusCode)
	readTS(t, first.Body)
	readTS(t, second.Body)

	assertTunerUsers(t, fake, 2, 0)

	// 全員が切断すると、チューナーは空く。
	first.Body.Close()
	second.Body.Close()
	require.Eventually(t, func() bool {
		return fake.Tuners()[0].IsFree
	}, 5*time.Second, 10*time.Millisecond)
}

func TestStreamErrors(t *testing.T) {
	t.Parallel()

	_, server := singleTunerServer(t)
	tests := []struct {
		name       string
		serviceID  int64
		wantStatus int
	}{
		{name: "unknown service", serviceID: 1, wantStatus: http.StatusNotFound},
		{name: "no tuner for the channel type", serviceID: serviceBS, wantStatus: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			res := openStream(t, server.URL, tt.serviceID, 0)
			assert.Equal(t, tt.wantStatus, res.StatusCode)
		})
	}
}

// 既定のチャンネルのサービス ID (networkId × 100000 + serviceId)。
const (
	// GR 27 の 2 つのサービス。
	serviceA  int64 = 32736_01024
	serviceA2 int64 = 32736_01025
	// GR 26 のサービス。
	serviceB int64 = 32737_01032
	// BS のサービス。
	serviceBS int64 = 4_00101
)

// singleTunerServer は、既定のチャンネルに地上波のチューナーを 1 本だけ持つ偽のチューナーサーバーを返す。
func singleTunerServer(t *testing.T) (*faketuner.Server, *httptest.Server) {
	t.Helper()

	opts := faketuner.DefaultOptions(testNow(), nil)
	opts.Tuners = []*faketuner.TunerConfig{{Name: "GR0", Types: []string{"GR"}}}
	fake := faketuner.New(opts)
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	return fake, server
}

// stream は、サービスのストリームの応答。本文はテストの終了時に閉じる。
type stream struct {
	StatusCode int
	Body       io.ReadCloser
}

func openStream(t *testing.T, baseURL string, serviceID int64, priority int) *stream {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/api/services/"+strconv.FormatInt(serviceID, 10)+"/stream?decode=1", nil)
	require.NoError(t, err)
	req.Header.Set("X-Mirakurun-Priority", strconv.Itoa(priority))
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })

	return &stream{StatusCode: res.StatusCode, Body: res.Body}
}

// readTS は、TS のパケットが届いていることを確かめる。
func readTS(t *testing.T, body io.Reader) {
	t.Helper()

	packet := make([]byte, 188)
	_, err := io.ReadFull(body, packet)
	require.NoError(t, err)
	assert.Equal(t, byte(0x47), packet[0], "a TS packet starts with the sync byte")
}

func assertTunerUsers(t *testing.T, fake *faketuner.Server, wantPriorities ...int) {
	t.Helper()

	tuner := fake.Tuners()[0]
	got := make([]int, 0, len(tuner.Users))
	for _, user := range tuner.Users {
		got = append(got, user.Priority)
	}
	assert.Equal(t, wantPriorities, got)
	assert.True(t, tuner.IsUsing)
}

type rawEvent struct {
	Resource string          `json:"resource"`
	Type     string          `json:"type"`
	Data     json.RawMessage `json:"data"`
	Time     int64           `json:"time"`
}

// subscribe はイベントストリームを購読し、Mirakurun の形式 ("[" の後、JSON と "," を改行で区切る) を読んで返す。
func subscribe(t *testing.T, url string) <-chan *rawEvent {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	require.Equal(t, http.StatusOK, res.StatusCode)

	reader := bufio.NewReader(res.Body)
	first, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "[\n", first)

	events := make(chan *rawEvent, 16)
	go func() {
		defer close(events)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSuffix(line, "\n")
			if line == "," {
				continue
			}
			var event rawEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				return
			}
			events <- &event
		}
	}()

	return events
}

func nextEvent(t *testing.T, events <-chan *rawEvent) *rawEvent {
	t.Helper()

	select {
	case event, ok := <-events:
		require.True(t, ok, "the event stream was closed")

		return event
	case <-time.After(5 * time.Second):
		t.Fatal("no event arrived")

		return nil
	}
}

func decode[T any](t *testing.T, data json.RawMessage) *T {
	t.Helper()

	var v T
	require.NoError(t, json.Unmarshal(data, &v))

	return &v
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "application/json; charset=utf-8", res.Header.Get("Content-Type"))
	require.NoError(t, json.NewDecoder(res.Body).Decode(v))
}
