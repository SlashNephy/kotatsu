package mirakurun_test

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

func TestSSEReader(t *testing.T) {
	t.Parallel()

	type event struct {
		Resource string
		Type     string
		Data     string
	}

	tests := []struct {
		name       string
		input      string
		wantEvents []event
		wantErr    error
	}{
		{
			name: "mirakc events with keep-alive comments",
			input: "event: tuner.status-changed\ndata: {\"tunerIndex\":0}\n\n" +
				":\n\n" +
				"event: epg.programs-updated\ndata: {\"serviceId\":3273601024}\n\n",
			wantEvents: []event{
				{Resource: resourceTuner, Type: typeStatusChanged, Data: `{"tunerIndex":0}`},
				{Resource: "epg", Type: "programs-updated", Data: `{"serviceId":3273601024}`},
			},
			wantErr: io.EOF,
		},
		{
			name:  "CRLF and CR line endings",
			input: "event: onair.program-changed\r\ndata: {\"serviceId\":1}\r\n\r\nevent: tuner.status-changed\rdata: {}\r\r",
			wantEvents: []event{
				{Resource: "onair", Type: "program-changed", Data: `{"serviceId":1}`},
				{Resource: resourceTuner, Type: typeStatusChanged, Data: `{}`},
			},
			wantErr: io.EOF,
		},
		{
			name:       "multiple data lines are joined",
			input:      "data: {\"a\":\ndata: 1}\n\n",
			wantEvents: []event{{Resource: "message", Type: "", Data: "{\"a\":\n1}"}},
			wantErr:    io.EOF,
		},
		{
			name:       "unknown fields are ignored",
			input:      "id: 1\nretry: 1000\nevent: epg.programs-updated\ndata:{\"serviceId\":2}\n\n",
			wantEvents: []event{{Resource: "epg", Type: "programs-updated", Data: `{"serviceId":2}`}},
			wantErr:    io.EOF,
		},
		{
			name:       "cut before the blank line",
			input:      "event: epg.programs-updated\ndata: {\"serviceId\":2}\n",
			wantEvents: []event{},
			wantErr:    io.ErrUnexpectedEOF,
		},
		{
			name:       "cut in the middle of a line",
			input:      "event: tuner.status-changed\ndata: {}\n\nevent: epg.prog",
			wantEvents: []event{{Resource: resourceTuner, Type: typeStatusChanged, Data: `{}`}},
			wantErr:    io.ErrUnexpectedEOF,
		},
		{
			name:       "empty input",
			input:      "",
			wantEvents: []event{},
			wantErr:    io.EOF,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			events, err := readAll(mirakurun.NewSSEReader(strings.NewReader(tt.input)).Next)
			require.ErrorIs(t, err, tt.wantErr)
			got := []event{}
			for _, e := range events {
				got = append(got, event{Resource: e.Resource, Type: e.Type, Data: string(e.Data)})
				assert.True(t, e.Time.IsZero(), "mirakc events have no timestamp")
			}
			assert.Equal(t, tt.wantEvents, got)
		})
	}
}
