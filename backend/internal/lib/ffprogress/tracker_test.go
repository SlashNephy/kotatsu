package ffprogress_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ffprogress"
)

// step は、-progress のまとまりが届いた時刻 (始まりからの秒) と、そのときに流した入力のバイト数。
type step struct {
	at    time.Duration
	frame string
	read  int64
	end   bool
}

// progressOutput は、FFmpeg の -progress の出力を模した、step のまとまりの並び。
func progressOutput(steps []step) string {
	var b strings.Builder
	for _, s := range steps {
		if s.frame != "" {
			fmt.Fprintf(&b, "frame=%s\n", s.frame)
		}
		b.WriteString("fps=29.97\nstream_0_0_q=28.0\nbitrate=2963.4kbits/s\ntotal_size=1048576\nout_time_us=1000000\n")
		b.WriteString("dup_frames=0\ndrop_frames=0\nspeed=  20x\n")
		if s.end {
			b.WriteString("progress=end\n")
		} else {
			b.WriteString("progress=continue\n")
		}
	}

	return b.String()
}

// result は、まとまりごとの割合と残り時間 (秒、不明なら -1)。
type result struct {
	ratio     float64
	remaining float64
}

// TestTracker は、-progress の出力から、割合と残り時間を求めることを確かめる (#1511)。
func TestTracker(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	cfg := &ffprogress.Config{Cap: 0.99, Window: 60 * time.Second, MinWindow: 10 * time.Second}
	tests := []struct {
		name  string
		est   ffprogress.Estimate
		steps []step
		want  []result
	}{
		{
			name: "monotonic", est: ffprogress.Estimate{Frames: 1000, Bytes: 0},
			// 読み損ねて減ったコマ数では、下げない。
			steps: []step{{at: 0, frame: "100"}, {at: time.Second, frame: "200"}, {at: 2 * time.Second, frame: "150"}, {at: 3 * time.Second, frame: "300"}},
			want:  []result{{0.1, -1}, {0.2, -1}, {0.2, -1}, {0.3, -1}},
		},
		{
			name: "capped until the end", est: ffprogress.Estimate{Frames: 1000, Bytes: 0},
			// 見込みに届いても、溜めたコマの書き出しと mux の後処理が残るので、progress=end までは頭打ちの割合で止める。
			steps: []step{{at: 0, frame: "980"}, {at: time.Second, frame: "1000"}, {at: 2 * time.Second, frame: "1100"}},
			want:  []result{{0.98, -1}, {0.99, -1}, {0.99, -1}},
		},
		{
			name: "end", est: ffprogress.Estimate{Frames: 1000, Bytes: 0},
			// 見込みより少ないコマ数で終わっても、progress=end で 1 にし、残り時間を 0 にする。
			steps: []step{{at: 0, frame: "500"}, {at: time.Second, frame: "900", end: true}},
			want:  []result{{0.5, -1}, {1, 0}},
		},
		{
			name: "remaining needs the minimum window", est: ffprogress.Estimate{Frames: 1000, Bytes: 0},
			// 窓の幅が 10 秒に満たないうちは出さない。10 秒で 100 コマ進めば、残りの 800 コマは 80 秒。
			steps: []step{{at: 0, frame: "100"}, {at: 5 * time.Second, frame: "150"}, {at: 9 * time.Second, frame: "190"}, {at: 10 * time.Second, frame: "200"}},
			want:  []result{{0.1, -1}, {0.15, -1}, {0.19, -1}, {0.2, 80}},
		},
		{
			name: "remaining uses the recent window", est: ffprogress.Estimate{Frames: 1000, Bytes: 0},
			// 窓 (60 秒) より古い記録は使わない。始めの 60 秒は 1 秒に 1 コマ、その後の 60 秒は 1 秒に 5 コマなら、
			// 120 秒の時点の窓は 60 秒から 120 秒で、残りの 640 コマは 128 秒。
			steps: []step{{at: 0, frame: "0"}, {at: 60 * time.Second, frame: "60"}, {at: 120 * time.Second, frame: "360"}},
			want:  []result{{0, -1}, {0.06, 1000 - 60}, {0.36, 128}},
		},
		{
			name: "too small progress means unknown remaining", est: ffprogress.Estimate{Frames: 0, Bytes: 1 << 50},
			// 大きな入力で、窓の中でごくわずかしか進まないと、見込みが time.Duration に入りきらない。負に回さず、不明にする。
			steps: []step{{at: 0, frame: "", read: 1}, {at: 60 * time.Second, frame: "", read: 2}},
			want:  []result{{1.0 / (1 << 50), -1}, {2.0 / (1 << 50), -1}},
		},
		{
			name: "no progress means unknown remaining", est: ffprogress.Estimate{Frames: 1000, Bytes: 0},
			steps: []step{{at: 0, frame: "100"}, {at: 30 * time.Second, frame: "100"}},
			want:  []result{{0.1, -1}, {0.1, -1}},
		},
		{
			name: "falls back to the read position", est: ffprogress.Estimate{Frames: 0, Bytes: 1000},
			// 見込みのコマ数が出せなければ、流したバイト数を入力の大きさで割る。
			steps: []step{{at: 0, frame: "100", read: 250}, {at: time.Second, frame: "200", read: 500}},
			want:  []result{{0.25, -1}, {0.5, -1}},
		},
		{
			name: "frames keep the previous value without a frame", est: ffprogress.Estimate{Frames: 1000, Bytes: 1000},
			// コマ数で数えているときは、frame のないまとまりで読み位置に切り替えず、前の値を保つ。
			steps: []step{{at: 0, frame: "100", read: 900}, {at: time.Second, frame: "", read: 950}, {at: 2 * time.Second, frame: "N/A", read: 990}},
			want:  []result{{0.1, -1}, {0.1, -1}, {0.1, -1}},
		},
		{
			name: "no estimate keeps zero", est: ffprogress.Estimate{Frames: 0, Bytes: 0},
			steps: []step{{at: 0, frame: "100", read: 100}, {at: time.Second, frame: "200", read: 200}},
			want:  []result{{0, -1}, {0, -1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tracker := ffprogress.NewTracker(cfg, tt.est)
			got := []result{}
			i := 0
			parser := ffprogress.NewParser(func(b *ffprogress.Block) {
				s := tt.steps[i]
				i++
				snap := tracker.Observe(start.Add(s.at), b, s.read)
				remaining := -1.0
				if snap.Remaining != nil {
					remaining = snap.Remaining.Seconds()
				}
				got = append(got, result{ratio: snap.Ratio, remaining: remaining})
			}, nil)
			_, err := parser.Write([]byte(progressOutput(tt.steps)))
			require.NoError(t, err)
			require.Len(t, got, len(tt.want))
			for j := range tt.want {
				assert.InDelta(t, tt.want[j].ratio, got[j].ratio, 1e-9, "ratio of block %d", j)
				assert.InDelta(t, tt.want[j].remaining, got[j].remaining, 1e-6, "remaining of block %d", j)
			}
		})
	}
}

// TestParserOtherLines は、標準エラー出力に混ぜた -progress の行を、ほかのログと見分けることを確かめる (#1511)。
// CM の区間の見分けでは、標準出力をシーンの切り替わりに使うので、-progress を標準エラー出力に出させる。
func TestParserOtherLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantFrames []int64
		wantLines  []string
	}{
		{
			name: "silencedetect and progress",
			input: "[silencedetect @ 0x55f3643cb440] silence_start: 12.3\nframe=12\nspeed=1.5x\nprogress=continue\n" +
				"[silencedetect @ 0x55f3643cb440] silence_end: 13.1 | silence_duration: 0.8\nframe=24\nprogress=end\n",
			wantFrames: []int64{12, 24},
			wantLines:  []string{"[silencedetect @ 0x55f3643cb440] silence_start: 12.3", "[silencedetect @ 0x55f3643cb440] silence_end: 13.1 | silence_duration: 0.8"},
		},
		{
			// -nostats でも終わりに出る統計の行は、frame の値として読まない。
			name:       "final statistics line",
			input:      "frame=30\nprogress=continue\nframe=    0 fps=0.0 q=0.0 Lsize=       0KiB time=00:00:00.00\nprogress=end\nConversion failed!\r\n",
			wantFrames: []int64{30, -1},
			wantLines:  []string{"Conversion failed!"},
		},
		{
			name:       "line split across writes",
			input:      "fra",
			wantFrames: []int64{},
			wantLines:  []string{"fra"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			frames, lines := []int64{}, []string{}
			parser := ffprogress.NewParser(func(b *ffprogress.Block) { frames = append(frames, b.Frame) }, func(line string) { lines = append(lines, line) })
			// 1 バイトずつ書いても、行に分けて読む。
			for _, c := range []byte(tt.input) {
				_, err := parser.Write([]byte{c})
				require.NoError(t, err)
			}
			parser.Flush()
			assert.Equal(t, tt.wantFrames, frames)
			assert.Equal(t, tt.wantLines, lines)
		})
	}
}
