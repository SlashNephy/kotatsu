package transcode

import (
	"time"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
)

// MinChapter は、チャプターにしない短い区間の長さ。
const MinChapter = minChapter

// BuildChapters は、録画を始めた日時 recordedFrom、途切れた区間 gaps、先頭を捨てた長さ head、MP4 の長さ duration で、buildChapters を呼ぶ。
func BuildChapters(sections []cmdetect.Section, recordedFrom time.Time, gaps []Gap, head, duration time.Duration) []Chapter {
	return buildChapters(sections, &chapterTimeline{recordedFrom: recordedFrom, gaps: gaps, head: head, duration: duration})
}

// FFMetadata は ffmetadata。
func FFMetadata(chapters []Chapter) []byte {
	return ffmetadata(chapters)
}

// ParseFieldDiffs は、FFmpeg の標準エラー出力 stderr を、chunk バイトずつ fieldDiffParser に書き、読んだ差を返す。
func ParseFieldDiffs(stderr string, chunk int) (*FieldDiffs, error) {
	parser := new(fieldDiffParser)
	for b := []byte(stderr); len(b) > 0; {
		n := min(chunk, len(b))
		_, _ = parser.Write(b[:n])
		b = b[n:]
	}

	return parser.diffs()
}
