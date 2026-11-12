package graph

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const (
	// maxJobs は、ジョブの一覧で一度に返す数の上限。
	maxJobs = 200
	// targetJob は、ジョブの操作の監査ログの対象の種類。
	targetJob = "job"
	// actionJobRetry と actionJobCancel は、ジョブの操作の監査ログの種類。
	actionJobRetry  = "job.retry"
	actionJobCancel = "job.cancel"
)

// uncancellableKinds は、画面から取り消させないジョブの種類 (#1279)。取り消すと、録画を失ったり、録画のファイルを壊したりしうる。
func uncancellableKinds() []string {
	return []string{"timeshift_promote", "recording_join", "recording_join_finalize"}
}

// errJobsUnavailable は、ジョブを扱う設定がないことを表す。
var errJobsUnavailable = errors.New("jobs are not configured")

// JobAdmin は、River のジョブを読み、試し直し、取り消す。*jobs.Client (river.Client) が満たす。
type JobAdmin interface {
	JobList(ctx context.Context, params *river.JobListParams) (*river.JobListResult, error)
	JobGet(ctx context.Context, id int64) (*rivertype.JobRow, error)
	JobRetry(ctx context.Context, id int64) (*rivertype.JobRow, error)
	JobCancel(ctx context.Context, id int64) (*rivertype.JobRow, error)
}

var _ JobAdmin = new(river.Client[pgx.Tx])

// listJobs は、状態 state のジョブを新しい順に返す (#1279)。
func (r *Resolver) listJobs(ctx context.Context, state model.JobState, first int, after *string) (*model.JobPage, error) {
	if _, _, err := r.administratorActor(ctx, "only administrators can list jobs"); err != nil {
		return nil, err
	}
	if r.jobAdmin == nil {
		return nil, errJobsUnavailable
	}
	if first < 1 || first > maxJobs {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("first must be between 1 and %d", maxJobs))
	}
	params := river.NewJobListParams().States(rivertype.JobState(strings.ToLower(string(state)))).First(first).
		OrderBy(river.JobListOrderByID, river.SortOrderDesc)
	if after != nil {
		raw, err := base64.RawURLEncoding.DecodeString(*after)
		if err != nil {
			return nil, NewError(CodeBadUserInput, "invalid cursor")
		}
		cursor := new(river.JobListCursor)
		if err := cursor.UnmarshalText(raw); err != nil {
			return nil, NewError(CodeBadUserInput, "invalid cursor")
		}
		params = params.After(cursor)
	}
	result, err := r.jobAdmin.JobList(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	nodes, err := r.jobModels(ctx, result.Jobs)
	if err != nil {
		return nil, err
	}
	page := &model.JobPage{Nodes: nodes, EndCursor: nil}
	if len(result.Jobs) == first && result.LastCursor != nil {
		text, err := result.LastCursor.MarshalText()
		if err != nil {
			return nil, fmt.Errorf("failed to encode the cursor: %w", err)
		}
		page.EndCursor = new(base64.RawURLEncoding.EncodeToString(text))
	}

	return page, nil
}

// jobCounts は、状態ごとのジョブの数を返す (#1279)。ジョブのない状態も 0 として返す。
func (r *Resolver) jobCounts(ctx context.Context) ([]*model.JobStateCount, error) {
	if _, _, err := r.administratorActor(ctx, "only administrators can list jobs"); err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, "SELECT state::text, count(*) FROM river_job GROUP BY state")
	if err != nil {
		return nil, fmt.Errorf("failed to count jobs: %w", err)
	}
	defer rows.Close()
	counts := map[model.JobState]int{}
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, fmt.Errorf("failed to read the job counts: %w", err)
		}
		counts[model.JobState(strings.ToUpper(state))] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the job counts: %w", err)
	}
	result := make([]*model.JobStateCount, 0, len(model.AllJobState))
	for _, state := range model.AllJobState {
		result = append(result, &model.JobStateCount{State: state, Count: counts[state]})
	}

	return result, nil
}

// changeJob は、ジョブ id を、操作 action (再試行か取り消し) で変え、監査ログに残す (#1279)。
func (r *Resolver) changeJob(ctx context.Context, id, action string) (*model.Job, error) {
	_, actor, err := r.administratorActor(ctx, "only administrators can change jobs")
	if err != nil {
		return nil, err
	}
	if r.jobAdmin == nil {
		return nil, errJobsUnavailable
	}
	jobID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return nil, NewError(CodeBadUserInput, "invalid job ID")
	}
	current, err := r.jobAdmin.JobGet(ctx, jobID)
	if errors.Is(err, river.ErrNotFound) {
		return nil, NewError(CodeNotFound, "job not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get the job: %w", err)
	}
	var changed *rivertype.JobRow
	switch action {
	case actionJobRetry:
		if !retryable(current) {
			return nil, NewError(CodeBadUserInput, "a running job cannot be retried")
		}
		changed, err = r.jobAdmin.JobRetry(ctx, jobID)
	default:
		if !cancellable(current) {
			return nil, NewError(CodeBadUserInput, "this job cannot be cancelled")
		}
		changed, err = r.jobAdmin.JobCancel(ctx, jobID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to change the job: %w", err)
	}
	if err := r.recorder.Record(ctx, r.db, &audit.Event{
		Actor: actor, Action: action, TargetType: targetJob, TargetID: id,
		Summary: map[string]string{"kind": current.Kind, "state": string(current.State)}, SourceIP: netip.Addr{}, UserAgent: "",
	}); err != nil {
		return nil, err
	}

	jobs, err := r.jobModels(ctx, []*rivertype.JobRow{changed})
	if err != nil {
		return nil, err
	}

	return jobs[0], nil
}

// retryable は、ジョブを画面から試し直せるかを返す。動いているジョブは試し直せない。
func retryable(job *rivertype.JobRow) bool {
	return job.State != rivertype.JobStateRunning
}

// cancellable は、ジョブを画面から取り消せるかを返す。終えたジョブと、取り消すと録画を失いうるジョブは取り消せない。
func cancellable(job *rivertype.JobRow) bool {
	if slices.Contains(uncancellableKinds(), job.Kind) {
		return false
	}

	return !slices.Contains([]rivertype.JobState{rivertype.JobStateCompleted, rivertype.JobStateCancelled, rivertype.JobStateDiscarded}, job.State)
}

// jobModels は、River のジョブを GraphQL の形にする。引数の JSON は返さず、録画を対象にするジョブなら、その録画を引く。
// 一覧は 5 秒ごとに読み直すので、録画はジョブごとに引かず、まとめて 1 回で引く。
func (r *Resolver) jobModels(ctx context.Context, rows []*rivertype.JobRow) ([]*model.Job, error) {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		if id, ok := jobRecordingID(row); ok && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	byID := map[string]*model.Recording{}
	if len(ids) > 0 {
		recordings, err := r.queries.ListRecordingsByIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("failed to list the recordings of the jobs: %w", err)
		}
		models, err := r.recordingModels(ctx, recordings)
		if err != nil {
			return nil, err
		}
		for i := range recordings {
			if i < len(models) {
				byID[recordings[i].ID.String()] = models[i]
			}
		}
	}
	progress, err := r.jobProgress(ctx, rows)
	if err != nil {
		return nil, err
	}
	jobs := make([]*model.Job, 0, len(rows))
	for _, row := range rows {
		job := jobModel(row)
		if id, ok := jobRecordingID(row); ok {
			// 録画が消えていれば、null のままにする。
			job.Recording = byID[id.String()]
		}
		job.Progress = progress[row.ID]
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// jobProgress は、動いているジョブの進み具合を、ジョブの ID ごとにまとめて引く (#1511)。
// 前の試行の行 (試し直す前に書いたもの) と、終えたジョブの行は返さない。
func (r *Resolver) jobProgress(ctx context.Context, rows []*rivertype.JobRow) (map[int64]*model.JobProgress, error) {
	attempts := map[int64]int{}
	ids := []int64{}
	for _, row := range rows {
		if row.State == rivertype.JobStateRunning {
			attempts[row.ID] = row.Attempt
			ids = append(ids, row.ID)
		}
	}
	result := map[int64]*model.JobProgress{}
	if len(ids) == 0 {
		return result, nil
	}
	progress, err := r.queries.ListJobProgress(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list the progress of the jobs: %w", err)
	}
	for _, p := range progress {
		if int(p.Attempt) != attempts[p.JobID] {
			continue
		}
		job := &model.JobProgress{Ratio: p.Ratio, RemainingSeconds: nil, UpdatedAt: p.UpdatedAt}
		if p.RemainingMs != nil {
			job.RemainingSeconds = new(int((time.Duration(*p.RemainingMs) * time.Millisecond).Round(time.Second).Seconds()))
		}
		result[p.JobID] = job
	}

	return result, nil
}

// jobModel は、River のジョブの、録画のほかの項目を GraphQL の形にする。
func jobModel(row *rivertype.JobRow) *model.Job {
	errs := make([]*model.JobError, 0, len(row.Errors))
	for _, e := range row.Errors {
		errs = append(errs, &model.JobError{At: e.At, Attempt: e.Attempt, Message: e.Error})
	}

	return &model.Job{
		ID: strconv.FormatInt(row.ID, 10), Kind: row.Kind, State: model.JobState(strings.ToUpper(string(row.State))),
		Queue: row.Queue, Attempt: row.Attempt, MaxAttempts: row.MaxAttempts, CreatedAt: row.CreatedAt,
		ScheduledAt: row.ScheduledAt, AttemptedAt: row.AttemptedAt, FinalizedAt: row.FinalizedAt, Errors: errs,
		Recording: nil, Retryable: retryable(row), Cancellable: cancellable(row), Progress: nil,
	}
}

// jobRecordingID は、ジョブの引数から、対象の録画の ID を読む。録画を対象にしないジョブでは false を返す。
func jobRecordingID(row *rivertype.JobRow) (uuid.UUID, bool) {
	var args struct {
		RecordingID *uuid.UUID `json:"recordingId"`
	}
	if err := json.Unmarshal(row.EncodedArgs, &args); err != nil || args.RecordingID == nil {
		return uuid.Nil(), false
	}

	return *args.RecordingID, true
}
