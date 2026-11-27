package aribcaption

import (
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// Cue は、画面に出た ARIB 字幕 1 つ。Start と End は、録画を始めた日時からの時間。
type Cue struct {
	Start time.Duration
	End   time.Duration
	Text  string
}

// buildCues は、セグメント 1 つの字幕文 captions (ts.CaptionScanner が返したもの) をつなぎ、画面に出た字幕の区間にする (設計書 7.4)。
//
//   - 字幕の区間は、画面を消してから次に消すまでに書いた字幕文をつないだ 1 つの文にする。字幕の履歴が、画面を消すまでの字幕文を
//     つないで出すのと同じ文になる。始まりは最初の字幕文、終わりは次に画面を消した字幕文の時刻にする。
//   - 最後まで画面を消さなかった字幕は、セグメントの最後の映像 (lastVideoPTS) まで出ていたものとする。最後の映像が分からなければ、
//     最後の字幕文の時刻までにする。
//   - 文の前後の空白と改行は除き、空になった字幕は残さない。始まりは、空白でない文を書いた最初の字幕文の時刻にする。
//
// 時刻は、CM の区間と同じく、セグメントの最初の I フレームの PTS (base) を 0 にした再生の時間に、offset (録画を始めた日時からの
// セグメントの始まり) を足したものにする (#1811)。PTS は一周するので、字幕文ごとに前の字幕文との差を足していく。
// 録画を始めた日時より前に出た字幕は始まりを 0 にそろえ、それまでに終わった字幕は破棄する (字幕の自動生成と同じ)。
func buildCues(captions []ts.Caption, base int64, lastVideoPTS *int64, offset time.Duration) []Cue {
	cues := []Cue{}
	if len(captions) == 0 {
		return cues
	}
	toDuration := func(ticks int64) time.Duration { return time.Duration(ticks) * time.Second / ts.PTSClock }
	at := toDuration(ts.PTSDiff(base, captions[0].PTS))
	prev := captions[0].PTS
	var screen strings.Builder
	var start time.Duration
	emit := func(end time.Duration) {
		text := strings.TrimSpace(screen.String())
		screen.Reset()
		e := offset + end
		if text == "" || e <= 0 {
			return
		}
		// 画面を消す字幕文の PTS が、前の字幕文より戻っていても、終わりは始まりより前にしない。
		s := max(offset+start, 0)
		cues = append(cues, Cue{Start: s, End: max(e, s), Text: text})
	}
	for _, c := range captions {
		at += toDuration(ts.PTSDiff(prev, c.PTS))
		prev = c.PTS
		if c.Cleared {
			emit(at)
		}
		if c.Text == "" {
			continue
		}
		// 空白だけの字幕文の後に書いた文は、空白の時刻からではなく、文を書いた時刻から始める。
		if strings.TrimSpace(screen.String()) == "" {
			start = at
		}
		screen.WriteString(c.Text)
	}
	end := at
	if lastVideoPTS != nil {
		end = max(end, at+toDuration(ts.PTSDiff(prev, *lastVideoPTS)))
	}
	emit(end)

	return cues
}
