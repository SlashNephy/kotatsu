package kyusu_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
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
	"github.com/SlashNephy/kotatsu/internal/kyusu"
	"github.com/SlashNephy/kotatsu/internal/stream"
	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	poolID    = "main"
	serviceID = 3273601024
	// videoFrame は、testdata/ts/test.ts の映像の 1 フレームの長さ (29.97 fps)。
	videoFrame = 33367 * time.Microsecond
)

type vodFixture struct {
	pool    *pgxpool.Pool
	clock   *clocktest.Fake
	dir     string
	vod     *kyusu.VOD
	server  *httptest.Server
	token   *kyusu.Token
	workDir string
}

func newVODFixture(t *testing.T, encoder kyusu.VODEncoder) *vodFixture {
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
	dir, workDir := t.TempDir(), t.TempDir()
	source := stream.NewRecordingSource(pool, kago.New(map[string]string{poolID: dir}, nil), 10*time.Millisecond)
	vod, err := kyusu.NewVOD(&kyusu.VODOptions{
		Opener: source, Encoder: encoder, WorkDir: workDir, Clock: clk, Logger: slog.New(slog.DiscardHandler), IdleTimeout: 0,
	})
	require.NoError(t, err)
	token, err := kyusu.NewToken(keyMaterial)
	require.NoError(t, err)
	server := httptest.NewServer(vod.Handler(token))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- vod.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})

	return &vodFixture{pool: pool, clock: clk, dir: dir, vod: vod, server: server, token: token, workDir: workDir}
}

// recording は、data を 1 つのセグメントに持ち、ts.Analyzer で作った索引のある録画を作る。
func (f *vodFixture) recording(t *testing.T, state string, data []byte) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	startAt := f.clock.Now().Add(-time.Hour)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, '録画', '', $3, $3, $4, false, $3, $3, $3)`, recordingID, serviceID, startAt, state)
	require.NoError(t, err)
	name := recordingID.String() + ".m2ts"
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, name), data, 0o600))
	var index bytes.Buffer
	writer, err := ts.NewIndexWriter(&index)
	require.NoError(t, err)
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer})
	_, err = analyzer.Write(data)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(f.dir, name+".kotatsu"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, kago.IndexPath(name)), index.Bytes(), 0o600))
	var endedAt *time.Time
	if state != "recording" {
		endedAt = new(startAt.Add(time.Minute))
	}
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'segment', $3, $4, 0, $5, $6, $5), ($7, $2, 'index', $3, $8, 0, $5, $6, $5)`,
		id.New(), recordingID, poolID, name, startAt, endedAt, id.New(), kago.IndexPath(name))
	require.NoError(t, err)

	return recordingID
}

// get は、トークンを付けて、録画を 360p にした file を取得する。
func (f *vodFixture) get(t *testing.T, recordingID uuid.UUID, file string) (int, []byte) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		f.server.URL+kyusu.RecordingPathPrefix+recordingID.String()+"/360p/1/"+file, nil)
	require.NoError(t, err)
	f.token.Authorize(req)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, body
}

// TestVODPlaylist は、索引から、2 秒ずつのセグメントの一覧を作ることを確かめる。
// 録画を終えていれば、最後のセグメントを残りの長さにして終わりの印を付け、録画中なら、書き終えた区間だけを載せる。
func TestVODPlaylist(t *testing.T) {
	t.Parallel()

	f := newVODFixture(t, new(fakeVODEncoder))
	data := loadTestTS(t)
	half := len(data) / 2 / ts.PacketSize * ts.PacketSize
	tests := []struct {
		name  string
		state string
		data  []byte
		want  string
	}{
		{
			// I フレームは 0〜5.5055 秒にあり、間隔は 0.5005 秒なので、録画の長さは 6.006 秒とみなす。
			name: "録画を終えた", state: "completed", data: data,
			want: "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n" +
				"#EXTINF:2.000,\nsegment00000.ts\n#EXTINF:2.000,\nsegment00001.ts\n#EXTINF:2.000,\nsegment00002.ts\n#EXTINF:0.006,\nsegment00003.ts\n" +
				"#EXT-X-ENDLIST\n",
		},
		{
			// 前半には I フレームが 0〜2.5025 秒にあるので、書き終えた区間は 3.003 秒で、載せるのは 1 つ目まで。
			name: "録画中", state: "recording", data: data[:half],
			want: "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:EVENT\n#EXT-X-INDEPENDENT-SEGMENTS\n" +
				"#EXTINF:2.000,\nsegment00000.ts\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body := f.get(t, f.recording(t, tt.state, tt.data), kyusu.PlaylistName)
			assert.Equal(t, http.StatusOK, status)
			assert.Equal(t, tt.want, string(body))
		})
	}
}

// fakeVODEncoder は、入力を読み捨てながら、StartNumber 番目から 20 ミリ秒ごとにセグメントを書き、止められるまで続ける。
// 起動した番号と、動いている数を数える。
type fakeVODEncoder struct {
	starts  atomic.Value
	runs    atomic.Int32
	running atomic.Int32
}

func (e *fakeVODEncoder) RunVOD(ctx context.Context, src io.Reader, _ *kyusu.Preset, _ kyusu.Audio, dir string, opts *kyusu.RunOptions) error {
	e.runs.Add(1)
	e.running.Add(1)
	defer e.running.Add(-1)
	starts, _ := e.starts.Load().([]int)
	e.starts.Store(append(append([]int{}, starts...), opts.StartNumber))
	go func() {
		_, _ = io.Copy(io.Discard, src)
	}()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for k := opts.StartNumber; ; k++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("segment%05d.ts", k)), []byte(strconv.Itoa(k)), 0o600); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

var _ kyusu.VODEncoder = new(fakeVODEncoder)

// TestVODRestartsEncoder は、まだ書いていないセグメントを要求されたら、動いているエンコーダーがすぐ後に書くものなら待ち、
// 手前のものなら、その番号からエンコーダーを起動し直すことを確かめる。エンコーダーは、セッションに 1 つしか動かさない。
// 録画の外のセグメントには、エンコーダーを起動し直さずに 404 を返す。
func TestVODRestartsEncoder(t *testing.T) {
	t.Parallel()

	encoder := new(fakeVODEncoder)
	f := newVODFixture(t, encoder)
	// 録画は 6.006 秒なので、セグメントは 0〜3 番目の 4 つ。
	recordingID := f.recording(t, "completed", loadTestTS(t))

	tests := []struct {
		segment    int
		wantStatus int
		wantStarts []int
	}{
		{segment: 2, wantStatus: http.StatusOK, wantStarts: []int{2}},
		// すぐ後のセグメントは、起動し直さずに待つ。
		{segment: 3, wantStatus: http.StatusOK, wantStarts: []int{2}},
		// 手前に巻き戻したら、その番号から起動し直す。
		{segment: 0, wantStatus: http.StatusOK, wantStarts: []int{2, 0}},
		// 書いたセグメントは、書き直さない。
		{segment: 2, wantStatus: http.StatusOK, wantStarts: []int{2, 0}},
		// 録画の外のセグメントには、起動し直さない。
		{segment: 900, wantStatus: http.StatusNotFound, wantStarts: []int{2, 0}},
	}
	for _, tt := range tests {
		status, body := f.get(t, recordingID, fmt.Sprintf("segment%05d.ts", tt.segment))
		require.Equal(t, tt.wantStatus, status, "segment %d", tt.segment)
		if tt.wantStatus == http.StatusOK {
			assert.Equal(t, strconv.Itoa(tt.segment), string(body))
		}
		assert.Equal(t, tt.wantStarts, encoder.starts.Load(), "segment %d", tt.segment)
	}
	assert.LessOrEqual(t, encoder.running.Load(), int32(1), "only one encoder runs for a session")

	// セグメントへのリクエストが途絶えたら、止めて消す。
	f.vod.Reap(f.clock.Now().Add(time.Minute))
	assert.Eventually(t, func() bool {
		entries, err := os.ReadDir(filepath.Join(f.workDir, "recordings"))

		return err == nil && len(entries) == 0 && encoder.running.Load() == 0
	}, 5*time.Second, 10*time.Millisecond)
}

// TestVODErrors は、配信できない理由に応じた状態を返すことを確かめる。
func TestVODErrors(t *testing.T) {
	t.Parallel()

	f := newVODFixture(t, new(fakeVODEncoder))
	unindexed := f.recording(t, "completed", loadTestTS(t))
	_, err := f.pool.Exec(t.Context(), "DELETE FROM recording_files WHERE recording_id = $1 AND kind = 'index'", unindexed)
	require.NoError(t, err)
	tests := []struct {
		name        string
		recordingID uuid.UUID
		file        string
		wantStatus  int
		wantBody    string
	}{
		{
			name: "録画がない", recordingID: uuid.New(), file: kyusu.PlaylistName,
			wantStatus: http.StatusNotFound, wantBody: "recording not found",
		},
		{
			name: "索引がまだない", recordingID: unindexed, file: kyusu.PlaylistName,
			wantStatus: http.StatusServiceUnavailable, wantBody: "the recording is not indexed yet; try again after the analysis",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body := f.get(t, tt.recordingID, tt.file)
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantBody, strings.TrimSpace(string(body)))
		})
	}
}

// videoPTS は、セグメントの映像のパケットの PTS を、小さい順に返す。
func videoPTS(t *testing.T, data []byte) []time.Duration {
	t.Helper()

	path := filepath.Join(t.TempDir(), "segment.ts")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	out, err := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time", "-of", "csv=p=0", path).Output()
	require.NoError(t, err)
	times := []time.Duration{}
	for line := range strings.Lines(string(out)) {
		seconds, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ",")), 64)
		require.NoError(t, err)
		times = append(times, time.Duration(seconds*float64(time.Second)))
	}
	require.NotEmpty(t, times)
	slices.Sort(times)

	return times
}

// TestVODTranscodesSegments は、実際の tsreadex と FFmpeg で、要求されたセグメントから書き、
// k 番目のセグメントが、k × 2 秒の時刻から始まることを確かめる。FFmpeg か tsreadex がなければ飛ばす。
// 起動し直したエンコーダーが書いたセグメントと、前から書いていたエンコーダーのセグメントが、1 フレーム以内の差でつながることも確かめる。
func TestVODTranscodesSegments(t *testing.T) {
	t.Parallel()

	transcoder := newTranscoder(t)
	f := newVODFixture(t, transcoder)
	data := loadTestTS(t)
	// 2 つの録画は同じ中身で、1 つ目は 1 番目のセグメントから、2 つ目は 2 番目のセグメントから書く。
	fromFirst := f.recording(t, "completed", data)
	fromSecond := f.recording(t, "completed", data)

	status, first1 := f.get(t, fromFirst, "segment00001.ts")
	require.Equal(t, http.StatusOK, status)
	status, first2 := f.get(t, fromFirst, "segment00002.ts")
	require.Equal(t, http.StatusOK, status)
	status, second2 := f.get(t, fromSecond, "segment00002.ts")
	require.Equal(t, http.StatusOK, status)

	// MPEG-TS の出力は、タイムスタンプに 1.4 秒の muxdelay を足す。
	const muxDelay = 1400 * time.Millisecond
	assert.InDelta(t, muxDelay+2*time.Second, videoPTS(t, first1)[0], float64(videoFrame), "segment 1 starts at 2 seconds")
	assert.InDelta(t, muxDelay+4*time.Second, videoPTS(t, second2)[0], float64(videoFrame), "segment 2 starts at 4 seconds after a restart")
	// 同じセグメントは、どちらのエンコーダーが書いても、1 フレーム以内の差で同じ時刻から始まる。
	assert.InDelta(t, videoPTS(t, first2)[0], videoPTS(t, second2)[0], float64(videoFrame))
	// 1 番目のセグメントの最後のフレームの次に、起動し直したエンコーダーの 2 番目のセグメントが続く。
	last := videoPTS(t, first1)
	assert.InDelta(t, last[len(last)-1]+videoFrame, videoPTS(t, second2)[0], float64(videoFrame))
}
