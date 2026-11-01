package kyusu_test

import (
	"bytes"
	"context"
	"errors"
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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

const (
	// stateRecording と stateCompleted は、録画中と、録画を終えた録画の状態。
	stateRecording = "recording"
	stateCompleted = "completed"
	poolID         = "main"
	serviceID      = 3273601024
	// videoFrame は、testdata/ts/test.ts の映像の 1 フレームの長さ (29.97 fps)。
	videoFrame = 33367 * time.Microsecond
)

type vodFixture struct {
	pool  *pgxpool.Pool
	clock *clocktest.Fake
	dir   string
	// bufferDir は、タイムシフトのバッファを置くディレクトリ。索引は dir に置く。
	bufferDir string
	vod       *kyusu.VOD
	server    *httptest.Server
	token     *kyusu.Token
	workDir   string
}

// newVODFixture は、VOD を動かす。modify で、VOD の設定を変えられる。
func newVODFixture(t *testing.T, encoder kyusu.VODEncoder, modify ...func(opts *kyusu.VODOptions)) *vodFixture {
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
	dir, workDir, bufferDir := t.TempDir(), t.TempDir(), t.TempDir()
	store := kago.New(map[string]string{poolID: dir}, nil).WithDataDir(dir).WithTimeshiftDir(bufferDir)
	source := stream.NewRecordingSource(pool, store, 10*time.Millisecond)
	opts := &kyusu.VODOptions{
		Opener: source, Timeshift: stream.NewTimeshiftSource(pool, store, 10*time.Millisecond), Encoder: encoder, WorkDir: workDir, Clock: clk, Logger: slog.New(slog.DiscardHandler), IdleTimeout: 0, MaxSessions: 0,
	}
	for _, m := range modify {
		m(opts)
	}
	vod, err := kyusu.NewVOD(opts)
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

	return &vodFixture{pool: pool, clock: clk, dir: dir, bufferDir: bufferDir, vod: vod, server: server, token: token, workDir: workDir}
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
	segmentID := id.New()
	indexLocation := kago.IndexLocation(segmentID)
	require.NoError(t, os.MkdirAll(filepath.Join(f.dir, segmentID.String()), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, indexLocation.Path), index.Bytes(), 0o600))
	var endedAt *time.Time
	if state != stateRecording {
		endedAt = new(startAt.Add(time.Minute))
	}
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'segment', $3, $4, 0, $5, $6, $5)`,
		segmentID, recordingID, poolID, name, startAt, endedAt)
	require.NoError(t, err)

	return recordingID
}

// keyframeBytes は、longRecording の I フレームの間隔のバイト数。録画を流す 1 回の書き込み (32 KiB) より、
// 1 セグメント (I フレーム 4 つ) を大きくし、先読みの上限で流すのを止められるようにする。
const keyframeBytes = 100 * ts.PacketSize

// longRecording は、索引だけを作り込んだ、duration の長さの録画を作る。I フレームは 0.5 秒ごとにある。
// 中身は TS として正しくないので、入力を読み捨てる偽のエンコーダーと使う。
func (f *vodFixture) longRecording(t *testing.T, duration time.Duration) uuid.UUID {
	t.Helper()

	keyframes := []time.Duration{}
	for at := time.Duration(0); at+500*time.Millisecond <= duration; at += 500 * time.Millisecond {
		keyframes = append(keyframes, at)
	}

	return f.keyframedRecording(t, keyframes, duration, stateCompleted)
}

// keyframedRecording は、索引だけを作り込んだ、I フレームが keyframes の時間にある、duration の長さの、state の状態の録画を作る。
// 中身は TS として正しくないので、入力を読み捨てる偽のエンコーダーと使う。
func (f *vodFixture) keyframedRecording(t *testing.T, keyframes []time.Duration, duration time.Duration, state string) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	startAt := f.clock.Now().Add(-time.Hour)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, '録画', '', $3, $3, $4, false, $3, $3, $3)`, recordingID, serviceID, startAt, state)
	require.NoError(t, err)
	name := recordingID.String() + ".m2ts"
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, name), make([]byte, len(keyframes)*keyframeBytes), 0o600))
	var index bytes.Buffer
	writer, err := ts.NewIndexWriter(&index)
	require.NoError(t, err)
	for i, at := range keyframes {
		require.NoError(t, writer.Append(ts.IndexEntry{Offset: int64(i * keyframeBytes), PTS: int64(at * ts.PTSClock / time.Second)}))
	}
	segmentID := id.New()
	indexLocation := kago.IndexLocation(segmentID)
	require.NoError(t, os.MkdirAll(filepath.Join(f.dir, segmentID.String()), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, indexLocation.Path), index.Bytes(), 0o600))
	var endedAt *time.Time
	if state != stateRecording {
		endedAt = new(startAt.Add(duration))
	}
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'segment', $3, $4, 0, $5, $6, $5)`,
		segmentID, recordingID, poolID, name, startAt, endedAt)
	require.NoError(t, err)

	return recordingID
}

// get は、トークンを付けて、録画を 360p にした file を取得する。
func (f *vodFixture) get(t *testing.T, recordingID uuid.UUID, file string) (int, []byte) {
	t.Helper()

	return f.getPreset(t, recordingID, "360p", file)
}

// getPreset は、トークンを付けて、録画を preset の画質にした file を取得する。
func (f *vodFixture) getPreset(t *testing.T, recordingID uuid.UUID, preset, file string) (int, []byte) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		f.server.URL+kyusu.RecordingPathPrefix+recordingID.String()+"/"+preset+"/1/"+file, nil)
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

// TestVODPassthroughPlaylist は、映像を変換しないプリセットでは、前のセグメントの始まりから 2 秒以上進んだ最初の I フレームで
// 区切った、長さのそろわないセグメントの一覧を作ることを確かめる (#806)。
func TestVODPassthroughPlaylist(t *testing.T) {
	t.Parallel()

	f := newVODFixture(t, new(fakeVODEncoder))
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	// 始まりは 0、2.1 (2 秒以上進んだ最初の I フレーム)、4.4 秒。録画の長さは、最後の I フレームに平均の間隔 (4.4 / 7) を足した 5.029 秒。
	keyframes := []time.Duration{0, ms(500), ms(1000), ms(1600), ms(2100), ms(2600), ms(3000), ms(4400)}
	recordingID := f.keyframedRecording(t, keyframes, 5*time.Second, stateCompleted)

	status, body := f.getPreset(t, recordingID, kyusu.PassthroughPreset, kyusu.PlaylistName)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:3\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"#EXTINF:2.100,\nsegment00000.ts\n#EXTINF:2.300,\nsegment00001.ts\n#EXTINF:0.629,\nsegment00002.ts\n#EXT-X-ENDLIST\n", string(body))
	status, _ = f.getPreset(t, recordingID, kyusu.PassthroughPreset, "segment00003.ts")
	assert.Equal(t, http.StatusNotFound, status, "a segment out of the recording")

	// 録画中は、最後のセグメント (4.4 秒から) は、次の始まりが索引に載るまで伸びるので載せず、終わりの印も付けない。
	recordingID = f.keyframedRecording(t, keyframes, 5*time.Second, stateRecording)
	status, body = f.getPreset(t, recordingID, kyusu.PassthroughPreset, kyusu.PlaylistName)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:3\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:EVENT\n#EXT-X-INDEPENDENT-SEGMENTS\n"+
		"#EXTINF:2.100,\nsegment00000.ts\n#EXTINF:2.300,\nsegment00001.ts\n", string(body))
}

// TestVODPassthroughSegments は、映像を変換しないプリセットのセグメントが、プレイリストの区切りの I フレームから始まり、
// その PTS が、録画の始まりからの時間に 10 秒 (出力の PTS の起点) を足したものになることを確かめる (#806)。
// 後のセグメントから先に要求し、途中から起動したエンコーダーでも、区切りと時刻がずれないことを確かめる。
func TestVODPassthroughSegments(t *testing.T) {
	t.Parallel()

	f := newVODFixture(t, newTranscoder(t))
	recordingID := f.recording(t, "completed", loadTS(t, "test-hevc-main10.ts"))
	status, body := f.getPreset(t, recordingID, kyusu.PassthroughPreset, kyusu.PlaylistName)
	require.Equal(t, http.StatusOK, status)
	// I フレームは 0.5 秒ごとに 8 つあるので、始まりは 0 と 2.002 秒。
	assert.Contains(t, string(body), "#EXTINF:2.002,\nsegment00000.ts\n#EXTINF:2.002,\nsegment00001.ts\n#EXT-X-ENDLIST\n")

	for _, tt := range []struct {
		file      string
		wantStart float64
	}{
		{file: "segment00001.ts", wantStart: 10 + 2.002},
		{file: "segment00000.ts", wantStart: 10},
	} {
		status, data := f.getPreset(t, recordingID, kyusu.PassthroughPreset, tt.file)
		require.Equal(t, http.StatusOK, status, tt.file)
		path := filepath.Join(t.TempDir(), tt.file)
		require.NoError(t, os.WriteFile(path, data, 0o600))
		got := map[string]probedStream{}
		for _, s := range probe(t, path) {
			got[s.CodecType] = s
		}
		assert.Equal(t, "hevc", got["video"].CodecName, tt.file)
		assert.Equal(t, "yuv420p10le", got["video"].PixFmt, "the video is not transcoded")
		assert.Equal(t, "aac", got["audio"].CodecName)
		out, err := exec.CommandContext(t.Context(), "ffprobe", "-v", "error", "-select_streams", "v", "-show_entries", "packet=pts_time,flags",
			"-of", "csv=p=0", "-read_intervals", "%+#1", path).Output()
		require.NoError(t, err)
		// pts_time と flags の後に、空の欄が続く。
		fields := strings.Split(strings.TrimSpace(string(out)), ",")
		require.GreaterOrEqual(t, len(fields), 2)
		start, err := strconv.ParseFloat(fields[0], 64)
		require.NoError(t, err)
		assert.InDelta(t, tt.wantStart, start, 0.001, tt.file)
		assert.Contains(t, fields[1], "K", "a segment starts with a keyframe")
	}
}

// fakeVODEncoder は、入力を読み捨てながら、StartNumber 番目から 20 ミリ秒ごとにセグメントを書き、止められるまで続ける。
// 起動した番号と、動いている数を数える。startup を与えると、起動してから最初のセグメントを書くまで、その時間だけ待つ。
// interval を与えると、20 ミリ秒の代わりに、その間隔で書く。
// paced なら、時間でなく入力に合わせて、longRecording の 1 セグメントぶん (I フレーム 4 つ) を読むごとに書く。
type fakeVODEncoder struct {
	startup  time.Duration
	interval time.Duration
	paced    bool
	// startsMu は、starts に足すのを 1 つずつ行わせる。2 つのエンコーダーが同時に起動すると、片方の記録が消えるため (#1203)。
	startsMu sync.Mutex
	starts   atomic.Value
	runs     atomic.Int32
	running  atomic.Int32
}

func (e *fakeVODEncoder) RunVOD(ctx context.Context, src io.Reader, _ *kyusu.Preset, _ kyusu.Audio, dir string, opts *kyusu.RunOptions) error {
	e.runs.Add(1)
	e.running.Add(1)
	defer e.running.Add(-1)
	e.startsMu.Lock()
	starts, _ := e.starts.Load().([]int)
	e.starts.Store(append(append([]int{}, starts...), opts.StartNumber))
	e.startsMu.Unlock()
	if e.paced {
		buf := make([]byte, 4*keyframeBytes)
		for k := opts.StartNumber; ; k++ {
			if _, err := io.ReadFull(src, buf); errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				// 入力を読み終えたら、書き終える。
				return nil
			} else if err != nil {
				return err
			}
			if err := writeFileAtomically(filepath.Join(dir, fmt.Sprintf("segment%05d.ts", k)), []byte(strconv.Itoa(k))); err != nil {
				return err
			}
		}
	}
	go func() {
		_, _ = io.Copy(io.Discard, src)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(e.startup):
	}
	interval := e.interval
	if interval == 0 {
		interval = 20 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for k := opts.StartNumber; ; k++ {
		if err := writeFileAtomically(filepath.Join(dir, fmt.Sprintf("segment%05d.ts", k)), []byte(strconv.Itoa(k))); err != nil {
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
// 手前のものなら、その番号からエンコーダーを起動することを確かめる。後ろのエンコーダーが前のエンコーダーに追いついたら、後ろを止める。
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
	// 0 番目から起動したエンコーダーは、2 番目から書いていたエンコーダーに追いつくので、止める。
	assert.Eventually(t, func() bool {
		f.vod.Reap(f.clock.Now())

		return encoder.running.Load() == 1
	}, 5*time.Second, 10*time.Millisecond)

	// セグメントへのリクエストが途絶えたら、止めて消す。
	f.vod.Reap(f.clock.Now().Add(time.Minute))
	assert.Eventually(t, func() bool {
		entries, err := os.ReadDir(filepath.Join(f.workDir, "recordings"))

		return err == nil && len(entries) == 0 && encoder.running.Load() == 0
	}, 5*time.Second, 10*time.Millisecond)
}

// TestVODServesDistantViewers は、同じ録画を同じ画質と音声で、離れた位置で見る 2 人に、それぞれのエンコーダーで
// セグメントを返すことを確かめる (#531)。1 つのエンコーダーを奪い合うと、互いに起動し直し合って、どちらも書き終わらない。
func TestVODServesDistantViewers(t *testing.T) {
	t.Parallel()

	// 実際の FFmpeg のように、起動してから最初のセグメントを書くまでに時間がかかる。
	encoder := &fakeVODEncoder{startup: 200 * time.Millisecond}
	f := newVODFixture(t, encoder)
	recordingID := f.longRecording(t, time.Minute)

	statuses := make(chan int, 6)
	view := func(from int) {
		for k := from; k < from+3; k++ {
			status, _ := f.get(t, recordingID, fmt.Sprintf("segment%05d.ts", k))
			statuses <- status
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		view(20)
	}()
	view(0)
	<-done
	close(statuses)
	for status := range statuses {
		assert.Equal(t, http.StatusOK, status)
	}
	starts, _ := encoder.starts.Load().([]int)
	slices.Sort(starts)
	assert.Equal(t, []int{0, 20}, starts)

	// 前のエンコーダーが書いた位置に、後ろのエンコーダーが追いついたら、後ろを止める。
	assert.Eventually(t, func() bool {
		f.vod.Reap(f.clock.Now())

		return encoder.running.Load() == 1
	}, 5*time.Second, 10*time.Millisecond)
}

// TestVODStopsAbandonedEncoder は、片方の視聴者が離れたら、ほかの視聴者がセッションを使い続けていても、
// 離れた視聴者のエンコーダーを止めることを確かめる (#531)。
func TestVODStopsAbandonedEncoder(t *testing.T) {
	t.Parallel()

	// 後ろのエンコーダーが前に追いつかないよう、セグメントをゆっくり書く。
	encoder := &fakeVODEncoder{interval: time.Second}
	f := newVODFixture(t, encoder)
	recordingID := f.longRecording(t, time.Minute)
	for _, segment := range []int{0, 20} {
		status, _ := f.get(t, recordingID, fmt.Sprintf("segment%05d.ts", segment))
		require.Equal(t, http.StatusOK, status)
	}
	require.Equal(t, int32(2), encoder.running.Load())

	// 20 番目から見る視聴者だけが、見続ける。
	f.clock.Advance(6 * time.Second)
	status, _ := f.get(t, recordingID, "segment00021.ts")
	require.Equal(t, http.StatusOK, status)
	f.clock.Advance(6 * time.Second)
	f.vod.Reap(f.clock.Now())
	assert.Eventually(t, func() bool { return encoder.running.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, f.vod.Sessions())
}

// TestVODKeepsEncoderWhileViewerPlaysWrittenSegments は、エンコーダーが先に書いたセグメントを視聴者が見ている間は、
// そのエンコーダーを止めないことを確かめる (#531)。止めると、書いた分を見終えたところで起動し直し、再生が止まる。
func TestVODKeepsEncoderWhileViewerPlaysWrittenSegments(t *testing.T) {
	t.Parallel()

	encoder := new(fakeVODEncoder)
	f := newVODFixture(t, encoder)
	recordingID := f.longRecording(t, time.Minute)
	status, _ := f.get(t, recordingID, "segment00000.ts")
	require.Equal(t, http.StatusOK, status)
	// エンコーダーは、視聴者より先に書き進める。
	require.Eventually(t, func() bool {
		entries, err := filepath.Glob(filepath.Join(f.workDir, "recordings", "*", "segment00010.ts"))

		return err == nil && len(entries) == 1
	}, 5*time.Second, 10*time.Millisecond)

	// 視聴者は、書き終えたセグメントを 2 秒ごとに見続ける。
	for segment := 1; segment <= 6; segment++ {
		f.clock.Advance(2 * time.Second)
		status, _ := f.get(t, recordingID, fmt.Sprintf("segment%05d.ts", segment))
		require.Equal(t, http.StatusOK, status)
		f.vod.Reap(f.clock.Now())
	}
	assert.Equal(t, int32(1), encoder.running.Load())
	assert.Equal(t, []int{0}, encoder.starts.Load())
}

// TestVODLimitsLookahead は、エンコーダーが、視聴者の要求したセグメントより先を書きすぎないことを確かめる (#532)。
// 見ない部分のために、CPU を使ったり、作業ディレクトリにセグメントをためたりしない。
func TestVODLimitsLookahead(t *testing.T) {
	t.Parallel()

	encoder := &fakeVODEncoder{paced: true}
	f := newVODFixture(t, encoder)
	// 録画は 5 分なので、セグメントは 150。
	recordingID := f.longRecording(t, 5*time.Minute)
	written := func() int {
		entries, err := filepath.Glob(filepath.Join(f.workDir, "recordings", "*", "segment*.ts"))
		require.NoError(t, err)

		return len(entries)
	}
	status, _ := f.get(t, recordingID, "segment00000.ts")
	require.Equal(t, http.StatusOK, status)

	// 書く量は、要求したセグメントから先読みの上限 (30) ほどで止まる。
	require.Eventually(t, func() bool { return written() >= 30 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(500 * time.Millisecond)
	assert.InDelta(t, 31, written(), 2)
	// 見進めると、その先を書く。
	status, _ = f.get(t, recordingID, "segment00030.ts")
	require.Equal(t, http.StatusOK, status)
	require.Eventually(t, func() bool { return written() >= 60 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(500 * time.Millisecond)
	assert.InDelta(t, 61, written(), 2)
	assert.Equal(t, []int{0}, encoder.starts.Load())
}

// TestVODLimitsSessions は、セッションの数が上限に達したら、新しいセッションを作らずに 503 を返すことを確かめる (#532)。
// 多くの録画や画質を開いても、CPU と作業ディレクトリを使い切らないようにする。
func TestVODLimitsSessions(t *testing.T) {
	t.Parallel()

	f := newVODFixture(t, new(fakeVODEncoder), func(opts *kyusu.VODOptions) { opts.MaxSessions = 1 })
	first := f.longRecording(t, time.Minute)
	second := f.longRecording(t, time.Minute)
	status, _ := f.get(t, first, "segment00000.ts")
	require.Equal(t, http.StatusOK, status)

	status, body := f.get(t, second, "segment00000.ts")
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Equal(t, "too many recording sessions", strings.TrimSpace(string(body)))
	assert.Equal(t, 1, f.vod.Sessions())

	// 使われなくなったセッションを止めたら、新しいセッションを作れる。
	f.vod.Reap(f.clock.Now().Add(time.Minute))
	status, _ = f.get(t, second, "segment00000.ts")
	assert.Equal(t, http.StatusOK, status)
}

// TestVODRemovesSegmentsFarBehind は、視聴者の位置から 150 セグメント (5 分) より前のセグメントを消し、
// そこへ巻き戻したら書き直すことを確かめる (#532)。長い録画を見続けても、作業ディレクトリにセグメントをためない。
func TestVODRemovesSegmentsFarBehind(t *testing.T) {
	t.Parallel()

	encoder := &fakeVODEncoder{paced: true}
	f := newVODFixture(t, encoder)
	recordingID := f.longRecording(t, 10*time.Minute)
	segment := func(number int) string {
		return filepath.Join(f.workDir, "recordings", "*", fmt.Sprintf("segment%05d.ts", number))
	}
	exists := func(number int) bool {
		matches, err := filepath.Glob(segment(number))
		require.NoError(t, err)

		return len(matches) == 1
	}
	for number := 0; number <= 200; number += 5 {
		status, _ := f.get(t, recordingID, fmt.Sprintf("segment%05d.ts", number))
		require.Equal(t, http.StatusOK, status, "segment %d", number)
	}

	f.vod.Reap(f.clock.Now())
	assert.False(t, exists(49), "segment 49 is far behind the viewer")
	assert.True(t, exists(50), "segment 50 is within 150 segments behind the viewer")
	// 消したセグメントへ巻き戻したら、そこから書き直す。
	status, body := f.get(t, recordingID, "segment00010.ts")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, "10", string(body))
	assert.Equal(t, []int{0, 10}, encoder.starts.Load())
}

// TestVODErrors は、配信できない理由に応じた状態を返すことを確かめる。
func TestVODErrors(t *testing.T) {
	t.Parallel()

	f := newVODFixture(t, new(fakeVODEncoder))
	unindexed := f.recording(t, "completed", loadTestTS(t))
	var segmentID uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT id FROM recording_files WHERE recording_id = $1", unindexed).Scan(&segmentID))
	require.NoError(t, os.Remove(filepath.Join(f.dir, kago.IndexLocation(segmentID).Path)))
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
