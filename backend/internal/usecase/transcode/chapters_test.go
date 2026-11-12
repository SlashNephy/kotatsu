package transcode_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
)

// TestPlaybackPosition は、日時を、途切れた区間を詰めた再生の時間にすることを確かめる。
// web の再生画面の recordingPosition (frontend/src/yunomi/position.test.ts) と同じ例で、同じ値になることを確かめる (#1510)。
func TestPlaybackPosition(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	at := func(h, m, s int) time.Time { return time.Date(2026, 10, 14, h, m, s, 0, time.UTC) }
	gap := transcode.Gap{From: at(12, 10, 0), To: at(12, 12, 0)}
	tests := []struct {
		name string
		gaps []transcode.Gap
		at   time.Time
		want time.Duration
	}{
		{name: "no gap", gaps: []transcode.Gap{}, at: at(12, 1, 30), want: 90 * time.Second},
		{name: "before the gap", gaps: []transcode.Gap{gap}, at: at(12, 9, 0), want: 9 * time.Minute},
		{name: "after the gap", gaps: []transcode.Gap{gap}, at: at(12, 13, 0), want: 11 * time.Minute},
		{name: "inside the gap", gaps: []transcode.Gap{gap}, at: at(12, 11, 0), want: 10 * time.Minute},
		{name: "before the start", gaps: []transcode.Gap{}, at: at(11, 59, 0), want: 0},
		{
			// 遅れて録り始めたときの冒頭の欠けは、基準 (書き始めた時刻) より前なので引かない。
			name: "gap before the start", gaps: []transcode.Gap{{From: at(11, 59, 50), To: at(12, 0, 0)}}, at: at(12, 1, 0), want: time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, transcode.PlaybackPosition(from, tt.gaps, tt.at))
		})
	}
}

// TestBuildChapters は、CM の区間から、本編と CM を交互に並べたチャプターを作ることを確かめる (#1510)。
func TestBuildChapters(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	sec := func(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }
	section := func(start, end float64) cmdetect.Section { return cmdetect.Section{Start: sec(start), End: sec(end)} }
	main := func(start, end float64) transcode.Chapter {
		return transcode.Chapter{Start: sec(start), End: sec(end), Title: "本編"}
	}
	cm := func(start, end float64) transcode.Chapter {
		return transcode.Chapter{Start: sec(start), End: sec(end), Title: "CM"}
	}
	const duration = 200
	tests := []struct {
		name     string
		sections []cmdetect.Section
		gaps     []transcode.Gap
		head     time.Duration
		want     []transcode.Chapter
	}{
		{name: "no CM", sections: []cmdetect.Section{}, want: []transcode.Chapter{}},
		{
			name:     "alternating",
			sections: []cmdetect.Section{section(10, 40), section(100, 130)},
			want:     []transcode.Chapter{main(0, 10), cm(10, 40), main(40, 100), cm(100, 130), main(130, duration)},
		},
		{
			// CM の間の 1 秒以下の本編は作らず、前後の CM を 1 つにする。
			name:     "short main between CMs",
			sections: []cmdetect.Section{section(10, 40), section(41, 70)},
			want:     []transcode.Chapter{main(0, 10), cm(10, 70), main(70, duration)},
		},
		{
			name:     "main just longer than the threshold",
			sections: []cmdetect.Section{section(10, 40), section(41.5, 70)},
			want:     []transcode.Chapter{main(0, 10), cm(10, 40), main(40, 41.5), cm(41.5, 70), main(70, duration)},
		},
		{
			name:     "short main at the head",
			sections: []cmdetect.Section{section(0.8, 30)},
			want:     []transcode.Chapter{cm(0, 30), main(30, duration)},
		},
		{
			name:     "short main at the tail",
			sections: []cmdetect.Section{section(170, 199.5)},
			want:     []transcode.Chapter{main(0, 170), cm(170, duration)},
		},
		{
			name:     "short CM",
			sections: []cmdetect.Section{section(10, 11), section(100, 130)},
			want:     []transcode.Chapter{main(0, 100), cm(100, 130), main(130, duration)},
		},
		{
			// 変換で先頭を捨てた分だけ、前に詰める。
			name:     "head",
			sections: []cmdetect.Section{section(10, 40)},
			head:     sec(5),
			want:     []transcode.Chapter{main(0, 5), cm(5, 35), main(35, duration)},
		},
		{
			// 詰めて負になった始まりは 0 にする。
			name:     "negative start",
			sections: []cmdetect.Section{section(10, 40)},
			head:     sec(15),
			want:     []transcode.Chapter{cm(0, 25), main(25, duration)},
		},
		{
			// 詰めて 1 秒以下になった区間は捨てる。CM が残らなければ、チャプターを外す。
			name:     "too short after the head",
			sections: []cmdetect.Section{section(10, 40)},
			head:     sec(39.5),
			want:     []transcode.Chapter{},
		},
		{
			// MP4 の映像が音声より後から始まれば (先頭を捨てた長さが負なら)、後ろにずらす。
			name:     "negative head",
			sections: []cmdetect.Section{section(10, 40)},
			head:     -sec(0.25),
			want:     []transcode.Chapter{main(0, 10.25), cm(10.25, 40.25), main(40.25, duration)},
		},
		{
			// 途切れた区間の分は、MP4 にないので詰める。区間の中の時刻は、区間の始まりの位置にする。
			name:     "gaps",
			sections: []cmdetect.Section{section(40, 60), section(65, 75), section(100, 130)},
			gaps:     []transcode.Gap{{From: from.Add(sec(50)), To: from.Add(sec(80))}},
			want:     []transcode.Chapter{main(0, 40), cm(40, 50), main(50, 70), cm(70, 100), main(100, duration)},
		},
		{
			name:     "beyond the end",
			sections: []cmdetect.Section{section(180, 260)},
			want:     []transcode.Chapter{main(0, 180), cm(180, duration)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, transcode.BuildChapters(tt.sections, from, tt.gaps, tt.head, sec(duration)))
		})
	}
}

// TestFFMetadata は、チャプターを、ffmetadata 形式のミリ秒で書くことを確かめる (#1510)。
func TestFFMetadata(t *testing.T) {
	t.Parallel()

	chapters := []transcode.Chapter{
		{Start: 0, End: 10500 * time.Millisecond, Title: "本編"},
		{Start: 10500 * time.Millisecond, End: 40 * time.Second, Title: "CM"},
	}
	want := ";FFMETADATA1\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=10500\ntitle=本編\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=10500\nEND=40000\ntitle=CM\n"
	assert.Equal(t, want, string(transcode.FFMetadata(chapters)))
}
