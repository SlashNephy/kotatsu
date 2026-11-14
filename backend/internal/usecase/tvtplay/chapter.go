// Package tvtplay は、TVTest のプラグインの TvtPlay が読む、CM のチャプターのファイル (.chapter) を作る (#1737)。
package tvtplay

import (
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
)

const (
	// mergeGap は、続いた CM とみなす、前の区間の終わりと次の区間の始まりの隙間。web の再生画面が CM を自動で飛ばすときの幅 (cmMergeGap、#1512) と、
	// 変換した MP4 のチャプターで捨てる短い区間 (#1510) に合わせる。これ以下の CM の区間も捨てる。
	mergeGap = time.Second
	// maxPosition は、TvtPlay が受け付けるチャプターの位置の上限 (99:59:59.999)。
	maxPosition = 99*time.Hour + 59*time.Minute + 59*time.Second + 999*time.Millisecond
	// skipIn と skipOut は、TvtPlay がその間を飛ばす、スキップの開始と終了のチャプターの名前。TvtPlay は "ix" と "ox" で始まる名前を、
	// スキップのチャプターとみなす。
	skipIn  = "ix"
	skipOut = "ox"
)

// bom は、UTF-8 の BOM。TvtPlay は、BOM 付きの UTF-8 だけを読む。
var bom = []byte{0xef, 0xbb, 0xbf}

// Timeline は、CM の区間 (録画を始めた日時からの時間) を、TS のファイルの始まりからの時間に直すのに要るもの。
type Timeline struct {
	// RecordedFrom と Gaps は、録画を始めた日時と、途切れた区間。
	RecordedFrom time.Time
	Gaps         []transcode.Gap
	// Duration は、TS のファイルの長さ (途切れた区間を詰めた再生の時間)。
	Duration time.Duration
}

// span は、TS のファイルの始まりからの、CM の区間。
type span struct {
	start time.Duration
	end   time.Duration
}

// Build は、CM の区間 sections (録画を始めた日時からの時間、始まりの順) から、TvtPlay の .chapter ファイルの中身を作る。
//
// 区間は、web の再生画面と変換した MP4 のチャプター (設計書 7.5、#1510) と同じく、途切れた区間を詰めた再生の時間に直す。
// 隙間が mergeGap 以下の区間は 1 つにまとめ、mergeGap 以下の区間は捨てる。区間ごとに、スキップの開始 (ix) と終了 (ox) のチャプターを置く。
// 終わりがファイルの末尾から mergeGap 以内なら、終了は末尾 (TvtPlay の "e") にする。CM がなければ、チャプターのない "c-c" を返す。
//
// 書式は、TvtPlay の _chwrite_test.cpp のチャプターコマンドの仕様に従う。"c-" で始め、チャプターごとに "{ミリ秒}c{名前}-" を足し、"c" で終える。
func Build(sections []cmdetect.Section, tl *Timeline) []byte {
	var b strings.Builder
	b.Write(bom)
	b.WriteString("c-")
	for _, s := range spans(sections, tl) {
		writeChapter(&b, strconv.FormatInt(s.start.Milliseconds(), 10)+"c", skipIn)
		if tl.Duration-s.end <= mergeGap {
			writeChapter(&b, "0e", skipOut)
		} else {
			writeChapter(&b, strconv.FormatInt(s.end.Milliseconds(), 10)+"c", skipOut)
		}
	}
	b.WriteString("c")

	return []byte(b.String())
}

// writeChapter は、位置 position (単位の文字まで) と名前 name のチャプターを 1 つ書く。
func writeChapter(b *strings.Builder, position, name string) {
	b.WriteString(position)
	b.WriteString(name)
	b.WriteByte('-')
}

// spans は、sections を TS のファイルの始まりからの時間に直し、まとめて、短いものを捨てる。
func spans(sections []cmdetect.Section, tl *Timeline) []span {
	position := func(offset time.Duration) time.Duration {
		at := transcode.PlaybackPosition(tl.RecordedFrom, tl.Gaps, tl.RecordedFrom.Add(offset))

		return min(at, tl.Duration, maxPosition)
	}
	merged := []span{}
	for _, section := range sections {
		start, end := position(section.Start), position(section.End)
		if end <= start {
			continue
		}
		if n := len(merged); n > 0 && start-merged[n-1].end <= mergeGap {
			merged[n-1].end = max(merged[n-1].end, end)

			continue
		}
		merged = append(merged, span{start: start, end: end})
	}
	out := make([]span, 0, len(merged))
	for _, s := range merged {
		if s.end-s.start > mergeGap {
			out = append(out, s)
		}
	}

	return out
}

// FileName は、TS のファイルの名前 tsName に対応する .chapter ファイルの名前を返す。TvtPlay は、foo.ts に対して foo.chapter を読む。
func FileName(tsName string) string {
	if i := strings.LastIndexByte(tsName, '.'); i > 0 {
		tsName = tsName[:i]
	}

	return tsName + ".chapter"
}
