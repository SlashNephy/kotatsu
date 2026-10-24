package graph_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	createStreamURL = `mutation($target: ID!) { createStreamURL(target: $target) { url expiresAt } }`
	varTarget       = "target"
)

// TestCreateStreamURL は、視聴者も、サービスと録画の署名付きの映像の URL を発行でき、
// その URL が、発行したユーザーで 6 時間だけ通ることを確かめる (設計書 7.6、9.6)。
func TestCreateStreamURL(t *testing.T) {
	t.Parallel()

	signer, err := stream.NewSigner("kotatsu-graph-test-application-key-0123456789")
	require.NoError(t, err)
	dir := t.TempDir()
	f := newFixture(t, &graph.Options{Pools: map[string]string{poolMain: dir}, StreamSigner: signer})
	f.syncEPG(t)
	recordingID := f.addRecording(t, dir, "こたつ特集", "completed", time.Hour)
	_, rawRecordingID, err := graph.ParseGlobalID(recordingID)
	require.NoError(t, err)
	viewer := f.as(t, "yuzu", auth.RoleViewer)

	tests := []struct {
		name     string
		target   string
		wantPath string
	}{
		{name: "サービス", target: graph.GlobalID("Service", "3273601024"), wantPath: "/streams/services/3273601024"},
		{name: "録画", target: recordingID, wantPath: "/streams/recordings/" + rawRecordingID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got struct {
				CreateStreamURL struct {
					URL       string    `json:"url"`
					ExpiresAt time.Time `json:"expiresAt"`
				} `json:"createStreamURL"`
			}
			require.Empty(t, viewer.query(t, createStreamURL, map[string]any{varTarget: tt.target}, &got))
			assert.Equal(t, testNow().Add(6*time.Hour), got.CreateStreamURL.ExpiresAt)
			parsed, err := url.Parse(got.CreateStreamURL.URL)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, parsed.Path)
			assert.Empty(t, parsed.Host, "the URL is relative to the page")

			userID, err := signer.Verify(parsed.Path, parsed.Query(), testNow().Add(6*time.Hour-time.Second))
			require.NoError(t, err)
			assert.Equal(t, viewer.userID, userID)
			_, err = signer.Verify(parsed.Path, parsed.Query(), testNow().Add(6*time.Hour))
			require.ErrorIs(t, err, stream.ErrInvalidSignature, "the URL expires after 6 hours")
		})
	}
}

// TestCreateStreamURLRejectsTarget は、ないものと、サービスか録画でないものの URL を発行しないことを確かめる。
func TestCreateStreamURLRejectsTarget(t *testing.T) {
	t.Parallel()

	signer, err := stream.NewSigner("kotatsu-graph-test-application-key-0123456789")
	require.NoError(t, err)
	f := newFixture(t, &graph.Options{StreamSigner: signer})
	f.syncEPG(t)

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "ないサービス", target: graph.GlobalID("Service", "3273601099"), want: codeNotFound},
		{name: "ない録画", target: graph.GlobalID("Recording", "0199d000-0000-7000-8000-000000000001"), want: codeNotFound},
		{name: "番組", target: graph.GlobalID("Program", "3273601024.1"), want: codeBadUserInput},
		{name: "ID でない", target: "こたつ", want: codeBadUserInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, []string{tt.want}, f.query(t, createStreamURL, map[string]any{varTarget: tt.target}, nil))
		})
	}
}

// TestTranscoding は、生きている kyusu が登録簿にいるときだけ、サーバー側でのトランスコードを使えるとすることを確かめる。
func TestTranscoding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// heartbeat は、登録簿に載せる kyusu の最後のハートビートの、今からの差。nil なら載せない。
		heartbeat     *time.Duration
		wantAvailable bool
	}{
		{name: "kyusu がいない", heartbeat: nil, wantAvailable: false},
		{name: "生きている kyusu がいる", heartbeat: new(-10 * time.Second), wantAvailable: true},
		{name: "ハートビートの途絶えた kyusu しかいない", heartbeat: new(-time.Hour), wantAvailable: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			if tt.heartbeat != nil {
				at := testNow().Add(*tt.heartbeat)
				_, err := f.pool.Exec(t.Context(), `INSERT INTO instances (id, role, address, started_at, heartbeat_at)
					VALUES (gen_random_uuid(), 'kyusu', '127.0.0.1:8080', $1, $1)`, at)
				require.NoError(t, err)
			}
			var got struct {
				Transcoding struct {
					Available bool `json:"available"`
					Presets   []struct {
						Name         string `json:"name"`
						Height       int    `json:"height"`
						VideoBitrate int    `json:"videoBitrate"`
						Codec        string `json:"codec"`
					} `json:"presets"`
				} `json:"transcoding"`
			}
			require.Empty(t, f.query(t, `{ transcoding { available presets { name height videoBitrate codec } } }`, nil, &got))
			assert.Equal(t, tt.wantAvailable, got.Transcoding.Available)
			// 画質は、kyusu の有無によらず、URL に使う名前とともに返す。
			require.NotEmpty(t, got.Transcoding.Presets)
			assert.Equal(t, "1080p", got.Transcoding.Presets[0].Name)
			assert.Equal(t, 6000, got.Transcoding.Presets[0].VideoBitrate)
			assert.Equal(t, "H264", got.Transcoding.Presets[0].Codec)
			codecs := map[string]string{}
			for _, preset := range got.Transcoding.Presets {
				codecs[preset.Name] = preset.Codec
			}
			assert.Equal(t, "HEVC", codecs["720p-hevc"])
			// 60fps の画質も返す (#499)。
			assert.Equal(t, "H264", codecs["720p60"])
		})
	}
}
