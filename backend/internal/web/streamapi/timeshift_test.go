package streamapi_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/web/streamapi"
)

const (
	// timeshiftPattern は、タイムシフトのバッファの再生のパス。
	timeshiftPattern = "GET /streams/timeshift/{id}"
	// atOrigin は、起点から流し始めたときに返す位置。
	atOrigin = "0.000"
)

type timeshiftFixture struct {
	pool *pgxpool.Pool
	// dataDir は索引を置くデータディレクトリ、bufferDir はチャンクを置くタイムシフトのディレクトリ。
	dataDir   string
	bufferDir string
	server    *httptest.Server
	// origin は、最初のチャンクの始まり。
	origin time.Time
}

// newTimeshiftFixture は、サービスをタイムシフトで録るチャンネルにし、バッファを流すサーバーを立てる。
// configured が false なら、タイムシフトのディレクトリを設定していない tenban にする。
func newTimeshiftFixture(t *testing.T, configured bool) *timeshiftFixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	origin := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $2, $2)`, serviceID, origin)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO timeshift_channels (service_id, retention_seconds, created_at, updated_at)
		VALUES ($1, 7200, $2, $2)`, serviceID, origin)
	require.NoError(t, err)
	f := &timeshiftFixture{pool: pool, dataDir: t.TempDir(), bufferDir: t.TempDir(), server: nil, origin: origin}
	store := kago.New(map[string]string{}, nil).WithDataDir(f.dataDir)
	if configured {
		store = store.WithTimeshiftDir(f.bufferDir)
	}
	mux := http.NewServeMux()
	mux.Handle(timeshiftPattern, streamapi.NewTimeshiftHandler(pool, store, slog.New(slog.DiscardHandler), pollInterval))
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)

	return f
}

// chunk は、data のチャンクと、ts.Analyzer で作った索引を置く。チャンクは origin から from 後に始まり、to 後に終わる。
func (f *timeshiftFixture) chunk(t *testing.T, name string, data []byte, from, to time.Duration) {
	t.Helper()

	f.writeChunk(t, name, data, f.origin.Add(from), new(f.origin.Add(to)))
}

// writeChunk は、data のチャンクと索引を置き、startedAt に始まり endedAt に終わったチャンクとして記録する。endedAt が nil なら、書いている途中にする。
func (f *timeshiftFixture) writeChunk(t *testing.T, name string, data []byte, startedAt time.Time, endedAt *time.Time) uuid.UUID {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Join(f.bufferDir, serviceID), 0o755))
	path := serviceID + "/" + name
	require.NoError(t, os.WriteFile(filepath.Join(f.bufferDir, path), data, 0o600))
	var index bytes.Buffer
	writer, err := ts.NewIndexWriter(&index)
	require.NoError(t, err)
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer})
	_, err = analyzer.Write(data)
	require.NoError(t, err)
	chunkID := id.New()
	indexLocation := kago.IndexLocation(chunkID)
	require.NoError(t, os.MkdirAll(filepath.Join(f.dataDir, chunkID.String()), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(f.dataDir, indexLocation.Path), index.Bytes(), 0o600))
	_, err = f.pool.Exec(t.Context(), `INSERT INTO timeshift_chunks (id, service_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $5)`, chunkID, serviceID, path, len(data), startedAt, endedAt)
	require.NoError(t, err)

	return chunkID
}

// get は、バッファを from の時刻を起点に、クエリ query の位置から取得する。
// バッファは終わらないので、応答のヘッダーを受け取ったら、サービスをタイムシフトから外して、流し終えさせる。
func (f *timeshiftFixture) get(t *testing.T, from time.Time, query string) *fetched {
	t.Helper()

	url := f.server.URL + "/streams/timeshift/" + serviceID + "?from=" + from.Format(time.RFC3339Nano) + query
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		_, err = f.pool.Exec(t.Context(), "DELETE FROM timeshift_channels WHERE service_id = $1", serviceID)
		require.NoError(t, err)
	}
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return &fetched{status: resp.StatusCode, contentType: resp.Header.Get("Content-Type"), disposition: "", start: resp.Header.Get(streamapi.StartHeader), body: body}
}

// TestTimeshiftHandler は、タイムシフトのバッファを、起点からの位置の手前の I フレームから流し、チャンクをつないで流すことを確かめる。
// mikan が区切ったチャンクは、バイトをそのままつなぎ、引き継ぎで重なったチャンクは切り詰める (設計書 5.7、7.2)。
func TestTimeshiftHandler(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	// cut は、GOP の途中のパケットの境目。mikan はチャンクを GOP の先頭に合わせずに替える。
	cut := packetAligned(350000)
	tests := []struct {
		name   string
		chunks func(t *testing.T, f *timeshiftFixture)
		// from は、起点の、最初のチャンクの始まりからの時間。
		from      time.Duration
		query     string
		want      []int64
		wantStart string
		// wantSuffix は、流したバイト列の終わり。区切ったチャンクの始まりを落とさずにつないだことを確かめる。
		wantSuffix []byte
	}{
		{
			name: "区切ったチャンクは、GOP の途中でもそのままつなぐ",
			chunks: func(t *testing.T, f *timeshiftFixture) {
				t.Helper()
				f.chunk(t, "a.m2ts", data[:cut], 0, time.Minute)
				f.chunk(t, "b.m2ts", data[cut:], time.Minute, 2*time.Minute)
			},
			from: 0, query: "", want: wantIFramePTS(), wantStart: atOrigin, wantSuffix: data[cut:],
		},
		{
			name: "引き継ぎで重なったチャンクは切り詰める",
			chunks: func(t *testing.T, f *timeshiftFixture) {
				t.Helper()
				f.chunk(t, "a.m2ts", data[:packetAligned(500000)], 0, time.Minute)
				f.chunk(t, "b.m2ts", data[packetAligned(200000):], time.Second, 2*time.Minute)
			},
			from: 0, query: "", want: wantIFramePTS(), wantStart: atOrigin, wantSuffix: nil,
		},
		{
			name: "起点からの位置の手前の I フレームから流す",
			chunks: func(t *testing.T, f *timeshiftFixture) {
				t.Helper()
				f.chunk(t, "a.m2ts", data, 0, time.Minute)
			},
			from: 0, query: "&start=2.01", want: wantIFramePTS()[4:], wantStart: "2.002", wantSuffix: nil,
		},
		{
			name: "起点がチャンクの途中なら、チャンクの始まりからの時間を引いた位置で表す",
			chunks: func(t *testing.T, f *timeshiftFixture) {
				t.Helper()
				f.chunk(t, "a.m2ts", data, 0, time.Minute)
			},
			from: time.Second, query: "&start=1.01", want: wantIFramePTS()[4:], wantStart: "1.002", wantSuffix: nil,
		},
		{
			name: "起点より前に終わったチャンクは流さない",
			chunks: func(t *testing.T, f *timeshiftFixture) {
				t.Helper()
				f.chunk(t, "a.m2ts", data[:cut], 0, time.Minute)
				f.chunk(t, "b.m2ts", data[cut:], time.Minute, 2*time.Minute)
			},
			from: time.Minute, query: "", want: iframes(t, data[cut:]), wantStart: atOrigin, wantSuffix: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newTimeshiftFixture(t, true)
			tt.chunks(t, f)

			got := f.get(t, f.origin.Add(tt.from), tt.query)
			require.Equal(t, http.StatusOK, got.status, string(got.body))
			assert.Equal(t, "video/mp2t", got.contentType)
			assert.Zero(t, len(got.body)%ts.PacketSize, "the body must consist of whole packets")
			assert.Equal(t, tt.want, iframes(t, got.body))
			assert.Equal(t, tt.wantStart, got.start)
			if tt.wantSuffix != nil {
				assert.True(t, bytes.HasSuffix(got.body, tt.wantSuffix), "the next chunk must be joined from its first byte")
			}
		})
	}
}

// TestTimeshiftHandlerFollowsChunks は、書いている途中のチャンクを流している間に、mikan がチャンクを区切ったら、
// 次のチャンクを、始まりのバイトからつないで流し続けることを確かめる (設計書 5.7)。
func TestTimeshiftHandlerFollowsChunks(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	cut := packetAligned(350000)
	f := newTimeshiftFixture(t, true)
	first := f.writeChunk(t, "a.m2ts", data[:cut], f.origin, nil)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.server.URL+"/streams/timeshift/"+serviceID+"?from="+f.origin.Format(time.RFC3339Nano), nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// mikan は、同じ時刻で前のチャンクを閉じ、次のチャンクを作る。
	rotatedAt := f.origin.Add(time.Minute)
	_, err = f.pool.Exec(t.Context(), "UPDATE timeshift_chunks SET ended_at = $1 WHERE id = $2", rotatedAt, first)
	require.NoError(t, err)
	f.writeChunk(t, "b.m2ts", data[cut:], rotatedAt, nil)

	// 次のチャンクを終わりまで流したら、サービスを外して、流し終えさせる。
	var body bytes.Buffer
	buf := make([]byte, ts.PacketSize*64)
	for !bytes.HasSuffix(body.Bytes(), data[cut:]) {
		n, err := resp.Body.Read(buf)
		body.Write(buf[:n])
		require.NoError(t, err, "the next chunk must be joined from its first byte")
	}
	_, err = f.pool.Exec(t.Context(), "UPDATE timeshift_chunks SET ended_at = $1 WHERE ended_at IS NULL", rotatedAt.Add(time.Minute))
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "DELETE FROM timeshift_channels WHERE service_id = $1", serviceID)
	require.NoError(t, err)
	rest, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	body.Write(rest)
	assert.Equal(t, wantIFramePTS(), iframes(t, body.Bytes()))
}

// TestTimeshiftHandlerRejects は、流せないリクエストを断ることを確かめる。
func TestTimeshiftHandlerRejects(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	tests := []struct {
		name       string
		configured bool
		// removed なら、サービスをタイムシフトから外しておく。
		removed bool
		path    string
		want    int
	}{
		{name: "タイムシフトのディレクトリがない", configured: false, removed: false, path: serviceID + "?from=2026-10-29T12:00:00Z", want: http.StatusServiceUnavailable},
		{name: "タイムシフトで録っていない", configured: true, removed: true, path: serviceID + "?from=2026-10-29T12:00:00Z", want: http.StatusNotFound},
		{name: "起点より後のチャンクがない", configured: true, removed: false, path: serviceID + "?from=2026-10-29T13:00:00Z", want: http.StatusNotFound},
		{name: "起点がない", configured: true, removed: false, path: serviceID, want: http.StatusBadRequest},
		{name: "起点が時刻でない", configured: true, removed: false, path: serviceID + "?from=1730203200", want: http.StatusBadRequest},
		{name: "位置が負", configured: true, removed: false, path: serviceID + "?from=2026-10-29T12:00:00Z&start=-1", want: http.StatusBadRequest},
		{name: "サービス ID が数でない", configured: true, removed: false, path: "abc?from=2026-10-29T12:00:00Z", want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newTimeshiftFixture(t, tt.configured)
			f.chunk(t, "a.m2ts", data, 0, time.Minute)
			if tt.removed {
				_, err := f.pool.Exec(t.Context(), "DELETE FROM timeshift_channels WHERE service_id = $1", serviceID)
				require.NoError(t, err)
			}

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+"/streams/timeshift/"+tt.path, nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, tt.want, resp.StatusCode)
		})
	}
}
