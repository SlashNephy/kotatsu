package epg_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/usecase/epg"
)

func TestBatchEvents(t *testing.T) {
	t.Parallel()

	const service = 32736*epg.ServiceIDBase + 1024
	const other = 32736*epg.ServiceIDBase + 1032
	tests := []struct {
		name         string
		events       []*tunerserver.Event
		wantServices map[int64]struct{}
		wantAll      bool
	}{
		{
			name: "program events collect their services",
			events: []*tunerserver.Event{
				{Type: tunerserver.EventProgramCreated, ServiceID: service},
				{Type: tunerserver.EventProgramUpdated, ServiceID: service},
				{Type: tunerserver.EventProgramRemoved, ServiceID: other},
			},
			wantServices: map[int64]struct{}{service: {}, other: {}},
		},
		{
			name:         "service changed",
			events:       []*tunerserver.Event{{Type: tunerserver.EventServiceChanged, ServiceID: 0}},
			wantServices: map[int64]struct{}{},
			wantAll:      true,
		},
		{
			name:         "tuner events do not sync programs",
			events:       []*tunerserver.Event{{Type: tunerserver.EventTunerChanged, ServiceID: 0}},
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
