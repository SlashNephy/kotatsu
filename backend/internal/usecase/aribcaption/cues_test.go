package aribcaption_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/aribcaption"
)

// sec は、PTS の 1 秒 (90 kHz)。
const sec = ts.PTSClock

// ptsMax は、PTS (33 ビット) が一周する値。
const ptsMax = 1 << 33

// lastLine は、最後まで画面を消さなかった字幕の文。
const lastLine = "おしまい"

// TestBuildCues は、画面を消すまでの字幕文をつないで字幕の区間にし、最初の I フレームを 0 にした時間に、セグメントの始まりを足すことを確認する。
func TestBuildCues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		captions  []ts.Caption
		base      int64
		lastVideo *int64
		offset    time.Duration
		want      []aribcaption.Cue
	}{
		{
			name: "画面を消すまでの字幕文をつなぎ、次に消した時刻で終える",
			captions: []ts.Caption{
				{PTS: 10 * sec, Text: "", Cleared: true},
				{PTS: 12 * sec, Text: "こんにちは", Cleared: true},
				{PTS: 13 * sec, Text: "\nさようなら", Cleared: false},
				{PTS: 15 * sec, Text: "またね", Cleared: true},
				{PTS: 17 * sec, Text: "", Cleared: true},
			},
			base: 10 * sec, offset: time.Minute,
			want: []aribcaption.Cue{
				{Start: time.Minute + 2*time.Second, End: time.Minute + 5*time.Second, Text: "こんにちは\nさようなら"},
				{Start: time.Minute + 5*time.Second, End: time.Minute + 7*time.Second, Text: "またね"},
			},
		},
		{
			name: "最後まで消さなかった字幕は、最後の映像まで出す",
			captions: []ts.Caption{
				{PTS: 12 * sec, Text: lastLine, Cleared: true},
			},
			base: 10 * sec, lastVideo: new(int64(40 * sec)),
			want: []aribcaption.Cue{{Start: 2 * time.Second, End: 30 * time.Second, Text: lastLine}},
		},
		{
			name: "最後の映像が分からなければ、最後の字幕文の時刻までにする",
			captions: []ts.Caption{
				{PTS: 12 * sec, Text: lastLine, Cleared: true},
			},
			base: 10 * sec,
			want: []aribcaption.Cue{{Start: 2 * time.Second, End: 2 * time.Second, Text: lastLine}},
		},
		{
			name: "前後の空白と改行は除き、空になった字幕は残さない",
			captions: []ts.Caption{
				{PTS: 11 * sec, Text: "　", Cleared: true},
				{PTS: 12 * sec, Text: "\n台詞 ", Cleared: true},
				{PTS: 14 * sec, Text: "", Cleared: true},
			},
			base: 10 * sec,
			want: []aribcaption.Cue{{Start: 2 * time.Second, End: 4 * time.Second, Text: "台詞"}},
		},
		{
			name: "録画を始める前に出た字幕は 0 から、それまでに終わった字幕は破棄する",
			captions: []ts.Caption{
				{PTS: 2 * sec, Text: "前", Cleared: true},
				{PTS: 5 * sec, Text: "またぐ", Cleared: true},
				{PTS: 20 * sec, Text: "", Cleared: true},
			},
			base: 10 * sec,
			want: []aribcaption.Cue{{Start: 0, End: 10 * time.Second, Text: "またぐ"}},
		},
		{
			name: "PTS が一周しても、前の字幕文からの差で時刻を進める",
			captions: []ts.Caption{
				{PTS: ptsMax - 2*sec, Text: "一周の前", Cleared: true},
				{PTS: 3 * sec, Text: "一周の後", Cleared: true},
				{PTS: 6 * sec, Text: "", Cleared: true},
			},
			base: ptsMax - 4*sec,
			want: []aribcaption.Cue{
				{Start: 2 * time.Second, End: 7 * time.Second, Text: "一周の前"},
				{Start: 7 * time.Second, End: 10 * time.Second, Text: "一周の後"},
			},
		},
		{
			name: "画面を消す字幕文の PTS が戻っても、終わりは始まりより前にしない",
			captions: []ts.Caption{
				{PTS: 20 * sec, Text: "未来", Cleared: true},
				{PTS: 18 * sec, Text: "", Cleared: true},
			},
			base: 10 * sec,
			want: []aribcaption.Cue{{Start: 10 * time.Second, End: 10 * time.Second, Text: "未来"}},
		},
		{
			name: "空白だけの字幕文の後に書いた文は、文を書いた時刻から始める",
			captions: []ts.Caption{
				{PTS: 11 * sec, Text: "　", Cleared: true},
				{PTS: 13 * sec, Text: "台詞", Cleared: false},
				{PTS: 15 * sec, Text: "", Cleared: true},
			},
			base: 10 * sec,
			want: []aribcaption.Cue{{Start: 3 * time.Second, End: 5 * time.Second, Text: "台詞"}},
		},
		{
			name: "字幕文がなければ、区間もない",
			base: 10 * sec,
			want: []aribcaption.Cue{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, aribcaption.BuildCues(tt.captions, tt.base, tt.lastVideo, tt.offset))
		})
	}
}
