package ts_test

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// pcrAt は、PCR の基準部が base (90 kHz) の、アダプテーションフィールドだけのパケットを作る。
func pcrAt(pid uint16, base uint64) []byte {
	p := bytes.Repeat([]byte{0xFF}, ts.PacketSize)
	p[0] = 0x47
	binary.BigEndian.PutUint16(p[1:3], pid&0x1FFF)
	p[3] = 0x20
	p[4] = ts.PacketSize - 5
	p[5] = 0x10
	// PCR は、基準部 33 ビット、予約 6 ビット (1)、拡張部 9 ビット (0) の 48 ビット。
	var pcr [8]byte
	binary.BigEndian.PutUint64(pcr[:], base<<15|0x3F<<9)
	copy(p[6:12], pcr[2:])

	return p
}

// at は、基準の時刻から seconds 秒後の時刻。
func at(seconds float64) time.Time {
	return time.Unix(1_763_000_000, 0).Add(time.Duration(seconds * float64(time.Second)))
}

// pcrSeconds は、秒を PCR の基準部にする。
func pcrSeconds(seconds float64) uint64 {
	return uint64(seconds * 90_000)
}

func TestPCRClock(t *testing.T) {
	t.Parallel()

	type feed struct {
		pcr        float64
		receivedAt float64
	}
	tests := []struct {
		name  string
		feeds []feed
		// wantPCR と wantReceivedAt は、使う組。wantPCR が負なら、組がない。
		wantPCR        float64
		wantReceivedAt float64
	}{
		{name: "no PCR yet", feeds: nil, wantPCR: -1, wantReceivedAt: 0},
		// 遅れて読んだ組 (受け取った時刻 − PCR が大きい) は採らない。
		{
			name:    "the pair read without delay wins",
			feeds:   []feed{{pcr: 10, receivedAt: 100}, {pcr: 10.2, receivedAt: 100.9}, {pcr: 11, receivedAt: 100.8}},
			wantPCR: 11, wantReceivedAt: 100.8,
		},
		// 区切ってすぐに、遅れた組しかなくても、1 つ前の区切りの最小の組を使う。
		{
			name:    "the previous window is kept",
			feeds:   []feed{{pcr: 10, receivedAt: 100}, {pcr: 41, receivedAt: 131.5}},
			wantPCR: 10, wantReceivedAt: 100,
		},
		// 2 つ前の区切りの組は使わない。
		{
			name:    "an older window is dropped",
			feeds:   []feed{{pcr: 10, receivedAt: 100}, {pcr: 41, receivedAt: 131.5}, {pcr: 72, receivedAt: 162}},
			wantPCR: 72, wantReceivedAt: 162,
		},
		// 選局で PCR が戻ったら、前の組を捨てて結び直す。
		{
			name:    "a rewind resyncs",
			feeds:   []feed{{pcr: 1000, receivedAt: 100}, {pcr: 500, receivedAt: 101}},
			wantPCR: 500, wantReceivedAt: 101,
		},
		// 一周の半分より先へ進んだら (選局で飛んだ)、前の組を捨てて結び直す。
		{
			name:    "a forward jump past half a cycle resyncs",
			feeds:   []feed{{pcr: 10, receivedAt: 100}, {pcr: 60_000, receivedAt: 101}},
			wantPCR: 60_000, wantReceivedAt: 101,
		},
		// 揺れほどの小さな戻りでは結び直さない。
		{
			name:    "a small jitter does not resync",
			feeds:   []feed{{pcr: 1000, receivedAt: 100}, {pcr: 999.5, receivedAt: 100.5}},
			wantPCR: 1000, wantReceivedAt: 100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clock := ts.NewPCRClock(programNum, 30*time.Second)
			for _, f := range tt.feeds {
				clock.Write(pcrAt(videoPID, pcrSeconds(f.pcr)), at(f.receivedAt))
			}
			pair, ok := clock.Pair()
			if tt.wantPCR < 0 {
				assert.False(t, ok)

				return
			}
			require.True(t, ok)
			assert.InDelta(t, tt.wantPCR, pair.PCR, 1e-6)
			assert.True(t, at(tt.wantReceivedAt).Equal(pair.ReceivedAt), "received at %v", pair.ReceivedAt)
		})
	}
}

// TestPCRClockUsesPCRPIDAfterPMT は、PMT を読むまではどの PID の PCR も受け付け、読んだ後は PCR_PID の PCR だけを使うことを確かめる。
func TestPCRClockUsesPCRPIDAfterPMT(t *testing.T) {
	t.Parallel()

	clock := ts.NewPCRClock(programNum, 30*time.Second)
	clock.Write(pcrAt(audioPID, pcrSeconds(50)), at(100))
	pair, ok := clock.Pair()
	require.True(t, ok)
	assert.InDelta(t, 50, pair.PCR, 1e-6)

	for _, p := range psiHeader(0, 0x0100) {
		clock.Write(p, at(100))
	}
	// PCR_PID (videoPID) でない PID の PCR は、遅れずに読んだ組でも使わない。
	clock.Write(pcrAt(audioPID, pcrSeconds(60)), at(100.5))
	pair, ok = clock.Pair()
	require.True(t, ok)
	assert.InDelta(t, 50, pair.PCR, 1e-6)
	clock.Write(pcrAt(videoPID, pcrSeconds(60)), at(100.5))
	pair, ok = clock.Pair()
	require.True(t, ok)
	assert.InDelta(t, 60, pair.PCR, 1e-6)
}

// TestPCRClockSplitChunks は、パケットの境目で切れていない塊でも、PCR を読めることを確かめる。
func TestPCRClockSplitChunks(t *testing.T) {
	t.Parallel()

	stream := append(pcrAt(videoPID, pcrSeconds(20)), pcrAt(videoPID, pcrSeconds(21))...)
	clock := ts.NewPCRClock(programNum, 30*time.Second)
	clock.Write(stream[:100], at(100))
	_, ok := clock.Pair()
	assert.False(t, ok)
	clock.Write(stream[100:300], at(100.1))
	clock.Write(stream[300:], at(100.2))
	pair, ok := clock.Pair()
	require.True(t, ok)
	// 2 つ目の PCR は、最後の塊で読めたので、その塊の時刻と組にする。
	assert.InDelta(t, 21, pair.PCR, 1e-6)
	assert.True(t, at(100.2).Equal(pair.ReceivedAt))
}
