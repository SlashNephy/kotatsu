package graph_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	aribCaptionBackfillQuery = `{ aribCaptionBackfill { pending queued extractorVersion } }`
	extractARIBCaptions      = `mutation { extractARIBCaptions }`
)

// TestExtractARIBCaptions は、管理者が、ARIB 字幕の文を取り出していない録画の数を見て、取り出すジョブをまとめて入れられることと、
// 待っている録画には重ねて入れないこと、操作を監査ログに残すこと、管理者でなければ使えないことを確認する (#2218)。
func TestExtractARIBCaptions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(dataRootOf(dir), 0o755))
	inserter := &lateInserter{client: nil}
	f := newFixture(t, &graph.Options{Pools: map[string]string{poolMain: dir}, DataDir: dataRootOf(dir), Jobs: inserter})
	f.syncEPG(t)
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	client, err := jobs.NewClient(f.pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	inserter.client = client
	f.addRecording(t, dir, "取り出していない録画", "completed", 2*time.Hour)
	extracted := f.addRecording(t, dir, "取り出した録画", "completed", time.Hour)
	f.addARIBCaptions(t, extracted, []testCue{})
	adminFixture := f.as(t, adminName, domain.RoleAdmin)

	var backfill struct {
		ARIBCaptionBackfill struct {
			Pending          int `json:"pending"`
			Queued           int `json:"queued"`
			ExtractorVersion int `json:"extractorVersion"`
		} `json:"aribCaptionBackfill"`
	}
	require.Empty(t, adminFixture.query(t, aribCaptionBackfillQuery, nil, &backfill))
	assert.Equal(t, 1, backfill.ARIBCaptionBackfill.Pending)
	assert.Equal(t, 0, backfill.ARIBCaptionBackfill.Queued)
	assert.Equal(t, 1, backfill.ARIBCaptionBackfill.ExtractorVersion)

	// 入れた数を返す。待っている録画には重ねて入れない。
	var queued struct {
		ExtractARIBCaptions int `json:"extractARIBCaptions"`
	}
	require.Empty(t, adminFixture.query(t, extractARIBCaptions, nil, &queued))
	assert.Equal(t, 1, queued.ExtractARIBCaptions)
	require.Empty(t, adminFixture.query(t, extractARIBCaptions, nil, &queued))
	assert.Equal(t, 0, queued.ExtractARIBCaptions)
	// ジョブが終わるまでは取り出していない録画のままで、ジョブを待っている録画として数える。
	require.Empty(t, adminFixture.query(t, aribCaptionBackfillQuery, nil, &backfill))
	assert.Equal(t, 1, backfill.ARIBCaptionBackfill.Pending)
	assert.Equal(t, 1, backfill.ARIBCaptionBackfill.Queued)

	// 管理者でなければ、見ることも操作もできない。
	for _, role := range []domain.Role{domain.RoleMember, domain.RoleViewer} {
		other := f.as(t, string(role), role)
		assert.Equal(t, []string{codeForbidden}, other.query(t, aribCaptionBackfillQuery, nil, nil), role)
		assert.Equal(t, []string{codeForbidden}, other.query(t, extractARIBCaptions, nil, nil), role)
	}

	// 操作は、対象の録画の数と入れた数を添えて、監査ログに残す。
	var actions []string
	rows, err := f.pool.Query(t.Context(), `SELECT action || ':' || (summary->>'recordings') || ':' || (summary->>'queued')
		FROM audit_events WHERE target_type = 'arib_captions' ORDER BY occurred_at, id`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var action string
		require.NoError(t, rows.Scan(&action))
		actions = append(actions, action)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"arib_captions.extract:1:1", "arib_captions.extract:1:0"}, actions)
}
