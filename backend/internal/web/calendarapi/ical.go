// Package calendarapi は、予約を iCalendar (RFC 5545) で配信する (#1255、設計書 9.8)。
package calendarapi

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// icalTimeLayout は、iCalendar の UTC の日時の書き方。
	icalTimeLayout = "20060102T150405Z"
	// maxLineOctets は、1 行の長さの上限 (改行を除くオクテット数)。これを超える行は折り返す (RFC 5545 3.1)。
	maxLineOctets = 75
	// productID は、カレンダーを作ったものを示す PRODID。
	productID = "-//SlashNephy//Kotatsu//JA"
)

// Event は、カレンダーに載せる予約 1 つ。
type Event struct {
	// UID は、予約ごとに変わらない識別子。カレンダーのアプリは、これで同じ予定を見分けて書き換える。
	UID         string
	Summary     string
	Location    string
	Description string
	StartAt     time.Time
	EndAt       time.Time
	UpdatedAt   time.Time
}

// Calendar は、name の名前のカレンダーに events を載せた iCalendar の本文を返す。now は、DTSTAMP に使う。
func Calendar(name string, events []*Event, now time.Time) string {
	var b strings.Builder
	line := func(property, value string) {
		writeFolded(&b, property+":"+value)
	}
	line("BEGIN", "VCALENDAR")
	line("VERSION", "2.0")
	line("PRODID", productID)
	line("CALSCALE", "GREGORIAN")
	line("METHOD", "PUBLISH")
	line("X-WR-CALNAME", escapeText(name))
	for _, event := range events {
		line("BEGIN", "VEVENT")
		line("UID", escapeText(event.UID))
		line("DTSTAMP", formatTime(now))
		line("DTSTART", formatTime(event.StartAt))
		line("DTEND", formatTime(event.EndAt))
		line("LAST-MODIFIED", formatTime(event.UpdatedAt))
		line("SUMMARY", escapeText(event.Summary))
		if event.Location != "" {
			line("LOCATION", escapeText(event.Location))
		}
		if event.Description != "" {
			line("DESCRIPTION", escapeText(event.Description))
		}
		line("END", "VEVENT")
	}
	line("END", "VCALENDAR")

	return b.String()
}

func formatTime(t time.Time) string {
	return t.UTC().Format(icalTimeLayout)
}

// escapeText は、TEXT の値の \ と ; と , と改行を、RFC 5545 3.3.11 のとおりにエスケープする。
func escapeText(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`).Replace(s)
}

// writeFolded は、content line を CRLF で終えて書く。75 オクテットを超える行は、文字の途中で切らないよう、
// UTF-8 の文字の境目で折り返し、続きの行の頭に空白を 1 つ置く (RFC 5545 3.1)。
func writeFolded(b *strings.Builder, content string) {
	limit := maxLineOctets
	for len(content) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(content[cut]) {
			cut--
		}
		b.WriteString(content[:cut])
		b.WriteString("\r\n ")
		content = content[cut:]
		// 続きの行は、頭の空白の分だけ短くする。
		limit = maxLineOctets - 1
	}
	b.WriteString(content)
	b.WriteString("\r\n")
}
