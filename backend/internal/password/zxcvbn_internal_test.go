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
		busy int
		// release は、判定を呼んでから枠を 1 つ空けるまでの時間。0 なら空けない。
		release time.Duration
		wantErr error
	}{
		{name: "枠が空いている", busy: maxConcurrentEstimates - 1, release: 0, wantErr: nil},
		{name: "待つ間に枠が空く", busy: maxConcurrentEstimates, release: 50 * time.Millisecond, wantErr: nil},
		{name: "枠が埋まったまま", busy: maxConcurrentEstimates, release: 0, wantErr: ErrStrengthTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// CI では、判定に 10 秒を超えることがあるので、枠が空くときは上限を長くする。
			e := NewEstimator(2 * time.Minute)
			// 時間切れを待つ時間を短くするため、枠が空かないときだけ、上限を短くする。
			if tt.wantErr != nil {
				e.timeout = 100 * time.Millisecond
			}
			for range tt.busy {
				e.slots <- struct{}{}
			}
			if tt.release > 0 {
				time.AfterFunc(tt.release, func() { <-e.slots })
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
