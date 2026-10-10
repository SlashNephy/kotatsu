package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	// kindSegment と kindIndex は、recording_files の、TS のセグメントと、その索引の種類。
	kindSegment = "segment"
	kindIndex   = "index"
	// stateRecording は、録画中の録画の状態。
	stateRecording = "recording"
	// defaultPollInterval は、追っかけ再生で、録画中のファイルが伸びるのを確かめる間隔。
	defaultPollInterval = 500 * time.Millisecond
)

var (
	errNotMounted = errors.New("the recording is on a storage pool that this server does not mount")
	errNoSegment  = errors.New("the recording has no segment to play")
)

// RecordingHandler は、録画を配信する (GET /streams/recordings/{id}?start=秒)。
//
// 索引を使って、録画の始まりから start 秒の位置を GOP の先頭のバイト位置に変換し、そこからセグメントを連結して流す。
// 引き継ぎで重なった区間は切り詰める。録画中の録画は、ファイルの末尾まで読んだら、ファイルが伸びるのを待ちながら流し続ける。
type RecordingHandler struct {
	db     sqlc.DBTX
	store  *kago.Store
	logger *slog.Logger
	// pollInterval は、追っかけ再生で、録画中のファイルが伸びるのを確かめる間隔。
	pollInterval time.Duration
}

var _ http.Handler = new(RecordingHandler)

// NewRecordingHandler は RecordingHandler を返す。store は、このインスタンスにマウントされたプール。
// pollInterval が 0 なら 500 ミリ秒にする。
func NewRecordingHandler(db sqlc.DBTX, store *kago.Store, logger *slog.Logger, pollInterval time.Duration) *RecordingHandler {
	if pollInterval == 0 {
		pollInterval = defaultPollInterval
	}

	return &RecordingHandler{db: db, store: store, logger: logger, pollInterval: pollInterval}
}

func (h *RecordingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	recordingID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid recording ID", http.StatusBadRequest)

		return
	}
	start, ok := parseStart(r.URL.Query().Get("start"))
	if !ok {
		http.Error(w, "start must be a non-negative number of seconds", http.StatusBadRequest)

		return
	}
	if _, err := sqlc.New(h.db).GetRecording(r.Context(), recordingID); errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "recording not found", http.StatusNotFound)

		return
	} else if err != nil {
		h.internalError(w, r, err)

		return
	}
	segments, err := h.load(r.Context(), recordingID)
	switch {
	case errors.Is(err, errNotMounted):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)

		return
	case err != nil:
		h.internalError(w, r, err)

		return
	case len(segments) == 0:
		http.Error(w, errNoSegment.Error(), http.StatusNotFound)

		return
	}

	writeStreamHeader(w)
	if r.Method == http.MethodHead {
		return
	}
	if err := h.serve(r.Context(), w, recordingID, plan(segments, start)); err != nil && r.Context().Err() == nil {
		h.logger.WarnContext(r.Context(), "failed to stream the recording", slog.String("recording_id", recordingID.String()), slog.Any("error", err))
	}
}

// parseStart は、開始位置の秒を読む。空なら 0 にする。
func parseStart(raw string) (time.Duration, bool) {
	if raw == "" {
		return 0, true
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(seconds) || seconds < 0 || seconds > float64(24*time.Hour/time.Second) {
		return 0, false
	}

	return time.Duration(seconds * float64(time.Second)), true
}

func (h *RecordingHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "failed to prepare the recording stream", slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// load は、録画のセグメントを、始まりの順に、索引と一緒に読む。
func (h *RecordingHandler) load(ctx context.Context, recordingID uuid.UUID) ([]*segment, error) {
	files, err := sqlc.New(h.db).ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recording files: %w", err)
	}
	indexes := map[string]*sqlc.RecordingFile{}
	for i := range files {
		if files[i].Kind == kindIndex {
			indexes[files[i].PoolID+"/"+files[i].Path] = &files[i]
		}
	}
	segments := []*segment{}
	for i := range files {
		file := &files[i]
		if file.Kind != kindSegment {
			continue
		}
		path, err := h.store.Path(file.PoolID, file.Path)
		if errors.Is(err, kago.ErrPoolNotMounted) {
			return nil, errNotMounted
		}
		if err != nil {
			return nil, err
		}
		seg := &segment{id: file.ID, path: path, startedAt: file.StartedAt, open: file.EndedAt == nil, index: []ts.IndexEntry{}}
		if index, ok := indexes[file.PoolID+"/"+kago.IndexPath(file.Path)]; ok {
			if seg.index, err = h.readIndex(index); err != nil {
				return nil, err
			}
		}
		segments = append(segments, seg)
	}

	return segments, nil
}

// readIndex は、索引を読む。書いている途中の索引の、書きかけの最後の記録は読まない。
func (h *RecordingHandler) readIndex(index *sqlc.RecordingFile) ([]ts.IndexEntry, error) {
	path, err := h.store.Path(index.PoolID, index.Path)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
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

// serve は、pieces を順に流す。流し終えたら、後に並ぶセグメントを探して流し続ける。
// 録画中の録画は、引き継ぎや再接続で新しいセグメントができるので、録画が終わるまで、それを待って流す。
// 追っかけ再生が録画に遅れていても、録画が終わる前にできたセグメントは、すべて流す。
func (h *RecordingHandler) serve(ctx context.Context, w http.ResponseWriter, recordingID uuid.UUID, pieces []*piece) error {
	var last *segment
	for {
		for _, p := range pieces {
			if err := h.servePiece(ctx, w, recordingID, p); err != nil {
				return err
			}
			last = p.segment
		}
		inProgress, err := h.recordingInProgress(ctx, recordingID)
		if err != nil {
			return err
		}
		next, wait, err := h.following(ctx, recordingID, last, inProgress)
		if err != nil {
			return err
		}
		if len(next) == 0 {
			if !inProgress && !wait {
				return nil
			}
			if err := sleep(ctx, h.pollInterval); err != nil {
				return err
			}
		}
		pieces = next
	}
}

// following は、流し終えた last より後に並ぶセグメントを、互いの重なりを切り詰めて返す。セグメントは始まりの順に並ぶ。
// last は最後まで流したので、last と次のセグメントの重なりは、次のセグメントの側で、last の最後の I フレームより後から流して切り詰める。
// 次のセグメントの索引がまだ追いついていなければ、wait を true にして、何も返さない。
func (h *RecordingHandler) following(ctx context.Context, recordingID uuid.UUID, last *segment, inProgress bool) ([]*piece, bool, error) {
	segments, err := h.load(ctx, recordingID)
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
		return nil, false, errNoSegment
	}
	pieces := trim(segments[position+1:])
	lastIndex := segments[position].index
	if len(pieces) == 0 || len(lastIndex) == 0 {
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

// recordingInProgress は、録画がまだ録画中かどうかを返す。
func (h *RecordingHandler) recordingInProgress(ctx context.Context, recordingID uuid.UUID) (bool, error) {
	recording, err := sqlc.New(h.db).GetRecording(ctx, recordingID)
	if err != nil {
		return false, fmt.Errorf("failed to get the recording: %w", err)
	}

	return recording.State == stateRecording, nil
}

// segmentClosed は、セグメントを書き終えたかどうかを返す。録画が終わっていれば、書き終えたものとみなす。
func (h *RecordingHandler) segmentClosed(ctx context.Context, recordingID, segmentID uuid.UUID) (bool, error) {
	recording, err := h.recordingInProgress(ctx, recordingID)
	if err != nil || !recording {
		return true, err
	}
	files, err := sqlc.New(h.db).ListRecordingFiles(ctx, recordingID)
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

// servePiece は、セグメントの範囲を流す。書いている途中のセグメントは、書き終えるまで、伸びるのを待ちながら流す。
func (h *RecordingHandler) servePiece(ctx context.Context, w http.ResponseWriter, recordingID uuid.UUID, p *piece) error {
	file, err := os.Open(p.segment.path)
	if err != nil {
		return fmt.Errorf("failed to open the segment: %w", err)
	}
	defer file.Close()
	if !p.segment.open {
		if err := writePrelude(w, file, p.from); err != nil {
			return err
		}
		if _, err := file.Seek(p.from, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek the segment: %w", err)
		}
		var src io.Reader = file
		if p.to >= 0 {
			src = io.LimitReader(file, p.to-p.from)
		}

		return copyFlushing(w, src)
	}

	return h.serveGrowing(ctx, w, recordingID, p, file)
}

// serveGrowing は、書いている途中のセグメントを、書き終えるまで、伸びるのを待ちながら流す。
func (h *RecordingHandler) serveGrowing(ctx context.Context, w http.ResponseWriter, recordingID uuid.UUID, p *piece, file *os.File) error {
	// 録画は TS をまとめて書くので、索引が指す位置を、まだファイルに書いていないことがある。書くまで待つ。
	offset := p.from
	for {
		info, err := file.Stat()
		if err != nil {
			return fmt.Errorf("failed to inspect the segment: %w", err)
		}
		if info.Size() >= offset {
			break
		}
		if err := sleep(ctx, h.pollInterval); err != nil {
			return err
		}
	}
	if err := writePrelude(w, file, offset); err != nil {
		return err
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek the segment: %w", err)
	}
	for {
		closed, err := h.segmentClosed(ctx, recordingID, p.segment.id)
		if err != nil {
			return err
		}
		// 書き終えたかを確かめてから末尾まで読むので、確かめた後に書かれた分も読み残さない。
		// 引き継ぎで切り詰める位置 (to) があれば、そこで止める。
		var src io.Reader = file
		if p.to >= 0 {
			src = io.LimitReader(file, p.to-offset)
		}
		if err := copyFlushing(w, src); err != nil {
			return err
		}
		if offset, err = file.Seek(0, io.SeekCurrent); err != nil {
			return fmt.Errorf("failed to seek the segment: %w", err)
		}
		if closed || (p.to >= 0 && offset >= p.to) {
			return nil
		}
		if err := sleep(ctx, h.pollInterval); err != nil {
			return err
		}
	}
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
