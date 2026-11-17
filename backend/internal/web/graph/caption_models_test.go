package graph_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	captionModelsQuery = `{ captionModels { kind state overridden installed { model { id } size } download { model { id } running failed }
		models { id recommended } } }`
	downloadCaptionModel = `mutation($id: String!) { downloadCaptionModel(id: $id) { kind state } }`
	cancelCaptionModel   = `mutation($kind: CaptionModelKind!) { cancelCaptionModelDownload(kind: $kind) { kind state } }`
	deleteCaptionModel   = `mutation($kind: CaptionModelKind!) { deleteCaptionModel(kind: $kind) { kind state } }`
	kindWhisper          = "WHISPER"
	kindVAD              = "VAD"
	stateNotPlaced       = "NOT_PLACED"
)

type captionModelSlot struct {
	Kind       string `json:"kind"`
	State      string `json:"state"`
	Overridden bool   `json:"overridden"`
	Installed  *struct {
		Model *struct {
			ID string `json:"id"`
		} `json:"model"`
		Size float64 `json:"size"`
	} `json:"installed"`
	Download *struct {
		Model *struct {
			ID string `json:"id"`
		} `json:"model"`
		Running bool `json:"running"`
		Failed  bool `json:"failed"`
	} `json:"download"`
	Models []struct {
		ID          string `json:"id"`
		Recommended bool   `json:"recommended"`
	} `json:"models"`
}

// TestCaptionModels は、管理者が、字幕の自動生成のモデルの状態を見て、ダウンロードを頼み、取り消し、置いたモデルを消せることと、
// 操作を監査ログに残すこと、管理者でなければ使えないことを確かめる (#1682)。
func TestCaptionModels(t *testing.T) {
	t.Parallel()

	inserter := &lateInserter{client: nil}
	admin := &lateJobAdmin{client: nil}
	dataDir := t.TempDir()
	f := newFixture(t, &graph.Options{
		DataDir: dataDir, Jobs: inserter, JobAdmin: admin,
		Captions: &config.Captions{WhisperCommand: "", WhisperModel: "", VADCommand: "", VADModel: "/models/vad.bin"},
	})
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), f.pool))
	client, err := jobs.NewClient(f.pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	inserter.client = client
	admin.client = client
	adminFixture := f.as(t, adminName, domain.RoleAdmin)
	slots := func() map[string]captionModelSlot {
		t.Helper()

		var got struct {
			CaptionModels []captionModelSlot `json:"captionModels"`
		}
		require.Empty(t, adminFixture.query(t, captionModelsQuery, nil, &got))
		byKind := map[string]captionModelSlot{}
		for _, s := range got.CaptionModels {
			byKind[s.Kind] = s
		}

		return byKind
	}

	// 置いていなければ、勧めるモデルを先にして、選べるモデルを返す。設定で場所を書いた種類は、そのことを返す。
	initial := slots()
	require.Len(t, initial, 2)
	assert.Equal(t, stateNotPlaced, initial[kindWhisper].State)
	assert.False(t, initial[kindWhisper].Overridden)
	assert.True(t, initial[kindVAD].Overridden)
	require.Len(t, initial[kindWhisper].Models, 3)
	assert.Equal(t, "kotoba-whisper-v2.0-q5_0", initial[kindWhisper].Models[0].ID)
	assert.True(t, initial[kindWhisper].Models[0].Recommended)

	// ダウンロードを頼むと、ジョブを待つ。同じ種類は重ねて頼めない。
	require.Empty(t, adminFixture.query(t, downloadCaptionModel, map[string]any{varID: "kotoba-whisper-v2.0-q5_0"}, nil))
	downloading := slots()[kindWhisper]
	assert.Equal(t, "DOWNLOADING", downloading.State)
	require.NotNil(t, downloading.Download)
	assert.Equal(t, "kotoba-whisper-v2.0-q5_0", downloading.Download.Model.ID)
	assert.Equal(t, []string{codeConflict}, adminFixture.query(t, downloadCaptionModel, map[string]any{varID: "whisper-small-q5_1"}, nil))
	assert.Equal(t, []string{codeBadUserInput}, adminFixture.query(t, downloadCaptionModel, map[string]any{varID: "https://example.com/model.bin"}, nil))

	// 取り消すと、置いていないに戻る。取り消すものがなければ NOT_FOUND。
	require.Empty(t, adminFixture.query(t, cancelCaptionModel, map[string]any{keyKind: kindWhisper}, nil))
	assert.Equal(t, stateNotPlaced, slots()[kindWhisper].State)
	assert.Equal(t, []string{codeNotFound}, adminFixture.query(t, cancelCaptionModel, map[string]any{keyKind: kindWhisper}, nil))

	// 手で置いたファイルも、置いたものとして出し、消せる。消すものがなければ NOT_FOUND。
	vad := filepath.Join(dataDir, "whisper", "ggml-silero-vad.bin")
	require.NoError(t, os.MkdirAll(filepath.Dir(vad), 0o755))
	require.NoError(t, os.WriteFile(vad, []byte("vad"), 0o600))
	placed := slots()[kindVAD]
	assert.Equal(t, "PLACED", placed.State)
	require.NotNil(t, placed.Installed)
	assert.Nil(t, placed.Installed.Model, "a file placed by hand is not one of the catalog")
	assert.InDelta(t, 3, placed.Installed.Size, 0)
	require.Empty(t, adminFixture.query(t, deleteCaptionModel, map[string]any{keyKind: kindVAD}, nil))
	assert.NoFileExists(t, vad)
	assert.Equal(t, stateNotPlaced, slots()[kindVAD].State)
	assert.Equal(t, []string{codeNotFound}, adminFixture.query(t, deleteCaptionModel, map[string]any{keyKind: kindVAD}, nil))

	// 管理者でなければ、見ることも操作もできない。
	for _, role := range []domain.Role{domain.RoleMember, domain.RoleViewer} {
		other := f.as(t, string(role), role)
		assert.Equal(t, []string{codeForbidden}, other.query(t, captionModelsQuery, nil, nil), role)
		assert.Equal(t, []string{codeForbidden}, other.query(t, downloadCaptionModel, map[string]any{varID: "silero-v5.1.2"}, nil), role)
		assert.Equal(t, []string{codeForbidden}, other.query(t, cancelCaptionModel, map[string]any{keyKind: kindWhisper}, nil), role)
		assert.Equal(t, []string{codeForbidden}, other.query(t, deleteCaptionModel, map[string]any{keyKind: kindVAD}, nil), role)
	}

	// 操作は監査ログに残す。
	var actions []string
	rows, err := f.pool.Query(t.Context(), "SELECT action || ':' || target_id || ':' || (summary->>'model') FROM audit_events WHERE target_type = 'caption_model' ORDER BY occurred_at, action")
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var action string
		require.NoError(t, rows.Scan(&action))
		actions = append(actions, action)
	}
	require.NoError(t, rows.Err())
	assert.ElementsMatch(t, []string{
		"caption_model.download:WHISPER:kotoba-whisper-v2.0-q5_0",
		"caption_model.cancel:WHISPER:kotoba-whisper-v2.0-q5_0",
		"caption_model.delete:VAD:",
	}, actions)
}
