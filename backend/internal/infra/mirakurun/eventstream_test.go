package mirakurun_test

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
)

// readAll は、エラーが出るまでイベントを読み、読めたイベントと最後のエラーを返す。
func readAll(next func() (*mirakurun.RawEvent, error)) ([]*mirakurun.RawEvent, error) {
	events := []*mirakurun.RawEvent{}
	for {
		event, err := next()
		if err != nil {
			return events, err
		}
		events = append(events, event)
	}
}

func TestEventReader(t *testing.T) {
	t.Parallel()

	const (
		created = `{"resource":"program","type":"create","data":{"id":1},"time":1791547800000}`
		removed = `{"resource":"program","type":"remove","data":{"id":2},"time":1791547801000}`
	)

	tests := []struct {
		name          string
		input         string
		wantResources []string
		wantErr       error
	}{
		{
			name:          "Mirakurun closes after a separator",
			input:         "[\n" + created + "\n,\n" + removed + "\n,\n",
			wantResources: []string{resourceProgram, resourceProgram},
			wantErr:       io.EOF,
		},
		{
			name:          "braces and escaped quotes in strings",
			input:         "[\n" + `{"resource":"program","type":"update","data":{"name":"}{[\\\"]"},"time":0}` + "\n,\n" + created + "\n,\n",
			wantResources: []string{resourceProgram, resourceProgram},
			wantErr:       io.EOF,
		},
		{
			name:          "no events yet",
			input:         "[\n",
			wantResources: []string{},
			wantErr:       io.EOF,
		},
		{
			name:          "closed array",
			input:         "[" + created + "]",
			wantResources: []string{resourceProgram},
			wantErr:       io.EOF,
		},
		{
			name:          "cut in the middle of an event",
			input:         "[\n" + created + "\n,\n" + removed[:20],
			wantResources: []string{resourceProgram},
			wantErr:       io.ErrUnexpectedEOF,
		},
		{
			name:          "cut before the first separator",
			input:         "[\n" + created,
			wantResources: []string{resourceProgram},
			wantErr:       io.EOF,
		},
		{
			name:          "empty input",
			input:         "",
			wantResources: []string{},
			wantErr:       io.EOF,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			events, err := readAll(mirakurun.NewEventReader(strings.NewReader(tt.input)).Next)
			require.ErrorIs(t, err, tt.wantErr)
			resources := []string{}
			for _, e := range events {
				resources = append(resources, e.Resource)
			}
			assert.Equal(t, tt.wantResources, resources)
		})
	}

	t.Run("fields", func(t *testing.T) {
		t.Parallel()

		event, err := mirakurun.NewEventReader(strings.NewReader("[\n" + removed + "\n,\n")).Next()
		require.NoError(t, err)
		assert.Equal(t, resourceProgram, event.Resource)
		assert.Equal(t, "remove", event.Type)
		assert.JSONEq(t, `{"id":2}`, string(event.Data))
		assert.Equal(t, time.UnixMilli(1791547801000).UTC(), event.Time)
	})

	t.Run("not an array", func(t *testing.T) {
		t.Parallel()

		_, err := mirakurun.NewEventReader(strings.NewReader(`{"resource":"program"}`)).Next()
		require.Error(t, err)
		require.NotErrorIs(t, err, io.EOF)
	})
}

// eventGenerator は、Mirakurun の形式で n 件のイベントを生成する io.Reader。全体を一度にメモリに置かない。
type eventGenerator struct {
	n, sent int
	buf     []byte
}

func (g *eventGenerator) Read(p []byte) (int, error) {
	for len(g.buf) == 0 {
		switch {
		case g.sent == 0 && g.n >= 0 && g.buf == nil:
			g.buf = []byte("[\n")
		case g.sent < g.n:
			g.sent++
			g.buf = fmt.Appendf(nil, `{"resource":"program","type":"update","data":{"id":%d,"name":"%s"},"time":%d}`+"\n,\n",
				g.sent, strings.Repeat("番組", 50), int64(g.sent))
		default:
			return 0, io.EOF
		}
	}
	n := copy(p, g.buf)
	g.buf = g.buf[n:]

	return n, nil
}

func TestEventReaderLongStream(t *testing.T) {
	t.Parallel()

	// 1 件 400 バイトほどのイベントを 5 万件 (20 MB ほど) 流す。
	const n = 50_000
	reader := mirakurun.NewEventReader(&eventGenerator{n: n})

	count := 0
	for {
		event, err := reader.Next()
		if err != nil {
			require.ErrorIs(t, err, io.EOF)

			break
		}
		count++
		var data struct {
			ID int `json:"id"`
		}
		require.NoError(t, json.Unmarshal(event.Data, &data))
		require.Equal(t, count, data.ID)
	}
	assert.Equal(t, n, count)
}

func TestOpenEventsMirakurun(t *testing.T) {
	t.Parallel()

	fake, rec, client := newFakeTuner(t)
	stream, err := client.OpenEvents(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Close() })
	require.NotNil(t, rec.last("/api/events/stream"))

	program := &faketuner.Program{
		ID:        faketuner.ProgramID(0x7FE0, 0x0400, 100),
		EventID:   100,
		ServiceID: 0x0400,
		NetworkID: 0x7FE0,
		StartAt:   testNow().Add(time.Hour).UnixMilli(),
		Duration:  (30 * time.Minute).Milliseconds(),
		IsFree:    true,
		Name:      "追加した番組",
	}
	// 購読の登録より前に送ったイベントは届かないので、届くまで送り直す。
	events := make(chan *tunerserver.Event, 1)
	go func() {
		event, err := stream.Next()
		if err == nil {
			events <- event
		}
	}()
	var event *tunerserver.Event
	require.Eventually(t, func() bool {
		fake.AddProgram(program)
		select {
		case event = <-events:
			return true
		case <-time.After(50 * time.Millisecond):
			return false
		}
	}, 5*time.Second, 10*time.Millisecond)

	// program.create は、番組が作られたことと、番組のサービスに読み替える。
	assert.Equal(t, &tunerserver.Event{Type: tunerserver.EventProgramCreated, ServiceID: faketuner.ServiceItemID(0x7FE0, 0x0400)}, event)
}

// TestToEvent は、Mirakurun、Mahiron と mirakc のイベントを、何が変わったかの知らせに読み替えることを確かめる。
func TestToEvent(t *testing.T) {
	t.Parallel()

	const service = 32736*100_000 + 1024
	tests := []struct {
		name   string
		raw    *mirakurun.RawEvent
		want   *tunerserver.Event
		wantOK bool
	}{
		{
			name:   "program created on Mirakurun",
			raw:    &mirakurun.RawEvent{Resource: resourceProgram, Type: "create", Data: json.RawMessage(`{"id":327360102400005,"networkId":32736,"serviceId":1024,"eventId":5}`)},
			want:   &tunerserver.Event{Type: tunerserver.EventProgramCreated, ServiceID: service},
			wantOK: true,
		},
		{
			name:   "program removed on Mirakurun has only its id",
			raw:    &mirakurun.RawEvent{Resource: resourceProgram, Type: "remove", Data: json.RawMessage(`{"id":327360102400005}`)},
			want:   &tunerserver.Event{Type: tunerserver.EventProgramRemoved, ServiceID: service},
			wantOK: true,
		},
		{
			name:   "service number without network falls back to the program id",
			raw:    &mirakurun.RawEvent{Resource: resourceProgram, Type: typeUpdate, Data: json.RawMessage(`{"id":327360102400005,"serviceId":1024}`)},
			want:   &tunerserver.Event{Type: tunerserver.EventProgramUpdated, ServiceID: service},
			wantOK: true,
		},
		{
			name:   "programs updated on mirakc",
			raw:    &mirakurun.RawEvent{Resource: resourceEPG, Type: typeProgramsUpdated, Data: json.RawMessage(`{"serviceId":3273601024}`)},
			want:   &tunerserver.Event{Type: tunerserver.EventProgramUpdated, ServiceID: service},
			wantOK: true,
		},
		{
			name:   "on-air program changed on mirakc",
			raw:    &mirakurun.RawEvent{Resource: "onair", Type: "program-changed", Data: json.RawMessage(`{"serviceId":3273601024}`)},
			want:   &tunerserver.Event{Type: tunerserver.EventProgramUpdated, ServiceID: service},
			wantOK: true,
		},
		{
			name:   "service changed",
			raw:    &mirakurun.RawEvent{Resource: "service", Type: typeUpdate, Data: json.RawMessage(`{"id":3273601024}`)},
			want:   &tunerserver.Event{Type: tunerserver.EventServiceChanged, ServiceID: 0},
			wantOK: true,
		},
		{
			name:   "tuner status changed on mirakc",
			raw:    &mirakurun.RawEvent{Resource: resourceTuner, Type: typeStatusChanged, Data: json.RawMessage(`{"tunerIndex":0}`)},
			want:   &tunerserver.Event{Type: tunerserver.EventTunerChanged, ServiceID: 0},
			wantOK: true,
		},
		{
			name:   "broken program data is ignored",
			raw:    &mirakurun.RawEvent{Resource: resourceProgram, Type: "create", Data: json.RawMessage(`[`)},
			want:   nil,
			wantOK: false,
		},
		{
			name:   "program without service is ignored",
			raw:    &mirakurun.RawEvent{Resource: resourceProgram, Type: typeUpdate, Data: json.RawMessage(`{"serviceId":1024}`)},
			want:   nil,
			wantOK: false,
		},
		{
			name:   "unused events are ignored",
			raw:    &mirakurun.RawEvent{Resource: "recording", Type: "started", Data: json.RawMessage(`{}`)},
			want:   nil,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			event, ok := mirakurun.ToEvent(tt.raw)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, event)
		})
	}
}
