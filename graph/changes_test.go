package graph_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/notification"
	"github.com/SlashNephy/kotatsu/internal/notify"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/tunerstate"
)

// subscribe は、subscription を SSE で購読し、届いた data を、届いた順に渡すチャンネルを返す。
func (f *fixture) subscribe(t *testing.T, query string) <-chan json.RawMessage {
	t.Helper()

	body, err := json.Marshal(struct {
		Query string `json:"query"`
	}{Query: query})
	require.NoError(t, err)
	r, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.server.URL+"/graphql", strings.NewReader(string(body)))
	require.NoError(t, err)
	r.Header.Set("Content-Type", contentTypeJSON)
	r.Header.Set("Accept", "text/event-stream")
	r.Header.Set("Cookie", auth.CookieName+"="+f.token)
	resp, err := http.DefaultClient.Do(r)
	require.NoError(t, err)
	// 本文は、テストが終わるときに閉じ、購読をやめる。
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	events := make(chan json.RawMessage, 100)
	go func() {
		defer close(events)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			data, ok := strings.CutPrefix(scanner.Text(), "data: ")
			if !ok {
				continue
			}
			var payload struct {
				Data   json.RawMessage `json:"data"`
				Errors []any           `json:"errors"`
			}
			if json.Unmarshal([]byte(data), &payload) != nil {
				continue
			}
			if len(payload.Errors) > 0 {
				events <- json.RawMessage(data)

				continue
			}
			events <- payload.Data
		}
	}()

	return events
}

func next(t *testing.T, events <-chan json.RawMessage) json.RawMessage {
	t.Helper()

	select {
	case event, ok := <-events:
		require.True(t, ok, "the stream ended")

		return event
	case <-time.After(10 * time.Second):
		t.Fatal("no event arrived")

		return nil
	}
}

func (f *fixture) publish(t *testing.T, channel string) {
	t.Helper()

	require.NoError(t, pgx.BeginFunc(t.Context(), f.pool, func(tx pgx.Tx) error {
		_, err := notify.Publish(t.Context(), tx, f.clock, channel)

		return err
	}))
}

// TestDataChanged は、購読を始めたときに今の版数が届き、ほかの tenban で起きた変更も届くことを確かめる (設計書 1.4、9.1)。
func TestDataChanged(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	events := f.as(t, "viewer", auth.RoleViewer).subscribe(t, `subscription { dataChanged { channel version } }`)
	initial := make([]string, 0, 7)
	for range 7 {
		initial = append(initial, string(next(t, events)))
	}
	assert.Equal(t, []string{
		`{"dataChanged":{"channel":"EPG","version":0}}`,
		`{"dataChanged":{"channel":"RULES","version":0}}`,
		`{"dataChanged":{"channel":"RESERVATIONS","version":0}}`,
		`{"dataChanged":{"channel":"ASSIGNMENTS","version":0}}`,
		`{"dataChanged":{"channel":"RESERVATION_CONFLICTS","version":0}}`,
		`{"dataChanged":{"channel":"TUNERS","version":0}}`,
		`{"dataChanged":{"channel":"RECORDINGS","version":0}}`,
	}, initial)

	// 別の tenban の変更は、DB の変更の通知だけで届く。ここでは、この tenban を通さずに DB で通知する。
	f.publish(t, rule.NotifyChannel)
	assert.JSONEq(t, `{"dataChanged":{"channel":"RULES","version":1}}`, string(next(t, events)))
	f.publish(t, notify.RecordingsChannel)
	assert.JSONEq(t, `{"dataChanged":{"channel":"RECORDINGS","version":1}}`, string(next(t, events)))
	f.publish(t, tunerstate.NotifyChannel)
	f.publish(t, tunerstate.NotifyChannel)
	// 立て続けの変更は 1 つにまとまることがあるので、最後の版数が届くまで読む。
	for {
		var got struct {
			DataChanged struct {
				Channel string `json:"channel"`
				Version int    `json:"version"`
			} `json:"dataChanged"`
		}
		require.NoError(t, json.Unmarshal(next(t, events), &got))
		assert.Equal(t, "TUNERS", got.DataChanged.Channel)
		if got.DataChanged.Version == 2 {
			break
		}
	}
}

func TestDataChangedRequiresLogin(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	r, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.server.URL+"/graphql",
		strings.NewReader(`{"query":"subscription { dataChanged { channel } }"}`))
	require.NoError(t, err)
	r.Header.Set("Content-Type", contentTypeJSON)
	r.Header.Set("Accept", "text/event-stream")
	resp, err := http.DefaultClient.Do(r)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestUnreadNotificationCount は、通知が増えたときと、ほかの端末で既読にしたときに、未読の数が届くことを確かめる (設計書 6.2)。
func TestUnreadNotificationCount(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	f.notify(t, notification.KindRecordingStarted, nil)
	events := f.subscribe(t, `subscription { unreadNotificationCount }`)
	assert.JSONEq(t, `{"unreadNotificationCount":1}`, string(next(t, events)))

	// nekonote が配った通知が届く。
	f.notify(t, notification.KindRecordingCompleted, nil)
	assert.JSONEq(t, `{"unreadNotificationCount":2}`, string(next(t, events)))

	// 別の端末 (同じユーザーの別のセッション) で既読にすると、数が減る。
	other := f.withNewSession(t)
	require.Empty(t, other.query(t, markAllRead, nil, nil))
	assert.JSONEq(t, `{"unreadNotificationCount":0}`, string(next(t, events)))
}

// TestUnreadNotificationCountIsPerUser は、ほかのユーザーの既読では、自分の未読の数を送らないことを確かめる。
func TestUnreadNotificationCountIsPerUser(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	other := f.as(t, "viewer", auth.RoleViewer)
	f.notify(t, notification.KindRecordingStarted, nil)
	events := f.subscribe(t, `subscription { unreadNotificationCount }`)
	assert.JSONEq(t, `{"unreadNotificationCount":1}`, string(next(t, events)))

	// ほかのユーザーが既読にしても、自分の数は変わらないので、何も届かない。
	require.Empty(t, other.query(t, markAllRead, nil, nil))
	// 自分の数が変わると、それだけが届く。
	f.notify(t, notification.KindRecordingFailed, nil)
	assert.JSONEq(t, `{"unreadNotificationCount":2}`, string(next(t, events)))
}

// TestViewerSettingsChanged は、ある端末で設定を変えると、同じユーザーのほかの端末にだけ届くことを確かめる (設計書 9.1)。
func TestViewerSettingsChanged(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	other := f.as(t, "viewer", auth.RoleViewer)
	mine := f.subscribe(t, `subscription { viewerSettingsChanged }`)
	theirs := other.subscribe(t, `subscription { viewerSettingsChanged }`)
	assert.JSONEq(t, `{"viewerSettingsChanged":0}`, string(next(t, mine)))
	assert.JSONEq(t, `{"viewerSettingsChanged":0}`, string(next(t, theirs)))

	tests := []struct {
		name   string
		change func(t *testing.T, device *fixture)
	}{
		{name: "通知の設定", change: func(t *testing.T, device *fixture) {
			t.Helper()

			input := map[string]any{keyKind: "STORAGE_LOW", keyInApp: false, keyWebPush: true}
			require.Empty(t, device.query(t, updatePreference, map[string]any{varInput: input}, nil))
		}},
		{name: "番組表ビュー", change: func(t *testing.T, device *fixture) {
			t.Helper()

			require.Empty(t, device.query(t, `mutation { createGuideView(input: {name: "地上波", hideEmptyServices: false}) { id } }`, nil, nil))
		}},
		{name: "セッションの失効", change: func(t *testing.T, device *fixture) {
			t.Helper()

			_, spare, err := device.sessions.Create(t.Context(), device.pool, device.userID, nil)
			require.NoError(t, err)
			require.Empty(t, device.query(t, `mutation($id: ID!) { revokeSession(id: $id) }`, map[string]any{varID: graph.GlobalID("Session", spare.ID.String())}, nil))
		}},
	}
	// 別の端末 (同じユーザーの別のセッション) で変える。
	device := f.withNewSession(t)
	for i, tt := range tests {
		tt.change(t, device)
		assert.JSONEq(t, `{"viewerSettingsChanged":`+strconv.Itoa(i+1)+`}`, string(next(t, mine)), tt.name)
	}
	// ほかのユーザーの端末には届かない。そのユーザーが自分で変えたときだけ届く。
	tests[0].change(t, other)
	assert.JSONEq(t, `{"viewerSettingsChanged":1}`, string(next(t, theirs)))
}

// TestViewerEvents は、データの変更、設定の版数、未読の通知の数が、1 つの購読にまとまって届くことを確かめる (設計書 9.1)。
func TestViewerEvents(t *testing.T) {
	t.Parallel()

	f := newFixtureWithChanges(t)
	events := f.subscribe(t, `subscription { viewerEvents { dataChange { channel version } viewerSettingsVersion unreadNotificationCount } }`)
	// 始めに、すべての今の値が届く。3 つの購読から届くので、順は決まらない。
	initial := make([]string, 0, 9)
	for range 9 {
		initial = append(initial, string(next(t, events)))
	}
	event := func(dataChange, settings, unread string) string {
		return `{"viewerEvents":{"dataChange":` + dataChange + `,"viewerSettingsVersion":` + settings + `,"unreadNotificationCount":` + unread + `}}`
	}
	want := make([]string, 0, 9)
	want = append(want, event("null", "0", "null"), event("null", "null", "0"))
	for _, channel := range []string{"EPG", "RULES", "RESERVATIONS", "ASSIGNMENTS", "RESERVATION_CONFLICTS", "TUNERS", "RECORDINGS"} {
		want = append(want, event(`{"channel":"`+channel+`","version":0}`, "null", "null"))
	}
	assert.ElementsMatch(t, want, initial)

	f.publish(t, rule.NotifyChannel)
	assert.JSONEq(t, event(`{"channel":"RULES","version":1}`, "null", "null"), string(next(t, events)))
	f.notify(t, notification.KindRecordingStarted, nil)
	assert.JSONEq(t, event("null", "null", "1"), string(next(t, events)))
	device := f.withNewSession(t)
	require.Empty(t, device.query(t, markAllRead, nil, nil))
	assert.JSONEq(t, event("null", "null", "0"), string(next(t, events)))
	input := map[string]any{keyKind: "STORAGE_LOW", keyInApp: false, keyWebPush: true}
	require.Empty(t, device.query(t, updatePreference, map[string]any{varInput: input}, nil))
	assert.JSONEq(t, event("null", "1", "null"), string(next(t, events)))
}
