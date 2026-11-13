package graph_test

import (
	"context"
	"log/slog"
	"strconv"
	"testing"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/usecase/thumbnail"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

// lateJobAdmin は、フィクスチャの DB ができてから River のクライアントを差し込む JobAdmin。
type lateJobAdmin struct {
	client *jobs.Client
}

var _ graph.JobAdmin = new(lateJobAdmin)

func (l *lateJobAdmin) JobList(ctx context.Context, params *river.JobListParams) (*river.JobListResult, error) {
	return l.client.JobList(ctx, params)
}

func (l *lateJobAdmin) JobGet(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	return l.client.JobGet(ctx, id)
}

func (l *lateJobAdmin) JobRetry(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	return l.client.JobRetry(ctx, id)
}

func (l *lateJobAdmin) JobCancel(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	return l.client.JobCancel(ctx, id)
}

// promoteArgs は、取り消させないジョブ (タイムシフトの昇格) の代わり。
type promoteArgs struct{}

func (promoteArgs) Kind() string { return "timeshift_promote" }

type jobNode struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	State       string `json:"state"`
	Retryable   bool   `json:"retryable"`
	Cancellable bool   `json:"cancellable"`
	Recording   *struct {
		Name string `json:"name"`
	} `json:"recording"`
}

const (
	listJobsQuery  = `query($state: JobState!) { jobs(state: $state) { nodes { id kind state retryable cancellable recording { name } } endCursor } }`
	jobCountsQuery = `{ jobCounts { state count } }`
	cancelJob      = `mutation($id: ID!) { cancelJob(id: $id) { id state } }`
	retryJob       = `mutation($id: ID!) { retryJob(id: $id) { id state } }`
	stateAvailable = "AVAILABLE"
)

// TestJobs は、管理者が、状態ごとにジョブを一覧し、対象の録画を引き、試し直し、取り消せることを確かめる (#1279)。
// 取り消すと録画を失いうるジョブ は取り消させず、管理者でなければ使えない。
func TestJobs(t *testing.T) {
	t.Parallel()

	admin := &lateJobAdmin{client: nil}
	dir := t.TempDir()
	f := newFixture(t, &graph.Options{Pools: map[string]string{poolMain: dir}, JobAdmin: admin})
	f.syncEPG(t)
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	client, err := jobs.NewClient(f.pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	admin.client = client
	recordingID := f.addRecording(t, dir, "こたつ特集", "completed", 0)
	_, rawID, err := graph.ParseGlobalID(recordingID)
	require.NoError(t, err)
	_, err = client.Insert(t.Context(), thumbnail.GenerateArgs{RecordingID: uuid.MustParse(rawID)}, nil)
	require.NoError(t, err)
	_, err = client.Insert(t.Context(), promoteArgs{}, nil)
	require.NoError(t, err)

	adminFixture := f.as(t, adminName, domain.RoleAdmin)
	list := func(fx *fixture, state string) []jobNode {
		t.Helper()

		var got struct {
			Jobs struct {
				Nodes []jobNode `json:"nodes"`
			} `json:"jobs"`
		}
		require.Empty(t, fx.query(t, listJobsQuery, map[string]any{"state": state}, &got))

		return got.Jobs.Nodes
	}
	available := list(adminFixture, stateAvailable)
	require.Len(t, available, 2)
	byKind := map[string]jobNode{}
	for _, job := range available {
		byKind[job.Kind] = job
	}
	require.NotNil(t, byKind["thumbnail_generate"].Recording)
	assert.Equal(t, "こたつ特集", byKind["thumbnail_generate"].Recording.Name, "the target recording is resolved by name")
	assert.True(t, byKind["thumbnail_generate"].Cancellable)
	assert.False(t, byKind["timeshift_promote"].Cancellable, "promoting timeshift is not cancellable")
	assert.Nil(t, byKind["timeshift_promote"].Recording)

	var counts struct {
		JobCounts []struct {
			State string `json:"state"`
			Count int    `json:"count"`
		} `json:"jobCounts"`
	}
	require.Empty(t, adminFixture.query(t, jobCountsQuery, nil, &counts))
	countOf := map[string]int{}
	for _, c := range counts.JobCounts {
		countOf[c.State] = c.Count
	}
	assert.Equal(t, 2, countOf[stateAvailable])
	assert.Equal(t, 0, countOf["DISCARDED"], "states without jobs are counted as 0")

	// 取り消すと、取り消した状態になる。取り消したジョブは、試し直せる。
	thumbID := byKind["thumbnail_generate"].ID
	require.Empty(t, adminFixture.query(t, cancelJob, map[string]any{varID: thumbID}, nil))
	cancelled := list(adminFixture, "CANCELLED")
	require.Len(t, cancelled, 1)
	assert.False(t, cancelled[0].Cancellable)
	assert.True(t, cancelled[0].Retryable)
	require.Empty(t, adminFixture.query(t, retryJob, map[string]any{varID: thumbID}, nil))
	assert.Len(t, list(adminFixture, stateAvailable), 2)

	// 取り消させないジョブと、管理者でない操作は断る。
	assert.Equal(t, []string{codeBadUserInput}, adminFixture.query(t, cancelJob, map[string]any{varID: byKind["timeshift_promote"].ID}, nil))
	assert.Equal(t, []string{codeNotFound}, adminFixture.query(t, cancelJob, map[string]any{varID: "999999"}, nil))
	member := f.as(t, otherUser, domain.RoleMember)
	assert.Equal(t, []string{codeForbidden}, member.query(t, listJobsQuery, map[string]any{"state": stateAvailable}, nil))
	assert.Equal(t, []string{codeForbidden}, member.query(t, retryJob, map[string]any{varID: thumbID}, nil))

	// 操作は監査ログに残す。
	var audited int
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE action IN ('job.cancel', 'job.retry')").Scan(&audited))
	assert.Equal(t, 2, audited)
}

// jobStateRunning は、River の動いているジョブの状態。
const jobStateRunning = "running"

// TestJobProgress は、動いているジョブの進み具合を、今の試行で書いたものだけ返すことを確かめる (#1511)。
// 試し直す前の試行の値や、終えたジョブに残った値は返さない。
func TestJobProgress(t *testing.T) {
	t.Parallel()

	admin := &lateJobAdmin{client: nil}
	f := newFixture(t, &graph.Options{Pools: map[string]string{poolMain: t.TempDir()}, JobAdmin: admin})
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	client, err := jobs.NewClient(f.pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	admin.client = client

	type progress struct {
		Ratio            float64 `json:"ratio"`
		RemainingSeconds *int    `json:"remainingSeconds"`
	}
	tests := []struct {
		name  string
		state string
		// written は、進み具合を書いた試行。0 なら書いていない。
		written     int
		remainingMs *int64
		want        *progress
	}{
		{name: jobStateRunning, state: jobStateRunning, written: 2, remainingMs: new(int64(61_500)), want: &progress{Ratio: 0.42, RemainingSeconds: new(62)}},
		{name: "unknown remaining", state: jobStateRunning, written: 2, remainingMs: nil, want: &progress{Ratio: 0.42, RemainingSeconds: nil}},
		{name: "previous attempt", state: jobStateRunning, written: 1, remainingMs: nil, want: nil},
		{name: "not written yet", state: jobStateRunning, written: 0, remainingMs: nil, want: nil},
		{name: "finished", state: "retryable", written: 2, remainingMs: nil, want: nil},
	}
	ids := map[string]int64{}
	for _, tt := range tests {
		inserted, err := client.Insert(t.Context(), promoteArgs{}, nil)
		require.NoError(t, err)
		id := inserted.Job.ID
		ids[tt.name] = id
		_, err = f.pool.Exec(t.Context(), "UPDATE river_job SET state = $2, attempt = 2, attempted_at = now() WHERE id = $1", id, tt.state)
		require.NoError(t, err)
		if tt.written > 0 {
			_, err = f.pool.Exec(t.Context(), "INSERT INTO job_progress (job_id, attempt, ratio, remaining_ms, updated_at) VALUES ($1, $2, 0.42, $3, now())",
				id, tt.written, tt.remainingMs)
			require.NoError(t, err)
		}
	}
	adminFixture := f.as(t, adminName, domain.RoleAdmin)
	got := map[string]*progress{}
	for _, state := range []string{"RUNNING", "RETRYABLE"} {
		var page struct {
			Jobs struct {
				Nodes []struct {
					ID       string    `json:"id"`
					Progress *progress `json:"progress"`
				} `json:"nodes"`
			} `json:"jobs"`
		}
		require.Empty(t, adminFixture.query(t, `query($state: JobState!) { jobs(state: $state) { nodes { id progress { ratio remainingSeconds } } } }`,
			map[string]any{varState: state}, &page))
		for _, node := range page.Jobs.Nodes {
			got[node.ID] = node.Progress
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			id := strconv.FormatInt(ids[tt.name], 10)
			require.Contains(t, got, id)
			assert.Equal(t, tt.want, got[id])
		})
	}
}
