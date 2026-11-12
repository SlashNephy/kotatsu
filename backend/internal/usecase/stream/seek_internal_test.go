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

// TestRecordingDuration は、録画の再生の時間を、最後の I フレームの後に 1 間隔を足して求め、索引のないセグメントがあれば
// 求めないことを確かめる。録画の変換の進み具合の見込みに使う (#1511)。
func TestRecordingDuration(t *testing.T) {
	t.Parallel()

	keyframes := func(n, from int64) []ts.IndexEntry {
		index := make([]ts.IndexEntry, 0, n)
		for k := range n {
			index = append(index, ts.IndexEntry{Offset: k * 1000, PTS: (from + k) * 45045})
		}

		return index
	}
	origin := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		segments []*segment
		want     time.Duration
		wantOK   bool
	}{
		{
			// 0.5005 秒ごとの 10 個の I フレームは、最後の I フレーム (4.5045 秒) の後に 1 間隔続く。
			name:     "one segment",
			segments: []*segment{{startedAt: origin, endedAt: new(origin.Add(time.Minute)), index: keyframes(10, 0)}},
			want:     5005 * time.Millisecond, wantOK: true,
		},
		{
			// 時間の離れた 2 つのセグメントは、途切れた区間を詰めてつなぐ。
			name: "gap between segments",
			segments: []*segment{
				{startedAt: origin, endedAt: new(origin.Add(time.Minute)), index: keyframes(10, 0)},
				{startedAt: origin.Add(10 * time.Minute), endedAt: new(origin.Add(11 * time.Minute)), continues: true, index: keyframes(10, 10000)},
			},
			want: 10010 * time.Millisecond, wantOK: true,
		},
		{
			name: "segment without the index",
			segments: []*segment{
				{startedAt: origin, endedAt: new(origin.Add(time.Minute)), index: keyframes(10, 0)},
				{startedAt: origin.Add(time.Minute), endedAt: new(origin.Add(2 * time.Minute)), continues: true, index: []ts.IndexEntry{}},
			},
			want: 0, wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &Recording{streamer: nil, timeline: nil, InProgress: false, segments: tt.segments, skip: 0}
			got, ok := r.Duration()
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
