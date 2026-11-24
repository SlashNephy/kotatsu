package tunerserver_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
)

// TestEPGCaptureNeeds は、EDCB の EPG の取得のために空けておくチューナーを、EpgTimerSrv の判定 (GetEpgCapTunerList) に合わせて決めることを
// 確認する (#1856)。
func TestEPGCaptureNeeds(t *testing.T) {
	t.Parallel()

	const (
		terrestrial = "bondriver_t.dll"
		satellite   = "bondriver_s.dll"
	)
	// 地上波の BonDriver に 3 本、衛星の BonDriver に 2 本のチューナーがある。名前の大文字と小文字は区別しない。
	tunerNames := []string{"BonDriver_T.dll", "BonDriver_T.dll", "bondriver_t.dll", "BonDriver_S.dll", "BonDriver_S.dll"}
	capture := func(known, near, anyBonDriver bool, bonDrivers map[string]tunerserver.EPGCaptureBonDriver) *tunerserver.EPGCapture {
		return &tunerserver.EPGCapture{
			Known: known, Near: near, AnyBonDriver: anyBonDriver, BonDrivers: bonDrivers,
			DefaultBonDriver: tunerserver.EPGCaptureBonDriver{Disabled: false, Tuners: 0},
		}
	}
	need := func(idle int, names ...string) *tunerserver.EPGCaptureNeed {
		return &tunerserver.EPGCaptureNeed{TunerNames: names, Idle: idle}
	}
	tests := []struct {
		name    string
		capture *tunerserver.EPGCapture
		want    []*tunerserver.EPGCaptureNeed
	}{
		{
			// EpgTimerSrv.ini を読めなければ、BonDriver ごとに 1 本を、時刻を問わずに空けておく。
			name:    "unknown",
			capture: capture(false, false, false, map[string]tunerserver.EPGCaptureBonDriver{}),
			want:    []*tunerserver.EPGCaptureNeed{need(1, terrestrial), need(1, satellite)},
		},
		{
			name:    "far from capture times",
			capture: capture(true, false, false, map[string]tunerserver.EPGCaptureBonDriver{}),
			want:    []*tunerserver.EPGCaptureNeed{},
		},
		{
			// EPGCount が 0 (既定) なら、BonDriver のすべてのチューナーで取得する。
			name:    "all tuners by default",
			capture: capture(true, true, false, map[string]tunerserver.EPGCaptureBonDriver{}),
			want:    []*tunerserver.EPGCaptureNeed{need(3, terrestrial), need(2, satellite)},
		},
		{
			name: "epg count and disabled bon driver",
			capture: capture(true, true, false, map[string]tunerserver.EPGCaptureBonDriver{
				terrestrial: {Disabled: false, Tuners: 1}, satellite: {Disabled: true, Tuners: 0},
			}),
			want: []*tunerserver.EPGCaptureNeed{need(1, terrestrial)},
		},
		{
			// EPGCount がチューナーより多い BonDriver は、EpgTimerSrv が取得しないので、満たせない数のまま返す。
			name: "epg count larger than the tuners",
			capture: capture(true, true, false, map[string]tunerserver.EPGCaptureBonDriver{
				terrestrial: {Disabled: false, Tuners: 4}, satellite: {Disabled: false, Tuners: 1},
			}),
			want: []*tunerserver.EPGCaptureNeed{need(4, terrestrial), need(1, satellite)},
		},
		{
			// NGEpgCapTime が 0 なら、取得に使う BonDriver のどれかに 1 本空いていればよい。
			name: "any bon driver",
			capture: capture(true, true, true, map[string]tunerserver.EPGCaptureBonDriver{
				satellite: {Disabled: true, Tuners: 0},
			}),
			want: []*tunerserver.EPGCaptureNeed{need(1, terrestrial)},
		},
		{
			name:    "any bon driver of two",
			capture: capture(true, true, true, map[string]tunerserver.EPGCaptureBonDriver{}),
			want:    []*tunerserver.EPGCaptureNeed{need(1, terrestrial, satellite)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.capture.Needs(tunerNames))
		})
	}
}
