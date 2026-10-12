package ts_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/ts"
)

// joinWindow は、テストで重なりを探す範囲。testdata/ts/test.ts の全体が入る大きさにする。
const joinWindow = 4 << 20

// shiftPATCC は、PAT のパケットの continuity_counter を delta だけずらした複製を返す。
// チューナーサーバーがクライアントごとに PAT を作り直すと、continuity_counter だけが違うストリームになる。
func shiftPATCC(t *testing.T, b []byte, delta byte) []byte {
	t.Helper()

	shifted := bytes.Clone(b)
	for i := 0; i+ts.PacketSize <= len(shifted); i += ts.PacketSize {
		p := shifted[i : i+ts.PacketSize]
		if uint16(p[1]&0x1F)<<8|uint16(p[2]) == 0 {
			p[3] = p[3]&0xF0 | (p[3]+delta)&0x0F
		}
	}

	return shifted
}

// patCount は、b の中の PAT のパケットの数を返す。
func patCount(b []byte) int {
	count := 0
	for i := 0; i+ts.PacketSize <= len(b); i += ts.PacketSize {
		if b[i+1]&0x1F == 0 && b[i+2] == 0 {
			count++
		}
	}

	return count
}

// joinSegments は、segments を順につなぎ、つないだものを返す。overlapping[i] が true なら、i 番目を前と重なるものとしてつなぐ。
// window は重なりを探す範囲 (バイト) で、0 ならテストの TS の全体が入る大きさにする。
func joinSegments(t *testing.T, segments [][]byte, overlapping []bool, window int) ([]byte, error) {
	t.Helper()

	out, err := os.Create(filepath.Join(t.TempDir(), "joined.m2ts"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = out.Close() })

	if window == 0 {
		window = joinWindow
	}
	joiner := ts.NewJoiner(out, window)
	for i, segment := range segments {
		if overlapping[i] {
			if err := joiner.AppendOverlapping(bytes.NewReader(segment)); err != nil {
				return nil, err
			}

			continue
		}
		require.NoError(t, joiner.Append(bytes.NewReader(segment)))
	}
	joined, err := os.ReadFile(out.Name())
	require.NoError(t, err)
	assert.Equal(t, int64(len(joined)), joiner.Written())

	return joined, nil
}

func TestJoiner(t *testing.T) {
	t.Parallel()

	original, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)
	packets := len(original) / ts.PacketSize
	at := func(n int) int { return n * ts.PacketSize }
	require.Positive(t, patCount(original[at(packets/3):at(packets*2/3)]), "the overlap must contain PAT packets")

	// 重なりに入る、PAT でないパケットを壊した複製を返す。
	corrupt := func(b []byte, index int) []byte {
		corrupted := bytes.Clone(b)
		isPAT := func(i int) bool { return corrupted[at(i)+1]&0x1F == 0 && corrupted[at(i)+2] == 0 }
		for isPAT(index) {
			index++
		}
		corrupted[at(index)+100] ^= 0xFF

		return corrupted
	}

	tests := []struct {
		name        string
		segments    [][]byte
		overlapping []bool
		// window は重なりを探す範囲。0 なら、テストの TS の全体が入る大きさ。
		window  int
		want    []byte
		wantErr error
	}{
		{
			name: "引き継ぎで重なった 2 つは、録り続けたものと同じになる",
			// 前は 0〜2/3、次は 1/3〜最後。次は別のクライアントなので、PAT の continuity_counter が違う。
			segments:    [][]byte{original[:at(packets*2/3)], shiftPATCC(t, original[at(packets/3):], 5)},
			overlapping: []bool{false, true},
			want:        original,
		},
		{
			name: "3 つでも同じになる",
			segments: [][]byte{
				original[:at(packets/2)],
				shiftPATCC(t, original[at(packets/4):at(packets*3/4)], 3),
				shiftPATCC(t, original[at(packets*3/5):], 11),
			},
			overlapping: []bool{false, true, true},
			want:        original,
		},
		{
			name: "重なりを探す範囲がセグメントより小さく、パケットの大きさで割り切れなくても、位置がずれない",
			// 重なりは 1/3 ほど。範囲は、重なりが入り、かつ 188 で割り切れない大きさにする。
			segments:    [][]byte{original[:at(packets*2/3)], shiftPATCC(t, original[at(packets*3/5):], 9)},
			overlapping: []bool{false, true},
			window:      at(packets/5) + 101,
			want:        original,
		},
		{
			name:        "前の末尾の、パケットに満たない端は捨てる",
			segments:    [][]byte{original[:at(packets*2/3)+50], original[at(packets/3):]},
			overlapping: []bool{false, true},
			want:        original,
		},
		{
			name:        "時間が重ならない 2 つは、そのまま後ろにつなぐ",
			segments:    [][]byte{original[:at(packets/3)], original[at(packets*2/3):]},
			overlapping: []bool{false, false},
			want:        append(bytes.Clone(original[:at(packets/3)]), original[at(packets*2/3):]...),
		},
		{
			name:        "重なりに違うパケットがあれば、つながない",
			segments:    [][]byte{original[:at(packets*2/3)], corrupt(original[at(packets/3):], packets/6)},
			overlapping: []bool{false, true},
			wantErr:     ts.ErrNoOverlap,
		},
		{
			name:        "重なっているはずなのに離れていれば、つながない",
			segments:    [][]byte{original[:at(packets/3)], original[at(packets*2/3):]},
			overlapping: []bool{false, true},
			wantErr:     ts.ErrNoOverlap,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := joinSegments(t, tt.segments, tt.overlapping, tt.window)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			// 違いが出たときに巨大な出力にならないよう、長さと一致だけを比べる。
			assert.Len(t, got, len(tt.want))
			assert.True(t, bytes.Equal(tt.want, got), "the joined stream differs from the expected one")
		})
	}
}
