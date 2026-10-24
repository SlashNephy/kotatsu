package recording_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// TestCountsAfter は、引き継がれた後の区間のドロップ、エラー、スクランブルを、控えた数から求めることを確かめる (#525)。
// 引き継ぎで重なった区間は、新しい mikan のセグメントでも数えるので、古い mikan はその分を数えない。
func TestCountsAfter(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC)
	samples := []recording.QualitySample{
		{At: base, Counts: recording.QualityCounts{Drops: 0, Errors: 0, Scrambled: 0}},
		{At: base.Add(time.Second), Counts: recording.QualityCounts{Drops: 2, Errors: 1, Scrambled: 0}},
		{At: base.Add(2 * time.Second), Counts: recording.QualityCounts{Drops: 3, Errors: 1, Scrambled: 4}},
	}
	final := recording.QualityCounts{Drops: 5, Errors: 2, Scrambled: 4}
	// end は、セグメントを書き終えた時刻。
	end := base.Add(4 * time.Second)
	tests := []struct {
		name  string
		since time.Time
		want  recording.QualityCounts
	}{
		{name: "after the last sample", since: base.Add(3 * time.Second), want: recording.QualityCounts{Drops: 2, Errors: 1, Scrambled: 0}},
		{name: "between samples", since: base.Add(1500 * time.Millisecond), want: recording.QualityCounts{Drops: 3, Errors: 1, Scrambled: 4}},
		{name: "exactly at a sample", since: base.Add(time.Second), want: recording.QualityCounts{Drops: 3, Errors: 1, Scrambled: 4}},
		// 控える前に引き継がれたなら、このセグメントの全体が重なっている。
		{name: "before the first sample", since: base.Add(-time.Second), want: final},
		// 書き終えた後に、新しいセグメントが始まったなら、重なっていない。
		{name: "after the segment ended", since: end, want: recording.QualityCounts{Drops: 0, Errors: 0, Scrambled: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, recording.CountsAfter(samples, tt.since, end, final))
		})
	}
}
