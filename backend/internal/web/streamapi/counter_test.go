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
			name: "ライブ視聴", method: http.MethodGet, kind: streamapi.StreamKindLive,
			want: map[streamapi.StreamKind]int{streamapi.StreamKindLive: 1, streamapi.StreamKindDataBroadcast: 0, streamapi.StreamKindRecording: 0, streamapi.StreamKindTimeshift: 0},
		},
		{
			name: "データ放送", method: http.MethodGet, kind: streamapi.StreamKindDataBroadcast,
			want: map[streamapi.StreamKind]int{streamapi.StreamKindLive: 0, streamapi.StreamKindDataBroadcast: 1, streamapi.StreamKindRecording: 0, streamapi.StreamKindTimeshift: 0},
		},
		{
			name: "録画の再生", method: http.MethodGet, kind: streamapi.StreamKindRecording,
			want: map[streamapi.StreamKind]int{streamapi.StreamKindLive: 0, streamapi.StreamKindDataBroadcast: 0, streamapi.StreamKindRecording: 1, streamapi.StreamKindTimeshift: 0},
		},
		{
			name: "タイムシフトの再生", method: http.MethodGet, kind: streamapi.StreamKindTimeshift,
			want: map[streamapi.StreamKind]int{streamapi.StreamKindLive: 0, streamapi.StreamKindDataBroadcast: 0, streamapi.StreamKindRecording: 0, streamapi.StreamKindTimeshift: 1},
		},
		{
			name: "HEAD", method: http.MethodHead, kind: streamapi.StreamKindLive,
			want: map[streamapi.StreamKind]int{streamapi.StreamKindLive: 0, streamapi.StreamKindDataBroadcast: 0, streamapi.StreamKindRecording: 0, streamapi.StreamKindTimeshift: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			counter := streamapi.NewStreamCounter()
			// 流している最中の数を、ハンドラーの中で読む。
			during := map[streamapi.StreamKind]int{}
			handler := counter.Count(tt.kind, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				for _, kind := range streamapi.StreamKindValues() {
					during[kind] = counter.Streams(kind)
				}
			}))

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), tt.method, "/", nil))
			assert.Equal(t, tt.want, during)
			// 流し終えたら引く。
			for _, kind := range streamapi.StreamKindValues() {
				assert.Zero(t, counter.Streams(kind), kind)
			}
		})
	}
}
