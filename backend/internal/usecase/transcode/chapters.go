package transcode

import (
	"bytes"
	"strconv"
	"time"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
)

const (
	// minChapter は、チャプターにしない短い区間の長さ (#1510)。これ以下の CM の区間は捨て、これ以下の本編は作らない。
	// チャプター送りで一瞬で飛ぶだけの区間を作らない長さにする。区間を選び直す見分け方 (#1506) の「捨てる短い区間」が決まるまでは、
	// 今の見分け方が、セグメントをまたいで続く CM の区間を 1 つにまとめる間隔 (1 秒、付録 A) に合わせる。
	minChapter = time.Second
	// chapterMain と chapterCM は、チャプターの名前。
	chapterMain = "本編"
	chapterCM   = "CM"
)

// Gap は、録画の途切れた区間 (recordings.gaps の要素)。区間の間は、ファイルに何も書いていない。
type Gap struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// Chapter は、MP4 に書くチャプター。Start と End は、MP4 の始まりからの時間。
type Chapter struct {
	Start time.Duration
	End   time.Duration
	Title string
}

// chapterTimeline は、CM の区間の時刻を、MP4 の時刻に直すのに要るもの。
type chapterTimeline struct {
	// recordedFrom と gaps は、録画を始めた日時と、途切れた区間。
	recordedFrom time.Time
	gaps         []Gap
	// head は、途切れた区間を詰めた再生の時間から、MP4 の時間を得るのに引く量。変換で先頭を捨てた長さ (headOffset)。
	head time.Duration
	// duration は、MP4 の長さ。
	duration time.Duration
}

// playbackPosition は、日時 at に当たる、録画の始まりからの再生の時間を返す。途切れた区間の分の映像はファイルにないので、
// それより前に終わった区間の長さを引く。区間の中の日時は、区間の始まりの位置にする。
// recordedFrom より前の区間 (遅れて録り始めたときの、冒頭の欠け) は、位置の基準の前なので引かない。
// web の再生画面が CM の区間を再生位置に直す recordingPosition (frontend/src/yunomi/position.ts、#1427) と同じ対応にし、
// 画面で飛ばす位置と、MP4 のチャプターの位置をそろえる (設計書 7.2)。
// ただし、FFmpeg は PTS が 10 秒を超えて飛んだところだけを詰めるので、10 秒以下の途切れは MP4 に残り、その後のチャプターは早く入る (既知の制約)。
func playbackPosition(recordedFrom time.Time, gaps []Gap, at time.Time) time.Duration {
	var missing time.Duration
	for _, gap := range gaps {
		from := gap.From
		if from.Before(recordedFrom) {
			from = recordedFrom
		}
		if from.Before(at) {
			to := gap.To
			if at.Before(to) {
				to = at
			}
			missing += max(to.Sub(from), 0)
		}
	}

	return max(at.Sub(recordedFrom)-missing, 0)
}

// buildChapters は、録画の CM の区間 sections (録画を始めた日時からの時間、始まりの順) から、MP4 のチャプターを作る (#1510)。
// CM の区間と、その裏返しの本編を、交互に並べる。CM がなければ空のスライスを返す (チャプターを外す)。
//
// 区間は、途切れた区間を詰めた再生の時間に直し、変換で先頭を捨てた分 (tl.head) だけ前に詰める。始まりが負になったら 0 にし、
// 終わりは MP4 の長さまでにする。詰めて minChapter 以下になった CM の区間は捨てる。CM の間の本編が minChapter 以下なら、
// 本編を作らずに前後の CM を 1 つにし、先頭か末尾の本編が minChapter 以下なら、CM を端まで延ばす。
func buildChapters(sections []cmdetect.Section, tl *chapterTimeline) []Chapter {
	toMP4 := func(offset time.Duration) time.Duration {
		at := playbackPosition(tl.recordedFrom, tl.gaps, tl.recordedFrom.Add(offset)) - tl.head

		return min(at, tl.duration)
	}
	chapters := []Chapter{}
	var cursor time.Duration
	for _, section := range sections {
		// 始まりは、前のチャプターの終わり (最初は 0) より前にしない。詰めて負になった始まりは、0 になる。
		start, end := max(toMP4(section.Start), cursor), toMP4(section.End)
		if end-start <= minChapter {
			continue
		}
		n := len(chapters)
		switch {
		case start-cursor > minChapter:
			chapters = append(chapters, Chapter{Start: cursor, End: start, Title: chapterMain})
		case n > 0:
			// 前は CM。間の短い本編ごと、前の CM につなぐ。
			chapters[n-1].End = end
			cursor = end

			continue
		default:
			// 先頭の短い本編は作らず、CM を 0 から始める。
			start = cursor
		}
		chapters = append(chapters, Chapter{Start: start, End: end, Title: chapterCM})
		cursor = end
	}
	if len(chapters) == 0 {
		return chapters
	}
	if tl.duration-cursor > minChapter {
		chapters = append(chapters, Chapter{Start: cursor, End: tl.duration, Title: chapterMain})
	} else {
		chapters[len(chapters)-1].End = tl.duration
	}

	return chapters
}

// ffmetadata は、chapters を FFmpeg の ffmetadata 形式にする。時刻はミリ秒で書く。
func ffmetadata(chapters []Chapter) []byte {
	var b bytes.Buffer
	b.WriteString(";FFMETADATA1\n")
	for _, c := range chapters {
		b.WriteString("[CHAPTER]\nTIMEBASE=1/1000\n")
		b.WriteString("START=" + strconv.FormatInt(c.Start.Milliseconds(), 10) + "\n")
		b.WriteString("END=" + strconv.FormatInt(c.End.Milliseconds(), 10) + "\n")
		// 名前は固定の「本編」と「CM」なので、ffmetadata で逃がす記号 (=;#\ と改行) を含まない。
		b.WriteString("title=" + c.Title + "\n")
	}

	return b.Bytes()
}
