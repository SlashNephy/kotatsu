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

const (
	ecmPID         = 0x0901
	scrambleWindow = 100
)

// esPacket は、PID pid のパケットを作る。scrambled なら transport_scrambling_control を 0b10 にする。
// payload が false なら、アダプテーションフィールドだけのパケットにする。
func esPacket(pid uint16, cc byte, scrambled, payload bool) []byte {
	p := bytes.Repeat([]byte{0xAA}, ts.PacketSize)
	p[0] = 0x47
	binary.BigEndian.PutUint16(p[1:3], pid&0x1FFF)
	p[3] = cc & 0x0F
	if payload {
		p[3] |= 0x10
	} else {
		p[3] |= 0x20
		p[4] = ts.PacketSize - 5
		p[5] = 0x00
	}
	if scrambled {
		p[3] |= 0x80
	}

	return p
}

// scrambleHeader は、PAT と、映像と音声を載せた PMT のパケットを作る。
func scrambleHeader() [][]byte {
	var patCC, pmtCC byte
	pkts := psiPackets(0x0000, &patCC, patSection(0, map[uint16]uint16{programNum: pmtPID}))

	return append(pkts, psiPackets(pmtPID, &pmtCC, pmtSection(0,
		esEntry{streamType: 0x02, pid: videoPID, infoLength: 0},
		esEntry{streamType: 0x0F, pid: audioPID, infoLength: 0},
	))...)
}

// scrambleStream は、PAT と PMT の後に、映像のパケットを n 個並べる。scrambled(i) が i 番目をスクランブルするかを決める。
func scrambleStream(n int, scrambled func(i int) bool) [][]byte {
	pkts := scrambleHeader()
	for i := range n {
		pkts = append(pkts, esPacket(videoPID, byte(i), scrambled(i), true))
	}

	return pkts
}

func always(int) bool { return true }

// feedDetector は、パケットを 1 つずつ書き、1 つ書くたびに時計を step 進める。書き込みが失敗したら、そこで止める。
func feedDetector(d *ts.ScrambleDetector, now *time.Time, step time.Duration, pkts [][]byte) error {
	for _, p := range pkts {
		*now = now.Add(step)
		if _, err := d.Write(p); err != nil {
			return err
		}
	}

	return nil
}

func TestScrambleDetector(t *testing.T) {
	t.Parallel()

	const windows, hold = 2, 10 * time.Second
	tests := []struct {
		name string
		pkts [][]byte
		// step は、パケット 1 つごとに進める時間。
		step time.Duration
		// finish なら、書き終えた後に Finish で決める。
		finish bool
		want   ts.ScrambleVerdict
	}{
		{
			name: "only the packets before the first window filled are scrambled",
			pkts: scrambleStream(10*scrambleWindow, func(i int) bool { return i < scrambleWindow/2+10 }),
			step: time.Millisecond, finish: false, want: ts.ScrambleClear,
		},
		{
			name: "all scrambled for enough windows past the hold",
			pkts: scrambleStream(10*scrambleWindow, always),
			step: 20 * time.Millisecond, finish: false, want: ts.ScrambleScrambled,
		},
		{
			name: "all scrambled for many windows but within the hold",
			pkts: scrambleStream(10*scrambleWindow, always),
			step: time.Millisecond, finish: false, want: ts.ScrambleUndecided,
		},
		{
			// 窓 1 つが抑止時間を越えて埋まるビットレートの低い局。窓が続ける数に届くまで決めない。
			name: "all scrambled past the hold but in fewer windows than required",
			pkts: scrambleStream(scrambleWindow+scrambleWindow/2, always),
			step: hold / scrambleWindow * 2, finish: false, want: ts.ScrambleUndecided,
		},
		{
			name: "plain sections do not dilute the ratio",
			pkts: func() [][]byte {
				pkts := scrambleHeader()
				for i := range 10 * scrambleWindow {
					pkts = append(pkts, esPacket(videoPID, byte(i), true, true))
					for range 3 {
						pkts = append(pkts, esPacket(ecmPID, byte(i), false, true), esPacket(eitPID, byte(i), false, true))
					}
				}

				return pkts
			}(),
			step: 5 * time.Millisecond, finish: false, want: ts.ScrambleScrambled,
		},
		{
			name: "packets without a payload are not counted",
			pkts: func() [][]byte {
				pkts := scrambleHeader()
				for i := range 10 * scrambleWindow {
					pkts = append(pkts, esPacket(videoPID, byte(i), true, false))
				}

				return pkts
			}(),
			step: 20 * time.Millisecond, finish: true, want: ts.ScrambleUndecided,
		},
		{
			name: "packets with the transport error indicator are not counted",
			pkts: func() [][]byte {
				pkts := scrambleHeader()
				for i := range 10 * scrambleWindow {
					p := esPacket(videoPID, byte(i), true, true)
					p[1] |= 0x80
					pkts = append(pkts, p)
				}

				return pkts
			}(),
			step: 20 * time.Millisecond, finish: true, want: ts.ScrambleUndecided,
		},
		{
			name: "packets before the PMT are not counted",
			pkts: func() [][]byte {
				pkts := make([][]byte, 0, 10*scrambleWindow)
				for i := range 10 * scrambleWindow {
					pkts = append(pkts, esPacket(videoPID, byte(i), true, true))
				}

				return pkts
			}(),
			step: 20 * time.Millisecond, finish: true, want: ts.ScrambleUndecided,
		},
		{
			name: "the encoder exits with a partial scrambled window",
			pkts: scrambleStream(scrambleWindow/2, always),
			step: time.Millisecond, finish: true, want: ts.ScrambleScrambled,
		},
		{
			name: "the encoder exits with a short partial window after a filled scrambled window",
			pkts: scrambleStream(scrambleWindow+scrambleWindow/10, func(i int) bool { return i < scrambleWindow }),
			step: time.Millisecond, finish: true, want: ts.ScrambleScrambled,
		},
		{
			name: "the encoder exits with a partial plain window",
			pkts: scrambleStream(scrambleWindow/2, func(int) bool { return false }),
			step: time.Millisecond, finish: true, want: ts.ScrambleClear,
		},
		{
			name: "the encoder exits with too few packets",
			pkts: scrambleStream(scrambleWindow/10, always),
			step: time.Millisecond, finish: true, want: ts.ScrambleUndecided,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
			called := 0
			d := ts.NewScrambleDetector(ts.ScrambleDetectorOptions{
				WindowPackets: scrambleWindow, Windows: windows, Hold: hold, MinPartialPackets: scrambleWindow / 5,
				Now: func() time.Time { return now }, OnScrambled: func() { called++ },
			})
			err := feedDetector(d, &now, tt.step, tt.pkts)
			got := d.Verdict()
			if tt.finish {
				got = d.Finish()
			}
			assert.Equal(t, tt.want, got)
			if tt.want == ts.ScrambleScrambled && !tt.finish {
				require.ErrorIs(t, err, ts.ErrScrambled)
				assert.Equal(t, 1, called)
			} else {
				require.NoError(t, err)
				assert.Zero(t, called)
			}
		})
	}
}

// TestScrambleDetectorStreakStart は、続き始めの時刻を、閾値を超えた最初の窓が埋まった時刻ではなく、その 1 個目のパケットを数えた時刻にすることを確かめる。
func TestScrambleDetectorStreakStart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// gap は、最初の窓の 1 個目のパケットの後に空ける時間。
		gap  time.Duration
		want ts.ScrambleVerdict
	}{
		{name: "the hold counts from the first packet of the window", gap: 9 * time.Second, want: ts.ScrambleScrambled},
		{name: "the hold has not passed yet", gap: 5 * time.Second, want: ts.ScrambleUndecided},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
			d := ts.NewScrambleDetector(ts.ScrambleDetectorOptions{
				WindowPackets: scrambleWindow, Windows: 2, Hold: 10 * time.Second, MinPartialPackets: 0,
				Now: func() time.Time { return now }, OnScrambled: nil,
			})
			pkts := scrambleStream(2*scrambleWindow, always)
			header := len(pkts) - 2*scrambleWindow
			require.NoError(t, feedDetector(d, &now, 0, pkts[:header+1]))
			now = now.Add(tt.gap)
			// 窓が埋まった時刻から数えると、抑止時間に届かない。
			_ = feedDetector(d, &now, 10*time.Millisecond, pkts[header+1:])
			assert.Equal(t, tt.want, d.Verdict())
		})
	}
}

// TestScrambleDetectorChunkedWrites は、パケットの途中で区切った書き込みでも、同じように数えることを確かめる。
func TestScrambleDetectorChunkedWrites(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		chunk int
	}{
		{name: "one byte", chunk: 1},
		{name: "not aligned", chunk: 1000},
		{name: "aligned", chunk: ts.PacketSize * 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := ts.NewScrambleDetector(ts.ScrambleDetectorOptions{
				WindowPackets: scrambleWindow, Windows: 0, Hold: 0, MinPartialPackets: scrambleWindow / 5, Now: nil, OnScrambled: nil,
			})
			data := join(scrambleStream(scrambleWindow/2, always))
			for len(data) > 0 {
				n := min(tt.chunk, len(data))
				_, err := d.Write(data[:n])
				require.NoError(t, err)
				data = data[n:]
			}
			assert.Equal(t, ts.ScrambleScrambled, d.Finish())
		})
	}
}

func TestAnalyzerPayloadCounts(t *testing.T) {
	t.Parallel()

	withError := esPacket(videoPID, 3, true, true)
	withError[1] |= 0x80
	pkts := [][]byte{
		esPacket(videoPID, 0, true, true),
		esPacket(videoPID, 1, false, true),
		esPacket(videoPID, 1, true, false),
		esPacket(videoPID, 2, false, true),
		withError,
	}
	a := ts.NewAnalyzer(nil)
	_, err := a.Write(join(pkts))
	require.NoError(t, err)
	got := statsOf(a.Stats(), videoPID)
	assert.Equal(t, ts.PIDStats{PID: videoPID, Packets: 5, Drops: 0, Errors: 1, Scrambled: 2, Payloads: 3, ScrambledPayloads: 1}, got)
}

func TestJudgeRecordingScramble(t *testing.T) {
	t.Parallel()

	quality := func(kind string, payloads, scrambled int64) ts.PIDQuality {
		return ts.PIDQuality{
			PID: 0x0100, Kind: kind, Packets: payloads, Drops: 0, Errors: 0, Scrambled: scrambled,
			Payloads: new(payloads), ScrambledPayloads: new(scrambled), StreamType: nil, ComponentTag: nil,
		}
	}
	const many = ts.MinRecordingScramblePackets
	tests := []struct {
		name     string
		segments [][]ts.PIDQuality
		want     ts.RecordingScramble
	}{
		{
			name:     "decrypted",
			segments: [][]ts.PIDQuality{{quality(ts.PIDKindVideo, many, 10), quality(ts.PIDKindAudio, many/10, 0)}},
			want:     ts.RecordingScramble{Judged: true, Ratio: 10.0 / (many + many/10), Warning: false},
		},
		{
			name:     "still scrambled",
			segments: [][]ts.PIDQuality{{quality(ts.PIDKindVideo, many, many), quality(ts.PIDKindOther, many/10, many/10)}},
			want:     ts.RecordingScramble{Judged: true, Ratio: 1, Warning: true},
		},
		{
			name: "plain sections do not dilute the ratio",
			segments: [][]ts.PIDQuality{{
				quality(ts.PIDKindVideo, many, many), quality(ts.PIDKindPSI, 10*many, 0), quality(ts.PIDKindUnknown, 10*many, 0),
			}},
			want: ts.RecordingScramble{Judged: true, Ratio: 1, Warning: true},
		},
		{
			name:     "too few packets",
			segments: [][]ts.PIDQuality{{quality(ts.PIDKindVideo, many-1, many-1)}},
			want:     ts.RecordingScramble{Judged: false, Ratio: 0, Warning: false},
		},
		{
			name: "the card stopped working in a later segment",
			segments: [][]ts.PIDQuality{
				{quality(ts.PIDKindVideo, 4*many, 0)},
				{quality(ts.PIDKindVideo, many, many)},
			},
			want: ts.RecordingScramble{Judged: true, Ratio: 0.2, Warning: true},
		},
		{
			name: "a short scrambled segment is not judged on its own",
			segments: [][]ts.PIDQuality{
				{quality(ts.PIDKindVideo, 4*many, 0)},
				{quality(ts.PIDKindVideo, many-1, many-1)},
			},
			want: ts.RecordingScramble{Judged: true, Ratio: float64(many-1) / float64(5*many-1), Warning: false},
		},
		{
			name: "segments recorded before the payload counts were added",
			segments: [][]ts.PIDQuality{
				nil,
				{{
					PID: 0x0100, Kind: ts.PIDKindVideo, Packets: 2 * many, Drops: 0, Errors: many / 2, Scrambled: many,
					Payloads: nil, ScrambledPayloads: nil, StreamType: nil, ComponentTag: nil,
				}},
			},
			want: ts.RecordingScramble{Judged: true, Ratio: float64(many) / (2*many - many/2), Warning: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ts.JudgeRecordingScramble(tt.segments)
			assert.Equal(t, tt.want.Judged, got.Judged)
			assert.InDelta(t, tt.want.Ratio, got.Ratio, 1e-9)
			assert.Equal(t, tt.want.Warning, got.Warning)
		})
	}
}
