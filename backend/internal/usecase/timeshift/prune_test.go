package timeshift_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/timeshift"
)

// writeUnder は、root の下の rel に、中身のあるファイルを作る。
func writeUnder(t *testing.T, root, rel string) {
	t.Helper()

	full := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte("data"), 0o600))
}

// TestPrune は、保持時間を過ぎたチャンクの、ファイルと索引と行を消し、書きかけのチャンクと保持時間の内のチャンクは残すことを確かめる。
// チャンネルの設定を消したサービスのチャンクは全部消え、ファイルのないチャンクも行は消え、古い番組の行も消えることも確かめる。
func TestPrune(t *testing.T) {
	t.Parallel()

	e := newRecorderEnv(t)
	// 0 番目のサービスだけ、保持時間 2 時間のチャンネルとして設定する。1 番目のサービスは設定がない。
	e.configure(t, 0)
	now := e.clk.Now()

	// startedAgo と endedAgo は、今から遡った時間。endedAgo が 0 なら書きかけ。
	tests := []struct {
		name       string
		service    int
		startedAgo time.Duration
		endedAgo   time.Duration
		noFile     bool
		kept       bool
	}{
		{name: "保持時間を 1 分過ぎた", service: 0, startedAgo: 2*time.Hour + 6*time.Minute, endedAgo: 2*time.Hour + time.Minute, noFile: false, kept: false},
		{name: "保持時間の 1 分前", service: 0, startedAgo: 2*time.Hour + 4*time.Minute, endedAgo: 2*time.Hour - time.Minute, noFile: false, kept: true},
		{name: "書きかけ", service: 0, startedAgo: 4 * time.Minute, endedAgo: 0, noFile: false, kept: true},
		{name: "ファイルのない", service: 0, startedAgo: 3*time.Hour + 5*time.Minute, endedAgo: 3 * time.Hour, noFile: true, kept: false},
		{name: "設定を消したサービスの新しい", service: 1, startedAgo: 15 * time.Minute, endedAgo: 10 * time.Minute, noFile: false, kept: false},
	}
	ids := make([]uuid.UUID, len(tests))
	paths := make([]string, len(tests))
	for i, tt := range tests {
		ids[i] = id.New()
		started := now.Add(-tt.startedAgo)
		paths[i] = chunkPath(grService(tt.service), started)
		require.NoError(t, e.q.InsertTimeshiftChunk(t.Context(), sqlc.InsertTimeshiftChunkParams{
			ID: ids[i], ServiceID: grService(tt.service), Path: paths[i], Size: 4, StartedAt: started, Now: started,
		}))
		if tt.endedAgo > 0 {
			n, err := e.q.CloseTimeshiftChunk(t.Context(), sqlc.CloseTimeshiftChunkParams{ID: ids[i], Size: 4, EndedAt: new(now.Add(-tt.endedAgo))})
			require.NoError(t, err)
			require.EqualValues(t, 1, n)
		}
		if !tt.noFile {
			writeUnder(t, e.dir, paths[i])
			writeUnder(t, e.dataDir, ids[i].String()+"/index")
		}
	}
	// 番組は、残るチャンクの最も早い始まりより前に終わったものを消す。放送中のものは消さない。
	// チャンクが 1 つも残らないサービスは、今より前に終わったものを消す。
	programRows := []struct {
		service int
		started time.Duration
		ended   time.Duration
	}{
		{service: 0, started: 4 * time.Hour, ended: 3*time.Hour + 30*time.Minute},
		{service: 0, started: 2 * time.Hour, ended: 90 * time.Minute},
		{service: 0, started: 10 * time.Minute, ended: 0},
		{service: 1, started: time.Hour, ended: 20 * time.Minute},
	}
	for _, p := range programRows {
		var ended *time.Time
		if p.ended > 0 {
			ended = new(now.Add(-p.ended))
		}
		_, err := e.pool.Exec(t.Context(), "INSERT INTO timeshift_programs (service_id, started_at, event_id, name, ended_at) VALUES ($1, $2, 1, 'p', $3)",
			grService(p.service), now.Add(-p.started), ended)
		require.NoError(t, err)
	}

	pruned, err := timeshift.NewPruneWorker(e.pool, e.clk, e.store, slog.New(slog.DiscardHandler)).RunOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, pruned)

	rows := map[uuid.UUID]bool{}
	for _, c := range chunks(t, e) {
		rows[c.id] = true
	}
	for i, tt := range tests {
		assert.Equal(t, tt.kept, rows[ids[i]], "%s: the row", tt.name)
		if tt.noFile {
			continue
		}
		_, fileErr := os.Stat(filepath.Join(e.dir, paths[i]))
		_, indexErr := os.Stat(filepath.Join(e.dataDir, ids[i].String()))
		assert.Equal(t, tt.kept, fileErr == nil, "%s: the chunk file", tt.name)
		assert.Equal(t, tt.kept, indexErr == nil, "%s: the index directory", tt.name)
	}

	remaining := programs(t, e)
	left := make([]int, 0, len(remaining))
	for _, p := range remaining {
		left = append(left, int(now.Sub(p.startedAt)/time.Minute))
	}
	assert.Equal(t, []int{120, 10}, left, "the programs that ended before the oldest chunk are removed, and the open one stays")
}

// TestPruneWithoutTimeshiftDir は、タイムシフトのディレクトリを設定していなければ、何も消さないことを確かめる。
func TestPruneWithoutTimeshiftDir(t *testing.T) {
	t.Parallel()

	e := newRecorderEnv(t)
	started := e.clk.Now().Add(-5 * time.Hour)
	chunkID := id.New()
	require.NoError(t, e.q.InsertTimeshiftChunk(t.Context(), sqlc.InsertTimeshiftChunkParams{
		ID: chunkID, ServiceID: grService(0), Path: chunkPath(grService(0), started), Size: 4, StartedAt: started, Now: started,
	}))
	_, err := e.q.CloseTimeshiftChunk(t.Context(), sqlc.CloseTimeshiftChunkParams{ID: chunkID, Size: 4, EndedAt: new(started.Add(5 * time.Minute))})
	require.NoError(t, err)

	store := kago.New(map[string]string{}, nil).WithDataDir(e.dataDir)
	pruned, err := timeshift.NewPruneWorker(e.pool, e.clk, store, slog.New(slog.DiscardHandler)).RunOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 0, pruned)
	assert.Len(t, chunks(t, e), 1)
}

// TestPruneClosesAbandonedChunks は、タイムシフトから外したチャンネルに、録る mikan のいない書きかけのチャンクと放送中のままの番組が
// 残っていたら、閉じてから消すことを確かめる。割り振りのあるサービスは、mikan が書いているかもしれないので触らない (#1055)。
func TestPruneClosesAbandonedChunks(t *testing.T) {
	t.Parallel()

	e := newRecorderEnv(t)
	// 0 番目のサービスだけ設定する。2 番目のサービスは、設定はないが割り振りが残っている (neko が消す前)。
	e.configure(t, 0)
	now := e.clk.Now()
	require.NoError(t, e.q.InsertTimeshiftAssignment(t.Context(), sqlc.InsertTimeshiftAssignmentParams{
		ID: id.New(), ServiceID: grService(2), Priority: timeshift.Priority, Now: now,
	}))

	tests := []struct {
		name       string
		service    int
		startedAgo time.Duration
		noFile     bool
		kept       bool
	}{
		{name: "設定のあるサービスの書きかけ", service: 0, startedAgo: 3 * time.Minute, noFile: false, kept: true},
		{name: "外したチャンネルの書きかけ", service: 1, startedAgo: 2 * time.Hour, noFile: false, kept: false},
		{name: "外したチャンネルのファイルのない書きかけ", service: 1, startedAgo: time.Hour, noFile: true, kept: false},
		{name: "割り振りの残るサービスの書きかけ", service: 2, startedAgo: 3 * time.Minute, noFile: false, kept: true},
	}
	ids := make([]uuid.UUID, len(tests))
	paths := make([]string, len(tests))
	for i, tt := range tests {
		ids[i] = id.New()
		started := now.Add(-tt.startedAgo)
		paths[i] = chunkPath(grService(tt.service), started)
		require.NoError(t, e.q.InsertTimeshiftChunk(t.Context(), sqlc.InsertTimeshiftChunkParams{
			ID: ids[i], ServiceID: grService(tt.service), Path: paths[i], Size: 0, StartedAt: started, Now: started,
		}))
		if !tt.noFile {
			writeUnder(t, e.dir, paths[i])
			writeUnder(t, e.dataDir, ids[i].String()+"/index")
			// 最後に書き込んだのは、書き始めて 1 分後。
			written := started.Add(time.Minute)
			require.NoError(t, os.Chtimes(filepath.Join(e.dir, paths[i]), written, written))
		}
	}
	// 放送中のままの番組。外したチャンネルのものは閉じて消し、ほかは残す。
	for _, service := range []int{0, 1, 2} {
		_, err := e.pool.Exec(t.Context(), "INSERT INTO timeshift_programs (service_id, started_at, event_id, name, ended_at) VALUES ($1, $2, 1, 'p', NULL)",
			grService(service), now.Add(-30*time.Minute))
		require.NoError(t, err)
	}

	pruned, err := timeshift.NewPruneWorker(e.pool, e.clk, e.store, slog.New(slog.DiscardHandler)).RunOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, pruned)

	rows := map[uuid.UUID]chunkRow{}
	for _, c := range chunks(t, e) {
		rows[c.id] = c
	}
	for i, tt := range tests {
		row, ok := rows[ids[i]]
		assert.Equal(t, tt.kept, ok, "%s: the row", tt.name)
		if ok {
			assert.Nil(t, row.endedAt, "%s: still open", tt.name)
		}
		if tt.noFile {
			continue
		}
		_, fileErr := os.Stat(filepath.Join(e.dir, paths[i]))
		_, indexErr := os.Stat(filepath.Join(e.dataDir, ids[i].String()))
		assert.Equal(t, tt.kept, fileErr == nil, "%s: the chunk file", tt.name)
		assert.Equal(t, tt.kept, indexErr == nil, "%s: the index directory", tt.name)
	}

	var left []int64
	require.NoError(t, e.pool.QueryRow(t.Context(),
		"SELECT coalesce(array_agg(service_id ORDER BY service_id), '{}') FROM timeshift_programs WHERE ended_at IS NULL").Scan(&left))
	assert.Equal(t, []int64{grService(0), grService(2)}, left, "the open program of the removed channel is closed and pruned")
	var total int
	require.NoError(t, e.pool.QueryRow(t.Context(), "SELECT count(*) FROM timeshift_programs").Scan(&total))
	assert.Equal(t, 2, total)
}
