package epgstationapi_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/epgstationapi"
)

const (
	grService     = int64(3273601024)
	bsService     = int64(400101)
	hiddenService = int64(3273601025)
	dataService   = int64(3273601026)

	newsName = "ニュース７🈑"
	bsName   = "ＢＳの番組"
)

func testNow() time.Time {
	return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
}

// newHandler は、GR と BS のサービスを 1 つずつと、出さないサービス (非表示、データ放送) を持つ DB で Handler を返す。
func newHandler(t *testing.T) (*epgstationapi.Handler, *pgxpool.Pool) {
	t.Helper()

	now := testNow()
	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	exec(t, pool, "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10), ('BS', 'BS', 20)")
	services := []struct {
		id          int64
		networkID   int64
		sid         int64
		name        string
		serviceType int64
		channelType string
		hidden      bool
		logo        *int64
	}{
		{id: grService, networkID: 32736, sid: 1024, name: "ＮＨＫ総合１・東京", serviceType: 1, channelType: "GR", hidden: false, logo: new(int64(5))},
		{id: bsService, networkID: 4, sid: 101, name: "ＮＨＫ　ＢＳ", serviceType: 1, channelType: "BS", hidden: false, logo: nil},
		{id: hiddenService, networkID: 32736, sid: 1025, name: "非表示", serviceType: 1, channelType: "GR", hidden: true, logo: nil},
		{id: dataService, networkID: 32736, sid: 1026, name: "データ", serviceType: 0xC0, channelType: "GR", hidden: false, logo: nil},
	}
	for _, s := range services {
		exec(t, pool, `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, logo_id, hidden, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $4, $5, $6, '27', $7, $8, $9, $9)`, s.id, s.networkID, s.sid, s.name, s.serviceType, s.channelType, s.logo, s.hidden, now)
	}
	programs := []struct {
		serviceID int64
		eventID   int64
		start     time.Time
		name      string
		extended  string
		genres    string
	}{
		{serviceID: grService, eventID: 100, start: now, name: newsName, extended: `{"出演者":"こたつ\nねこ　と\r\nいぬ"}`, genres: `[{"lv1":0,"lv2":1,"un1":15,"un2":15}]`},
		{serviceID: grService, eventID: 101, start: now.Add(2 * time.Hour), name: "あとの番組", extended: `{}`, genres: `[]`},
		{serviceID: bsService, eventID: 200, start: now, name: bsName, extended: `{}`, genres: `[]`},
	}
	for _, p := range programs {
		exec(t, pool, `INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
			name, description, name_normalized, description_normalized, extended, genres, related_items, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, false, true, $6, '', $6, '', $7, $8, '[]', $9, $9)`,
			p.serviceID, p.eventID, p.serviceID*100000+p.eventID, p.start, p.start.Add(time.Hour), p.name, p.extended, p.genres, now)
	}

	return epgstationapi.New(pool, slog.New(slog.DiscardHandler)), pool
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()

	_, err := pool.Exec(t.Context(), sql, args...)
	require.NoError(t, err)
}

func get(t *testing.T, handler http.HandlerFunc, target string, out any) int {
	t.Helper()

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody))
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), out))
	}

	return rec.Code
}

type channel struct {
	ID            int64  `json:"id"`
	ServiceID     int64  `json:"serviceId"`
	NetworkID     int64  `json:"networkId"`
	Name          string `json:"name"`
	HalfWidthName string `json:"halfWidthName"`
	HasLogoData   bool   `json:"hasLogoData"`
	ChannelType   string `json:"channelType"`
	Type          int64  `json:"type"`
}

type program struct {
	ID        int64  `json:"id"`
	ChannelID int64  `json:"channelId"`
	StartAt   int64  `json:"startAt"`
	EndAt     int64  `json:"endAt"`
	Duration  int64  `json:"duration"`
	Name      string `json:"name"`
	Extended  string `json:"extended"`
	Genres    []struct {
		Lv1 int `json:"lv1"`
		Lv2 int `json:"lv2"`
	} `json:"genres"`
}

type schedule struct {
	Channel  channel   `json:"channel"`
	Programs []program `json:"programs"`
}

// TestChannelsAndConfig は、番組表に出すサービスだけを、EPGStation の形で返すことを確かめる (#431)。
func TestChannelsAndConfig(t *testing.T) {
	t.Parallel()

	h, _ := newHandler(t)
	var channels []channel
	require.Equal(t, http.StatusOK, get(t, h.Channels, "/api/channels", &channels))
	assert.Equal(t, []channel{
		{ID: grService, ServiceID: 1024, NetworkID: 32736, Name: "ＮＨＫ総合１・東京", HalfWidthName: "NHK総合1・東京", HasLogoData: true, ChannelType: "GR", Type: 1},
		{ID: bsService, ServiceID: 101, NetworkID: 4, Name: "ＮＨＫ　ＢＳ", HalfWidthName: "NHK BS", HasLogoData: false, ChannelType: "BS", Type: 1},
	}, channels)

	var config struct {
		Broadcast map[string]bool `json:"broadcast"`
	}
	require.Equal(t, http.StatusOK, get(t, h.Config, "/api/config", &config))
	assert.Equal(t, map[string]bool{"GR": true, "BS": true, "CS": false, "SKY": false}, config.Broadcast)
}

// TestSchedules は、時間帯と放送波の種別で番組を絞り、時刻をミリ秒で返すことを確かめる。
func TestSchedules(t *testing.T) {
	t.Parallel()

	h, _ := newHandler(t)
	now := testNow()
	from, to := now.Add(-time.Minute), now.Add(time.Hour)
	ms := func(v time.Time) string { return strconv.FormatInt(v.UnixMilli(), 10) }
	sec := func(v time.Time) string { return strconv.FormatInt(v.Unix(), 10) }
	news := program{
		ID: grService*100000 + 100, ChannelID: grService, StartAt: now.UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(),
		Duration: time.Hour.Milliseconds(), Name: newsName, Extended: "出演者\nこたつ ねこ　と いぬ",
		Genres: []struct {
			Lv1 int `json:"lv1"`
			Lv2 int `json:"lv2"`
		}{{Lv1: 0, Lv2: 1}},
	}

	tests := []struct {
		name  string
		query string
		want  map[int64][]string
	}{
		{name: "ミリ秒", query: "startAt=" + ms(from) + "&endAt=" + ms(to), want: map[int64][]string{grService: {newsName}, bsService: {bsName}}},
		{name: "秒", query: "startAt=" + sec(from) + "&endAt=" + sec(to), want: map[int64][]string{grService: {newsName}, bsService: {bsName}}},
		{name: "BS を外す", query: "startAt=" + ms(from) + "&endAt=" + ms(to) + "&GR=true&BS=false", want: map[int64][]string{grService: {newsName}}},
		{name: "半角", query: "startAt=" + ms(from) + "&endAt=" + ms(to) + "&isHalfWidth=true", want: map[int64][]string{grService: {"ニュース7🈑"}, bsService: {"BSの番組"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var schedules []schedule
			require.Equal(t, http.StatusOK, get(t, h.Schedules, "/api/schedules?"+tt.query, &schedules))
			got := map[int64][]string{}
			for _, s := range schedules {
				for _, p := range s.Programs {
					got[s.Channel.ID] = append(got[s.Channel.ID], p.Name)
				}
			}
			assert.Equal(t, tt.want, got)
			if tt.name == "ミリ秒" {
				assert.Equal(t, news, schedules[0].Programs[0])
			}
		})
	}

	for _, query := range []string{"", "startAt=x&endAt=1", "startAt=" + ms(to) + "&endAt=" + ms(from), "startAt=" + ms(from) + "&endAt=" + ms(from.Add(10*24*time.Hour))} {
		assert.Equal(t, http.StatusBadRequest, get(t, h.Schedules, "/api/schedules?"+query, nil), query)
	}
}
