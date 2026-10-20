package library_test

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/id"
	"github.com/SlashNephy/kotatsu/internal/kago"
	"github.com/SlashNephy/kotatsu/internal/library"
)

const (
	poolID    = "main"
	serviceID = 3273601024
	// recordingName は、テストの録画の名前。録画ファイルは「録画.m2ts」になる。
	recordingName = "録画"
	completed     = "completed"
)

type fixture struct {
	pool  *pgxpool.Pool
	clock *clocktest.Fake
	dir   string
	// dataRoot は、データディレクトリ。
	dataRoot string
	library  *library.Library
	userID   uuid.UUID
	// segmentID は、最後に recording で作ったセグメントのファイルの ID。
	segmentID uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
		VALUES ($1, 32736, 1024, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $2, $2)`, serviceID, clk.Now())
	require.NoError(t, err)
	userID := id.New()
	require.NoError(t, sqlc.New(pool).InsertUser(t.Context(), sqlc.InsertUserParams{
		ID: userID, Username: "mikan", DisplayName: "みかん", Role: "member", PasswordHash: nil, CreatedAt: clk.Now(),
	}))
	dir, dataRoot := t.TempDir(), t.TempDir()
	store := kago.New(map[string]string{poolID: dir}, nil).WithDataDir(dataRoot)

	return &fixture{
		pool: pool, clock: clk, dir: dir, dataRoot: dataRoot,
		library: library.New(clk, audit.NewRecorder(clk), store, slog.New(slog.DiscardHandler)), userID: userID, segmentID: uuid.Nil,
	}
}

// recording は、録画と、その録画ファイルと索引を作る。
func (f *fixture) recording(t *testing.T, state string, protected bool) uuid.UUID {
	t.Helper()

	const name = recordingName
	recordingID := id.New()
	now := f.clock.Now()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at,
		state, protected, recorded_from, created_at, updated_at)
		VALUES ($1, NULL, $2, NULL, $3, '', $4, $4, $5, $6, $4, $4, $4)`, recordingID, serviceID, name, now, state, protected)
	require.NoError(t, err)
	path := name + ".m2ts"
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, path), []byte("ts"), 0o600))
	f.segmentID = id.New()
	index := kago.IndexLocation(f.segmentID)
	require.NoError(t, os.Mkdir(filepath.Join(f.dataRoot, f.segmentID.String()), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(f.dataRoot, index.Path), []byte("index"), 0o600))
	// EPGStation から取り込んだ録画には、エンコード済みの MP4 のような、その他のファイルもある。
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, name+".mp4"), []byte("mp4"), 0o600))
	for _, file := range []struct {
		id               uuid.UUID
		kind, pool, path string
	}{{f.segmentID, "segment", poolID, path}, {id.New(), "other", poolID, name + ".mp4"}} {
		_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
			VALUES ($1, $2, $3, $4, $5, 2, $6, $6, $6)`, file.id, recordingID, file.kind, file.pool, file.path, now)
		require.NoError(t, err)
	}
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at)
		VALUES ($1, $2, $3, $3, $4, $4, $4)`, id.New(), serviceID, name, now)
	require.NoError(t, err)

	return recordingID
}

func (f *fixture) actions(t *testing.T, recordingID uuid.UUID) []string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), "SELECT action FROM audit_events WHERE target_type = 'recording' AND target_id = $1 ORDER BY occurred_at, id",
		recordingID.String())
	require.NoError(t, err)
	actions, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)

	return actions
}

// recordingsVersion は、録画の変更の通知の版数を読む。
const recordingsVersion = "SELECT coalesce((SELECT version FROM change_versions WHERE channel = 'recordings'), 0)"

func (f *fixture) count(t *testing.T, query string) int {
	t.Helper()

	var n int
	require.NoError(t, f.pool.QueryRow(t.Context(), query).Scan(&n))

	return n
}

// TestDelete は、録画を消すと、録画ファイルと生成物が消え、録画履歴は残り、監査ログに残ることを確かめる (設計書 4.4、9.7)。
// 保護した録画と、録画中の録画は消さない。
func TestDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		state     string
		protected bool
		missing   bool
		wantErr   error
	}{
		{name: "録画済みの録画", state: completed, protected: false, missing: false, wantErr: nil},
		{name: "失敗した録画", state: "failed", protected: false, missing: false, wantErr: nil},
		{name: "保護した録画", state: completed, protected: true, missing: false, wantErr: library.ErrProtected},
		{name: "録画中の録画", state: "recording", protected: false, missing: false, wantErr: library.ErrRecording},
		{name: "ない録画", state: completed, protected: false, missing: true, wantErr: library.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.recording(t, tt.state, tt.protected)
			target := recordingID
			if tt.missing {
				target = id.New()
			}

			err := f.library.Delete(t.Context(), f.pool, audit.User(f.userID), target, "user")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM recordings"))
				for _, path := range []string{"録画.m2ts", "録画.mp4"} {
					_, err := os.Stat(filepath.Join(f.dir, path))
					require.NoError(t, err, "%s is kept", path)
				}
				assert.Empty(t, f.actions(t, recordingID))

				return
			}
			require.NoError(t, err)
			assert.Zero(t, f.count(t, "SELECT count(*) FROM recordings"))
			assert.Zero(t, f.count(t, "SELECT count(*) FROM recording_files"))
			for _, path := range []string{filepath.Join(f.dir, "録画.m2ts"), filepath.Join(f.dataRoot, f.segmentID.String()), filepath.Join(f.dir, "録画.mp4")} {
				_, err := os.Stat(path)
				require.ErrorIs(t, err, fs.ErrNotExist, path)
			}
			assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM recording_history"), "the history is kept")
			assert.Equal(t, []string{"recording.deleted"}, f.actions(t, recordingID))
			assert.Equal(t, 1, f.count(t, recordingsVersion))
		})
	}
}

// TestSetProtected は、保護の印を付け外しし、変えたときだけ監査ログに残すことを確かめる。
func TestSetProtected(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, completed, false)
	for _, protected := range []bool{true, true, false} {
		require.NoError(t, f.library.SetProtected(t.Context(), f.pool, audit.User(f.userID), recordingID, protected))
		assert.Equal(t, protected, f.count(t, "SELECT count(*) FROM recordings WHERE protected") == 1)
	}
	assert.Equal(t, []string{"recording.protected", "recording.unprotected"}, f.actions(t, recordingID))
	// 変えたときだけ、録画の変更を知らせる (#868)。
	assert.Equal(t, 2, f.count(t, recordingsVersion))
	require.ErrorIs(t, f.library.SetProtected(t.Context(), f.pool, audit.User(f.userID), id.New(), true), library.ErrNotFound)
}

// TestSetTags は、タグを置き換え、名前を整えて、監査ログに残すことを確かめる。
func TestSetTags(t *testing.T) {
	t.Parallel()

	tooMany := make([]string, 21)
	for i := range tooMany {
		tooMany[i] = strings.Repeat("あ", i+1)
	}
	tests := []struct {
		name    string
		tags    []string
		want    []string
		wantErr error
	}{
		{name: "前後の空白を落とし、重なりを除いて、名前の順に並べる", tags: []string{" アニメ ", "ドラマ", "アニメ"}, want: []string{"アニメ", "ドラマ"}, wantErr: nil},
		{name: "空にする", tags: []string{}, want: []string{}, wantErr: nil},
		{name: "空の名前", tags: []string{" "}, want: nil, wantErr: library.ErrInvalidTags},
		{name: "長すぎる名前", tags: []string{strings.Repeat("あ", 51)}, want: nil, wantErr: library.ErrInvalidTags},
		{name: "多すぎるタグ", tags: tooMany, want: nil, wantErr: library.ErrInvalidTags},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			recordingID := f.recording(t, completed, false)
			_, err := f.library.SetTags(t.Context(), f.pool, audit.User(f.userID), recordingID, []string{"古いタグ"})
			require.NoError(t, err)

			got, err := f.library.SetTags(t.Context(), f.pool, audit.User(f.userID), recordingID, tt.tags)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			rows, err := sqlc.New(f.pool).ListRecordingTags(t.Context(), []uuid.UUID{recordingID})
			require.NoError(t, err)
			stored := []string{}
			for _, row := range rows {
				stored = append(stored, row.Name)
			}
			assert.Equal(t, tt.want, stored)
			assert.Equal(t, []string{"recording.tagged", "recording.tagged"}, f.actions(t, recordingID))

			// 同じタグに置き換えても、変わらないので、監査ログに残さない。
			_, err = f.library.SetTags(t.Context(), f.pool, audit.User(f.userID), recordingID, tt.tags)
			require.NoError(t, err)
			assert.Len(t, f.actions(t, recordingID), 2)
		})
	}
}

// TestSaveProgress は、ユーザーごとに再生位置を保存し、上書きすることを確かめる。
func TestSaveProgress(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	recordingID := f.recording(t, completed, false)
	for _, position := range []time.Duration{90 * time.Second, 15*time.Minute + 500*time.Millisecond} {
		require.NoError(t, f.library.SaveProgress(t.Context(), f.pool, f.userID, recordingID, position))
		rows, err := sqlc.New(f.pool).ListWatchProgress(t.Context(), sqlc.ListWatchProgressParams{UserID: f.userID, RecordingIds: []uuid.UUID{recordingID}})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, position.Milliseconds(), rows[0].PositionMs)
	}
	require.ErrorIs(t, f.library.SaveProgress(t.Context(), f.pool, f.userID, id.New(), time.Second), library.ErrNotFound)
	require.ErrorIs(t, f.library.SaveProgress(t.Context(), f.pool, f.userID, recordingID, -time.Second), library.ErrInvalidPosition)
	// 再生位置は個人のものなので、監査ログに残さない。
	assert.Empty(t, f.actions(t, recordingID))
}
