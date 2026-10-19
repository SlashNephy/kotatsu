package clocktest_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
)

// TestFakeStopwatch は、Stopwatch の経過時間が Advance でだけ進み、Set で壁時計を動かしても変わらないことを確かめる。
func TestFakeStopwatch(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		// before は、Stopwatch を測り始める前の操作。after は、測り始めた後の操作。
		before      func(f *clocktest.Fake)
		after       func(f *clocktest.Fake)
		wantElapsed time.Duration
		wantNow     time.Time
	}{
		{
			name:        "進めた分だけ経つ",
			before:      func(*clocktest.Fake) {},
			after:       func(f *clocktest.Fake) { f.Advance(3 * time.Second); f.Advance(2 * time.Second) },
			wantElapsed: 5 * time.Second,
			wantNow:     start.Add(5 * time.Second),
		},
		{
			name:        "測り始める前に進めた分は数えない",
			before:      func(f *clocktest.Fake) { f.Advance(time.Minute) },
			after:       func(f *clocktest.Fake) { f.Advance(time.Second) },
			wantElapsed: time.Second,
			wantNow:     start.Add(time.Minute + time.Second),
		},
		{
			name:        "壁時計を戻しても経過時間は戻らない",
			before:      func(*clocktest.Fake) {},
			after:       func(f *clocktest.Fake) { f.Advance(time.Second); f.Set(start.Add(-time.Hour)); f.Advance(time.Second) },
			wantElapsed: 2 * time.Second,
			wantNow:     start.Add(-time.Hour + time.Second),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := clocktest.NewFake(start)
			tt.before(f)
			stopwatch := f.StartStopwatch()
			tt.after(f)

			assert.Equal(t, tt.wantElapsed, stopwatch.Elapsed())
			assert.Equal(t, tt.wantNow, f.Now())
		})
	}
}
