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

	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

// readAll は、エラーが出るまでイベントを読み、読めたイベントと最後のエラーを返す。
func readAll(next func() (*mirakurun.Event, error)) ([]*mirakurun.Event, error) {
	events := []*mirakurun.Event{}
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
	stream, err := client.OpenEvents(t.Context(), &mirakurun.EventsQuery{Resource: resourceProgram, Type: ""})
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Close() })
	assert.Equal(t, resourceProgram, rec.last("/api/events/stream").URL.Query().Get("resource"))

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
	events := make(chan *mirakurun.Event, 1)
	go func() {
		event, err := stream.Next()
		if err == nil {
			events <- event
		}
	}()
	var event *mirakurun.Event
	require.Eventually(t, func() bool {
		fake.AddProgram(program)
		select {
		case event = <-events:
			return true
		case <-time.After(50 * time.Millisecond):
			return false
		}
	}, 5*time.Second, 10*time.Millisecond)

	assert.Equal(t, resourceProgram, event.Resource)
	assert.Equal(t, "create", event.Type)
	var data mirakurun.Program
	require.NoError(t, json.Unmarshal(event.Data, &data))
	assert.Equal(t, "追加した番組", data.Name)
	assert.Equal(t, program.ID, data.ID)
}
