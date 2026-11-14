package tvtplay_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
	"github.com/SlashNephy/kotatsu/internal/usecase/tvtplay"
)

// TestBuild は、CM の区間を、TvtPlay のスキップのチャプター (ix と ox) にすることを確かめる。
func TestBuild(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 11, 15, 21, 0, 0, 0, time.UTC)
	sec := func(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }
	tests := []struct {
		name     string
		sections []cmdetect.Section
		gaps     []transcode.Gap
		duration time.Duration
		want     string
	}{
		{
			name:     "CM がなければ、チャプターのないコマンドにする",
			sections: []cmdetect.Section{},
			gaps:     []transcode.Gap{},
			duration: sec(1800),
			want:     "c-c",
		},
		{
			name:     "CM の区間ごとに、スキップの開始と終了をミリ秒で置く",
			sections: []cmdetect.Section{{Start: sec(10), End: sec(40.5)}, {Start: sec(600), End: sec(660)}},
			gaps:     []transcode.Gap{},
			duration: sec(1800),
			want:     "c-10000cix-40500cox-600000cix-660000cox-c",
		},
		{
			name:     "隙間が 1 秒以内の区間は 1 つにまとめ、1 秒以下の区間は捨てる",
			sections: []cmdetect.Section{{Start: sec(10), End: sec(25)}, {Start: sec(25.8), End: sec(40)}, {Start: sec(100), End: sec(101)}},
			gaps:     []transcode.Gap{},
			duration: sec(1800),
			want:     "c-10000cix-40000cox-c",
		},
		{
			name:     "末尾まで続く CM は、終了をファイルの末尾にする",
			sections: []cmdetect.Section{{Start: sec(1770), End: sec(1799.5)}},
			gaps:     []transcode.Gap{},
			duration: sec(1800),
			want:     "c-1770000cix-0eox-c",
		},
		{
			name:     "途切れた区間の分を詰めた位置にする",
			sections: []cmdetect.Section{{Start: sec(600), End: sec(660)}},
			gaps:     []transcode.Gap{{From: from.Add(sec(100)), To: from.Add(sec(130))}},
			duration: sec(1770),
			want:     "c-570000cix-630000cox-c",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tvtplay.Build(tt.sections, &tvtplay.Timeline{RecordedFrom: from, Gaps: tt.gaps, Duration: tt.duration})
			assert.Equal(t, "\xef\xbb\xbf"+tt.want, string(got))
		})
	}
}

// TestFileName は、TS のファイルの名前の拡張子を .chapter に替えることを確かめる。
func TestFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ts   string
		want string
	}{
		{name: "拡張子を替える", ts: "2026-11-15 2100 番組.m2ts", want: "2026-11-15 2100 番組.chapter"},
		{name: "名前に点があっても、最後の拡張子だけを替える", ts: "foo.bar.ts", want: "foo.bar.chapter"},
		{name: "拡張子がなければ足す", ts: "foo", want: "foo.chapter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tvtplay.FileName(tt.ts))
		})
	}
}
