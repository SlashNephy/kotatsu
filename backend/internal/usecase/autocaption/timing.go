package autocaption

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Span は、音声の始まりからの時間の範囲。VAD が声とみなした区間に使う。
type Span struct {
	Start time.Duration
	End   time.Duration
}

const (
	// minCueDuration は、字幕を出す最短の長さ。whisper は、短い台詞を 0.02 秒のような長さで返すことがあり、そのままでは見えない。
	minCueDuration = time.Second
	// maxCueDuration は、字幕を出す最長の長さ。
	maxCueDuration = 7 * time.Second
	// baseReadTime と perCharReadTime は、字幕を読むのにかかる時間の見積もり (0.5 秒と 1 文字あたり 0.15 秒)。
	// 放送の字幕のおおよその速さ (1 秒に 4〜7 文字) に合わせた、仮の値 (#1682)。
	baseReadTime    = 500 * time.Millisecond
	perCharReadTime = 150 * time.Millisecond
)

// readTime は、text を読むのにかかる時間の見積もりを、minCueDuration から maxCueDuration に収めて返す。
func readTime(text string) time.Duration {
	d := baseReadTime + time.Duration(utf8.RuneCountInString(strings.TrimSpace(text)))*perCharReadTime

	return min(max(d, minCueDuration), maxCueDuration)
}

// FitCues は、whisper が返した字幕の時刻を、出して読みやすい範囲に直す (#1682)。
// whisper (kotoba-whisper) の時刻は、区切りの単位でしかなく、前の区切りの終わりから次の区切りの始まりまでをつなげて埋める。
// そのため、台詞の間の無音や音楽の分だけ、字幕が早く出て、長く残る。
//   - speech (VAD が声とみなした区間、始まりの順) が字幕の範囲に重なれば、字幕の始まりを、重なる最初の声の始まりまで遅らせ、
//     終わりを、その声の終わりか読む時間の見積もりの長い方にする。speech が nil なら、この修正はしない。
//   - 終わりは、whisper の終わりを超えず、始まりから読む時間の見積もり (最長 7 秒) を超えない。
//   - 短すぎる字幕は、次の字幕の始まりを超えない範囲で、最短 1 秒にのばす。
func FitCues(cues []Cue, speech []Span) []Cue {
	out := make([]Cue, 0, len(cues))
	for _, cue := range cues {
		start, end := cue.Start, cue.End
		limit := start + readTime(cue.Text)
		if span, ok := firstOverlap(speech, start, end); ok {
			start = max(start, span.Start)
			limit = max(start+readTime(cue.Text), min(span.End, start+maxCueDuration))
		}
		end = min(end, limit)
		out = append(out, Cue{Start: start, End: max(end, start), Text: cue.Text})
	}
	for i := range out {
		end := max(out[i].End, out[i].Start+minCueDuration)
		if i+1 < len(out) {
			// 次の字幕に重ねない。次の字幕が先に始まっていれば (whisper が重ねて返したとき)、今の終わりを縮めない。
			end = min(end, max(out[i+1].Start, out[i].End))
		}
		out[i].End = end
	}

	return out
}

// firstOverlap は、speech のうち、[start, end) に重なる最初の区間を返す。
func firstOverlap(speech []Span, start, end time.Duration) (Span, bool) {
	for _, span := range speech {
		if span.End > start && span.Start < max(end, start+time.Millisecond) {
			return span, true
		}
		if span.Start >= end {
			break
		}
	}

	return Span{Start: 0, End: 0}, false
}
