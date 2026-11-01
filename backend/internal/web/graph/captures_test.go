package graph_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/capture"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const capturesQuery = `query($keyword: String, $after: String) {
	captures(first: 1, keyword: $keyword, after: $after) {
		edges { node { id imageUrl serviceName programName recordingId position } }
		pageInfo { hasNextPage endCursor }
	}
}`

type capturesResult struct {
	Captures struct {
		Edges []struct {
			Node struct {
				ID          string   `json:"id"`
				ImageURL    string   `json:"imageUrl"`
				ServiceName string   `json:"serviceName"`
				ProgramName *string  `json:"programName"`
				RecordingID *string  `json:"recordingId"`
				Position    *float64 `json:"position"`
			} `json:"node"`
		} `json:"edges"`
		PageInfo struct {
			HasNextPage bool    `json:"hasNextPage"`
			EndCursor   *string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"captures"`
}

// saveCapture は、ユーザー userID のキャプチャを、データディレクトリ dataDir に残す。
func saveCapture(t *testing.T, f *fixture, dataDir string, userID uuid.UUID, serviceName string) uuid.UUID {
	t.Helper()

	saved, err := capture.New(kago.New(nil, nil).WithDataDir(dataDir), f.clock, slog.New(slog.DiscardHandler), 0).
		Save(t.Context(), f.pool, userID, &capture.Upload{
			ServiceName: serviceName, ProgramName: new("ニュース"), Caption: nil, RecordingID: nil, Position: nil,
			BroadcastAt: f.clock.Now(), TakenAt: f.clock.Now(), Image: []byte("\x89PNG\r\n\x1a\nimage"),
		})
	require.NoError(t, err)
	f.clock.Advance(time.Second)

	return saved.ID
}

// TestCaptures は、ログイン中のユーザーのキャプチャだけを、新しい順にページを送って返し、探す言葉で絞ることを確かめる。
func TestCaptures(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	f := newFixture(t, &graph.Options{DataDir: dataDir})
	other := f.as(t, "other", auth.RoleViewer)
	older := saveCapture(t, f, dataDir, f.userID, "こたつテスト1")
	newer := saveCapture(t, f, dataDir, f.userID, "ＮＨＫ総合")
	_ = saveCapture(t, f, dataDir, other.userID, "ＮＨＫ総合")

	var first capturesResult
	require.Empty(t, f.query(t, capturesQuery, map[string]any{}, &first))
	require.Len(t, first.Captures.Edges, 1)
	assert.Equal(t, graph.CaptureID(newer), first.Captures.Edges[0].Node.ID)
	assert.Equal(t, "/media/captures/"+newer.String(), first.Captures.Edges[0].Node.ImageURL)
	assert.True(t, first.Captures.PageInfo.HasNextPage)

	var second capturesResult
	require.Empty(t, f.query(t, capturesQuery, map[string]any{"after": first.Captures.PageInfo.EndCursor}, &second))
	require.Len(t, second.Captures.Edges, 1)
	assert.Equal(t, graph.CaptureID(older), second.Captures.Edges[0].Node.ID)
	assert.False(t, second.Captures.PageInfo.HasNextPage)

	var searched capturesResult
	require.Empty(t, f.query(t, capturesQuery, map[string]any{"keyword": "nhk"}, &searched))
	require.Len(t, searched.Captures.Edges, 1)
	assert.Equal(t, graph.CaptureID(newer), searched.Captures.Edges[0].Node.ID)
	assert.False(t, searched.Captures.PageInfo.HasNextPage, "the other user's capture is not listed")
}

// TestDeleteCapture は、撮ったユーザーだけが、キャプチャを画像ごと消せることを確かめる。
func TestDeleteCapture(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	f := newFixture(t, &graph.Options{DataDir: dataDir})
	other := f.as(t, "other", auth.RoleViewer)
	captureID := saveCapture(t, f, dataDir, f.userID, "こたつテスト1")
	image := filepath.Join(dataDir, kago.CaptureLocation(f.userID, captureID).Path)
	const mutation = `mutation($id: ID!) { deleteCapture(id: $id) }`
	variables := map[string]any{"id": graph.CaptureID(captureID)}

	assert.Equal(t, []string{"NOT_FOUND"}, other.query(t, mutation, variables, nil))
	assert.FileExists(t, image)

	var deleted struct {
		DeleteCapture string `json:"deleteCapture"`
	}
	require.Empty(t, f.query(t, mutation, variables, &deleted))
	assert.Equal(t, graph.CaptureID(captureID), deleted.DeleteCapture)
	_, err := os.Stat(image)
	require.ErrorIs(t, err, os.ErrNotExist)
}

// TestCapturesAvailable は、データディレクトリを設定しているかで、キャプチャを残せるかを答えることを確かめる。
func TestCapturesAvailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dataDir string
		want    bool
	}{
		{name: "データディレクトリがある", dataDir: t.TempDir(), want: true},
		{name: "データディレクトリがない", dataDir: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, &graph.Options{DataDir: tt.dataDir})
			var got struct {
				CapturesAvailable bool `json:"capturesAvailable"`
			}
			require.Empty(t, f.query(t, `{ capturesAvailable }`, nil, &got))
			assert.Equal(t, tt.want, got.CapturesAvailable)
		})
	}
}
