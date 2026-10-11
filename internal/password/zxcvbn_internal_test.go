package password

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEstimateWaitsForSlot は、同時に動かす判定が上限に達している間は、枠が空くまで待ち、
// 1 回の判定の時間の上限までに空かなければ、時間切れとすることを確かめる。
func TestEstimateWaitsForSlot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// busy は、判定を呼ぶ前に埋めておく枠の数。
		busy    int
		wantErr error
	}{
		{name: "枠が空いている", busy: maxConcurrentEstimates - 1, wantErr: nil},
		{name: "枠が埋まっている", busy: maxConcurrentEstimates, wantErr: ErrStrengthTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := NewEstimator(10 * time.Second)
			// 時間切れを待つ時間を短くするため、枠を埋めるときだけ、上限を短くする。
			if tt.busy == maxConcurrentEstimates {
				e.timeout = 100 * time.Millisecond
			}
			for range tt.busy {
				e.slots <- struct{}{}
			}

			strength, err := e.Estimate(t.Context(), "correct horse battery staple", []string{})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.NotNil(t, strength)
		})
	}
}
