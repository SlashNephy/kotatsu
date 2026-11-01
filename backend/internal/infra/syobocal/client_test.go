package syobocal_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/syobocal"
)

// progLookupXML は、2026 年 11 月に ProgLookup が返した応答を、項目を減らして写したもの。
const progLookupXML = `<?xml version="1.0" encoding="UTF-8"?><ProgLookupResponse><ProgItems>` +
	`<ProgItem id="717433"><LastUpdate>2026-09-10 01:58:11</LastUpdate><PID>717433</PID><TID>8018</TID>` +
	`<StTime>2026-11-01 22:00:00</StTime><StOffset>0</StOffset><EdTime>2026-11-01 22:30:00</EdTime><Count>5</Count>` +
	`<SubTitle></SubTitle><ProgComment></ProgComment><Flag>0</Flag><Deleted>0</Deleted><Warn>0</Warn><ChID>19</ChID>` +
	`<Revision>0</Revision><STSubTitle>サブタイトル</STSubTitle></ProgItem>` +
	`<ProgItem id="1"><LastUpdate>2026-10-25 22:28:42</LastUpdate><PID>1</PID><TID>2</TID>` +
	`<StTime>2026-11-02 01:00:00</StTime><StOffset>60</StOffset><EdTime>2026-11-02 02:00:00</EdTime><Count></Count>` +
	`<SubTitle>#1～#2</SubTitle><Flag>6</Flag><Deleted>1</Deleted><ChID>19</ChID></ProgItem>` +
	`</ProgItems><Result><Code>200</Code><Message></Message></Result></ProgLookupResponse>`

// recorder は、サーバーが受け取ったリクエストの時刻とクエリを覚える。
type recorder struct {
	mu       sync.Mutex
	times    []time.Time
	queries  []url.Values
	agents   []string
	response string
}

func (r *recorder) handle(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.times = append(r.times, time.Now())
	r.queries = append(r.queries, req.URL.Query())
	r.agents = append(r.agents, req.Header.Get("User-Agent"))
	response := r.response
	r.mu.Unlock()
	w.Header().Set("Content-Type", "text/xml;charset=UTF-8")
	_, _ = w.Write([]byte(response))
}

func newClient(t *testing.T, response string, interval time.Duration) (*syobocal.Client, *recorder) {
	t.Helper()

	rec := &recorder{mu: sync.Mutex{}, times: []time.Time{}, queries: []url.Values{}, agents: []string{}, response: response}
	server := httptest.NewServer(http.HandlerFunc(rec.handle))
	t.Cleanup(server.Close)

	return syobocal.NewClient(&syobocal.Options{HTTPClient: server.Client(), URL: server.URL, Interval: interval}), rec
}

// TestProgLookup は、ChID と期間を日本時間で渡し、放送時間データを日本時間として読むことを確かめる。
// 問い合わせには、Kotatsu からの問い合わせだと分かる User-Agent を付ける。
func TestProgLookup(t *testing.T) {
	t.Parallel()

	client, rec := newClient(t, progLookupXML, time.Millisecond)
	from := time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC)
	programs, err := client.ProgLookup(t.Context(), []int32{19, 1}, from, from.Add(24*time.Hour))
	require.NoError(t, err)

	jst := syobocal.Tokyo()
	assert.Equal(t, []*syobocal.Program{
		{
			PID: 717433, TID: 8018, ChannelID: 19,
			StartAt: time.Date(2026, 11, 1, 22, 0, 0, 0, jst), EndAt: time.Date(2026, 11, 1, 22, 30, 0, 0, jst), OffsetSeconds: 0,
			Count: "5", SubTitle: "", STSubTitle: "サブタイトル", Flag: 0, Deleted: false,
			LastUpdate: time.Date(2026, 9, 10, 1, 58, 11, 0, jst),
		},
		{
			PID: 1, TID: 2, ChannelID: 19,
			StartAt: time.Date(2026, 11, 2, 1, 0, 0, 0, jst), EndAt: time.Date(2026, 11, 2, 2, 0, 0, 0, jst), OffsetSeconds: 60,
			Count: "", SubTitle: "#1～#2", STSubTitle: "", Flag: syobocal.FlagNew | syobocal.FlagFinal, Deleted: true,
			LastUpdate: time.Date(2026, 10, 25, 22, 28, 42, 0, jst),
		},
	}, programs)

	require.Len(t, rec.queries, 1)
	query := rec.queries[0]
	assert.Equal(t, "ProgLookup", query.Get("Command"))
	assert.Equal(t, "19,1", query.Get("ChID"))
	assert.Equal(t, "20261102_000000-20261103_000000", query.Get("Range"), "the range is written in JST")
	assert.Equal(t, "SubTitles", query.Get("JOIN"))
	assert.True(t, strings.HasPrefix(rec.agents[0], "Kotatsu/"), rec.agents[0])
}

// TestClientInterval は、続けて問い合わせても、間隔を空けることを確かめる。
func TestClientInterval(t *testing.T) {
	t.Parallel()

	const interval = 200 * time.Millisecond
	client, rec := newClient(t, progLookupXML, interval)
	from := time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC)
	for range 3 {
		_, err := client.ProgLookup(t.Context(), []int32{19}, from, from.Add(time.Hour))
		require.NoError(t, err)
	}
	require.Len(t, rec.times, 3)
	for i := 1; i < len(rec.times); i++ {
		assert.GreaterOrEqual(t, rec.times[i].Sub(rec.times[i-1]), interval/2, "the client waits between requests")
	}
}

// TestClientErrors は、しょぼいカレンダーが失敗を返したら、エラーにすることを確かめる。
func TestClientErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response string
	}{
		{name: "result code", response: `<ProgLookupResponse><Result><Code>400</Code><Message>bad</Message></Result></ProgLookupResponse>`},
		{name: "broken XML", response: `<ProgLookupResponse><ProgItems>`},
		{name: "broken time", response: `<ProgLookupResponse><ProgItems><ProgItem><StTime>yesterday</StTime></ProgItem></ProgItems>` +
			`<Result><Code>200</Code></Result></ProgLookupResponse>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, _ := newClient(t, tt.response, time.Millisecond)
			from := time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC)
			_, err := client.ProgLookup(t.Context(), []int32{19}, from, from.Add(time.Hour))
			require.ErrorIs(t, err, syobocal.ErrUnexpectedResponse)
		})
	}
}

// titleLookupXML は、TitleLookup が返す応答を、項目を減らして写したもの。
const titleLookupXML = `<?xml version="1.0" encoding="UTF-8"?><TitleLookupResponse><Result><Code>200</Code><Message></Message></Result>` +
	`<TitleItems><TitleItem id="8018"><TID>8018</TID><Title> 作品の名前 </Title><LastUpdate>2026-10-30 12:34:56</LastUpdate></TitleItem>` +
	`<TitleItem id="2"><TID>2</TID><Title>別の作品</Title><LastUpdate></LastUpdate></TitleItem></TitleItems></TitleLookupResponse>`

// TestTitleLookup は、TID とタイトル名を問い合わせ、since が零値でなければ、それより後に更新されたタイトルに絞ることを確かめる。
// 更新日時が読めないタイトルは、零値の更新日時で返す。
func TestTitleLookup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		since      time.Time
		lastUpdate string
	}{
		{name: "all titles", since: time.Time{}, lastUpdate: ""},
		{name: "updated titles", since: time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC), lastUpdate: "20261101_000000-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, rec := newClient(t, titleLookupXML, time.Millisecond)
			titles, err := client.TitleLookup(t.Context(), tt.since)
			require.NoError(t, err)

			assert.Equal(t, []*syobocal.Title{
				{TID: 8018, Title: "作品の名前", LastUpdate: time.Date(2026, 10, 30, 12, 34, 56, 0, syobocal.Tokyo())},
				{TID: 2, Title: "別の作品", LastUpdate: time.Time{}},
			}, titles)

			require.Len(t, rec.queries, 1)
			query := rec.queries[0]
			assert.Equal(t, "TitleLookup", query.Get("Command"))
			assert.Equal(t, "*", query.Get("TID"))
			assert.Equal(t, "TID,Title,LastUpdate", query.Get("Fields"))
			assert.Equal(t, tt.lastUpdate, query.Get("LastUpdate"), "the filter is written in JST")
			assert.Equal(t, tt.since.IsZero(), !query.Has("LastUpdate"))
		})
	}
}
