package kyusu_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
)

// TestFormatTimeshiftID は、タイムシフトのバッファの仮想 HLS のパスの {id} を、サービス ID と起点の時刻に読み戻せることを確かめる。
func TestFormatTimeshiftID(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 10, 29, 9, 47, 29, 439652000, time.UTC)
	timeshiftID := kyusu.FormatTimeshiftID(serviceID, from)
	assert.Equal(t, "3273601024-"+strconv.FormatInt(from.UnixMicro(), 10), timeshiftID)
	gotService, gotFrom, ok := kyusu.ParseTimeshiftID(timeshiftID)
	require.True(t, ok)
	assert.Equal(t, int64(serviceID), gotService)
	assert.True(t, from.Equal(gotFrom))

	for _, invalid := range []string{"3273601024", "abc-1", "1-abc", ""} {
		_, _, ok := kyusu.ParseTimeshiftID(invalid)
		assert.False(t, ok, invalid)
	}
}

// bufferChunk は、data のチャンクと索引を置き、サービスをタイムシフトで録るチャンネルにする。チャンクは startedAt に始まり、書いている途中。
func (f *vodFixture) bufferChunk(t *testing.T, data []byte, startedAt time.Time) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO timeshift_channels (service_id, retention_seconds, created_at, updated_at)
		VALUES ($1, 7200, $2, $2) ON CONFLICT DO NOTHING`, serviceID, startedAt)
	require.NoError(t, err)
	path := strconv.Itoa(serviceID) + "/" + strconv.FormatInt(startedAt.Unix(), 10) + ".m2ts"
	require.NoError(t, os.MkdirAll(filepath.Join(f.bufferDir, strconv.Itoa(serviceID)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(f.bufferDir, path), data, 0o600))
	var index bytes.Buffer
	writer, err := ts.NewIndexWriter(&index)
	require.NoError(t, err)
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{Index: writer})
	_, err = analyzer.Write(data)
	require.NoError(t, err)
	chunkID := id.New()
	require.NoError(t, os.MkdirAll(filepath.Join(f.dir, chunkID.String()), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, kago.IndexLocation(chunkID).Path), index.Bytes(), 0o600))
	_, err = f.pool.Exec(t.Context(), `INSERT INTO timeshift_chunks (id, service_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, $3, $4, $5, NULL, $5)`, chunkID, serviceID, path, len(data), startedAt)
	require.NoError(t, err)
}

// getTimeshift は、トークンを付けて、タイムシフトのバッファの {id} の file を取得する。
func (f *vodFixture) getTimeshift(t *testing.T, timeshiftID, file string) (int, string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, f.server.URL+kyusu.TimeshiftPathPrefix+timeshiftID+"/360p/1/"+file, nil)
	require.NoError(t, err)
	f.token.Authorize(req)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(body)
}

// TestVODTimeshift は、タイムシフトのバッファを、起点からの仮想 HLS のプレイリストにすることを確かめる (設計書 5.7、7.3)。
// バッファは伸び続けるので、EVENT にし、終わりの印を付けない。起点より前の I フレームは載せない。
func TestVODTimeshift(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	f := newVODFixture(t, nil)
	origin := f.clock.Now().Add(-time.Hour)
	f.bufferChunk(t, data, origin)

	status, whole := f.getTimeshift(t, kyusu.FormatTimeshiftID(serviceID, origin), kyusu.PlaylistName)
	require.Equal(t, http.StatusOK, status, whole)
	assert.Contains(t, whole, "#EXT-X-PLAYLIST-TYPE:EVENT")
	assert.NotContains(t, whole, "#EXT-X-ENDLIST")
	// testdata/ts/test.ts の I フレームは、6 秒ほど並ぶ。書き終えた 2 秒ずつのセグメントだけを載せる。
	assert.Equal(t, 3, strings.Count(whole, "#EXTINF:"))

	// 起点を 2 秒後にすると、起点より前の I フレームを載せないので、セグメントが 1 つ減る。
	status, later := f.getTimeshift(t, kyusu.FormatTimeshiftID(serviceID, origin.Add(2*time.Second)), kyusu.PlaylistName)
	require.Equal(t, http.StatusOK, status, later)
	assert.Equal(t, 2, strings.Count(later, "#EXTINF:"))

	tests := []struct {
		name        string
		timeshiftID string
		want        int
	}{
		{name: "読めない ID", timeshiftID: "abc", want: http.StatusBadRequest},
		{name: "タイムシフトで録っていないサービス", timeshiftID: kyusu.FormatTimeshiftID(serviceID+1, origin), want: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body := f.getTimeshift(t, tt.timeshiftID, kyusu.PlaylistName)
			assert.Equal(t, tt.want, status, body)
		})
	}
}
