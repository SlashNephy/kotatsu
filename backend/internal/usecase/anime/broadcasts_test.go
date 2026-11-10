package anime_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/syobocal"
)

const broadcastTID = 5000

// linkRecording は、録画を作品 broadcastTID の話に結びつけ、終えた録画にする。
func linkRecording(t *testing.T, db *pgxpool.Pool, recordingID uuid.UUID, until time.Time) {
	t.Helper()

	_, err := db.Exec(t.Context(), `UPDATE recordings SET recorded_until = $2 WHERE id = $1`, recordingID, until)
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), `INSERT INTO recording_anime_matches (recording_id, matched_at) VALUES ($1, now())`, recordingID)
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), `INSERT INTO recording_anime_episodes (recording_id, position, source, syobocal_tid, count_start, count_end, is_first, is_last)
		VALUES ($1, 0, 'syobocal', $2, 6, 6, false, false)`, recordingID, broadcastTID)
	require.NoError(t, err)
}

// storedBroadcasts は、持っている放送の PID を並べて返す。
func storedBroadcasts(t *testing.T, db *pgxpool.Pool, tid int32) []int32 {
	t.Helper()

	rows, err := db.Query(t.Context(), `SELECT pid FROM anime_episode_broadcasts WHERE tid = $1 ORDER BY pid`, tid)
	require.NoError(t, err)
	defer rows.Close()
	pids := []int32{}
	for rows.Next() {
		var pid int32
		require.NoError(t, rows.Scan(&pid))
		pids = append(pids, pid)
	}
	require.NoError(t, rows.Err())

	return pids
}

// TestCollectBroadcasts は、録画に結びついた作品の放送を引いて持ち、削除された放送と話数のない放送を除き、
// 引いたばかりの作品は引き直さないことを確かめる。
func TestCollectBroadcasts(t *testing.T) {
	t.Parallel()

	db, clk, client, matcher := newMatchFixture(t)
	recordingID := insertRecording(t, db, grService, jst(1, 0, 0), jst(1, 0, 30), false)
	linkRecording(t, db, recordingID, jst(1, 0, 30))
	program := func(pid int32, count string, deleted bool) *syobocal.Program {
		return &syobocal.Program{PID: pid, TID: broadcastTID, ChannelID: 19, StartAt: jst(1, 0, 0), EndAt: jst(1, 0, 30), Count: count, Deleted: deleted}
	}
	client.programs = append(client.programs, program(101, "6", false), program(102, "6", true), program(103, "", false), program(104, "7", false))

	result, err := matcher.CollectBroadcasts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Requests)
	assert.Equal(t, []int32{101, 104}, storedBroadcasts(t, db, broadcastTID))

	_, err = matcher.CollectBroadcasts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []int32{broadcastTID}, client.tidCalls, "a title looked up just now is not looked up again")

	// 1 日たつと、録画を終えて間もない作品は引き直す。消えた放送は、持っている放送からも消す。
	clk.Advance(25 * time.Hour)
	client.programs = client.programs[:len(client.programs)-1]
	_, err = matcher.CollectBroadcasts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []int32{broadcastTID, broadcastTID}, client.tidCalls)
	assert.Equal(t, []int32{101}, storedBroadcasts(t, db, broadcastTID))
}

// TestCollectBroadcastsDisabled は、連携が無効なら、しょぼいカレンダーに問い合わせないことを確かめる。
func TestCollectBroadcastsDisabled(t *testing.T) {
	t.Parallel()

	db, _, client, matcher := newMatchFixture(t)
	recordingID := insertRecording(t, db, grService, jst(1, 0, 0), jst(1, 0, 30), false)
	linkRecording(t, db, recordingID, jst(1, 0, 30))
	_, err := db.Exec(t.Context(), `UPDATE system_settings SET anime_linking_enabled = false`)
	require.NoError(t, err)

	result, err := matcher.CollectBroadcasts(t.Context())
	require.NoError(t, err)
	assert.True(t, result.Disabled)
	assert.Empty(t, client.tidCalls)
}

// TestCollectBroadcastsFailure は、問い合わせに失敗したら、ジョブを失敗にして、次の機会に引き直せるようにすることを確かめる。
func TestCollectBroadcastsFailure(t *testing.T) {
	t.Parallel()

	db, _, client, matcher := newMatchFixture(t)
	recordingID := insertRecording(t, db, grService, jst(1, 0, 0), jst(1, 0, 30), false)
	linkRecording(t, db, recordingID, jst(1, 0, 30))
	client.fail = true

	_, err := matcher.CollectBroadcasts(t.Context())
	require.Error(t, err)

	client.fail = false
	_, err = matcher.CollectBroadcasts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []int32{broadcastTID, broadcastTID}, client.tidCalls)
}
