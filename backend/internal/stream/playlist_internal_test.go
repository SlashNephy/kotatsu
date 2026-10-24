package stream

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/ts"
)

// TestKeyframesSkipGaps は、録画が途切れた区間を、I フレームの再生の時間に含めないことを確かめる (#754)。
// 配信もトランスコードも、途切れた区間の前後をつないで流すので、プレイヤーとエンコーダーの時間には、途切れた区間がない。
func TestKeyframesSkipGaps(t *testing.T) {
	t.Parallel()

	// interval は、I フレームの間隔 (45045 / 90000 秒)。
	const interval = 500500 * time.Microsecond
	origin := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	// entries は、PTS が from から 45045 ずつ進む n 個の I フレームの索引を、offset から 1000 バイトおきに作る。
	entries := func(offset, from int64, n int) []ts.IndexEntry {
		index := make([]ts.IndexEntry, 0, n)
		for k := range int64(n) {
			index = append(index, ts.IndexEntry{Offset: offset + k*1000, PTS: from + k*45045})
		}

		return index
	}
	// gap は、2 分の途切れの PTS の刻み。
	const gap = 120 * ts.PTSClock
	tests := []struct {
		name     string
		segments []*segment
		start    time.Duration
		want     []time.Duration
		// wantPiece と wantFrom は、start から流すときの、最初の範囲の添え字とバイト位置。
		wantPiece int
		wantFrom  int64
		wantAt    time.Duration
	}{
		{
			name: "再接続で分かれたセグメントの間の途切れは詰める",
			segments: []*segment{
				{startedAt: origin, endedAt: new(origin.Add(1500 * time.Millisecond)), index: entries(0, 0, 3)},
				{startedAt: origin.Add(2 * time.Minute), endedAt: nil, open: true, index: entries(0, 2*45045+gap, 2)},
			},
			start: 1600 * time.Millisecond,
			want:  []time.Duration{0, interval, 2 * interval, 3 * interval, 4 * interval},
			// 途切れの後の最初の I フレーム (1.5015 秒) から流す。
			wantPiece: 1, wantFrom: 0, wantAt: 3 * interval,
		},
		{
			name: "つないだファイルの中の途切れも詰める",
			segments: []*segment{
				{startedAt: origin, endedAt: new(origin.Add(3 * time.Minute)), index: append(entries(0, 0, 3), entries(3000, 2*45045+gap, 2)...)},
			},
			start:     1600 * time.Millisecond,
			want:      []time.Duration{0, interval, 2 * interval, 3 * interval, 4 * interval},
			wantPiece: 0, wantFrom: 3000, wantAt: 3 * interval,
		},
		{
			name: "索引のないセグメントは、書いていた時間だけ続くものとする",
			segments: []*segment{
				{startedAt: origin, endedAt: new(origin.Add(time.Minute)), index: []ts.IndexEntry{}},
				{startedAt: origin.Add(2 * time.Minute), endedAt: new(origin.Add(3 * time.Minute)), index: entries(0, gap, 2)},
			},
			start: 30 * time.Second,
			want:  []time.Duration{0, time.Minute, time.Minute + interval},
			// 索引のないセグメントには、始まりからしか入れない。
			wantPiece: 0, wantFrom: 0, wantAt: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, keyframes(tt.segments))
			// keyframePoints は、keyframes と同じ I フレームを返す。
			points := keyframePoints(tt.segments)
			ats := make([]time.Duration, 0, len(points))
			for _, point := range points {
				ats = append(ats, point.At)
			}
			assert.Equal(t, tt.want, ats)
			pieces, at := plan(tt.segments, tt.start)
			assert.Equal(t, tt.wantAt, at)
			assert.Same(t, tt.segments[tt.wantPiece], pieces[0].segment)
			assert.Equal(t, tt.wantFrom, pieces[0].from)
		})
	}
}
