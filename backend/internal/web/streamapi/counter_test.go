package streamapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/web/streamapi"
)

// TestStreamCounter は、流している最中の応答を、種類ごとに数えることを確かめる (#959)。HEAD はストリームを開かないので数えない。
func TestStreamCounter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		kind   streamapi.StreamKind
		want   map[streamapi.StreamKind]int
	}{
		{
			name: "ライブ視聴", method: http.MethodGet, kind: streamapi.StreamLive,
			want: map[streamapi.StreamKind]int{streamapi.StreamLive: 1, streamapi.StreamDataBroadcast: 0, streamapi.StreamRecording: 0, streamapi.StreamTimeshift: 0},
		},
		{
			name: "データ放送", method: http.MethodGet, kind: streamapi.StreamDataBroadcast,
			want: map[streamapi.StreamKind]int{streamapi.StreamLive: 0, streamapi.StreamDataBroadcast: 1, streamapi.StreamRecording: 0, streamapi.StreamTimeshift: 0},
		},
		{
			name: "録画の再生", method: http.MethodGet, kind: streamapi.StreamRecording,
			want: map[streamapi.StreamKind]int{streamapi.StreamLive: 0, streamapi.StreamDataBroadcast: 0, streamapi.StreamRecording: 1, streamapi.StreamTimeshift: 0},
		},
		{
			name: "タイムシフトの再生", method: http.MethodGet, kind: streamapi.StreamTimeshift,
			want: map[streamapi.StreamKind]int{streamapi.StreamLive: 0, streamapi.StreamDataBroadcast: 0, streamapi.StreamRecording: 0, streamapi.StreamTimeshift: 1},
		},
		{
			name: "HEAD", method: http.MethodHead, kind: streamapi.StreamLive,
			want: map[streamapi.StreamKind]int{streamapi.StreamLive: 0, streamapi.StreamDataBroadcast: 0, streamapi.StreamRecording: 0, streamapi.StreamTimeshift: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			counter := streamapi.NewStreamCounter()
			// 流している最中の数を、ハンドラーの中で読む。
			during := map[streamapi.StreamKind]int{}
			handler := counter.Count(tt.kind, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				for _, kind := range streamapi.StreamKinds() {
					during[kind] = counter.Streams(kind)
				}
			}))

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), tt.method, "/", nil))
			assert.Equal(t, tt.want, during)
			// 流し終えたら引く。
			for _, kind := range streamapi.StreamKinds() {
				assert.Zero(t, counter.Streams(kind), kind)
			}
		})
	}
}
