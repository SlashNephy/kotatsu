package ts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// TestPIDQualities は、PID ごとの統計に、PMT と決まった PID から分かる種類を添えることを確かめる (#473)。
func TestPIDQualities(t *testing.T) {
	t.Parallel()

	stats := &ts.Stats{
		Programs: []*ts.Program{{Number: 1024, PMTPID: 0x1F0, PCRPID: 0x1FF, Version: 0, Streams: []*ts.Stream{
			{PID: 0x100, StreamType: 0x02, Kind: ts.StreamVideo},
			{PID: 0x110, StreamType: 0x0F, Kind: ts.StreamAudio, ComponentTag: new(uint8(0x10))},
			{PID: 0x130, StreamType: 0x06, Kind: ts.StreamOther},
		}}},
	}
	pids := []*ts.PIDStats{
		{PID: 0x0000, Packets: 10, Drops: 0, Errors: 0, Scrambled: 0},
		{PID: 0x0012, Packets: 20, Drops: 1, Errors: 0, Scrambled: 0},
		{PID: 0x0100, Packets: 1000, Drops: 3, Errors: 2, Scrambled: 1},
		{PID: 0x0110, Packets: 100, Drops: 0, Errors: 0, Scrambled: 0},
		{PID: 0x0130, Packets: 5, Drops: 0, Errors: 0, Scrambled: 0},
		{PID: 0x01F0, Packets: 10, Drops: 0, Errors: 0, Scrambled: 0},
		{PID: 0x0901, Packets: 7, Drops: 2, Errors: 0, Scrambled: 0},
	}

	assert.Equal(t, []ts.PIDQuality{
		{PID: 0x0000, Kind: ts.PIDKindPSI, Packets: 10, Drops: 0, Errors: 0, Scrambled: 0},
		{PID: 0x0012, Kind: ts.PIDKindPSI, Packets: 20, Drops: 1, Errors: 0, Scrambled: 0},
		{PID: 0x0100, Kind: ts.PIDKindVideo, Packets: 1000, Drops: 3, Errors: 2, Scrambled: 1, StreamType: new(uint8(0x02))},
		{PID: 0x0110, Kind: ts.PIDKindAudio, Packets: 100, Drops: 0, Errors: 0, Scrambled: 0, StreamType: new(uint8(0x0F)), ComponentTag: new(uint8(0x10))},
		{PID: 0x0130, Kind: ts.PIDKindOther, Packets: 5, Drops: 0, Errors: 0, Scrambled: 0, StreamType: new(uint8(0x06))},
		{PID: 0x01F0, Kind: ts.PIDKindPSI, Packets: 10, Drops: 0, Errors: 0, Scrambled: 0},
		{PID: 0x0901, Kind: ts.PIDKindUnknown, Packets: 7, Drops: 2, Errors: 0, Scrambled: 0},
	}, ts.PIDQualities(stats, pids))
}
