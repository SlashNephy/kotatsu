package jikkyo_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/jikkyo"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

const (
	// grServiceID は、実況のチャンネル (jk1) がある地上波のサービス。bsServiceID は、対応表にない BS のサービス。
	grServiceID = 3273601024
	bsServiceID = 400999
)

// recordedFrom は、テストの録画を始めた時刻を返す。
func recordedFrom() time.Time {
	return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
}

type commentsFixture struct {
	pool    *pgxpool.Pool
	handler http.Handler
	fake    *fakeJikkyo
}

func newCommentsFixture(t *testing.T, kakolog string) *commentsFixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10), ('BS', 'BS', 20)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $3, $3),
		       ($2, 4, 999, 'こたつ BS', 'こたつ BS', 1, 'BS', 'BS01_0', $3, $3)`, grServiceID, bsServiceID, recordedFrom())
	require.NoError(t, err)

	fake, server := newFakeJikkyo(t, channelsJSON, kakolog)
	table, err := jikkyo.NewTable(nil)
	require.NoError(t, err)
	client := jikkyo.NewClient(&jikkyo.ClientOptions{NXJikkyoURL: server.URL, KakologURL: server.URL, HTTPClient: nil, Clock: nil})
	// 今は、録画を始めて 40 分後。録画中の録画は、今までの過去ログを引く。
	clk := clocktest.NewFake(recordedFrom().Add(40 * time.Minute))
	handler := jikkyo.NewCommentsHandler(pool, table, client, clk, slog.New(slog.DiscardHandler))
	mux := http.NewServeMux()
	mux.Handle("GET /media/recordings/{id}/"+jikkyo.CommentsFileName, handler)

	return &commentsFixture{pool: pool, handler: mux, fake: fake}
}

// recording は、録画を作る。until が nil なら録画中にする。
func (f *commentsFixture) recording(t *testing.T, serviceID int64, from time.Time, until *time.Time) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	state := "completed"
	if until == nil {
		state = "recording"
	}
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, recorded_until, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, '録画', '', $3, $3, $4, false, $3, $5, $3, $3)`, recordingID, serviceID, from, state, until)
	require.NoError(t, err)

	return recordingID
}

// segment は、録画に、startedAt に書き始めたセグメントの行を作る。
func (f *commentsFixture) segment(t *testing.T, recordingID uuid.UUID, startedAt time.Time) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
		VALUES ($1, $2, 'segment', 'main', $3, 0, $4, NULL, $4)`, id.New(), recordingID, recordingID.String()+".m2ts", startedAt)
	require.NoError(t, err)
}

type commentsBody struct {
	Channel *struct {
		ID string `json:"id"`
	} `json:"channel"`
	Origin   *time.Time `json:"origin"`
	Comments []struct {
		Time    float64 `json:"time"`
		Mail    string  `json:"mail"`
		Content string  `json:"content"`
	} `json:"comments"`
}

func TestCommentsHandler(t *testing.T) {
	t.Parallel()

	// 録画を始める 0.5 秒前、始めて 1.25 秒後、31 分後 (30 分で終えた録画では、終えた後) のコメント。
	kakolog := `{"packet":[
		{"chat":{"date":"1791633599","date_usec":"500000","mail":"184","user_id":"a:x","content":"始まる前"}},
		{"chat":{"date":"1791633601","date_usec":"250000","mail":"184 shita","user_id":"a:y","content":"始まった"}},
		{"chat":{"date":"1791635460","date_usec":"0","mail":"184","user_id":"a:z","content":"終わった後"}}]}`
	finished := recordedFrom().Add(30 * time.Minute)
	const (
		started       = "始まった"
		finishedCache = "private, max-age=600"
	)
	tests := []struct {
		name      string
		serviceID int64
		// from は、録画を始めた時刻。ゼロなら recordedFrom。
		from time.Time
		// segmentDelay は、録画を始めてから、最初のセグメントを書き始めるまでの時間。0 ならセグメントを作らない。
		segmentDelay time.Duration
		until        *time.Time
		wantChannel  string
		wantContents []string
		wantTimes    []float64
		wantQuery    string
		wantCache    string
	}{
		{
			name: "録画した時間帯のコメントを、録画の始まりからの秒で返す", serviceID: grServiceID, until: &finished,
			wantChannel: jkNHK, wantContents: []string{started}, wantTimes: []float64{1.25},
			wantQuery: "jk1?endtime=1791635400&format=json&starttime=1791633600", wantCache: finishedCache,
		},
		{
			name: "コメントの時刻は、最初のセグメントを書き始めた時刻からの秒にする", serviceID: grServiceID, until: &finished, segmentDelay: time.Second,
			wantChannel: jkNHK, wantContents: []string{started}, wantTimes: []float64{0.25},
			wantQuery: "jk1?endtime=1791635400&format=json&starttime=1791633601", wantCache: finishedCache,
		},
		{
			name: "録画中なら、今までのコメントを引き、使い回させない", serviceID: grServiceID, until: nil,
			wantChannel: jkNHK, wantContents: []string{started, "終わった後"}, wantTimes: []float64{1.25, 1860},
			wantQuery: "jk1?endtime=1791636000&format=json&starttime=1791633600", wantCache: "no-store",
		},
		{
			name: "録画を始めた時刻が今より後なら、過去ログ API に問い合わせず空の一覧を返す", serviceID: grServiceID, from: recordedFrom().Add(time.Hour), until: nil,
			wantChannel: "jk1", wantContents: []string{}, wantTimes: []float64{}, wantQuery: "", wantCache: "no-store",
		},
		{
			name: "実況のチャンネルがないサービスは、過去ログ API に問い合わせず空の一覧を返す", serviceID: bsServiceID, until: &finished,
			wantChannel: "", wantContents: []string{}, wantTimes: []float64{}, wantQuery: "", wantCache: finishedCache,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newCommentsFixture(t, kakolog)
			from := tt.from
			if from.IsZero() {
				from = recordedFrom()
			}
			recordingID := f.recording(t, tt.serviceID, from, tt.until)
			if tt.segmentDelay > 0 {
				f.segment(t, recordingID, from.Add(tt.segmentDelay))
			}
			rec := httptest.NewRecorder()
			f.handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
				"/media/recordings/"+recordingID.String()+"/"+jikkyo.CommentsFileName, http.NoBody))

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tt.wantCache, rec.Header().Get("Cache-Control"))
			var body commentsBody
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			if tt.wantChannel == "" {
				assert.Nil(t, body.Channel)
				assert.Nil(t, body.Origin)
			} else {
				require.NotNil(t, body.Channel)
				assert.Equal(t, tt.wantChannel, body.Channel.ID)
				// 時刻の基準は、最初のセグメントを書き始めた時刻。セグメントがまだなければ、録画を始めた時刻。
				require.NotNil(t, body.Origin)
				assert.True(t, from.Add(tt.segmentDelay).Equal(*body.Origin), "origin = %s", body.Origin)
			}
			contents, times := make([]string, 0, len(body.Comments)), make([]float64, 0, len(body.Comments))
			for _, comment := range body.Comments {
				contents, times = append(contents, comment.Content), append(times, comment.Time)
			}
			assert.Equal(t, tt.wantContents, contents)
			assert.InDeltaSlice(t, tt.wantTimes, times, 1e-6)
			// ミュートに使うので、ユーザー ID も返す (#488)。
			if slices.Contains(tt.wantContents, "始まった") {
				assert.Contains(t, rec.Body.String(), `"userId":"a:y"`)
			}
			query := ""
			if q := f.fake.query.Load(); q != nil {
				query = *q
			}
			assert.Equal(t, tt.wantQuery, query)
		})
	}
}

func TestCommentsHandlerErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		kakolog    string
		recording  bool
		path       string
		wantStatus int
	}{
		{name: "録画がない", recording: false, path: id.New().String(), wantStatus: http.StatusNotFound},
		{name: "ID の形でない", recording: false, path: "not-a-uuid", wantStatus: http.StatusBadRequest},
		{name: "過去ログ API が理由を返す", kakolog: `{"error":"取得開始時刻または取得終了時刻が不正です。"}`, recording: true, wantStatus: http.StatusBadGateway},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newCommentsFixture(t, tt.kakolog)
			path := tt.path
			if tt.recording {
				finished := recordedFrom().Add(time.Hour)
				path = f.recording(t, grServiceID, recordedFrom(), &finished).String()
			}
			rec := httptest.NewRecorder()
			f.handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
				"/media/recordings/"+path+"/"+jikkyo.CommentsFileName, http.NoBody))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
