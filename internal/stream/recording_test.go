package stream_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/stream"
	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	poolID = "main"
	// recordingPattern は、録画の再生のパス。
	recordingPattern = "GET /streams/recordings/{id}"
	// pollInterval は、追っかけ再生のテストで、ファイルが伸びるのを確かめる間隔。
	pollInterval = 10 * time.Millisecond
)

// wantIFramePTS は、testdata/ts/test.ts の I フレームの PTS。internal/ts の索引のテストと同じ値で、0.5005 秒 (45045 刻み) ごとに並ぶ。
func wantIFramePTS() []int64 {
	return []int64{129003, 174048, 219093, 264138, 309183, 354228, 399273, 444318, 489363, 534408, 579453, 624498}
}

type recordingFixture struct {
	pool      *pgxpool.Pool
	clock     *clocktest.Fake
	dir       string
	server    *httptest.Server
	startedAt time.Time
}

func newRecordingFixture(t *testing.T, pools map[string]string) *recordingFixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $2, $2)`, serviceID, clk.Now())
	require.NoError(t, err)
	dir := t.TempDir()
	if pools == nil {
		pools = map[string]string{poolID: dir}
	}
	mux := http.NewServeMux()
	mux.Handle(recordingPattern, stream.NewRecordingHandler(pool, kago.New(pools, nil), slog.New(slog.DiscardHandler), pollInterval))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return &recordingFixture{pool: pool, clock: clk, dir: dir, server: server, startedAt: clk.Now().Add(-time.Hour)}
}

// recording は、録画の行を作る。
func (f *recordingFixture) recording(t *testing.T, state string) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, '録画', '', $3, $3, $4, false, $3, $3, $3)`, recordingID, serviceID, f.startedAt, state)
	require.NoError(t, err)

	return recordingID
}

// segment は、data のセグメントと、ts.Analyzer で作った索引を、録画のファイルとして置く。
// セグメントの始まりは、録画の始まりから offset 後にする。open なら、書いている途中のセグメントにする。
func (f *recordingFixture) segment(t *testing.T, recordingID uuid.UUID, name string, data []byte, offset time.Duration, open bool) uuid.UUID {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(f.dir, name), data, 0o600))
	var index bytes.Buffer
	writer, err := ts.NewIndexWriter(&index)
	require.NoError(t, err)
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer})
	_, err = analyzer.Write(data)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(f.dir, name+".kotatsu"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, kago.IndexPath(name)), index.Bytes(), 0o600))

	startedAt := f.startedAt.Add(offset)
	var endedAt *time.Time
	if !open {
		endedAt = new(startedAt.Add(time.Minute))
	}
	// セグメントと索引の行は、1 つの文で入れる。録画では、セグメントを書き終える前に索引の行ができているので、
	// 書き終えたセグメントの索引の行だけがない、という途中の状態を作らない。
	segmentID := id.New()
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'segment', $3, $4, 0, $5, $6, $5), ($7, $2, 'index', $3, $8, 0, $5, $6, $5)`,
		segmentID, recordingID, poolID, name, startedAt, endedAt, id.New(), kago.IndexPath(name))
	require.NoError(t, err)

	return segmentID
}

// fetched は、取得した応答。
type fetched struct {
	status      int
	contentType string
	body        []byte
}

// get は、録画を、クエリ query の位置から最後まで取得する。
func (f *recordingFixture) get(t *testing.T, recordingID uuid.UUID, query string) *fetched {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+"/streams/recordings/"+recordingID.String()+query, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return &fetched{status: resp.StatusCode, contentType: resp.Header.Get("Content-Type"), body: body}
}

// iframes は、TS を解析し、I フレームの PTS を順に返す。
func iframes(t *testing.T, data []byte) []int64 {
	t.Helper()

	var index bytes.Buffer
	writer, err := ts.NewIndexWriter(&index)
	require.NoError(t, err)
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer})
	_, err = analyzer.Write(data)
	require.NoError(t, err)
	entries, err := ts.ReadIndex(&index)
	require.NoError(t, err)
	pts := make([]int64, 0, len(entries))
	for _, entry := range entries {
		pts = append(pts, entry.PTS)
	}

	return pts
}

// pidOf は、パケットの PID を返す。
func pidOf(packet []byte) uint16 {
	return uint16(packet[1]&0x1F)<<8 | uint16(packet[2])
}

// packetAligned は、offset 以下で最も近いパケットの境目を返す。
func packetAligned(offset int) int {
	return offset / ts.PacketSize * ts.PacketSize
}

// TestRecordingHandler は、録画を、指定した位置の手前の I フレームから流し、引き継ぎで重なった区間を切り詰めることを確かめる。
// 流した TS を解析し直して、I フレームが重複も欠落もなく並ぶことを確かめる (設計書 5.4、7.2)。
func TestRecordingHandler(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	// 引き継ぎを再現する。前のセグメントは 500000 バイト目 (6 個目の I フレームの後) まで、
	// 後のセグメントは 200000 バイト目 (2 個目と 3 個目の I フレームの間) から始まり、その間が重なる。
	first := data[:packetAligned(500000)]
	second := data[packetAligned(200000):]
	tests := []struct {
		name     string
		handover bool
		query    string
		want     []int64
	}{
		{name: "最初から流す", handover: false, query: "", want: wantIFramePTS()},
		{name: "2.01 秒の位置は、2.002 秒の I フレームから流す", handover: false, query: "?start=2.01", want: wantIFramePTS()[4:]},
		{name: "I フレームの間の位置は、手前の I フレームに丸める", handover: false, query: "?start=2", want: wantIFramePTS()[3:]},
		{name: "引き継ぎで重なった区間を切り詰める", handover: true, query: "", want: wantIFramePTS()},
		{name: "後のセグメントの位置から流す", handover: true, query: "?start=1", want: wantIFramePTS()[2:]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newRecordingFixture(t, nil)
			recordingID := f.recording(t, "completed")
			if tt.handover {
				f.segment(t, recordingID, "a.m2ts", first, 0, false)
				// 後のセグメントは、前のセグメントの 1 秒後に始まった。最初の I フレームは、前のセグメントの 3 個目と同じ。
				f.segment(t, recordingID, "b.m2ts", second, time.Second, false)
			} else {
				f.segment(t, recordingID, "a.m2ts", data, 0, false)
			}

			got := f.get(t, recordingID, tt.query)
			require.Equal(t, http.StatusOK, got.status, string(got.body))
			assert.Equal(t, "video/mp2t", got.contentType)
			assert.Zero(t, len(got.body)%ts.PacketSize, "the body must consist of whole packets")
			assert.Equal(t, tt.want, iframes(t, got.body))
		})
	}
}

// TestRecordingHandlerStartsAtIndexedOffset は、指定した位置から流すとき、索引が指す I フレームのパケットから流すことを確かめる。
func TestRecordingHandlerStartsAtIndexedOffset(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	f := newRecordingFixture(t, nil)
	recordingID := f.recording(t, "completed")
	f.segment(t, recordingID, "a.m2ts", data, 0, false)

	body := f.get(t, recordingID, "?start=2.01").body
	// 5 個目の I フレーム (2.002 秒の位置) は、386340 バイト目から始まる。その手前に、プレイヤーが番組の構成を知るための PAT と PMT を 1 つずつ足す。
	require.Len(t, body, 2*ts.PacketSize+len(data)-386340)
	assert.Equal(t, data[386340:], body[2*ts.PacketSize:])
	prelude := body[:2*ts.PacketSize]
	assert.Equal(t, []uint16{0x0000, 0x01F0}, []uint16{pidOf(prelude[:ts.PacketSize]), pidOf(prelude[ts.PacketSize:])}, "PAT and PMT")
}

// TestRecordingHandlerFollowsGrowingFile は、録画中のファイルの末尾まで流した後も、ファイルが伸びるのを待って流し続け、
// 書き終えたら流し終えることを確かめる (追っかけ再生)。
func TestRecordingHandlerFollowsGrowingFile(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	half := packetAligned(len(data) / 2)
	f := newRecordingFixture(t, nil)
	recordingID := f.recording(t, "recording")
	segmentID := f.segment(t, recordingID, "a.m2ts", data[:half], 0, true)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+"/streams/recordings/"+recordingID.String(), nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	head := make([]byte, half)
	_, err = io.ReadFull(resp.Body, head)
	require.NoError(t, err)
	assert.Equal(t, data[:half], head)

	// 録画が続き、ファイルが伸びてから、書き終える。
	file, err := os.OpenFile(filepath.Join(f.dir, "a.m2ts"), os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = file.Write(data[half:])
	require.NoError(t, err)
	require.NoError(t, file.Close())
	_, err = f.pool.Exec(t.Context(), "UPDATE recording_files SET ended_at = $1 WHERE id = $2", f.clock.Now(), segmentID)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE recordings SET state = 'completed' WHERE id = $1", recordingID)
	require.NoError(t, err)

	rest, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, data[half:], rest)
}

// TestRecordingHandlerErrors は、録画を流せない理由に応じた状態を返すことを確かめる。
func TestRecordingHandlerErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pools      map[string]string
		recording  bool
		segment    bool
		query      string
		wantStatus int
		wantBody   string
	}{
		{name: "録画がない", recording: false, segment: false, wantStatus: http.StatusNotFound, wantBody: "recording not found"},
		{name: "セグメントがない", recording: true, segment: false, wantStatus: http.StatusNotFound, wantBody: "the recording has no segment to play"},
		{
			name: "開始位置が負", recording: true, segment: true, query: "?start=-1",
			wantStatus: http.StatusBadRequest, wantBody: "start must be a non-negative number of seconds",
		},
		{
			name: "開始位置が数でない", recording: true, segment: true, query: "?start=NaN",
			wantStatus: http.StatusBadRequest, wantBody: "start must be a non-negative number of seconds",
		},
		{
			name: "プールをマウントしていない", pools: map[string]string{"sub": os.TempDir()}, recording: true, segment: true,
			wantStatus: http.StatusServiceUnavailable, wantBody: "the recording is on a storage pool that this server does not mount",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newRecordingFixture(t, tt.pools)
			recordingID := id.New()
			if tt.recording {
				recordingID = f.recording(t, "completed")
			}
			if tt.segment {
				f.segment(t, recordingID, "a.m2ts", loadTestTS(t), 0, false)
			}

			got := f.get(t, recordingID, tt.query)
			assert.Equal(t, tt.wantStatus, got.status)
			assert.Equal(t, tt.wantBody, strings.TrimSpace(string(got.body)))
		})
	}
}

// TestRecordingHandlerFollowsHandover は、追っかけ再生の途中で引き継ぎが起きても、新しいセグメントを続けて流し、
// 重なった区間を、流し終えたセグメントの最後の I フレームより後から流すことを確かめる (設計書 5.4)。
func TestRecordingHandlerFollowsHandover(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	first := data[:packetAligned(500000)]
	second := data[packetAligned(200000):]
	f := newRecordingFixture(t, nil)
	recordingID := f.recording(t, "recording")
	f.segment(t, recordingID, "a.m2ts", first, 0, false)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+"/streams/recordings/"+recordingID.String(), nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body bytes.Buffer
	_, err = io.CopyN(&body, resp.Body, int64(len(first)))
	require.NoError(t, err)

	// 新しい mikan が引き継ぎ、新しいセグメントを書き始める。流し始めたのを確かめてから、録画を終える。
	segmentID := f.segment(t, recordingID, "b.m2ts", second, time.Second, true)
	_, err = io.CopyN(&body, resp.Body, ts.PacketSize)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE recording_files SET ended_at = $1 WHERE id = $2", f.clock.Now(), segmentID)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE recordings SET state = 'completed' WHERE id = $1", recordingID)
	require.NoError(t, err)
	_, err = io.Copy(&body, resp.Body)
	require.NoError(t, err)

	assert.Equal(t, wantIFramePTS(), iframes(t, body.Bytes()))
}

// TestRecordingHandlerAcrossHandovers は、追っかけ再生の途中で引き継ぎが重なって起き、録画が終わっても、
// 録画が終わる前にできたセグメントをすべて流し、セグメントどうしの重なりも切り詰めることを確かめる (設計書 5.4)。
func TestRecordingHandlerAcrossHandovers(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	// 3 つのセグメントが、前後のセグメントと重なる。
	a := data[:packetAligned(500000)]
	b := data[packetAligned(200000):packetAligned(800000)]
	c := data[packetAligned(600000):]
	tests := []struct {
		name string
		// openA は、流し始めた時点で、最初のセグメントを書いている途中かどうか。
		openA bool
	}{
		{name: "最初のセグメントを書き終えた後に流し始める", openA: false},
		{name: "最初のセグメントを書いている間に流し始める", openA: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newRecordingFixture(t, nil)
			recordingID := f.recording(t, "recording")
			segmentA := f.segment(t, recordingID, "a.m2ts", a, 0, tt.openA)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+"/streams/recordings/"+recordingID.String(), nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// 流している間に、引き継ぎが 2 度起き、録画が終わる。
			f.segment(t, recordingID, "b.m2ts", b, time.Second, false)
			f.segment(t, recordingID, "c.m2ts", c, 2*time.Second, false)
			_, err = f.pool.Exec(t.Context(), "UPDATE recording_files SET ended_at = $1 WHERE id = $2", f.clock.Now(), segmentA)
			require.NoError(t, err)
			_, err = f.pool.Exec(t.Context(), "UPDATE recordings SET state = 'completed' WHERE id = $1", recordingID)
			require.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, wantIFramePTS(), iframes(t, body))
		})
	}
}

// TestRecordingHandlerTrimsOpenPredecessor は、引き継ぎの最中で、前後のセグメントをどちらも書いている途中でも、
// 前のセグメントを切り詰める位置で止めることを確かめる。
func TestRecordingHandlerTrimsOpenPredecessor(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	f := newRecordingFixture(t, nil)
	recordingID := f.recording(t, "recording")
	segmentA := f.segment(t, recordingID, "a.m2ts", data[:packetAligned(500000)], 0, true)
	segmentB := f.segment(t, recordingID, "b.m2ts", data[packetAligned(200000):], time.Second, true)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+"/streams/recordings/"+recordingID.String(), nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	// 後のセグメントを流し始めたのを確かめてから、引き継ぎと録画を終える。
	var body bytes.Buffer
	_, err = io.CopyN(&body, resp.Body, 300000)
	require.NoError(t, err)
	for _, segmentID := range []uuid.UUID{segmentA, segmentB} {
		_, err = f.pool.Exec(t.Context(), "UPDATE recording_files SET ended_at = $1 WHERE id = $2", f.clock.Now(), segmentID)
		require.NoError(t, err)
	}
	_, err = f.pool.Exec(t.Context(), "UPDATE recordings SET state = 'completed' WHERE id = $1", recordingID)
	require.NoError(t, err)
	_, err = io.Copy(&body, resp.Body)
	require.NoError(t, err)

	assert.Equal(t, wantIFramePTS(), iframes(t, body.Bytes()))
}
