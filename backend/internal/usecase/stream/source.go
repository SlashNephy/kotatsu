package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// kindSegment は、recording_files の、TS のセグメントの種類。
	kindSegment = "segment"
	// stateRecording は、録画中の録画の状態。
	stateRecording = "recording"
	// defaultPollInterval は、追っかけ再生で、録画中のファイルが伸びるのを確かめる間隔。
	defaultPollInterval = 500 * time.Millisecond
)

var (
	// ErrRecordingNotFound は、録画がないことを表す。
	ErrRecordingNotFound = errors.New("recording not found")
	// ErrNotMounted は、録画が、このインスタンスにマウントしていないプールにあることを表す。
	ErrNotMounted = errors.New("the recording is on a storage pool that this server does not mount")
	// ErrNoSegment は、録画に流せるセグメントがないことを表す。
	ErrNoSegment = errors.New("the recording has no segment to play")
)

// timeline は、連結して流すセグメントの並び。録画のセグメントか、タイムシフトのチャンクを、DB から引く。
type timeline interface {
	// load は、セグメントを、始まりの順に、索引と一緒に読む。
	load(ctx context.Context) ([]*segment, error)
	// inProgress は、セグメントがまだ増えるかどうかを返す。録画なら録画中かどうか。
	inProgress(ctx context.Context) (bool, error)
	// segmentClosed は、セグメントを書き終えたかどうかを返す。行が消えたら、書き終えたものとみなす。
	segmentClosed(ctx context.Context, segmentID uuid.UUID) (bool, error)
}

// streamer は、セグメントの並びを、指定した範囲から連結して流す。書いている途中のセグメントは、伸びるのを待ちながら流す。
type streamer struct {
	store *kago.Store
	// pollInterval は、追っかけ再生で、書いている途中のファイルが伸びるのを確かめる間隔。
	pollInterval time.Duration
}

// newStreamer は streamer を返す。pollInterval が 0 なら 500 ミリ秒にする。
func newStreamer(store *kago.Store, pollInterval time.Duration) *streamer {
	if pollInterval == 0 {
		pollInterval = defaultPollInterval
	}

	return &streamer{store: store, pollInterval: pollInterval}
}

// RecordingSource は、録画のセグメントを、索引を使って、指定した位置から連結して流す。
// 引き継ぎで重なった区間は切り詰める。録画中の録画は、ファイルの末尾まで読んだら、ファイルが伸びるのを待ちながら流し続ける。
// tenban の録画の配信と、kyusu の録画のトランスコードが使う。
type RecordingSource struct {
	db       sqlc.DBTX
	streamer *streamer
}

// NewRecordingSource は RecordingSource を返す。store は、このインスタンスにマウントされたプール。
// pollInterval が 0 なら 500 ミリ秒にする。
func NewRecordingSource(db sqlc.DBTX, store *kago.Store, pollInterval time.Duration) *RecordingSource {
	return &RecordingSource{db: db, streamer: newStreamer(store, pollInterval)}
}

// Recording は、開いた録画か、タイムシフトのバッファ。開いた時点のセグメントと索引を持つ。
type Recording struct {
	streamer *streamer
	timeline timeline
	// InProgress は、開いた時点で録画中だったかどうか。タイムシフトのバッファは、常に録画中として扱う。
	InProgress bool
	segments   []*segment
	// skip は、最初のセグメントの始まりから、再生の時間の起点までの時間。タイムシフトのバッファを、番組の頭から流すのに使う。
	// Seek と Position.At の時間は、ここからの時間で表す。起点が最初のセグメントの始まりより前なら負になる。録画では 0。
	skip time.Duration
}

// FileName は、保存させるときのファイル名。最初のセグメントのファイルの名前にする。
func (r *Recording) FileName() string {
	return path.Base(r.segments[0].path)
}

// Position は、録画を流し始める位置。
type Position struct {
	// At は、録画の始まりから、流し始める I フレームまでの時間。
	At     time.Duration
	pieces []*piece
}

// Open は、録画を開く。録画がなければ ErrRecordingNotFound、このインスタンスにマウントしていないプールにあれば ErrNotMounted、
// 流せるセグメントがなければ ErrNoSegment を返す。
func (s *RecordingSource) Open(ctx context.Context, recordingID uuid.UUID) (*Recording, error) {
	recording, err := sqlc.New(s.db).GetRecording(ctx, recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRecordingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the recording: %w", err)
	}
	tl := &recordingTimeline{source: s, id: recordingID}
	segments, err := tl.load(ctx)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		return nil, ErrNoSegment
	}

	return &Recording{streamer: s.streamer, timeline: tl, InProgress: recording.State == stateRecording, segments: segments, skip: 0}, nil
}

// Seek は、録画の始まりから start の位置の手前の I フレームを、流し始める位置として返す。
// 起点より手前の I フレームから流すときは、At を 0 にする。
func (r *Recording) Seek(start time.Duration) *Position {
	pieces, at := plan(r.segments, start+r.skip)

	return &Position{At: max(at-r.skip, 0), pieces: pieces}
}

// Keyframes は、流す I フレームの、録画の始まりからの時間を、順に返す。引き継ぎで切り詰める区間の I フレームは含めない。
// 索引のないセグメントは、その始まりだけを含める。
func (r *Recording) Keyframes() []time.Duration {
	return keyframes(r.segments)
}

// KeyframePoints は、Keyframes と同じ I フレームの、録画の始まりからの時間と PTS を、順に返す。
// kyusu が、録画の映像を変換せずに配信するとき、I フレームの位置でセグメントを切るのに使う (#806)。
func (r *Recording) KeyframePoints() []Keyframe {
	return keyframePoints(r.segments)
}

// Indexed は、索引のあるセグメントがあるかどうかを返す。取り込んだ直後の録画は、解析するまで索引がない。
func (r *Recording) Indexed() bool {
	return slices.ContainsFunc(r.segments, func(seg *segment) bool { return len(seg.index) > 0 })
}

// Write は、pos から録画を w に流す。録画中なら、録画が終わるまで流し続ける。
func (r *Recording) Write(ctx context.Context, w io.Writer, pos *Position) error {
	return r.streamer.write(ctx, w, r.timeline, pos.pieces)
}

// recordingTimeline は、録画のセグメントの並び。
type recordingTimeline struct {
	source *RecordingSource
	id     uuid.UUID
}

var _ timeline = new(recordingTimeline)

func (t *recordingTimeline) load(ctx context.Context) ([]*segment, error) {
	files, err := sqlc.New(t.source.db).ListRecordingFiles(ctx, t.id)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recording files: %w", err)
	}
	store := t.source.streamer.store
	segments := []*segment{}
	for i := range files {
		file := &files[i]
		if file.Kind != kindSegment {
			continue
		}
		if _, err := store.Path(file.PoolID, file.Path); errors.Is(err, kago.ErrPoolNotMounted) {
			return nil, ErrNotMounted
		} else if err != nil {
			return nil, err
		}
		seg := &segment{
			id: file.ID, poolID: file.PoolID, path: file.Path, startedAt: file.StartedAt, endedAt: file.EndedAt, open: file.EndedAt == nil,
			continues: false, continued: false, index: []ts.IndexEntry{},
		}
		if seg.index, err = t.source.streamer.readIndex(kago.IndexLocation(file.ID)); err != nil {
			return nil, err
		}
		segments = append(segments, seg)
	}

	return segments, nil
}

// inProgress は、録画がまだ録画中かどうかを返す。
// 流している間に DB を引くときは、DB への接続が詰まっても流すのを止めないよう、database.FallbackQueryTimeout で打ち切る。
func (t *recordingTimeline) inProgress(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, database.FallbackQueryTimeout)
	defer cancel()
	recording, err := sqlc.New(t.source.db).GetRecording(ctx, t.id)
	if err != nil {
		return false, fmt.Errorf("failed to get the recording: %w", err)
	}

	return recording.State == stateRecording, nil
}

// segmentClosed は、セグメントを書き終えたかどうかを返す。録画が終わっていれば、書き終えたものとみなす。
func (t *recordingTimeline) segmentClosed(ctx context.Context, segmentID uuid.UUID) (bool, error) {
	recording, err := t.inProgress(ctx)
	if err != nil || !recording {
		return true, err
	}
	listCtx, cancel := context.WithTimeout(ctx, database.FallbackQueryTimeout)
	defer cancel()
	files, err := sqlc.New(t.source.db).ListRecordingFiles(listCtx, t.id)
	if err != nil {
		return false, fmt.Errorf("failed to list the recording files: %w", err)
	}
	for _, file := range files {
		if file.ID == segmentID {
			return file.EndedAt != nil, nil
		}
	}

	// セグメントの行が消えたら、録画を消したので、流すのをやめる。
	return true, nil
}

// readIndex は、索引を読む。書いている途中の索引の、書きかけの最後の記録は読まない。索引がなければ、空を返す。
func (s *streamer) readIndex(index *kago.Location) ([]ts.IndexEntry, error) {
	file, err := s.store.Open(index.PoolID, index.Path)
	if errors.Is(err, os.ErrNotExist) {
		return []ts.IndexEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open the index: %w", err)
	}
	defer file.Close()
	entries, err := ts.ReadIndex(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read the index %s/%s: %w", index.PoolID, index.Path, err)
	}

	return entries, nil
}

// write は、pieces を順に流す。流し終えたら、後に並ぶセグメントを探して流し続ける。
// 録画中の録画は、引き継ぎや再接続で新しいセグメントができるので、録画が終わるまで、それを待って流す。
// 追っかけ再生が録画に遅れていても、録画が終わる前にできたセグメントは、すべて流す。
func (s *streamer) write(ctx context.Context, w io.Writer, tl timeline, pieces []*piece) error {
	var last *segment
	for {
		for _, p := range pieces {
			// joined は、前のセグメントに続けて流すかどうか。最初の範囲は、プレイヤーが読み始めるところなので、つなぎ目にならない。
			if err := s.servePiece(ctx, w, tl, p, last != nil); err != nil {
				return err
			}
			last = p.segment
		}
		pieces = []*piece{}
		inProgress, err := tl.inProgress(ctx)
		var next []*piece
		var wait bool
		if err == nil {
			next, wait, err = s.following(ctx, tl, last, inProgress)
		}
		if err != nil {
			// DB に届かない間は、流し終えたところで、届くようになるのを待つ (#1084)。録画が終わったかも、次のセグメントも分からないためである。
			if err := s.waitForDatabase(ctx, err); err != nil {
				return err
			}

			continue
		}
		if len(next) == 0 {
			if !inProgress && !wait {
				return nil
			}
			if err := sleep(ctx, s.pollInterval); err != nil {
				return err
			}
		}
		pieces = next
	}
}

// following は、流し終えた last より後に並ぶセグメントを、互いの重なりを切り詰めて返す。セグメントは始まりの順に並ぶ。
// last は最後まで流したので、last と次のセグメントの重なりは、次のセグメントの側で、last の最後の I フレームより後から流して切り詰める。
// 次のセグメントが last の続き (continues) なら、バイトをそのままつなぐので、始まりから流す。
// 次のセグメントの索引がまだ追いついていなければ、wait を true にして、何も返さない。
func (s *streamer) following(ctx context.Context, tl timeline, last *segment, inProgress bool) ([]*piece, bool, error) {
	loadCtx, cancel := context.WithTimeout(ctx, database.FallbackQueryTimeout)
	defer cancel()
	segments, err := tl.load(loadCtx)
	if err != nil {
		return nil, false, err
	}
	position := -1
	for i, seg := range segments {
		if seg.id == last.id {
			position = i
		}
	}
	if position < 0 {
		// 流していたセグメントの行が消えたら、録画を消したので、流すのをやめる。
		return nil, false, ErrNoSegment
	}
	pieces := trim(segments[position+1:])
	lastIndex := segments[position].index
	if len(pieces) == 0 || len(lastIndex) == 0 || pieces[0].segment.continues {
		return pieces, false, nil
	}
	next := pieces[0]
	// 録画が終わっていれば、書いている途中のセグメントも、もう伸びない。
	growing := next.segment.open && inProgress
	if len(next.segment.index) == 0 {
		// 書き始めたばかりのセグメントには、まだ索引がない。索引ができるまで待たないと、重なりを切り詰められない。
		// 索引を作れなかったセグメントは、書き終えてから、始まりから流す。
		return []*piece{}, growing, nil
	}
	tail := lastIndex[len(lastIndex)-1].PTS
	for _, entry := range next.segment.index {
		if entry.Offset >= next.from && (next.to < 0 || entry.Offset < next.to) && ts.PTSDiff(tail, entry.PTS) > 0 {
			next.from = entry.Offset

			return pieces, false, nil
		}
	}
	if growing {
		// 新しいセグメントに、まだ重なりより後の I フレームがない。索引が伸びるのを待つ。
		return []*piece{}, true, nil
	}

	// セグメント全体が、流し終えた区間と重なっている。
	return pieces[1:], false, nil
}

// waitForDatabase は、err が DB に届かないことによるものなら、pollInterval だけ待って nil を返す。そうでなければ err を返す。
func (s *streamer) waitForDatabase(ctx context.Context, err error) error {
	if ctx.Err() != nil || !database.IsUnavailable(err) {
		return err
	}

	return sleep(ctx, s.pollInterval)
}

// segmentClosedOrUnknown は、segmentClosed と同じく、セグメントを書き終えたかどうかを返す。
// DB に届かなければ、書き終えたか分からないので、まだ書いているものとして扱い、ファイルが伸びるのを待ちながら流し続ける (#1084)。
func segmentClosedOrUnknown(ctx context.Context, tl timeline, segmentID uuid.UUID) (bool, error) {
	closed, err := tl.segmentClosed(ctx, segmentID)
	if err != nil && ctx.Err() == nil && database.IsUnavailable(err) {
		return false, nil
	}

	return closed, err
}

// servePiece は、セグメントの範囲を流す。書いている途中のセグメントは、書き終えるまで、伸びるのを待ちながら流す。
func (s *streamer) servePiece(ctx context.Context, w io.Writer, tl timeline, p *piece, joined bool) error {
	file, err := s.store.Open(p.segment.poolID, p.segment.path)
	if err != nil {
		return fmt.Errorf("failed to open the segment: %w", err)
	}
	defer file.Close()
	if !p.segment.open {
		to, err := servedEnd(file, p)
		if err != nil {
			return err
		}
		if err := writePrelude(w, file, p.from); err != nil {
			return err
		}
		from, err := writeHead(w, file, p, joined, p.from, to)
		if err != nil {
			return err
		}
		if _, err := file.Seek(from, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek the segment: %w", err)
		}

		return CopyFlushing(w, io.LimitReader(file, max(to-from, 0)))
	}

	return s.serveGrowing(ctx, w, tl, p, joined, file)
}

// serveGrowing は、書いている途中のセグメントを、書き終えるまで、伸びるのを待ちながら流す。
func (s *streamer) serveGrowing(ctx context.Context, w io.Writer, tl timeline, p *piece, joined bool, file *os.File) error {
	// 録画は TS をまとめて書くので、索引が指す位置を、まだファイルに書いていないことがある。書くまで待つ。
	// 書き込みのエラーなどで、その位置まで書かないままセグメントを閉じたら、流すものがないので終える。
	offset := p.from
	for {
		// 閉じたかを確かめてからファイルの大きさを見るので、確かめた後に書かれた分も見落とさない。
		closed, err := segmentClosedOrUnknown(ctx, tl, p.segment.id)
		if err != nil {
			return err
		}
		info, err := file.Stat()
		if err != nil {
			return fmt.Errorf("failed to inspect the segment: %w", err)
		}
		if info.Size() >= offset {
			break
		}
		if closed {
			return nil
		}
		if err := sleep(ctx, s.pollInterval); err != nil {
			return err
		}
	}
	if err := writePrelude(w, file, offset); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to inspect the segment: %w", err)
	}
	if offset, err = writeHead(w, file, p, joined, offset, info.Size()); err != nil {
		return err
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek the segment: %w", err)
	}
	for {
		closed, err := segmentClosedOrUnknown(ctx, tl, p.segment.id)
		if err != nil {
			return err
		}
		// 書き終えたかを確かめてから末尾まで読むので、確かめた後に書かれた分も読み残さない。
		// 引き継ぎで切り詰める位置 (to) があれば、そこで止める。
		var src io.Reader = file
		if p.to >= 0 {
			src = io.LimitReader(file, p.to-offset)
		}
		if err := CopyFlushing(w, src); err != nil {
			return err
		}
		if offset, err = file.Seek(0, io.SeekCurrent); err != nil {
			return fmt.Errorf("failed to seek the segment: %w", err)
		}
		if closed || (p.to >= 0 && offset >= p.to) {
			return nil
		}
		if err := sleep(ctx, s.pollInterval); err != nil {
			return err
		}
	}
}

// servedEnd は、書き終えたセグメントの範囲 p を、どこまで流すかを返す。続きのないセグメントは、終わりで途中で切れた PES を流さない
// (completeTail)。mikan が区切っただけで、次のセグメントが続き (continues) のものは、次のセグメントが PES の残りを運ぶので切らない。
func servedEnd(file *os.File, p *piece) (int64, error) {
	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("failed to inspect the segment: %w", err)
	}
	to := info.Size()
	if p.to >= 0 {
		to = min(p.to, to)
	}
	if p.segment.continued {
		return to, nil
	}

	return completeTail(file, p.from, to)
}

// writeHead は、セグメントの範囲 p を from から流すとき、前のセグメントにつなぐ (joined) なら、頭の前から続く PES の残りを落として書き
// (completeHead)、続きを流す位置を返す。前のセグメントの続き (continues) は、残りが前の PES の続きなので、落とさずに from を返す。
// 最初の範囲は、プレイヤーが前から続く残りを捨てるので、そのまま流す。
func writeHead(w io.Writer, file io.ReaderAt, p *piece, joined bool, from, end int64) (int64, error) {
	if !joined || p.segment.continues {
		return from, nil
	}
	head, consumed, err := completeHead(file, from, end)
	if err != nil {
		return 0, err
	}
	if _, err := w.Write(head); err != nil {
		return 0, err
	}

	return from + consumed, nil
}

// writePrelude は、セグメントを途中の offset から流すとき、その手前の PAT と PMT を先に書く。
func writePrelude(w io.Writer, file io.ReaderAt, offset int64) error {
	if offset == 0 {
		return nil
	}
	prelude, err := psiPrelude(file, offset)
	if err != nil {
		return err
	}
	if _, err := w.Write(prelude); err != nil {
		return err
	}

	return nil
}

// sleep は、d だけ待つ。ctx が終わったら、すぐに ctx のエラーを返す。
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// CopyFlushing は、src を読み終えるまで、読んだ分を w に書いて送り出す。w が HTTP の応答なら、書くたびに送り出す。
func CopyFlushing(w io.Writer, src io.Reader) error {
	flush := func() error { return nil }
	if rw, ok := w.(http.ResponseWriter); ok {
		controller := http.NewResponseController(rw)
		flush = func() error {
			if err := controller.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return err
			}

			return nil
		}
	}
	buf := make([]byte, CopyBufferSize)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			if ferr := flush(); ferr != nil {
				return ferr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

const (
	// CopyBufferSize は、上流から読んでクライアントに書く単位。TS のパケット 1024 個分。
	CopyBufferSize = 188 * 1024
)
