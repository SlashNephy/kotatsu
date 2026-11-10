package jikkyoapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/web/jikkyoapi"
)

const otherTID = 5000

type othersBody struct {
	Broadcasts []struct {
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
		Comments []struct {
			Time    float64 `json:"time"`
			Content string  `json:"content"`
		} `json:"comments"`
	} `json:"broadcasts"`
}

// linkOther は、録画を話に結びつけ、同じ話の放送と、チャンネルの対応を足す。
func (f *commentsFixture) linkOther(t *testing.T, recordingID uuid.UUID, episodes int) {
	t.Helper()

	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := f.pool.Exec(t.Context(), sql, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO recording_anime_matches (recording_id, matched_at) VALUES ($1, now())`, recordingID)
	for position := range episodes {
		exec(`INSERT INTO recording_anime_episodes (recording_id, position, source, syobocal_tid, count_start, count_end, is_first, is_last)
			VALUES ($1, $2, 'syobocal', $3, 6, 6, false, false)`, recordingID, position, otherTID)
	}
	exec(`INSERT INTO anime_channel_definitions (channel_type, network_id, sid, name, syobocal_channel_id, jikkyo_channel_id)
		VALUES ('BS', 4, 211, 'BS', 128, 211), ('GR', 0, 1024, 'GR', 19, 1)`)
	from, to := recordedFrom(), recordedFrom().Add(30*time.Minute)
	exec(`INSERT INTO anime_episode_broadcasts (pid, tid, channel_id, start_at, end_at, offset_seconds, episode_count) VALUES
		(1, $1, 128, $2, $3, 0, 6), (2, $1, 19, $2, $3, 0, 6), (3, $1, 128, $2, $3, 0, 7), (4, $1, 128, $4, $5, 0, 6)`,
		otherTID, from, to, from.Add(time.Hour), to.Add(time.Hour))
}

func (f *commentsFixture) setMix(t *testing.T, enabled bool) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), `UPDATE user_settings SET comment_settings = jsonb_build_object('mixOtherBroadcasts', $2::boolean) WHERE user_id = $1`, f.userID, enabled)
	require.NoError(t, err)
}

// TestOtherCommentsHandler は、同じ話の別の放送の過去ログを、その放送の始まりからの秒で返すことを確かめる。
// 録画と同じチャンネルの放送、ほかの話、まだ終わっていない放送は返さない。
func TestOtherCommentsHandler(t *testing.T) {
	t.Parallel()

	kakolog := `{"packet":[
		{"chat":{"date":"1791633599","date_usec":"500000","mail":"184","user_id":"a:x","content":"始まる前"}},
		{"chat":{"date":"1791633601","date_usec":"250000","mail":"184","user_id":"a:y","content":"始まった"}},
		{"chat":{"date":"1791635460","date_usec":"0","mail":"184","user_id":"a:z","content":"終わった後"}}]}`
	finished := recordedFrom().Add(30 * time.Minute)
	f := newCommentsFixture(t, kakolog)
	recordingID := f.recording(t, grServiceID, recordedFrom(), &finished)
	f.linkOther(t, recordingID, 1)
	f.setMix(t, true)

	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/media/recordings/"+recordingID.String()+"/"+jikkyoapi.OtherCommentsFileName, nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body othersBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Broadcasts, 1)
	assert.Equal(t, "jk211", body.Broadcasts[0].Channel.ID)
	require.Len(t, body.Broadcasts[0].Comments, 1)
	assert.Equal(t, "始まった", body.Broadcasts[0].Comments[0].Content)
	assert.InDelta(t, 1.25, body.Broadcasts[0].Comments[0].Time, 0.001)
	assert.Equal(t, "private, max-age=600", rec.Header().Get("Cache-Control"))
}

// TestOtherCommentsHandlerNotApplicable は、混ぜない設定や、1 話に結びつかない録画では、過去ログ API に問い合わせないことを確かめる。
func TestOtherCommentsHandlerNotApplicable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mix        bool
		jikkyo     bool
		episodes   int
		recording  bool
		wantStatus int
	}{
		{name: "録画中の録画", mix: true, jikkyo: true, episodes: 1, recording: true, wantStatus: http.StatusOK},
		{name: "別の放送を混ぜる設定が切れている", mix: false, jikkyo: true, episodes: 1, wantStatus: http.StatusNotFound},
		{name: "実況を切っている", mix: true, jikkyo: false, episodes: 1, wantStatus: http.StatusNotFound},
		{name: "2 話に結びついた録画", mix: true, jikkyo: true, episodes: 2, wantStatus: http.StatusOK},
		{name: "話に結びついていない録画", mix: true, jikkyo: true, episodes: 0, wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			finished := new(recordedFrom().Add(30 * time.Minute))
			if tt.recording {
				finished = nil
			}
			f := newCommentsFixture(t, `{"packet":[]}`)
			recordingID := f.recording(t, grServiceID, recordedFrom(), finished)
			f.linkOther(t, recordingID, tt.episodes)
			f.setMix(t, tt.mix)
			f.setJikkyo(t, true, tt.jikkyo)

			rec := httptest.NewRecorder()
			f.handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/media/recordings/"+recordingID.String()+"/"+jikkyoapi.OtherCommentsFileName, nil))

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Nil(t, f.fake.query.Load(), "the kakolog API is not called")
		})
	}
}
