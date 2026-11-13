package cli_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/cli"
)

// TestTraceable は、パスに秘密の値を含む予約の iCalendar (#1255) と IPTV の配信 (#1324) を、トレースに残さないことを確かめる。
func TestTraceable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "calendar", path: "/api/calendar/kotatsu_cal_x/reservations.ics", want: false},
		{name: "double slash", path: "//api//calendar/kotatsu_cal_x/reservations.ics", want: false},
		{name: "dot segment", path: "/api/epgstation/../calendar/kotatsu_cal_x/reservations.ics", want: false},
		{name: "calendar directory", path: "/api/calendar", want: false},
		{name: "similar prefix", path: "/api/calendars", want: true},
		{name: "IPTV playlist", path: "/api/iptv/kotatsu_iptv_x/channels.m3u", want: false},
		{name: "IPTV stream", path: "//api/iptv/kotatsu_iptv_x/services/1/stream.m2ts", want: false},
		{name: "IPTV similar prefix", path: "/api/iptvs", want: true},
		{name: "graphql", path: "/graphql", want: true},
		{name: "EPGStation", path: "/api/epgstation/api/recorded", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, cli.Traceable(httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil)))
		})
	}
}
