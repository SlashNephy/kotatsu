package anime_test

import (
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/syobocal"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
)

// linkedEpisode は、番組か録画に結びついた話の、TID と話数の範囲。
type linkedEpisode struct {
	TID        *int32
	CountStart *float64
	CountEnd   *float64
}

// insertNamedRecording は、番組名 name の録画を足し、番組 (serviceID, start) の結びつきをコピーする。
func insertNamedRecording(t *testing.T, db *pgxpool.Pool, serviceID int64, start, end time.Time, name string) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := db.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, $3, '', $4, $5, 'completed', $4, now(), now())`, id, serviceID, name, start, end)
	require.NoError(t, err)
	require.NoError(t, anime.CopyToRecording(t.Context(), db, id, serviceID, start, time.Now()))

	return id
}

func linkedOfProgram(t *testing.T, db *pgxpool.Pool, serviceID int64, start time.Time) []linkedEpisode {
	t.Helper()

	rows := episodesOfProgram(t, db, serviceID, start)
	linked := make([]linkedEpisode, 0, len(rows))
	for _, row := range rows {
		linked = append(linked, linkedEpisode{TID: row.TID, CountStart: row.CountStart, CountEnd: row.CountEnd})
	}

	return linked
}

func linkedOfRecording(t *testing.T, db *pgxpool.Pool, recordingID uuid.UUID) []linkedEpisode {
	t.Helper()

	rows, err := db.Query(t.Context(), `SELECT syobocal_tid, count_start, count_end FROM recording_anime_episodes
		WHERE recording_id = $1 ORDER BY position`, recordingID)
	require.NoError(t, err)
	defer rows.Close()
	linked := []linkedEpisode{}
	for rows.Next() {
		var row linkedEpisode
		require.NoError(t, rows.Scan(&row.TID, &row.CountStart, &row.CountEnd))
		linked = append(linked, row)
	}
	require.NoError(t, rows.Err())

	return linked
}

// TestTitleAliases は、しょぼいカレンダーで結びついた番組の番組名から作品名の対応を作り、しょぼいカレンダーで引けない番組
// (アニメ専門チャンネルのリピート放送など) の番組名の解析で、作品と話数を決めることを確認する (設計書 3.8)。
//   - 作品名がタイトルの一覧のどのタイトルとも一致しなくても、同じ作品名の本放送の作品に結びつける。
//   - 本放送の番組名の話数と、しょぼいカレンダーの話数がずれていれば、同じだけずらす。
//   - 作品名の対応が 2 つ以上あれば (話数のずれが食い違うものを含む)、結びつけない。
//   - 対応ができる前に確定させた番組と、その録画も、確定させるときに引き直す。
func TestTitleAliases(t *testing.T) {
	t.Parallel()

	db, clk, client, matcher := newMatchFixture(t)
	client.set([]*syobocal.Program{
		syobocalProgram(400, 40, grChannel, jst(3, 0, 0), jst(3, 0, 30), "6", "", "", 0),
		syobocalProgram(410, 41, grChannel, jst(3, 1, 0), jst(3, 1, 30), "33", "", "", 0),
		syobocalProgram(420, 42, grChannel, jst(3, 2, 0), jst(3, 2, 30), "2", "", "", 0),
		syobocalProgram(421, 42, grChannel, jst(3, 3, 0), jst(3, 3, 30), "4", "", "", 0),
	}, false)
	// 本放送 (しょぼいカレンダーで引けるもの)。
	insertNamedProgram(t, db, grService, 11, jst(3, 0, 0), jst(3, 0, 30), "別名作品 【特別版】 #06")
	insertNamedProgram(t, db, grService, 12, jst(3, 1, 0), jst(3, 1, 30), "通し作品 #8")
	insertNamedProgram(t, db, grService, 13, jst(3, 2, 0), jst(3, 2, 30), "ゆれる作品 #2")
	insertNamedProgram(t, db, grService, 14, jst(3, 3, 0), jst(3, 3, 30), "ゆれる作品 #3")
	// ChID のないサービスのリピート放送。1 つ目は本放送より前に放送して確定させる。
	insertNamedProgram(t, db, csService, 21, jst(2, 12, 0), jst(2, 12, 30), "別名作品 【特別版】 #05")
	insertNamedProgram(t, db, csService, 22, jst(4, 12, 0), jst(4, 12, 30), "通し作品 #8")
	insertNamedProgram(t, db, csService, 23, jst(4, 13, 0), jst(4, 13, 30), "ゆれる作品 #2")

	clk.Set(jst(2, 13, 0))
	_, err := matcher.Finalize(t.Context())
	require.NoError(t, err)
	unlinked := []linkedEpisode{{TID: nil, CountStart: new(5.0), CountEnd: new(5.0)}}
	require.Equal(t, unlinked, linkedOfProgram(t, db, csService, jst(2, 12, 0)), "no alias exists before the original broadcast")
	recording := insertNamedRecording(t, db, csService, jst(2, 12, 0), jst(2, 12, 30), "別名作品 【特別版】 #05")
	require.Equal(t, unlinked, linkedOfRecording(t, db, recording))

	clk.Set(jst(5, 0, 0))
	result, err := matcher.Finalize(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 4, result.Aliases)
	assert.Equal(t, 3, result.Relinked, "the finalized repeat, its recording and the repeat finalized in this run are linked")

	tests := []struct {
		name  string
		start time.Time
		want  []linkedEpisode
	}{
		{
			name: "a title that matches no syobocal title is linked through the original broadcast", start: jst(2, 12, 0),
			want: []linkedEpisode{{TID: new(int32(40)), CountStart: new(5.0), CountEnd: new(5.0)}},
		},
		{
			name: "the episode number is shifted as in the original broadcast", start: jst(4, 12, 0),
			want: []linkedEpisode{{TID: new(int32(41)), CountStart: new(33.0), CountEnd: new(33.0)}},
		},
		{
			name: "aliases with different offsets are ambiguous", start: jst(4, 13, 0),
			want: []linkedEpisode{{TID: nil, CountStart: new(2.0), CountEnd: new(2.0)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, linkedOfProgram(t, db, csService, tt.start))
		})
	}
	assert.Equal(t, []linkedEpisode{{TID: new(int32(40)), CountStart: new(5.0), CountEnd: new(5.0)}}, linkedOfRecording(t, db, recording),
		"the recording of a finalized program is linked too")

	// 対応ができた後は、照合するときに結びつける。
	insertNamedProgram(t, db, csService, 24, jst(6, 0, 0), jst(6, 0, 30), "通し作品 #10")
	_, err = matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []linkedEpisode{{TID: new(int32(41)), CountStart: new(35.0), CountEnd: new(35.0)}}, linkedOfProgram(t, db, csService, jst(6, 0, 0)))
}
