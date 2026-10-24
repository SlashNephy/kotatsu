package graph_test

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/thumbnail"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	recordingFields = `id name state protected drops gaps { from to } tags { name } files { kind poolId path size } watchPositionMs thumbnailUrl previewUrl streamUrl service { name }`
	listRecordings  = `query($first: Int!, $after: String, $filter: RecordingFilter) {
		recordings(first: $first, after: $after, filter: $filter) { edges { node { ` + recordingFields + ` } } pageInfo { hasNextPage endCursor } } }`
	deleteRecording    = `mutation($id: ID!) { deleteRecording(id: $id) }`
	varPositionMs      = "positionMs"
	protectRecording   = `mutation($id: ID!, $protected: Boolean!) { setRecordingProtected(id: $id, protected: $protected) { ` + recordingFields + ` } }`
	tagRecording       = `mutation($id: ID!, $tags: [String!]!) { setRecordingTags(id: $id, tags: $tags) { ` + recordingFields + ` } }`
	saveWatchPosition  = `mutation($id: ID!, $positionMs: Int64!) { saveWatchPosition(id: $id, positionMs: $positionMs) { ` + recordingFields + ` } }`
	codeConflict       = "CONFLICT"
	fieldGenres        = "genres"
	varProtected       = "protected"
	episode1           = "こたつ特集 第1話"
	episode2           = "こたつ特集 第2話"
	tagKotatsu         = "こたつ特集"
	varTags            = "tags"
	recordingServiceID = 3273601024
)

type recordingNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	Protected bool   `json:"protected"`
	Drops     int64  `json:"drops"`
	Gaps      []struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
	} `json:"gaps"`
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
	Files []struct {
		Kind   string `json:"kind"`
		PoolID string `json:"poolId"`
		Path   string `json:"path"`
		Size   int64  `json:"size"`
	} `json:"files"`
	WatchPositionMs *int64  `json:"watchPositionMs"`
	ThumbnailURL    *string `json:"thumbnailUrl"`
	PreviewURL      *string `json:"previewUrl"`
	StreamURL       string  `json:"streamUrl"`
	Service         struct {
		Name string `json:"name"`
	} `json:"service"`
}

type recordingPage struct {
	Recordings struct {
		Edges []struct {
			Node recordingNode `json:"node"`
		} `json:"edges"`
		PageInfo struct {
			HasNextPage bool    `json:"hasNextPage"`
			EndCursor   *string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"recordings"`
}

func (p *recordingPage) names() []string {
	names := make([]string, 0, len(p.Recordings.Edges))
	for _, edge := range p.Recordings.Edges {
		names = append(names, edge.Node.Name)
	}

	return names
}

// newRecordingFixture は、EPG を取り込み、プール main を一時ディレクトリに置いた fixture を返す。
// データディレクトリは、dataRootOf(dir) に置く。
func newRecordingFixture(t *testing.T) (*fixture, string) {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(dataRootOf(dir), 0o755))
	f := newFixture(t, &graph.Options{Pools: map[string]string{poolMain: dir}, DataDir: dataRootOf(dir)})
	f.syncEPG(t)

	return f, dir
}

// dataRootOf は、プールのディレクトリ dir の fixture が使う、データディレクトリを返す。t.TempDir の隣に置く。
func dataRootOf(dir string) string {
	return filepath.Join(filepath.Dir(dir), "data")
}

// writeImage は、第 1 話の録画 (episode1) のセグメントの、データディレクトリのディレクトリに、file を置く。更新日時は modifiedAt にする。
func (f *fixture) writeImage(t *testing.T, dir, file, body string, modifiedAt time.Time) {
	t.Helper()

	var segmentID uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT id FROM recording_files WHERE path = $1", episode1+".m2ts").Scan(&segmentID))
	path := filepath.Join(dataRootOf(dir), kago.DataLocation(segmentID, file).Path)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	require.NoError(t, os.Chtimes(path, modifiedAt, modifiedAt))
}

// addRecording は、録画と、その録画ファイルを作る。古い順に作るよう、録画を始めた日時を ago だけ前にする。
func (f *fixture) addRecording(t *testing.T, dir, name, state string, ago time.Duration) string {
	t.Helper()

	recordingID := id.New()
	at := testNow().Add(-ago)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, drops, recorded_from, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, $3, '', $4, $4, $5, false, 1, $4, $4, $4)`, recordingID, recordingServiceID, name, at, state)
	require.NoError(t, err)
	path := name + ".m2ts"
	require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte("ts"), 0o600))
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'segment', $3, $4, 2, $5, $5, $5)`, id.New(), recordingID, poolMain, path, at)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at)
		VALUES ($1, $2, $3, $3, $4, $4, $4)`, id.New(), recordingServiceID, name, at)
	require.NoError(t, err)

	return graph.GlobalID("Recording", recordingID.String())
}

func (f *fixture) recordings(t *testing.T, first int, after *string, filter map[string]any) *recordingPage {
	t.Helper()

	var page recordingPage
	require.Empty(t, f.query(t, listRecordings, map[string]any{varFirst: first, varAfter: after, "filter": filter}, &page))

	return &page
}

// TestRecordings は、録画を新しい順にページを送って読み、タグや条件で絞り込めることを確かめる (設計書 9.7)。
func TestRecordings(t *testing.T) {
	t.Parallel()

	const travelogue = "みかん紀行"
	f, dir := newRecordingFixture(t)
	oldest := f.addRecording(t, dir, episode1, "completed", 3*time.Hour)
	f.addRecording(t, dir, travelogue, "failed", 2*time.Hour)
	newest := f.addRecording(t, dir, episode2, "partial", time.Hour)

	all := f.recordings(t, 10, nil, nil)
	assert.Equal(t, []string{episode2, travelogue, episode1}, all.names())
	first := all.Recordings.Edges[0].Node
	assert.Equal(t, "PARTIAL", first.State)
	assert.Equal(t, "こたつテスト1", first.Service.Name)
	require.Len(t, first.Files, 1)
	assert.Equal(t, "SEGMENT", first.Files[0].Kind)
	assert.Equal(t, int64(2), first.Files[0].Size)

	// nekonote が作った代表画像とプレビュー画像は、DB に記録せず、データディレクトリにあれば URL を返す。
	generatedAt := testNow().Add(-30 * time.Minute)
	f.writeImage(t, dir, "thumbnail.jpg", "jpeg", generatedAt)
	f.writeImage(t, dir, "preview.jpg", "jpeg", generatedAt)
	withImages := f.recordings(t, 10, nil, nil).Recordings.Edges[2].Node
	files := withImages.Files
	// 画像は /media で配信するので、その URL のパスを返す。画像のない録画では null にする。
	// 作り直したら URL が変わるよう、画像のファイルの更新日時を添える (#476)。
	var rawID string
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT id::text FROM recordings WHERE name = $1`, episode1).Scan(&rawID))
	suffix := "?v=" + strconv.FormatInt(generatedAt.Unix(), 10)
	assert.Equal(t, new("/media/recordings/"+rawID+"/thumbnail.jpg"+suffix), withImages.ThumbnailURL)
	assert.Equal(t, new("/media/recordings/"+rawID+"/preview.jpg"+suffix), withImages.PreviewURL)
	assert.Equal(t, "/streams/recordings/"+rawID, withImages.StreamURL)
	assert.Nil(t, f.recordings(t, 10, nil, nil).Recordings.Edges[0].Node.ThumbnailURL)
	kinds := make([]string, 0, len(files))
	for _, file := range files {
		kinds = append(kinds, file.Kind)
	}
	assert.Equal(t, []string{"SEGMENT"}, kinds)

	page := f.recordings(t, 2, nil, nil)
	assert.True(t, page.Recordings.PageInfo.HasNextPage)
	next := f.recordings(t, 2, page.Recordings.PageInfo.EndCursor, nil)
	assert.Equal(t, []string{episode1}, next.names())

	// 古い順でも、ページを送って読める。
	oldestFirst := func(first int, after *string) *recordingPage {
		var page recordingPage
		require.Empty(t, f.query(t, `query($first: Int!, $after: String) {
			recordings(first: $first, after: $after, order: OLDEST_FIRST) { edges { node { `+recordingFields+` } } pageInfo { hasNextPage endCursor } } }`,
			map[string]any{varFirst: first, varAfter: after}, &page))

		return &page
	}
	oldPage := oldestFirst(2, nil)
	assert.Equal(t, []string{episode1, travelogue}, oldPage.names())
	assert.True(t, oldPage.Recordings.PageInfo.HasNextPage)
	assert.Equal(t, []string{episode2}, oldestFirst(2, oldPage.Recordings.PageInfo.EndCursor).names())

	var tagged struct {
		SetRecordingTags recordingNode `json:"setRecordingTags"`
	}
	require.Empty(t, f.query(t, tagRecording, map[string]any{varID: oldest, varTags: []string{tagKotatsu, " アニメ "}}, &tagged))
	assert.Len(t, tagged.SetRecordingTags.Tags, 2)
	// 別の録画にも同じタグを付けると、どちらの録画から見ても、タグの録画の数が 2 になる。
	var another struct {
		SetRecordingTags struct {
			Tags []struct {
				Name       string `json:"name"`
				Recordings int    `json:"recordings"`
			} `json:"tags"`
		} `json:"setRecordingTags"`
	}
	require.Empty(t, f.query(t, `mutation($id: ID!, $tags: [String!]!) { setRecordingTags(id: $id, tags: $tags) { tags { name recordings } } }`,
		map[string]any{varID: newest, varTags: []string{tagKotatsu}}, &another))
	assert.Equal(t, 2, another.SetRecordingTags.Tags[0].Recordings)
	require.Empty(t, f.query(t, tagRecording, map[string]any{varID: newest, varTags: []string{}}, nil))
	var tags struct {
		Tags []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Recordings int    `json:"recordings"`
		} `json:"tags"`
	}
	require.Empty(t, f.query(t, `{ tags { id name recordings } }`, nil, &tags))
	require.Len(t, tags.Tags, 2)
	assert.Equal(t, "アニメ", tags.Tags[0].Name)
	assert.Equal(t, 1, tags.Tags[0].Recordings)

	tests := []struct {
		name   string
		filter map[string]any
		want   []string
	}{
		{name: "タグ", filter: map[string]any{"tagIds": []string{tags.Tags[1].ID}}, want: []string{episode1}},
		{name: "状態", filter: map[string]any{"states": []string{"COMPLETED", "PARTIAL"}}, want: []string{episode2, episode1}},
		{name: "番組名", filter: map[string]any{"keyword": "こたつ特集 第"}, want: []string{episode2, episode1}},
		{name: "LIKE の特別な文字は文字そのもの", filter: map[string]any{"keyword": "%"}, want: []string{}},
		{name: "放送の期間", filter: map[string]any{"since": testNow().Add(-150 * time.Minute)}, want: []string{episode2, travelogue}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, f.recordings(t, 10, nil, tt.filter).names())
		})
	}

	// node で引き直せる。
	var node struct {
		Node recordingNode `json:"node"`
	}
	require.Empty(t, f.query(t, `query($id: ID!) { node(id: $id) { ... on Recording { `+recordingFields+` } } }`, map[string]any{varID: newest}, &node))
	assert.Equal(t, episode2, node.Node.Name)
}

// TestRecordingOperations は、録画の保護、削除と、ユーザーごとの再生位置を確かめる (設計書 9.7)。
func TestRecordingOperations(t *testing.T) {
	t.Parallel()

	f, dir := newRecordingFixture(t)
	recordingID := f.addRecording(t, dir, "こたつ特集", "completed", time.Hour)
	viewerFixture := f.as(t, "viewer", auth.RoleViewer)

	// 視聴者は、消せず、保護できず、タグを付けられないが、再生位置は保存できる。
	assert.Equal(t, []string{codeForbidden}, viewerFixture.query(t, deleteRecording, map[string]any{varID: recordingID}, nil))
	assert.Equal(t, []string{codeForbidden}, viewerFixture.query(t, protectRecording, map[string]any{varID: recordingID, varProtected: true}, nil))
	assert.Equal(t, []string{codeForbidden}, viewerFixture.query(t, tagRecording, map[string]any{varID: recordingID, varTags: []string{"x"}}, nil))
	var saved struct {
		SaveWatchPosition recordingNode `json:"saveWatchPosition"`
	}
	require.Empty(t, viewerFixture.query(t, saveWatchPosition, map[string]any{varID: recordingID, varPositionMs: 90_000}, &saved))
	assert.Equal(t, new(int64(90_000)), saved.SaveWatchPosition.WatchPositionMs)
	// 再生位置は、ユーザーごとに持つ。
	assert.Nil(t, f.recordings(t, 1, nil, nil).Recordings.Edges[0].Node.WatchPositionMs)

	var protected struct {
		SetRecordingProtected recordingNode `json:"setRecordingProtected"`
	}
	require.Empty(t, f.query(t, protectRecording, map[string]any{varID: recordingID, varProtected: true}, &protected))
	assert.True(t, protected.SetRecordingProtected.Protected)
	// 保護した録画は消せない。
	assert.Equal(t, []string{codeConflict}, f.query(t, deleteRecording, map[string]any{varID: recordingID}, nil))
	require.Empty(t, f.query(t, protectRecording, map[string]any{varID: recordingID, varProtected: false}, nil))

	var deleted struct {
		DeleteRecording string `json:"deleteRecording"`
	}
	require.Empty(t, f.query(t, deleteRecording, map[string]any{varID: recordingID}, &deleted))
	assert.Equal(t, recordingID, deleted.DeleteRecording)
	assert.Empty(t, f.recordings(t, 10, nil, nil).names())
	_, err := os.Stat(filepath.Join(dir, "こたつ特集.m2ts"))
	require.ErrorIs(t, err, fs.ErrNotExist)
	var history int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM recording_history").Scan(&history))
	assert.Equal(t, 1, history, "the history is kept")

	var actions []string
	rows, err := f.pool.Query(t.Context(), "SELECT action FROM audit_events WHERE target_type = 'recording' ORDER BY occurred_at, id")
	require.NoError(t, err)
	for rows.Next() {
		var action string
		require.NoError(t, rows.Scan(&action))
		actions = append(actions, action)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"recording.protected", "recording.unprotected", "recording.deleted"}, actions)

	missing := graph.GlobalID("Recording", uuid.New().String())
	assert.Equal(t, []string{codeNotFound}, f.query(t, deleteRecording, map[string]any{varID: missing}, nil))
	assert.Equal(t, []string{codeBadUserInput}, f.query(t, saveWatchPosition, map[string]any{varID: recordingID, varPositionMs: -1}, nil))
}

// lateInserter は、フィクスチャの DB ができてから River のクライアントを差し込む Inserter。
type lateInserter struct {
	client *jobs.Client
}

var _ thumbnail.Inserter = new(lateInserter)

func (l *lateInserter) InsertManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) ([]*rivertype.JobInsertResult, error) {
	return l.client.InsertManyTx(ctx, tx, params)
}

// TestRegenerateRecordingImages は、録画の画像を作り直すジョブを入れ、待っているジョブには重ねず、
// 終わったジョブがあっても、また入れられることを確かめる (#476)。
func TestRegenerateRecordingImages(t *testing.T) {
	t.Parallel()

	inserter := &lateInserter{client: nil}
	dir := t.TempDir()
	f := newFixture(t, &graph.Options{Pools: map[string]string{poolMain: dir}, Jobs: inserter})
	f.syncEPG(t)
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	client, err := jobs.NewClient(f.pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	inserter.client = client
	recordingID := f.addRecording(t, dir, "こたつ特集", "completed", time.Hour)
	_, rawID, err := graph.ParseGlobalID(recordingID)
	require.NoError(t, err)
	const regenerate = `mutation($id: ID!) { regenerateRecordingImages(id: $id) }`
	queued := func() int {
		var n int
		require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind = 'thumbnail_generate' AND args->>'recordingId' = $1`,
			rawID).Scan(&n))

		return n
	}
	var got struct {
		RegenerateRecordingImages bool `json:"regenerateRecordingImages"`
	}

	assert.Equal(t, []string{codeForbidden}, f.as(t, "viewer", auth.RoleViewer).query(t, regenerate, map[string]any{varID: recordingID}, nil))
	missing := graph.GlobalID("Recording", uuid.New().String())
	assert.Equal(t, []string{codeNotFound}, f.query(t, regenerate, map[string]any{varID: missing}, nil))

	require.Empty(t, f.query(t, regenerate, map[string]any{varID: recordingID}, &got))
	assert.True(t, got.RegenerateRecordingImages)
	assert.Equal(t, 1, queued())

	// 待っているジョブがあれば、重ねて入れない。
	require.Empty(t, f.query(t, regenerate, map[string]any{varID: recordingID}, &got))
	assert.False(t, got.RegenerateRecordingImages)
	assert.Equal(t, 1, queued())

	// 作り終えた録画は、また作り直せる。
	_, err = f.pool.Exec(t.Context(), `UPDATE river_job SET state = 'completed', finalized_at = now() WHERE kind = 'thumbnail_generate'`)
	require.NoError(t, err)
	require.Empty(t, f.query(t, regenerate, map[string]any{varID: recordingID}, &got))
	assert.True(t, got.RegenerateRecordingImages)
	assert.Equal(t, 2, queued())
}

// TestRecordingsByRuleAndGenre は、録画を、録ったルールとジャンルで絞れることと、
// ルールを消しても、録画にはルールの名前が残ることを確かめる (#468)。
func TestRecordingsByRuleAndGenre(t *testing.T) {
	t.Parallel()

	f, dir := newRecordingFixture(t)
	ruleID := id.New()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO rules (id, name, memo, enabled, conditions, settings, avoid_duplicates, follow_event_relay, created_at, updated_at)
		VALUES ($1, 'アニメのルール', '', true, '{"version":1,"conditions":{}}', '{}', false, true, $2, $2)`, ruleID, testNow())
	require.NoError(t, err)
	byRule := f.addRecording(t, dir, "ルールの録画", "completed", 2*time.Hour)
	f.addRecording(t, dir, "手動の録画", "completed", time.Hour)
	_, rawID, err := graph.ParseGlobalID(byRule)
	require.NoError(t, err)
	// ルールで録った録画は、ルールとジャンル (アニメ・特撮、lv1 = 7) を写してある。
	_, err = f.pool.Exec(t.Context(), `UPDATE recordings SET rule_id = $2, rule_name = 'アニメのルール', genres = '[{"lv1":7,"lv2":0}]' WHERE id = $1`,
		rawID, ruleID)
	require.NoError(t, err)

	ruleGlobalID := graph.GlobalID("Rule", ruleID.String())
	assert.Equal(t, []string{"ルールの録画"}, f.recordings(t, 10, nil, map[string]any{"ruleId": ruleGlobalID}).names())
	assert.Equal(t, []string{"ルールの録画"}, f.recordings(t, 10, nil, map[string]any{fieldGenres: []int{7}}).names())
	assert.Empty(t, f.recordings(t, 10, nil, map[string]any{fieldGenres: []int{0}}).names())
	assert.Equal(t, []string{codeBadUserInput}, f.query(t, listRecordings, map[string]any{varFirst: 10, "filter": map[string]any{fieldGenres: []int{16}}}, nil))

	const ruleOf = `query($id: ID!) { node(id: $id) { ... on Recording { ruleName genres { lv1 lv2 } rule { name } } } }`
	type result struct {
		Node struct {
			RuleName *string `json:"ruleName"`
			Genres   []struct {
				Lv1 int `json:"lv1"`
			} `json:"genres"`
			Rule *struct {
				Name string `json:"name"`
			} `json:"rule"`
		} `json:"node"`
	}
	var got result
	require.Empty(t, f.query(t, ruleOf, map[string]any{varID: byRule}, &got))
	require.NotNil(t, got.Node.Rule)
	assert.Equal(t, "アニメのルール", got.Node.Rule.Name)
	require.Len(t, got.Node.Genres, 1)
	assert.Equal(t, 7, got.Node.Genres[0].Lv1)

	// ルールを消すと、録画からルールは引けなくなるが、名前は残る。
	_, err = f.pool.Exec(t.Context(), "DELETE FROM rules WHERE id = $1", ruleID)
	require.NoError(t, err)
	got = result{}
	require.Empty(t, f.query(t, ruleOf, map[string]any{varID: byRule}, &got))
	assert.Nil(t, got.Node.Rule)
	require.NotNil(t, got.Node.RuleName)
	assert.Equal(t, "アニメのルール", *got.Node.RuleName)
}

// TestReanalyzeRecording は、録画の解析をやり直すジョブを、視聴者でなければ入れられることを確かめる (#476)。
// 取り込んだ録画の解析より先に回し、録画中の録画は断り、待っているジョブには重ねない。
func TestReanalyzeRecording(t *testing.T) {
	t.Parallel()

	inserter := &lateInserter{client: nil}
	dir := t.TempDir()
	f := newFixture(t, &graph.Options{Pools: map[string]string{poolMain: dir}, Jobs: inserter})
	f.syncEPG(t)
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	client, err := jobs.NewClient(f.pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	inserter.client = client
	recordingID := f.addRecording(t, dir, "こたつ特集", "completed", time.Hour)
	recordingNow := f.addRecording(t, dir, "こたつ中継", "recording", time.Hour)
	_, rawID, err := graph.ParseGlobalID(recordingID)
	require.NoError(t, err)
	const reanalyze = `mutation($id: ID!) { reanalyzeRecording(id: $id) }`
	queued := func() []int {
		rows, err := f.pool.Query(t.Context(), `SELECT priority FROM river_job WHERE kind = 'recording_analyze' AND args->>'recordingId' = $1`, rawID)
		require.NoError(t, err)
		priorities, err := pgx.CollectRows(rows, pgx.RowTo[int])
		require.NoError(t, err)

		return priorities
	}
	var got struct {
		ReanalyzeRecording bool `json:"reanalyzeRecording"`
	}

	assert.Equal(t, []string{codeForbidden}, f.as(t, "viewer", auth.RoleViewer).query(t, reanalyze, map[string]any{varID: recordingID}, nil))
	assert.Equal(t, []string{codeConflict}, f.query(t, reanalyze, map[string]any{varID: recordingNow}, nil), "a recording in progress")

	require.Empty(t, f.query(t, reanalyze, map[string]any{varID: recordingID}, &got))
	assert.True(t, got.ReanalyzeRecording)
	assert.Equal(t, []int{jobs.PriorityDefault}, queued(), "requested jobs run before those of imported recordings")

	// 待っているジョブがあれば、重ねて入れない。
	require.Empty(t, f.query(t, reanalyze, map[string]any{varID: recordingID}, &got))
	assert.False(t, got.ReanalyzeRecording)
	assert.Len(t, queued(), 1)
}

// TestDeleteRecordings は、録画をまとめて消し、保護した録画、録画中の録画、見つからない録画は飛ばして数えることを確かめる (#471)。
func TestDeleteRecordings(t *testing.T) {
	t.Parallel()

	f, dir := newRecordingFixture(t)
	first := f.addRecording(t, dir, "消す録画 1", "completed", 4*time.Hour)
	second := f.addRecording(t, dir, "消す録画 2", "failed", 3*time.Hour)
	protected := f.addRecording(t, dir, "保護した録画", "completed", 2*time.Hour)
	recordingNow := f.addRecording(t, dir, "録画中の録画", "recording", time.Hour)
	_, rawProtected, err := graph.ParseGlobalID(protected)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE recordings SET protected = true WHERE id = $1", rawProtected)
	require.NoError(t, err)
	missing := graph.GlobalID("Recording", uuid.New().String())
	const deleteMany = `mutation($ids: [ID!]!) { deleteRecordings(ids: $ids) { processed skipped } }`
	ids := []string{first, second, protected, recordingNow, missing}

	assert.Equal(t, []string{codeForbidden}, f.as(t, "viewer", auth.RoleViewer).query(t, deleteMany, map[string]any{varIDs: ids}, nil))
	tooMany := make([]string, 501)
	for i := range tooMany {
		tooMany[i] = missing
	}
	assert.Equal(t, []string{codeBadUserInput}, f.query(t, deleteMany, map[string]any{varIDs: tooMany}, nil))

	var got struct {
		DeleteRecordings struct {
			Processed int `json:"processed"`
			Skipped   int `json:"skipped"`
		} `json:"deleteRecordings"`
	}
	require.Empty(t, f.query(t, deleteMany, map[string]any{varIDs: ids}, &got))
	assert.Equal(t, 2, got.DeleteRecordings.Processed)
	assert.Equal(t, 3, got.DeleteRecordings.Skipped)
	assert.ElementsMatch(t, []string{"保護した録画", "録画中の録画"}, f.recordings(t, 10, nil, nil).names())
	assert.NoFileExists(t, filepath.Join(dir, "消す録画 1.m2ts"))
	assert.FileExists(t, filepath.Join(dir, "保護した録画.m2ts"))
	var audited int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE action = 'recording.deleted'").Scan(&audited))
	assert.Equal(t, 2, audited, "each deletion is audited")
}

// TestRecordingWatchFrame は、前回見た位置に合うプレビュー画像のコマを返すことを確かめる。
func TestRecordingWatchFrame(t *testing.T) {
	t.Parallel()

	f, dir := newRecordingFixture(t)
	recordingID := f.addRecording(t, dir, episode1, "completed", time.Hour)
	// 録画は 30 分。プレビュー画像は、0 秒、60 秒、120 秒の 3 コマ (1 行に収まる)。
	_, err := f.pool.Exec(t.Context(), `UPDATE recordings SET recorded_until = recorded_from + interval '30 minutes' WHERE name = $1`, episode1)
	require.NoError(t, err)
	generatedAt := testNow().Add(-30 * time.Minute)
	f.writeImage(t, dir, "preview.jpg", "jpeg", generatedAt)
	f.writeImage(t, dir, "preview.json", `{"width":160,"height":90,"columns":10,"times":[0,60,120]}`, generatedAt)

	type frame struct {
		URL         string `json:"url"`
		X           int    `json:"x"`
		Y           int    `json:"y"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		SheetWidth  int    `json:"sheetWidth"`
		SheetHeight int    `json:"sheetHeight"`
	}
	watchFrame := func(t *testing.T) *frame {
		t.Helper()

		var page struct {
			Recordings struct {
				Edges []struct {
					Node struct {
						WatchFrame *frame `json:"watchFrame"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"recordings"`
		}
		require.Empty(t, f.query(t, `query { recordings(first: 1) { edges { node { watchFrame { url x y width height sheetWidth sheetHeight } } } } }`, nil, &page))
		require.Len(t, page.Recordings.Edges, 1)

		return page.Recordings.Edges[0].Node.WatchFrame
	}
	save := func(t *testing.T, positionMs int64) {
		t.Helper()

		var saved struct {
			SaveWatchPosition recordingNode `json:"saveWatchPosition"`
		}
		require.Empty(t, f.query(t, saveWatchPosition, map[string]any{varID: recordingID, varPositionMs: positionMs}, &saved))
	}

	// 再生していなければ、コマを返さない。
	assert.Nil(t, watchFrame(t))

	// 再生位置より前で、いちばん近いコマを返す。
	save(t, 119_000)
	got := watchFrame(t)
	require.NotNil(t, got)
	var rawID string
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT id::text FROM recordings WHERE name = $1`, episode1).Scan(&rawID))
	assert.Equal(t, &frame{
		URL: "/media/recordings/" + rawID + "/preview.jpg?v=" + strconv.FormatInt(generatedAt.Unix(), 10), X: 160, Y: 0, Width: 160, Height: 90, SheetWidth: 480, SheetHeight: 90,
	}, got)

	// 終わりの近くまで見ていたら、見終えたとみなして返さない。
	save(t, 30*60*1000-5_000)
	assert.Nil(t, watchFrame(t))
}

// TestContinueWatching は、途中まで見た録画を、最後に見た新しい順に返し、少しだけ見た録画と見終えた録画を外すことを確かめる (#478)。
func TestContinueWatching(t *testing.T) {
	t.Parallel()

	f, dir := newRecordingFixture(t)
	ids := map[string]string{}
	for _, name := range []string{"前半", "後半", "冒頭だけ", "見終えた"} {
		ids[name] = f.addRecording(t, dir, name, "completed", 2*time.Hour)
	}
	// 録画中の録画は、終わり (recorded_until) が決まっていない。
	ids["録画中"] = f.addRecording(t, dir, "録画中", "recording", 2*time.Hour)
	// 録画中のほかは、30 分の録画にする。
	_, err := f.pool.Exec(t.Context(), `UPDATE recordings SET recorded_until = recorded_from + interval '30 minutes' WHERE name <> '録画中'`)
	require.NoError(t, err)
	save := func(name string, positionMs int) {
		t.Helper()

		require.Empty(t, f.query(t, saveWatchPosition, map[string]any{varID: ids[name], varPositionMs: positionMs}, nil))
		f.clock.Advance(time.Minute)
	}
	save("前半", 60_000)
	// 続きから再生しても最初から始まる位置 (10 秒より前) は、含めない。
	save("冒頭だけ", 9_000)
	save("見終えた", 30*60_000-5_000)
	save("録画中", 60_000)
	save("後半", 20*60_000)

	var got struct {
		ContinueWatching []struct {
			Name string `json:"name"`
		} `json:"continueWatching"`
	}
	require.Empty(t, f.query(t, `{ continueWatching { name } }`, nil, &got))
	names := make([]string, 0, len(got.ContinueWatching))
	for _, recording := range got.ContinueWatching {
		names = append(names, recording.Name)
	}
	assert.Equal(t, []string{"後半", "録画中", "前半"}, names)

	// ほかのユーザーの再生位置は含めない。
	var other struct {
		ContinueWatching []struct {
			Name string `json:"name"`
		} `json:"continueWatching"`
	}
	require.Empty(t, f.as(t, "other", auth.RoleMember).query(t, `{ continueWatching { name } }`, nil, &other))
	assert.Empty(t, other.ContinueWatching)
}
