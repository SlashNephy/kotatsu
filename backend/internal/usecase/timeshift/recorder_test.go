package timeshift_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
	"github.com/SlashNephy/kotatsu/internal/usecase/timeshift"
)

const (
	waitTimeout = 10 * time.Second
	// testTick は、テストの Recorder が時計と割り振りを確かめる間隔。
	testTick = 5 * time.Millisecond
	// grSID は、地上波の 0 番目のサービスの、EIT[p/f] の service_id。
	grSID = 0x0400
)

// fakeTuner は、開いたときの時計の時刻と、渡された StreamOptions を控える偽のチューナーサーバー。
// cutAtOnce なら、開いたストリームをすぐに切る。そうでなければ、next で送ったデータを 1 つずつ流す。
type fakeTuner struct {
	clk       *clocktest.Fake
	cutAtOnce bool

	mu     sync.Mutex
	opens  []time.Time
	opts   []mirakurun.StreamOptions
	chunks chan *streamChunk
	// reads は、Recorder がストリームを読もうとした回数。読もうとしたなら、それまでに渡したデータを処理し終えている。
	reads atomic.Int64
}

// streamChunk は、next で流すデータ。読んだストリームは、何回目の読み込みで読んだかを taken に返す。
type streamChunk struct {
	data  []byte
	taken chan int64
}

func newFakeTuner(clk *clocktest.Fake, cutAtOnce bool) *fakeTuner {
	return &fakeTuner{
		clk: clk, cutAtOnce: cutAtOnce, mu: sync.Mutex{}, opens: []time.Time{}, opts: []mirakurun.StreamOptions{},
		chunks: make(chan *streamChunk), reads: atomic.Int64{},
	}
}

var _ recording.StreamOpener = new(fakeTuner)

func (f *fakeTuner) OpenServiceStream(ctx context.Context, _ int64, opts *mirakurun.StreamOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	f.opens = append(f.opens, f.clk.Now())
	f.opts = append(f.opts, *opts)
	f.mu.Unlock()
	if f.cutAtOnce {
		return io.NopCloser(strings.NewReader("")), nil
	}

	return &fakeStream{tuner: f, ctxDone: ctx.Done(), closed: make(chan struct{}), once: sync.Once{}}, nil
}

// openTimes は、ストリームを開いた時計の時刻を返す。
func (f *fakeTuner) openTimes() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]time.Time{}, f.opens...)
}

// next は、data を流し、Recorder が処理し終えて、次を読もうとするまで待つ。
func (f *fakeTuner) next(t *testing.T, data []byte) {
	t.Helper()

	chunk := &streamChunk{data: data, taken: make(chan int64, 1)}
	select {
	case f.chunks <- chunk:
	case <-time.After(waitTimeout):
		require.FailNow(t, "the recorder did not read the stream")
	}
	// data を読んだ読み込みの、次の読み込みまで待つ。
	taken := <-chunk.taken
	require.Eventually(t, func() bool { return f.reads.Load() > taken }, waitTimeout, time.Millisecond)
}

// fakeStream は、fakeTuner の chunks を 1 つずつ返すストリーム。
type fakeStream struct {
	tuner *fakeTuner
	// ctxDone は、開いたときの ctx の Done。本物と同じく、ctx が終わると切れる。
	ctxDone <-chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (s *fakeStream) Read(p []byte) (int, error) {
	n := s.tuner.reads.Add(1)
	select {
	case chunk := <-s.tuner.chunks:
		chunk.taken <- n

		return copy(p, chunk.data), nil
	case <-s.closed:
		return 0, io.ErrClosedPipe
	case <-s.ctxDone:
		return 0, context.Canceled
	}
}

func (s *fakeStream) Close() error {
	s.once.Do(func() { close(s.closed) })

	return nil
}

// crcMPEG は、MPEG-2 の PSI の CRC32 を求める。
func crcMPEG(b []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, v := range b {
		crc ^= uint32(v) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
	}

	return crc
}

// eitPFPackets は、地上波の 0 番目のサービスの EIT[p/f] (自 TS) の放送中 present と次 following の番組のセクションを、
// continuity_counter を cc から続けて、TS のパケットにして返す。recording の broadcast_record_test.go と同じもの。
func eitPFPackets(cc *byte, present, following uint16) []byte {
	out := make([]byte, 0, 2*188)
	for sectionNumber, eventID := range []uint16{present, following} {
		body := []byte{0x7F, 0xE0, 0x7F, 0xE0, 0x01, 0x4E}
		body = binary.BigEndian.AppendUint16(body, eventID)
		body = append(body, 0xE8, 0x1F, 0x23, 0x00, 0x00, 0x00, 0x30, 0x00, 0x80, 0x00)
		const sectionLength = 5 + 18 + 4
		section := []byte{0x4E, 0xF0, sectionLength}
		section = binary.BigEndian.AppendUint16(section, grSID)
		section = append(section, 0xC1, byte(sectionNumber), 1)
		section = append(section, body...)
		section = binary.BigEndian.AppendUint32(section, crcMPEG(section))
		packet := make([]byte, 188)
		for i := range packet {
			packet[i] = 0xFF
		}
		packet[0], packet[1], packet[2], packet[3] = 0x47, 0x40, 0x12, 0x10|*cc
		packet[4] = 0
		copy(packet[5:], section)
		*cc = (*cc + 1) & 0x0F
		out = append(out, packet...)
	}

	return out
}

// nullPackets は、中身のない TS のパケット (PID 0x1FFF) を n 個返す。
func nullPackets(n int) []byte {
	out := make([]byte, 0, n*188)
	for range n {
		packet := make([]byte, 188)
		for i := range packet {
			packet[i] = 0xFF
		}
		packet[0], packet[1], packet[2], packet[3] = 0x47, 0x1F, 0xFF, 0x10
		out = append(out, packet...)
	}

	return out
}

// recorderEnv は、タイムシフトの Recorder のテストの環境。dir はタイムシフトのディレクトリ、dataDir はデータディレクトリ。
type recorderEnv struct {
	*env

	dir     string
	dataDir string
	store   *kago.Store
}

func newRecorderEnv(t *testing.T) *recorderEnv {
	t.Helper()

	e := newEnv(t)
	root := t.TempDir()
	dir, dataDir := filepath.Join(root, "timeshift"), filepath.Join(root, "data")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	store := kago.New(map[string]string{}, nil).WithDataDir(dataDir).WithTimeshiftDir(dir)

	return &recorderEnv{env: e, dir: dir, dataDir: dataDir, store: store}
}

func (e *recorderEnv) newRecorder(tuner recording.StreamOpener) *timeshift.Recorder {
	return e.newRecorderWithPeers(tuner, nil)
}

// newRecorderWithPeers は、ほかに動いている mikan の数を peers で数える Recorder を返す。nil なら、ほかの mikan はいない。
func (e *recorderEnv) newRecorderWithPeers(tuner recording.StreamOpener, peers recording.PeersFunc) *timeshift.Recorder {
	return e.newRecorderOn(e.pool, tuner, peers)
}

// newRecorderOn は、pool で DB を読み書きする Recorder を返す。DB に届かない状態を作るテストで、切り替えられるプールを渡す。
func (e *recorderEnv) newRecorderOn(pool *pgxpool.Pool, tuner recording.StreamOpener, peers recording.PeersFunc) *timeshift.Recorder {
	return timeshift.NewRecorder(pool, e.clk, tuner, e.store, slog.New(slog.DiscardHandler), &timeshift.RecorderOptions{
		ChunkDuration: 5 * time.Minute, Backoff: nil, Tick: testTick, Peers: peers,
	})
}

// assign は、地上波の 0 番目のサービスの割り振りを書き、取ったものとして返す。
func (e *recorderEnv) assign(t *testing.T, handedOver bool) *recording.Assignment {
	t.Helper()

	assignmentID := id.New()
	require.NoError(t, e.q.InsertTimeshiftAssignment(t.Context(), sqlc.InsertTimeshiftAssignmentParams{
		ID: assignmentID, ServiceID: grService(0), Priority: timeshift.Priority, Now: e.clk.Now(),
	}))

	return &recording.Assignment{
		ID: assignmentID, ReservationID: uuid.Nil, ServiceID: grService(0), Priority: timeshift.Priority, Generation: 0,
		HandedOver: handedOver, PreviousLeaseExpiresAt: nil,
	}
}

// session は、Recorder の Session を、割り振り assignment を取ったものとして、別の goroutine で動かす。
func session(t *testing.T, recorder *timeshift.Recorder, assignment *recording.Assignment) (context.CancelCauseFunc, <-chan error) {
	t.Helper()

	ctx, cancel := context.WithCancelCause(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- recorder.Session()(ctx, assignment)
	}()
	t.Cleanup(func() { cancel(nil) })

	return cancel, done
}

func wait(t *testing.T, done <-chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(waitTimeout):
		require.FailNow(t, "the timeshift session did not return")

		return nil
	}
}

type chunkRow struct {
	id        uuid.UUID
	path      string
	size      int64
	startedAt time.Time
	endedAt   *time.Time
}

func chunks(t *testing.T, e *recorderEnv) []chunkRow {
	t.Helper()

	rows, err := e.pool.Query(t.Context(), "SELECT id, path, size, started_at, ended_at FROM timeshift_chunks ORDER BY started_at, id")
	require.NoError(t, err)
	defer rows.Close()
	got := []chunkRow{}
	for rows.Next() {
		var c chunkRow
		require.NoError(t, rows.Scan(&c.id, &c.path, &c.size, &c.startedAt, &c.endedAt))
		got = append(got, c)
	}
	require.NoError(t, rows.Err())

	return got
}

type programRow struct {
	eventID   int64
	name      string
	startedAt time.Time
	endedAt   *time.Time
}

func programs(t *testing.T, e *recorderEnv) []programRow {
	t.Helper()

	rows, err := e.pool.Query(t.Context(), "SELECT event_id, name, started_at, ended_at FROM timeshift_programs ORDER BY started_at")
	require.NoError(t, err)
	defer rows.Close()
	got := []programRow{}
	for rows.Next() {
		var p programRow
		require.NoError(t, rows.Scan(&p.eventID, &p.name, &p.startedAt, &p.endedAt))
		got = append(got, p)
	}
	require.NoError(t, rows.Err())

	return got
}

// chunkPath は、サービス serviceID の、startedAt に始まるチャンクの、タイムシフトのディレクトリからのパス。
func chunkPath(serviceID int64, startedAt time.Time) string {
	return fmt.Sprintf("%d/%s.m2ts", serviceID, startedAt.UTC().Format("20060102T150405Z"))
}

// TestTimeshiftRecorderRotatesChunks は、ChunkDuration ごとにチャンクを閉じて次を始め、チャンクごとに索引を書くことを確かめる (設計書 5.7)。
func TestTimeshiftRecorderRotatesChunks(t *testing.T) {
	t.Parallel()

	e := newRecorderEnv(t)
	tuner := newFakeTuner(e.clk, false)
	start := e.clk.Now()
	_, done := session(t, e.newRecorder(tuner), e.assign(t, false))

	// 12 分の間、1 分ごとにデータを流す。
	tuner.next(t, nullPackets(10))
	for range 12 {
		e.clk.Advance(time.Minute)
		tuner.next(t, nullPackets(10))
	}

	got := chunks(t, e)
	require.Len(t, got, 3)
	wantStarts := []time.Time{start, start.Add(5 * time.Minute), start.Add(10 * time.Minute)}
	wantEnds := []*time.Time{new(start.Add(5 * time.Minute)), new(start.Add(10 * time.Minute)), nil}
	for i, c := range got {
		assert.True(t, wantStarts[i].Equal(c.startedAt), "chunk %d started at %s", i, c.startedAt)
		assert.Equal(t, chunkPath(grService(0), wantStarts[i]), c.path)
		if wantEnds[i] == nil {
			// 書いている途中のチャンクは、終わりも大きさも埋めない。
			assert.Nil(t, c.endedAt, "chunk %d", i)

			continue
		}
		require.NotNil(t, c.endedAt, "chunk %d", i)
		assert.True(t, wantEnds[i].Equal(*c.endedAt), "chunk %d ended at %s", i, *c.endedAt)
		info, err := os.Stat(filepath.Join(e.dir, c.path))
		require.NoError(t, err)
		assert.Positive(t, c.size)
		assert.Equal(t, info.Size(), c.size, "the size in the DB matches the file")
		// TS パケットの区切りで閉じる。
		assert.Zero(t, c.size%188)
		// 索引は、データディレクトリの、チャンクの ID のディレクトリに置く。
		_, err = os.Stat(filepath.Join(e.dataDir, kago.IndexLocation(c.id).Path))
		require.NoError(t, err)
	}
	// タイムシフトのストリームは、デコードして、予約の録画にもライブ視聴にも譲る優先度で開く。
	tuner.mu.Lock()
	assert.Equal(t, []mirakurun.StreamOptions{{Decode: true, Priority: -1}}, tuner.opts)
	tuner.mu.Unlock()

	select {
	case err := <-done:
		require.FailNow(t, "the session returned while streaming", "%v", err)
	default:
	}
}

// TestTimeshiftRecorderRotatesChunksWhileDatabaseIsUnreachable は、DB に届かない間も、ストリームを閉じずにチャンクを替えて書き続け、
// DB が戻ったら、書けなかったチャンクを記録することを確かめる (#1073)。DB への問い合わせが詰まっても、読み込みを止めない。
func TestTimeshiftRecorderRotatesChunksWhileDatabaseIsUnreachable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		outage func(db *databasetest.Switch)
	}{
		{name: "connection refused", outage: (*databasetest.Switch).Down},
		{name: "queries hang", outage: (*databasetest.Switch).Hang},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newRecorderEnv(t)
			db := databasetest.NewSwitch(t, e.pool)
			tuner := newFakeTuner(e.clk, false)
			start := e.clk.Now()
			_, done := session(t, e.newRecorderOn(db.Pool, tuner, nil), e.assign(t, false))

			tuner.next(t, nullPackets(10))
			tt.outage(db)
			// 12 分の間、1 分ごとにデータを流す。チャンクは 5 分ごとに替わる。
			for range 12 {
				e.clk.Advance(time.Minute)
				tuner.next(t, nullPackets(10))
			}
			select {
			case err := <-done:
				require.FailNow(t, "the session returned while the database was unreachable", "%v", err)
			default:
			}
			db.Up()

			wantStarts := []time.Time{start, start.Add(5 * time.Minute), start.Add(10 * time.Minute)}
			require.Eventually(t, func() bool {
				got := chunks(t, e)

				return len(got) == 3 && got[1].endedAt != nil
			}, waitTimeout, 10*time.Millisecond)
			got := chunks(t, e)
			for i, c := range got {
				assert.True(t, wantStarts[i].Equal(c.startedAt), "chunk %d started at %s", i, c.startedAt)
				info, err := os.Stat(filepath.Join(e.dir, c.path))
				require.NoError(t, err, "chunk %d is kept on the disk", i)
				if i == len(got)-1 {
					assert.Nil(t, c.endedAt, "the last chunk is still being written")

					continue
				}
				require.NotNil(t, c.endedAt, "chunk %d", i)
				assert.True(t, wantStarts[i+1].Equal(*c.endedAt), "chunk %d ended at %s", i, *c.endedAt)
				assert.Equal(t, info.Size(), c.size, "chunk %d: the size in the DB matches the file", i)
			}
			// ストリームは開き直していない。
			tuner.mu.Lock()
			assert.Len(t, tuner.opts, 1)
			tuner.mu.Unlock()
		})
	}
}

// TestTimeshiftRecorderReconnectBackoff は、開いてすぐ切れ続けると、開き直す間隔を、Backoff の間隔で延ばしていくことを確かめる。
func TestTimeshiftRecorderReconnectBackoff(t *testing.T) {
	t.Parallel()

	e := newRecorderEnv(t)
	tuner := newFakeTuner(e.clk, true)
	session(t, e.newRecorder(tuner), e.assign(t, false))
	require.Eventually(t, func() bool { return len(tuner.openTimes()) == 1 }, waitTimeout, time.Millisecond)

	wantIntervals := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 60 * time.Second, 60 * time.Second}
	for i, interval := range wantIntervals {
		// 間隔の 1 秒前までは、開き直さない。
		e.clk.Advance(interval - time.Second)
		time.Sleep(20 * testTick)
		require.Len(t, tuner.openTimes(), i+1, "reopened before %s", interval)
		e.clk.Advance(time.Second)
		require.Eventually(t, func() bool { return len(tuner.openTimes()) == i+2 }, waitTimeout, time.Millisecond, "did not reopen after %s", interval)
	}

	opens := tuner.openTimes()
	got := make([]time.Duration, 0, len(opens)-1)
	for i := 1; i < len(opens); i++ {
		got = append(got, opens[i].Sub(opens[i-1]))
	}
	assert.Equal(t, wantIntervals, got)
	// データが届かなければ、チャンクを作らない。
	assert.Empty(t, chunks(t, e))
}

// TestTimeshiftRecorderRecordsPrograms は、EIT[p/f] の放送中の番組が替わった時刻を、timeshift_programs に記録することを確かめる。
// 番組名は programs から引き、なければ空にする。
func TestTimeshiftRecorderRecordsPrograms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// unreachable は、番組が替わる間、DB に届かないかどうか。届くようになってから、同じ順で記録する (#1073)。
		unreachable bool
	}{
		{name: "database reachable", unreachable: false},
		{name: "database unreachable", unreachable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newRecorderEnv(t)
			db := databasetest.NewSwitch(t, e.pool)
			tuner := newFakeTuner(e.clk, false)
			start := e.clk.Now()
			_, done := session(t, e.newRecorderOn(db.Pool, tuner, nil), e.assign(t, false))

			// 偽チューナーサーバーの EPG では、12:00 から eventId 3、12:30 から eventId 4 を放送する。eventId 9999 は EPG にない。
			cc := byte(0)
			tuner.next(t, eitPFPackets(&cc, 3, 4))
			if tt.unreachable {
				db.Down()
			}
			e.clk.Advance(time.Minute)
			// 同じ番組のままなら、行を足さない。
			tuner.next(t, eitPFPackets(&cc, 3, 4))
			e.clk.Advance(time.Minute)
			tuner.next(t, eitPFPackets(&cc, 4, 5))
			e.clk.Advance(time.Minute)
			tuner.next(t, eitPFPackets(&cc, 9999, 5))
			db.Up()

			want := []programRow{
				{eventID: 3, name: "こたつテスト1 テスト番組 3", startedAt: start, endedAt: new(start.Add(2 * time.Minute))},
				{eventID: 4, name: "こたつテスト1 テスト番組 4", startedAt: start.Add(2 * time.Minute), endedAt: new(start.Add(3 * time.Minute))},
				{eventID: 9999, name: "", startedAt: start.Add(3 * time.Minute), endedAt: nil},
			}
			require.Eventually(t, func() bool {
				return len(programs(t, e)) == len(want)
			}, waitTimeout, 10*time.Millisecond)
			got := programs(t, e)
			for i := range want {
				assert.Equal(t, want[i].eventID, got[i].eventID, "program %d", i)
				assert.Equal(t, want[i].name, got[i].name, "program %d", i)
				assert.True(t, want[i].startedAt.Equal(got[i].startedAt), "program %d started at %s", i, got[i].startedAt)
				if want[i].endedAt == nil {
					assert.Nil(t, got[i].endedAt, "program %d", i)

					continue
				}
				require.NotNil(t, got[i].endedAt, "program %d", i)
				assert.True(t, want[i].endedAt.Equal(*got[i].endedAt), "program %d ended at %s", i, *got[i].endedAt)
			}
			select {
			case err := <-done:
				require.FailNow(t, "the session returned while streaming", "%v", err)
			default:
			}
		})
	}
}

// TestTimeshiftRecorderClosesChunkOnStop は、Session を止めると、書きかけのチャンクを閉じて返ることを確かめる。
// 止めても番組は放送中のままなので、番組は閉じない。
func TestTimeshiftRecorderClosesChunkOnStop(t *testing.T) {
	t.Parallel()

	e := newRecorderEnv(t)
	tuner := newFakeTuner(e.clk, false)
	start := e.clk.Now()
	cancel, done := session(t, e.newRecorder(tuner), e.assign(t, false))
	cc := byte(0)
	tuner.next(t, eitPFPackets(&cc, 3, 4))
	e.clk.Advance(time.Minute)
	tuner.next(t, nullPackets(10))

	cancel(recording.ErrStopped)
	require.ErrorIs(t, wait(t, done), recording.ErrStopped)

	got := chunks(t, e)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].endedAt)
	assert.True(t, start.Add(time.Minute).Equal(*got[0].endedAt))
	info, err := os.Stat(filepath.Join(e.dir, got[0].path))
	require.NoError(t, err)
	assert.Equal(t, info.Size(), got[0].size)
	assert.Equal(t, int64(12*188), got[0].size)
	gotPrograms := programs(t, e)
	require.Len(t, gotPrograms, 1)
	assert.Nil(t, gotPrograms[0].endedAt)
}

// TestTimeshiftRecorderStopsWhenAssignmentDeleted は、neko が割り振りを消したら (チューナーを予約に渡すとき)、
// ストリームを閉じ、チャンクと番組を閉じて返ることを確かめる (設計書 5.7)。
func TestTimeshiftRecorderStopsWhenAssignmentDeleted(t *testing.T) {
	t.Parallel()

	e := newRecorderEnv(t)
	tuner := newFakeTuner(e.clk, false)
	start := e.clk.Now()
	_, done := session(t, e.newRecorder(tuner), e.assign(t, false))
	cc := byte(0)
	tuner.next(t, eitPFPackets(&cc, 3, 4))
	e.clk.Advance(time.Minute)

	_, err := e.q.DeleteTimeshiftAssignment(t.Context(), grService(0))
	require.NoError(t, err)
	require.NoError(t, wait(t, done))

	got := chunks(t, e)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].endedAt)
	assert.True(t, start.Add(time.Minute).Equal(*got[0].endedAt))
	gotPrograms := programs(t, e)
	require.Len(t, gotPrograms, 1)
	require.NotNil(t, gotPrograms[0].endedAt)
	assert.True(t, start.Add(time.Minute).Equal(*gotPrograms[0].endedAt))
}

// TestTimeshiftRecorderClosesStaleChunks は、割り振りを取ったときに、落ちた mikan が残した書きかけのチャンクを、ファイルの大きさと
// 最後に書き込んだ時刻で閉じ、放送中のままの番組を、チャンクの最後の終わりで閉じることを確かめる。落ちてから今までは、バッファの穴になる。
// 止まる mikan から引き継いだときは、古い mikan がまだ書いているので、閉じない。
func TestTimeshiftRecorderClosesStaleChunks(t *testing.T) {
	t.Parallel()

	const stalePackets = 5
	tests := []struct {
		name       string
		handedOver bool
		// writeFile は、落ちた mikan のチャンクのファイルを、ディスクに残すかどうか。
		writeFile  bool
		wantClosed bool
		wantSize   int64
		// wantEnd は、チャンクと番組を閉じた時刻の、チャンクの開始からの長さ。
		wantEnd time.Duration
	}{
		{name: "落ちた mikan の代わり", handedOver: false, writeFile: true, wantClosed: true, wantSize: stalePackets * 188, wantEnd: 3 * time.Minute},
		// ファイルを読めなければ、書き始めた時刻で閉じる。
		{name: "ファイルがない", handedOver: false, writeFile: false, wantClosed: true, wantSize: 0, wantEnd: 0},
		{name: "引き継ぎ", handedOver: true, writeFile: true, wantClosed: false, wantSize: 0, wantEnd: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newRecorderEnv(t)
			staleStart := e.clk.Now().Add(-10 * time.Minute)
			staleID := id.New()
			stalePath := chunkPath(grService(0), staleStart)
			require.NoError(t, e.q.InsertTimeshiftChunk(t.Context(), sqlc.InsertTimeshiftChunkParams{
				ID: staleID, ServiceID: grService(0), Path: stalePath, Size: 0, StartedAt: staleStart, Now: staleStart,
			}))
			if tt.writeFile {
				// 落ちた mikan は、チャンクを始めて 3 分後まで書いていた。
				file := filepath.Join(e.dir, stalePath)
				require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
				require.NoError(t, os.WriteFile(file, nullPackets(stalePackets), 0o600))
				lastWrite := staleStart.Add(3 * time.Minute)
				require.NoError(t, os.Chtimes(file, lastWrite, lastWrite))
			}
			require.NoError(t, e.q.StartTimeshiftProgram(t.Context(), sqlc.StartTimeshiftProgramParams{
				ServiceID: grService(0), StartedAt: staleStart, EventID: 3, Name: "こたつテスト1 テスト番組 3",
			}))
			tuner := newFakeTuner(e.clk, false)
			_, done := session(t, e.newRecorder(tuner), e.assign(t, tt.handedOver))
			// 同じ番組を読む。引き継いだなら、前の mikan の行の続きとして扱う。
			cc := byte(0)
			tuner.next(t, eitPFPackets(&cc, 3, 4))

			got := chunks(t, e)
			require.Len(t, got, 2)
			assert.Equal(t, staleID, got[0].id)
			gotPrograms := programs(t, e)
			if tt.wantClosed {
				wantEnd := staleStart.Add(tt.wantEnd)
				require.NotNil(t, got[0].endedAt)
				assert.True(t, wantEnd.Equal(*got[0].endedAt), "the stale chunk ended at %s", *got[0].endedAt)
				assert.Equal(t, tt.wantSize, got[0].size)
				// 閉じた番組と、読み直した番組の 2 行になる。間は、録れていない穴になる。
				require.Len(t, gotPrograms, 2)
				require.NotNil(t, gotPrograms[0].endedAt)
				assert.True(t, wantEnd.Equal(*gotPrograms[0].endedAt), "the stale program ended at %s", *gotPrograms[0].endedAt)
				assert.True(t, e.clk.Now().Equal(gotPrograms[1].startedAt))
				assert.Nil(t, gotPrograms[1].endedAt)
			} else {
				assert.Nil(t, got[0].endedAt)
				require.Len(t, gotPrograms, 1)
				assert.Nil(t, gotPrograms[0].endedAt)
			}
			select {
			case err := <-done:
				require.FailNow(t, "the session returned while streaming", "%v", err)
			default:
			}
		})
	}
}

// TestTimeshiftRecorderReleasesWhenUnwritable は、タイムシフトのディレクトリにチャンクを作れないとき、ほかに mikan がいれば、
// 割り振りを手放すために ErrRecorderUnavailable を返し、いなければ、開き直してやり直すことを確かめる (#524)。
func TestTimeshiftRecorderReleasesWhenUnwritable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		peers        int
		wantReleased bool
	}{
		{name: "ほかに mikan がいる", peers: 1, wantReleased: true},
		{name: "ほかに mikan がいない", peers: 0, wantReleased: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newRecorderEnv(t)
			// タイムシフトのディレクトリが、マウントが外れるなどして、ディレクトリでなくなった。
			require.NoError(t, os.RemoveAll(e.dir))
			require.NoError(t, os.WriteFile(e.dir, nil, 0o600))
			tuner := newFakeTuner(e.clk, false)
			recorder := e.newRecorderWithPeers(tuner, func(context.Context) (int, error) { return tt.peers, nil })
			_, done := session(t, recorder, e.assign(t, false))
			// チャンクを作れずにストリームを閉じるので、次の読み込みは待たない。
			select {
			case tuner.chunks <- &streamChunk{data: nullPackets(10), taken: make(chan int64, 1)}:
			case <-time.After(waitTimeout):
				require.FailNow(t, "the recorder did not read the stream")
			}

			if tt.wantReleased {
				require.ErrorIs(t, wait(t, done), recording.ErrRecorderUnavailable)
				assert.Len(t, tuner.openTimes(), 1)
			} else {
				e.clk.Advance(5 * time.Second)
				require.Eventually(t, func() bool { return len(tuner.openTimes()) == 2 }, waitTimeout, time.Millisecond)
				select {
				case err := <-done:
					require.FailNow(t, "the session returned", "%v", err)
				default:
				}
			}
			assert.Empty(t, chunks(t, e))
		})
	}
}

// panicTuner は、ストリームを開くとき (atOpen) か、data を流した後の読み込みで panic する偽のチューナーサーバー。
type panicTuner struct {
	atOpen bool
	data   []byte
}

var _ recording.StreamOpener = new(panicTuner)

func (p *panicTuner) OpenServiceStream(context.Context, int64, *mirakurun.StreamOptions) (io.ReadCloser, error) {
	if p.atOpen {
		panic("the tuner exploded on open")
	}

	return &panicStream{data: p.data, read: false}, nil
}

// panicStream は、最初の読み込みで data を返し、次の読み込みで panic するストリーム。
type panicStream struct {
	data []byte
	read bool
}

func (s *panicStream) Read(p []byte) (int, error) {
	if s.read {
		panic("the tuner exploded while streaming")
	}
	s.read = true

	return copy(p, s.data), nil
}

func (s *panicStream) Close() error { return nil }

// TestTimeshiftRecorderRecoversPanic は、タイムシフトの Session の中で panic しても、mikan を落とさず、
// スタックをログに残して、エラーとして返ることを確かめる。書いていたチャンクは閉じる (#1055)。
func TestTimeshiftRecorderRecoversPanic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		atOpen     bool
		wantChunks int
	}{
		{name: "ストリームを開くときの panic", atOpen: true, wantChunks: 0},
		{name: "書いている途中の panic では、チャンクを閉じる", atOpen: false, wantChunks: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newRecorderEnv(t)
			var logs bytes.Buffer
			recorder := timeshift.NewRecorder(e.pool, e.clk, &panicTuner{atOpen: tt.atOpen, data: nullPackets(10)}, e.store,
				slog.New(slog.NewTextHandler(&logs, nil)), &timeshift.RecorderOptions{ChunkDuration: 5 * time.Minute, Backoff: nil, Tick: testTick, Peers: nil})
			_, done := session(t, recorder, e.assign(t, false))

			err := wait(t, done)
			require.ErrorContains(t, err, "panicked")
			assert.Contains(t, logs.String(), "the timeshift session panicked")
			assert.Contains(t, logs.String(), "the tuner exploded")
			assert.Contains(t, logs.String(), "goroutine", "the stack is logged")
			got := chunks(t, e)
			require.Len(t, got, tt.wantChunks)
			for _, c := range got {
				require.NotNil(t, c.endedAt, "the chunk is closed")
				assert.Equal(t, int64(10*188), c.size)
			}
		})
	}
}
