package recording_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// packets は、先頭のバイトを b にした TS のパケットを n 個つないで返す。どのパケットが残ったかを見分けるのに使う。
func packets(b byte, n int) []byte {
	out := bytes.Repeat([]byte{0xFF}, 188*n)
	for i := range n {
		out[i*188] = b
	}

	return out
}

// TestPreroll は、遅れた番組を待つ間に、直近の window の分だけを、TS のパケットの区切りで溜めることを確かめる (#1046)。
func TestPreroll(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 10, 10, 13, 30, 0, 0, time.UTC)
	type read struct {
		after time.Duration
		data  []byte
	}
	tests := []struct {
		name        string
		window      time.Duration
		reads       []read
		want        []byte
		wantStarted time.Time
	}{
		{
			name:   "window より前に読んだ分を捨てる",
			window: 15 * time.Second,
			reads: []read{
				{after: 0, data: packets(1, 2)},
				{after: 10 * time.Minute, data: packets(2, 1)},
				{after: 10*time.Minute + 5*time.Second, data: packets(3, 1)},
			},
			want:        append(packets(2, 1), packets(3, 1)...),
			wantStarted: base.Add(10 * time.Minute),
		},
		{
			name:   "パケットの途中で読み終えた分は、次に読んだ分とつなぐ",
			window: 15 * time.Second,
			reads: []read{
				{after: 0, data: packets(1, 1)[:100]},
				{after: time.Second, data: packets(1, 1)[100:]},
			},
			want:        packets(1, 1),
			wantStarted: base.Add(time.Second),
		},
		{
			name:   "パケットの途中までしか読んでいなくても、捨てずに返す",
			window: 15 * time.Second,
			reads: []read{
				{after: 0, data: packets(1, 1)[:100]},
			},
			want:        packets(1, 1)[:100],
			wantStarted: base.Add(time.Minute),
		},
		{
			name:        "何も読んでいなければ空",
			window:      15 * time.Second,
			reads:       []read{},
			want:        []byte{},
			wantStarted: base.Add(time.Minute),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := recording.NewPreroll(tt.window)
			for _, r := range tt.reads {
				p.Add(base.Add(r.after), r.data)
			}
			assert.Equal(t, tt.want, p.Bytes())
			assert.Equal(t, tt.wantStarted, p.StartedAt(base.Add(time.Minute)))
		})
	}
}
