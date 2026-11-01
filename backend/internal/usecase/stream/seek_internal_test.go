package stream

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// TestRecordingSeekBefore は、タイムシフトのバッファの起点が I フレームの間にあるとき、手前の I フレームから流し、
// 起点より前にはみ出た分を Before で返すことを確かめる。kyusu は、その分も切り落とす (#1128)。
func TestRecordingSeekBefore(t *testing.T) {
	t.Parallel()

	// I フレームは 0.5005 秒 (45045 刻み) ごとに 10 個ある。
	index := make([]ts.IndexEntry, 0, 10)
	for k := range int64(10) {
		index = append(index, ts.IndexEntry{Offset: k * 1000, PTS: k * 45045})
	}
	origin := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	segments := []*segment{{startedAt: origin, endedAt: new(origin.Add(time.Minute)), index: index}}
	tests := []struct {
		name       string
		skip       time.Duration
		start      time.Duration
		wantAt     time.Duration
		wantBefore time.Duration
	}{
		{name: "起点が I フレームの上にあれば、はみ出ない", skip: 0, start: 0, wantAt: 0, wantBefore: 0},
		{name: "起点が I フレームの間にあれば、手前の I フレームからはみ出た分を返す", skip: 1200 * time.Millisecond, start: 0, wantAt: 0, wantBefore: 1200*time.Millisecond - 1001*time.Millisecond},
		{name: "起点より後の位置は、はみ出ない", skip: 1200 * time.Millisecond, start: time.Second, wantAt: 2002*time.Millisecond - 1200*time.Millisecond, wantBefore: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &Recording{streamer: nil, timeline: nil, InProgress: true, segments: segments, skip: tt.skip}
			pos := r.Seek(tt.start)
			assert.Equal(t, tt.wantAt, pos.At)
			assert.Equal(t, tt.wantBefore, pos.Before)
		})
	}
}
