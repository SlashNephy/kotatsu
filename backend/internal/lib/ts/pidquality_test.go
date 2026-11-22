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
			{PID: 0x100, StreamType: 0x02, Kind: ts.StreamKindVideo},
			{PID: 0x110, StreamType: 0x0F, Kind: ts.StreamKindAudio, ComponentTag: new(uint8(0x10))},
			{PID: 0x130, StreamType: 0x06, Kind: ts.StreamKindOther},
		}}},
	}
	pids := []*ts.PIDStats{
		{PID: 0x0000, Packets: 10, Drops: 0, Errors: 0, Scrambled: 0},
		{PID: 0x0012, Packets: 20, Drops: 1, Errors: 0, Scrambled: 0},
		{PID: 0x0100, Packets: 1000, Drops: 3, Errors: 2, Scrambled: 1, Payloads: 990, ScrambledPayloads: 1},
		{PID: 0x0110, Packets: 100, Drops: 0, Errors: 0, Scrambled: 0, Payloads: 100, ScrambledPayloads: 0},
		{PID: 0x0130, Packets: 5, Drops: 0, Errors: 0, Scrambled: 0},
		{PID: 0x01F0, Packets: 10, Drops: 0, Errors: 0, Scrambled: 0},
		{PID: 0x0901, Packets: 7, Drops: 2, Errors: 0, Scrambled: 0},
	}

	assert.Equal(t, []ts.PIDQuality{
		{PID: 0x0000, Kind: ts.PIDKindPSI, Packets: 10, Drops: 0, Errors: 0, Scrambled: 0, Payloads: new(int64(0)), ScrambledPayloads: new(int64(0))},
		{PID: 0x0012, Kind: ts.PIDKindPSI, Packets: 20, Drops: 1, Errors: 0, Scrambled: 0, Payloads: new(int64(0)), ScrambledPayloads: new(int64(0))},
		{PID: 0x0100, Kind: ts.PIDKindVideo, Packets: 1000, Drops: 3, Errors: 2, Scrambled: 1, Payloads: new(int64(990)), ScrambledPayloads: new(int64(1)), StreamType: new(uint8(0x02))},
		{PID: 0x0110, Kind: ts.PIDKindAudio, Packets: 100, Drops: 0, Errors: 0, Scrambled: 0, Payloads: new(int64(100)), ScrambledPayloads: new(int64(0)), StreamType: new(uint8(0x0F)), ComponentTag: new(uint8(0x10))},
		{PID: 0x0130, Kind: ts.PIDKindOther, Packets: 5, Drops: 0, Errors: 0, Scrambled: 0, Payloads: new(int64(0)), ScrambledPayloads: new(int64(0)), StreamType: new(uint8(0x06))},
		{PID: 0x01F0, Kind: ts.PIDKindPSI, Packets: 10, Drops: 0, Errors: 0, Scrambled: 0, Payloads: new(int64(0)), ScrambledPayloads: new(int64(0))},
		{PID: 0x0901, Kind: ts.PIDKindUnknown, Packets: 7, Drops: 2, Errors: 0, Scrambled: 0, Payloads: new(int64(0)), ScrambledPayloads: new(int64(0))},
	}, ts.PIDQualities(stats, pids))
}

// TestJudgedDrops は、判定に数えるドロップから、字幕・データの PID のドロップを除くことを確かめる。
func TestJudgedDrops(t *testing.T) {
	t.Parallel()

	programs := []*ts.Program{{Number: 101, PMTPID: 0x1000, PCRPID: 0x1FF, Version: 0, Streams: []*ts.Stream{
		{PID: 0x100, StreamType: 0x24, Kind: ts.StreamKindVideo},
		{PID: 0x110, StreamType: 0x0F, Kind: ts.StreamKindAudio},
		{PID: 0x130, StreamType: 0x06, Kind: ts.StreamKindOther},
		{PID: 0x138, StreamType: 0x06, Kind: ts.StreamKindOther},
	}}}

	tests := []struct {
		name     string
		programs []*ts.Program
		pids     []*ts.PIDStats
		want     int64
	}{
		{
			name:     "字幕だけのドロップは数えない",
			programs: programs,
			pids:     []*ts.PIDStats{{PID: 0x100, Drops: 0}, {PID: 0x110, Drops: 0}, {PID: 0x130, Drops: 3}, {PID: 0x138, Drops: 1}},
			want:     0,
		},
		{
			name:     "映像、音声、番組情報、不明の PID のドロップは数える",
			programs: programs,
			pids:     []*ts.PIDStats{{PID: 0x0012, Drops: 1}, {PID: 0x100, Drops: 2}, {PID: 0x110, Drops: 4}, {PID: 0x130, Drops: 3}, {PID: 0x1FF, Drops: 8}},
			want:     15,
		},
		{
			name:     "PMT を読めていなければ、すべて数える",
			programs: nil,
			pids:     []*ts.PIDStats{{PID: 0x100, Drops: 2}, {PID: 0x130, Drops: 3}},
			want:     5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ts.JudgedDrops(&ts.Stats{Programs: tt.programs}, tt.pids))
		})
	}
}
