package epg_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

// resourceProgram、typeUpdate、typeCreate は、Mirakurun のイベントのリソースと種類。
const (
	resourceProgram = "program"
	typeUpdate      = "update"
	typeCreate      = "create"
)

func TestBatchEvents(t *testing.T) {
	t.Parallel()

	const service = 32736*epg.ServiceIDBase + 1024
	tests := []struct {
		name         string
		events       []*mirakurun.Event
		wantServices map[int64]struct{}
		wantAll      bool
	}{
		{
			name:         "program created on Mirakurun",
			events:       []*mirakurun.Event{{Resource: resourceProgram, Type: typeCreate, Data: json.RawMessage(`{"id":327360102400005,"networkId":32736,"serviceId":1024,"eventId":5}`)}},
			wantServices: map[int64]struct{}{service: {}},
		},
		{
			name:         "program removed on Mirakurun has only its id",
			events:       []*mirakurun.Event{{Resource: resourceProgram, Type: "remove", Data: json.RawMessage(`{"id":327360102400005}`)}},
			wantServices: map[int64]struct{}{service: {}},
		},
		{
			name:         "service number without network falls back to the program id",
			events:       []*mirakurun.Event{{Resource: resourceProgram, Type: typeUpdate, Data: json.RawMessage(`{"id":327360102400005,"serviceId":1024}`)}},
			wantServices: map[int64]struct{}{service: {}},
		},
		{
			name:         "programs updated on mirakc",
			events:       []*mirakurun.Event{{Resource: "epg", Type: "programs-updated", Data: json.RawMessage(`{"serviceId":3273601024}`)}},
			wantServices: map[int64]struct{}{service: {}},
		},
		{
			name:         "service changed",
			events:       []*mirakurun.Event{{Resource: "service", Type: typeUpdate, Data: json.RawMessage(`{"id":3273601024}`)}},
			wantServices: map[int64]struct{}{},
			wantAll:      true,
		},
		{
			name: "tuner events and broken data are ignored",
			events: []*mirakurun.Event{
				{Resource: "tuner", Type: typeUpdate, Data: json.RawMessage(`{"index":0}`)},
				{Resource: resourceProgram, Type: typeCreate, Data: json.RawMessage(`[`)},
			},
			wantServices: map[int64]struct{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			services, all := epg.BatchEvents(tt.events)
			assert.Equal(t, tt.wantServices, services)
			assert.Equal(t, tt.wantAll, all)
		})
	}
}
