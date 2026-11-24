package mirakurunapi_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/usecase/nabe"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
	"github.com/SlashNephy/kotatsu/internal/web/mirakurunapi"
)

// fakeSource は、決めた値を返すチューナーサーバー。
type fakeSource struct {
	channels []*tunerserver.Channel
	services []*tunerserver.Service
	programs []*tunerserver.Program
	tuners   []*tunerserver.Tuner
	// err は、一覧の API で返すエラー。
	err error
	// streams は、サービス ID ごとに開くストリーム。なければ tunerserver.ErrNotFound を返す。
	streams map[int64]func() (io.ReadCloser, error)
	logo    []byte
	events  []*tunerserver.Event

	mu    sync.Mutex
	query *tunerserver.ProgramsQuery
}

var _ tunerserver.Client = new(fakeSource)

func (f *fakeSource) Channels(context.Context) ([]*tunerserver.Channel, error) {
	return f.channels, f.err
}

func (f *fakeSource) Services(context.Context) ([]*tunerserver.Service, error) {
	return f.services, f.err
}

func (f *fakeSource) Programs(_ context.Context, query *tunerserver.ProgramsQuery) ([]*tunerserver.Program, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.query = query

	return f.programs, f.err
}

func (f *fakeSource) OnAirPrograms(context.Context) ([]*tunerserver.OnAirProgram, error) {
	return []*tunerserver.OnAirProgram{}, nil
}

func (f *fakeSource) Tuners(context.Context) ([]*tunerserver.Tuner, error) { return f.tuners, f.err }

func (f *fakeSource) ServerDate(context.Context) (time.Time, error) {
	return time.Time{}, tunerserver.ErrClockUnavailable
}

func (f *fakeSource) Identify(context.Context) (*tunerserver.Identity, error) {
	return &tunerserver.Identity{Kind: tunerserver.KindEdcb, Version: ""}, nil
}

func (f *fakeSource) OpenEvents(context.Context) (tunerserver.EventStream, error) {
	return &fakeEvents{events: f.events}, nil
}

func (f *fakeSource) OpenServiceStream(_ context.Context, id int64, _ *tunerserver.StreamOptions) (io.ReadCloser, error) {
	open, ok := f.streams[id]
	if !ok {
		return nil, tunerserver.ErrNotFound
	}

	return open()
}

func (f *fakeSource) OpenChannelStream(ctx context.Context, channelType, channel string, opts *tunerserver.StreamOptions) (io.ReadCloser, error) {
	if channelType == "GR" && channel == "32736" {
		return f.OpenServiceStream(ctx, 3273601024, opts)
	}

	return nil, tunerserver.ErrNotFound
}

func (f *fakeSource) ServiceLogo(_ context.Context, id int64) ([]byte, error) {
	if id != 3273601024 {
		return nil, tunerserver.ErrNotFound
	}
	if f.logo == nil {
		return nil, tunerserver.ErrLogoUnavailable
	}

	return f.logo, nil
}

// fakeEvents は、決めたイベントを返し、使い切ったら閉じるまで待つ。
type fakeEvents struct {
	events []*tunerserver.Event
	once   sync.Once
	closed chan struct{}
	mu     sync.Mutex
}

func (e *fakeEvents) Next() (*tunerserver.Event, error) {
	e.mu.Lock()
	e.once.Do(func() { e.closed = make(chan struct{}) })
	if len(e.events) > 0 {
		event := e.events[0]
		e.events = e.events[1:]
		e.mu.Unlock()

		return event, nil
	}
	closed := e.closed
	e.mu.Unlock()
	<-closed

	return nil, io.EOF
}

func (e *fakeEvents) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.once.Do(func() { e.closed = make(chan struct{}) })
	select {
	case <-e.closed:
	default:
		close(e.closed)
	}

	return nil
}

// errReader は、data を返した後に err を返すストリーム。
type errReader struct {
	data []byte
	err  error
}

func (r *errReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]

	return n, nil
}

func (r *errReader) Close() error { return nil }

func newClient(t *testing.T, source tunerserver.Client) *mirakurun.Client {
	t.Helper()

	server := httptest.NewServer(http.StripPrefix("/hub", mirakurunapi.NewServer(source, "2026.11.0", slog.New(slog.DiscardHandler))))
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL + "/hub/", HTTPClient: server.Client()})
	require.NoError(t, err)

	return client
}

func testSource() *fakeSource {
	ref := &tunerserver.ChannelRef{Type: "GR", Channel: "32736"}
	service := &tunerserver.Service{
		ID: 3273601024, ServiceID: 1024, NetworkID: 32736, Name: "NHK総合1・東京", Type: 1,
		LogoID: nil, RemoteControlKeyID: new(1), Channel: ref,
	}

	return &fakeSource{
		channels: []*tunerserver.Channel{{Type: "GR", Channel: "32736", Name: "NHK総合", Services: []*tunerserver.Service{service}}},
		services: []*tunerserver.Service{service},
		programs: []*tunerserver.Program{{
			ID: 327360102412345, EventID: 12345, ServiceID: 1024, NetworkID: 32736, StartAt: 1_800_000_000_000, Duration: 1_800_000,
			IsFree: true, Name: "ニュース", Description: "概要",
			Genres:       []*domain.Genre{{Lv1: 0, Lv2: 1, Un1: 15, Un2: 15}},
			Extended:     map[string]string{"出演者": "だれか"},
			RelatedItems: []*domain.RelatedItem{{Type: "shared", NetworkID: 0, ServiceID: 1025, EventID: 12345}},
			Audios:       []*domain.Audio{{ComponentType: 3, ComponentTag: new(16), IsMain: true, SamplingRate: 48000, Langs: []string{"jpn"}}},
			Video:        &tunerserver.Video{Resolution: "1080i"},
			Series:       &tunerserver.Series{ID: 1, Repeat: 0, Episode: 2, LastEpisode: 12, Name: "シリーズ"},
		}},
		tuners: []*tunerserver.Tuner{{
			Index: 1, Name: "BonDriver_LinuxMirakurun.so", Types: []string{"GR"}, Users: []*tunerserver.TunerUser{},
			IsAvailable: true, IsRemote: false, IsFree: false, IsUsing: true, IsFault: false,
		}},
		streams: map[int64]func() (io.ReadCloser, error){},
	}
}

func TestServerLists(t *testing.T) {
	t.Parallel()

	source := testSource()
	client := newClient(t, source)
	ctx := t.Context()

	services, err := client.Services(ctx)
	require.NoError(t, err)
	assert.Equal(t, source.services, services)
	channels, err := client.Channels(ctx)
	require.NoError(t, err)
	assert.Equal(t, source.channels, channels)
	tuners, err := client.Tuners(ctx)
	require.NoError(t, err)
	assert.Equal(t, source.tuners, tuners)
	version, err := client.Version(ctx)
	require.NoError(t, err)
	assert.Equal(t, "2026.11.0", version.Current)
	// /events を持たないので、Mirakurun として扱われ、/api/onair は読まない。
	server, err := client.Detect(ctx)
	require.NoError(t, err)
	assert.Equal(t, mirakurun.ServerKindMirakurun, server.Kind)
}

func TestServerPrograms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query *tunerserver.ProgramsQuery
	}{
		{name: "all", query: nil},
		{name: "service", query: &tunerserver.ProgramsQuery{NetworkID: 32736, ServiceID: 1024, EventID: 0}},
		{name: "event", query: &tunerserver.ProgramsQuery{NetworkID: 32736, ServiceID: 1024, EventID: 12345}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := testSource()
			programs, err := newClient(t, source).Programs(t.Context(), tt.query)
			require.NoError(t, err)
			assert.Equal(t, source.programs, programs)
			source.mu.Lock()
			defer source.mu.Unlock()
			assert.Equal(t, tt.query, source.query)
		})
	}
}

func TestServerListErrors(t *testing.T) {
	t.Parallel()

	// 一覧を読めなければ、空の一覧ではなくエラーで返す。受け手が DB のサービスを消さないようにする。
	source := testSource()
	source.err = errors.New("EpgTimerSrv is busy")
	_, err := newClient(t, source).Services(t.Context())
	var apiErr *mirakurun.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
}

func TestServerStream(t *testing.T) {
	t.Parallel()

	ts := bytes.Repeat([]byte{0x47, 0x01, 0x00, 0x10}, 47)
	tests := []struct {
		name string
		// open は、nabe の側でストリームを開く。nil ならサービスがない。
		open    func() (io.ReadCloser, error)
		channel bool
		wantErr error
		// wantEOF なら、受け手は読み終えて io.EOF を受け取る。
		wantEOF bool
	}{
		{name: "service", open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(ts)), nil }, wantEOF: true},
		{name: "channel", open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(ts)), nil }, channel: true, wantEOF: true},
		{name: "no tuner", open: func() (io.ReadCloser, error) { return nil, tunerserver.ErrTunerUnavailable }, wantErr: tunerserver.ErrTunerUnavailable},
		{name: "no service", open: nil, wantErr: tunerserver.ErrNotFound},
		// チューナーを取られたことは、トレーラーで伝わる。
		{name: "preempted", open: func() (io.ReadCloser, error) {
			return &errReader{data: ts, err: tunerserver.ErrTunerPreempted}, nil
		}, wantErr: tunerserver.ErrTunerPreempted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := testSource()
			if tt.open != nil {
				source.streams[3273601024] = tt.open
			}
			client := newClient(t, source)
			var stream io.ReadCloser
			var err error
			if tt.channel {
				stream, err = client.OpenChannelStream(t.Context(), "GR", "32736", nil)
			} else {
				stream, err = client.OpenServiceStream(t.Context(), 3273601024, nil)
			}
			if err != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			defer stream.Close()
			got, err := io.ReadAll(stream)
			assert.Equal(t, ts, got)
			if tt.wantEOF {
				require.NoError(t, err)

				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestServerStreamCutOff(t *testing.T) {
	t.Parallel()

	// チューナーを取られたほかの理由で切れたら、読み終えたのではなく、切れたと分かる。
	source := testSource()
	source.streams[3273601024] = func() (io.ReadCloser, error) {
		return &errReader{data: []byte{0x47}, err: tunerserver.ErrStreamStalled}, nil
	}
	stream, err := newClient(t, source).OpenServiceStream(t.Context(), 3273601024, nil)
	require.NoError(t, err)
	defer stream.Close()
	_, err = io.ReadAll(stream)
	require.Error(t, err)
}

func TestServerLogo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		logo    []byte
		id      int64
		wantErr error
	}{
		{name: "logo", logo: []byte("\x89PNG"), id: 3273601024},
		{name: "not received", logo: nil, id: 3273601024, wantErr: tunerserver.ErrLogoUnavailable},
		{name: "no service", logo: []byte("\x89PNG"), id: 1, wantErr: tunerserver.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := testSource()
			source.logo = tt.logo
			got, err := newClient(t, source).ServiceLogo(t.Context(), tt.id)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.logo, got)
		})
	}
}

func TestServerEvents(t *testing.T) {
	t.Parallel()

	source := testSource()
	source.events = []*tunerserver.Event{
		{Type: tunerserver.EventTypeServiceChanged, ServiceID: 0},
		{Type: tunerserver.EventTypeProgramCreated, ServiceID: 3273601024},
		{Type: tunerserver.EventTypeProgramUpdated, ServiceID: 3273601024},
		{Type: tunerserver.EventTypeProgramRemoved, ServiceID: 3273601024},
		{Type: tunerserver.EventTypeTunerChanged, ServiceID: 0},
	}
	events, err := newClient(t, source).OpenEvents(t.Context())
	require.NoError(t, err)
	defer events.Close()
	for _, want := range source.events {
		got, err := events.Next()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

// TestServerEventsSendsEPGChangedAsServiceEvent は、番組表の全体が変わったかもしれない知らせ (EDCB の EPG の読み込み直し) を、
// Mirakurun にない種類なので、サービスのイベントとして送ることを確認する (#2035)。
func TestServerEventsSendsEPGChangedAsServiceEvent(t *testing.T) {
	t.Parallel()

	source := testSource()
	source.events = []*tunerserver.Event{{Type: tunerserver.EventTypeEPGChanged, ServiceID: 0}}
	events, err := newClient(t, source).OpenEvents(t.Context())
	require.NoError(t, err)
	defer events.Close()
	got, err := events.Next()
	require.NoError(t, err)
	assert.Equal(t, &tunerserver.Event{Type: tunerserver.EventTypeServiceChanged, ServiceID: 0}, got)
}

// nabeVersionPath は、nabe の下の、バージョンを返すパス。
const nabeVersionPath = "/internal/nabe/api/version"

// TestNabeHandlerRoutes は、http.ServeMux で受けていたときと同じく、トークンをルートより先に確認し、
// 1 つの区切りにだけ当たる名前の部分でルートを選ぶことを確認する (#2016)。
func TestNabeHandlerRoutes(t *testing.T) {
	t.Parallel()

	token, err := nabe.NewToken("kotatsu-nabe-test-application-key-0123456789")
	require.NoError(t, err)
	handler := mirakurunapi.NabeHandler(testSource(), token, "2026.11.0", slog.New(slog.DiscardHandler))

	tests := []struct {
		name      string
		method    string
		path      string
		noToken   bool
		wantCode  int
		wantAllow string
	}{
		{name: "トークンがなければ、当たらないパスでも 401", method: http.MethodGet, path: "/internal/nabe/nope", noToken: true, wantCode: http.StatusUnauthorized},
		{name: "版を返す", method: http.MethodGet, path: nabeVersionPath, wantCode: http.StatusOK},
		{name: "HEAD でも版を返す", method: http.MethodHead, path: nabeVersionPath, wantCode: http.StatusOK},
		{name: "POST すると 405", method: http.MethodPost, path: nabeVersionPath, wantCode: http.StatusMethodNotAllowed, wantAllow: "GET, HEAD"},
		{name: "サービスの ID が読めなければ 400", method: http.MethodGet, path: "/internal/nabe/api/services/abc/logo", wantCode: http.StatusBadRequest},
		{name: "区切りが多いと当たらない", method: http.MethodGet, path: "/internal/nabe/api/services/1/2/logo", wantCode: http.StatusNotFound},
		{name: "知らないパスは 404", method: http.MethodGet, path: "/internal/nabe/api/nope", wantCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil)
			if !tt.noToken {
				token.Authorize(req)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Equal(t, tt.wantAllow, rec.Header().Get("Allow"))
		})
	}
}

// authorizing は、リクエストに nabe のトークンを付ける。
type authorizing struct {
	token interface{ Authorize(r *http.Request) }
}

func (a *authorizing) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	a.token.Authorize(r)

	return http.DefaultTransport.RoundTrip(r)
}

// TestNabeThroughAPIRouter は、API の待ち受けの一番外側のルーターに載せた nabe から、ストリームとイベントを
// 読み切れることを確認する (#2016)。どちらも、echo のルーターを 2 層通って、書くたびに届く。
func TestNabeThroughAPIRouter(t *testing.T) {
	t.Parallel()

	ts := bytes.Repeat([]byte{0x47, 0x01, 0x00, 0x10}, 47)
	source := testSource()
	source.streams[3273601024] = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(ts)), nil }
	source.events = []*tunerserver.Event{
		{Type: tunerserver.EventTypeServiceChanged, ServiceID: 0},
		{Type: tunerserver.EventTypeProgramUpdated, ServiceID: 3273601024},
	}
	token, err := nabe.NewToken("kotatsu-nabe-test-application-key-0123456789")
	require.NoError(t, err)
	logger := slog.New(slog.DiscardHandler)
	router := httpserver.NewAPIRouter(logger, nil, map[string]http.Handler{nabe.PathPrefix: mirakurunapi.NabeHandler(source, token, "2026.11.0", logger)}, nil)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL + nabe.PathPrefix, HTTPClient: &http.Client{Transport: &authorizing{token: token}}})
	require.NoError(t, err)

	stream, err := client.OpenServiceStream(t.Context(), 3273601024, nil)
	require.NoError(t, err)
	got, err := io.ReadAll(stream)
	require.NoError(t, err)
	require.NoError(t, stream.Close())
	assert.Equal(t, ts, got)

	events, err := client.OpenEvents(t.Context())
	require.NoError(t, err)
	defer events.Close()
	for _, want := range source.events {
		got, err := events.Next()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}
