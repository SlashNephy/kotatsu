package stream

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// TestCutAt は、昇格した録画の終わり (#1057) を、終わりの時刻か、その後の最初の I フレームの手前で切ることを確かめる。
func TestCutAt(t *testing.T) {
	t.Parallel()

	// I フレームは 0.5005 秒 (45045 刻み) ごとにある。2 つ目のチャンクは、1 つ目の続き (continues) で、PTS も続く。
	indexOf := func(first, n int64) []ts.IndexEntry {
		index := make([]ts.IndexEntry, 0, n)
		for k := range n {
			index = append(index, ts.IndexEntry{Offset: k * 1000, PTS: (first + k) * 45045})
		}

		return index
	}
	origin := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		length time.Duration
		// want は、切った後の範囲ごとの [from, to]。
		want [][2]int64
	}{
		{name: "I フレームの上で切る", length: 1001 * time.Millisecond, want: [][2]int64{{0, 2000}}},
		{name: "I フレームの間なら、その後の I フレームの手前で切る", length: 1200 * time.Millisecond, want: [][2]int64{{0, 3000}}},
		{name: "次のチャンクの始まりの I フレームで切るなら、次のチャンクを流さない", length: 4 * 1001 * time.Millisecond / 2, want: [][2]int64{{0, -1}}},
		{name: "次のチャンクの途中で切る", length: 5 * 1001 * time.Millisecond / 2, want: [][2]int64{{0, -1}, {0, 1000}}},
		{name: "並びの終わりより長ければ、切らない", length: time.Hour, want: [][2]int64{{0, -1}, {0, -1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			first := &segment{startedAt: origin, endedAt: new(origin.Add(2 * time.Second)), continued: true, index: indexOf(0, 4)}
			second := &segment{startedAt: origin.Add(2 * time.Second), continues: true, index: indexOf(4, 4)}
			pieces := cutAt([]*piece{{segment: first, from: 0, to: -1}, {segment: second, from: 0, to: -1}}, tt.length)
			got := make([][2]int64, 0, len(pieces))
			for _, p := range pieces {
				got = append(got, [2]int64{p.from, p.to})
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
