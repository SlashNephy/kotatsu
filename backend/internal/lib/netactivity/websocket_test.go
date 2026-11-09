package netactivity_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
)

// TestClientRecordsWebSocket は、実況のコメントで使う WebSocket の接続を、読み書きを妨げずに数えることを確かめる (#1369)。
func TestClientRecordsWebSocket(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		kind, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		_ = conn.Write(r.Context(), kind, data)
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}))
	t.Cleanup(server.Close)

	store := &fakeStore{mu: sync.Mutex{}, enabled: true, err: nil, saved: nil}
	recorder, flush := enabledRecorder(t, store, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC))
	client := netactivity.NewClient(recorder, netactivity.PurposeJikkyoComments, netactivity.ClientOptions{Transport: nil, Timeout: 0, CheckRedirect: nil})

	conn, response, err := websocket.Dial(t.Context(), server.URL, &websocket.DialOptions{HTTPClient: client})
	require.NoError(t, err)
	if response.Body != nil {
		_ = response.Body.Close()
	}
	defer func() { _ = conn.CloseNow() }()

	require.NoError(t, conn.Write(t.Context(), websocket.MessageText, []byte("hello")))
	_, data, err := conn.Read(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
	flush()

	require.Len(t, store.saved, 1)
	assert.Equal(t, int64(1), store.saved[0].Count)
	assert.Positive(t, store.saved[0].SentBytes)
	assert.Positive(t, store.saved[0].ReceivedBytes)
}
