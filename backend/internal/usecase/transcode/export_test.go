package transcode

import (
	"time"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
)

// MinChapter は、チャプターにしない短い区間の長さ。
const MinChapter = minChapter

// PlaybackPosition は playbackPosition。
func PlaybackPosition(recordedFrom time.Time, gaps []Gap, at time.Time) time.Duration {
	return playbackPosition(recordedFrom, gaps, at)
}

// BuildChapters は、録画を始めた日時 recordedFrom、途切れた区間 gaps、先頭を捨てた長さ head、MP4 の長さ duration で、buildChapters を呼ぶ。
func BuildChapters(sections []cmdetect.Section, recordedFrom time.Time, gaps []Gap, head, duration time.Duration) []Chapter {
	return buildChapters(sections, &chapterTimeline{recordedFrom: recordedFrom, gaps: gaps, head: head, duration: duration})
}

// FFMetadata は ffmetadata。
func FFMetadata(chapters []Chapter) []byte {
	return ffmetadata(chapters)
}
