package stream_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

// TestRecordingURLLifetime は、録画の署名付き URL の有効期限を、録画の長さに 1 時間を足した長さにし、
// 長さが負なら 0 とみなすことを確かめる (#1756)。
func TestRecordingURLLifetime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		length time.Duration
		want   time.Duration
	}{
		{name: "30 分の録画", length: 30 * time.Minute, want: 90 * time.Minute},
		{name: "長さが 0", length: 0, want: time.Hour},
		{name: "長さが負", length: -10 * time.Minute, want: time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, stream.RecordingURLLifetime(tt.length))
		})
	}
}
