package calendarapi_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/web/calendarapi"
)

// TestCalendar は、予約を VEVENT にし、日時を UTC で書き、TEXT の値をエスケープすることを確かめる (#1255)。
func TestCalendar(t *testing.T) {
	t.Parallel()

	jst := time.FixedZone("JST", 9*60*60)
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	got := calendarapi.Calendar("録画予約", []*calendarapi.Event{{
		UID: "0192-a@kotatsu", Summary: "ニュース; 速報, 特集\\", Location: "こたつテレビ", Description: "1行目\n2行目",
		StartAt: time.Date(2026, 11, 8, 21, 0, 0, 0, jst), EndAt: time.Date(2026, 11, 8, 21, 54, 0, 0, jst),
		UpdatedAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}}, now)

	assert.Equal(t, strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//SlashNephy//Kotatsu//JA",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"X-WR-CALNAME:録画予約",
		"BEGIN:VEVENT",
		"UID:0192-a@kotatsu",
		"DTSTAMP:20261107T120000Z",
		"DTSTART:20261108T120000Z",
		"DTEND:20261108T125400Z",
		"LAST-MODIFIED:20261101T000000Z",
		`SUMMARY:ニュース\; 速報\, 特集\\`,
		"LOCATION:こたつテレビ",
		`DESCRIPTION:1行目\n2行目`,
		"END:VEVENT",
		"END:VCALENDAR",
		"",
	}, "\r\n"), got)
}

// TestCalendarFolding は、75 オクテットを超える行を、文字の途中で切らずに折り返すことを確かめる (RFC 5545 3.1)。
func TestCalendarFolding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		summary string
	}{
		{name: "ascii", summary: strings.Repeat("a", 200)},
		{name: "multibyte", summary: strings.Repeat("番組", 60)},
		{name: "mixed", summary: "a" + strings.Repeat("あ", 70)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
			got := calendarapi.Calendar("", []*calendarapi.Event{{UID: "x", Summary: tt.summary, StartAt: now, EndAt: now, UpdatedAt: now}}, now)
			lines := strings.Split(strings.TrimSuffix(got, "\r\n"), "\r\n")
			var unfolded strings.Builder
			folded := false
			for _, line := range lines {
				assert.LessOrEqual(t, len(line), 75, "a line is at most 75 octets")
				require.True(t, utf8.ValidString(line), "a character is not split")
				if rest, ok := strings.CutPrefix(line, " "); ok {
					folded = true
					unfolded.WriteString(rest)

					continue
				}
				unfolded.WriteString("\n" + line)
			}
			assert.True(t, folded)
			assert.Contains(t, unfolded.String(), "\nSUMMARY:"+tt.summary+"\n", "unfolding restores the value")
		})
	}
}
