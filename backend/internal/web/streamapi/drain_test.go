package streamapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/web/streamapi"
)

// TestDrainer は、止める前の知らせを始めたら、ライブ視聴と録画のストリームがなくなるまで待ち、
// なくならなければ上限の時間で諦めることを確かめる (#1085)。データ放送のストリームは待たない。
func TestDrainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind streamapi.StreamKind
		// closeAfter は、知らせを始めてから、ストリームを閉じるまでの時間。0 なら閉じない。
		closeAfter time.Duration
		timeout    time.Duration
		wantMin    time.Duration
		wantMax    time.Duration
	}{
		{name: "ストリームがない", kind: "", timeout: time.Minute, wantMin: 0, wantMax: 100 * time.Millisecond},
		{name: "ライブ視聴がつなぎ直した", kind: streamapi.StreamLive, closeAfter: 300 * time.Millisecond, timeout: time.Minute, wantMin: 300 * time.Millisecond, wantMax: time.Second},
		{name: "録画の再生がつなぎ直さない", kind: streamapi.StreamRecording, timeout: 500 * time.Millisecond, wantMin: 500 * time.Millisecond, wantMax: 900 * time.Millisecond},
		{name: "データ放送は待たない", kind: streamapi.StreamDataBroadcast, timeout: time.Minute, wantMin: 0, wantMax: 100 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			streams := streamapi.NewStreamCounter()
			drainer := streamapi.NewDrainer(streams)
			release := make(chan struct{})
			streaming := make(chan struct{})
			if tt.kind != "" {
				handler := streams.Count(tt.kind, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					close(streaming)
					<-release
				}))
				go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
				<-streaming
			}
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			if tt.closeAfter > 0 {
				time.AfterFunc(tt.closeAfter, func() { close(release) })
			}

			select {
			case <-drainer.Draining():
				t.Fatal("the drainer started before Drain was called")
			default:
			}
			start := time.Now()
			drainer.Drain(t.Context(), tt.timeout)
			elapsed := time.Since(start)

			assert.GreaterOrEqual(t, elapsed, tt.wantMin)
			assert.LessOrEqual(t, elapsed, tt.wantMax)
			select {
			case <-drainer.Draining():
			default:
				t.Fatal("Draining was not closed")
			}
		})
	}
}
