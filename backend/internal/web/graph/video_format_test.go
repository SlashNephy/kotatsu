package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// TestVideoFormat は、サービスで放送中の番組と、録画を始めたときの番組の、映像の解像度とチャンネル種別から、
// 映像の種類を決めることを確かめる (#806)。BS4K は、解像度が分からなくても、2K と書かれていても、MPEG-2 でないので UHD にする。
func TestVideoFormat(t *testing.T) {
	t.Parallel()

	// bs4k は、Mahiron が返す BS4K のチャンネル種別。
	const bs4k = "BS4K"
	tests := []struct {
		name        string
		channelType string
		// resolution は、番組と録画の映像の解像度。nil なら分からない。
		resolution *string
		// noProgram なら、サービスの番組を消し、放送中の番組がない状態にする。
		noProgram bool
		want      model.VideoFormat
	}{
		{name: "2K", channelType: "GR", resolution: new("1080i"), want: model.VideoFormatHd},
		{name: "unknown on GR", channelType: "GR", resolution: nil, want: model.VideoFormatHd},
		{name: "4K", channelType: bs4k, resolution: new("2160p"), want: model.VideoFormatUhd},
		{name: "unknown on BS4K", channelType: bs4k, resolution: nil, want: model.VideoFormatUhd},
		{name: "2K on BS4K", channelType: bs4k, resolution: new("1080i"), want: model.VideoFormatUhd},
		// Mahiron は、NHK BS8K の種別も BS4K と返す。
		{name: "8K", channelType: bs4k, resolution: new("4320p"), want: model.VideoFormatShv},
		// 放送中の番組がなければ、サービスはチャンネル種別から決める。
		{name: "no program on GR", channelType: "GR", resolution: nil, noProgram: true, want: model.VideoFormatHd},
		{name: "no program on BS4K", channelType: bs4k, resolution: nil, noProgram: true, want: model.VideoFormatUhd},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, dir := newRecordingFixture(t)
			_, err := f.pool.Exec(t.Context(), `INSERT INTO channel_types (type, display_name, sort_order) VALUES ('BS4K', 'BS4K', 30)
				ON CONFLICT (type) DO NOTHING`)
			require.NoError(t, err)
			_, err = f.pool.Exec(t.Context(), `UPDATE services SET channel_type = $2 WHERE id = $1`, recordingServiceID, tt.channelType)
			require.NoError(t, err)
			_, err = f.pool.Exec(t.Context(), `UPDATE programs SET video_resolution = $2 WHERE service_id = $1`, recordingServiceID, tt.resolution)
			require.NoError(t, err)
			if tt.noProgram {
				_, err = f.pool.Exec(t.Context(), `DELETE FROM programs WHERE service_id = $1`, recordingServiceID)
				require.NoError(t, err)
			}
			recordingID := f.addRecording(t, dir, "録画", "completed", time.Hour)
			_, err = f.pool.Exec(t.Context(), `UPDATE recordings SET video_resolution = $1`, tt.resolution)
			require.NoError(t, err)

			var got struct {
				Services []struct {
					ServiceID   int               `json:"serviceId"`
					VideoFormat model.VideoFormat `json:"videoFormat"`
				} `json:"services"`
				Node struct {
					VideoFormat model.VideoFormat `json:"videoFormat"`
				} `json:"node"`
			}
			require.Empty(t, f.query(t, `query($id: ID!) { services { serviceId videoFormat } node(id: $id) { ... on Recording { videoFormat } } }`,
				map[string]any{"id": recordingID}, &got))
			formats := map[int]model.VideoFormat{}
			for _, s := range got.Services {
				formats[s.ServiceID] = s.VideoFormat
			}
			assert.Equal(t, tt.want, formats[recordingServiceID%100000], "service")
			assert.Equal(t, tt.want, got.Node.VideoFormat, "recording")
		})
	}
}
