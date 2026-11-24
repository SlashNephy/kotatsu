package stream

import (
	"context"
	"time"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// OpenFile は、録画として登録していないファイル 1 本 (ライブラリのファイル。#1734) を、インデックス index を使って流すものとして開く。
// 録画の再生と同じく、Seek で求めた位置の手前の I フレームから流す。open は、流すたびにファイルを開く。
//
// indexing は、インデックスをまだ作っている途中かどうか。途中なら InProgress を true にし、kyusu の仮想 HLS のプレイリストに終わりのタグを付けない。
// インデックスのできた所より先は、最後の I フレームまでしかシークできない。ファイルはもう伸びないので、流すときは、インデックスによらず末尾まで流して終える。
func OpenFile(name string, open func(ctx context.Context) (File, error), index []ts.IndexEntry, indexing bool) *Recording {
	seg := &segment{
		id: uuid.Nil(), poolID: "", path: name, startedAt: time.Time{}, endedAt: nil, open: false,
		continues: false, continued: false, index: index, opener: open,
	}

	return &Recording{
		streamer: newStreamer(nil, 0), timeline: &fileTimeline{segment: seg}, InProgress: indexing,
		segments: []*segment{seg}, skip: 0,
	}
}

// fileTimeline は、ファイル 1 本だけの並び。次のセグメントは増えない。
type fileTimeline struct {
	segment *segment
}

var _ timeline = new(fileTimeline)

func (t *fileTimeline) load(context.Context) ([]*segment, error) {
	return []*segment{t.segment}, nil
}

func (t *fileTimeline) inProgress(context.Context) (bool, error) {
	return false, nil
}

func (t *fileTimeline) segmentClosed(context.Context, uuid.UUID) (bool, error) {
	return true, nil
}
