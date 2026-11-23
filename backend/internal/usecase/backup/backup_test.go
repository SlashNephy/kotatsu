package backup_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"filippo.io/age"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/backup"
	"github.com/SlashNephy/kotatsu/internal/usecase/epg"
)

const testSecretKey = "0123456789abcdef0123456789abcdef-backup-test"

func testNow() time.Time {
	return time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC)
}

// newEmptyDB は、表のない空の DB を作る。
func newEmptyDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}

// newDB は、最新までマイグレーションを当てた DB を作る。
func newDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool := newEmptyDB(t)
	logger := slog.New(slog.DiscardHandler)
	require.NoError(t, database.Migrate(t.Context(), logger, pool))
	require.NoError(t, jobs.Migrate(t.Context(), logger, pool))

	return pool
}

func newIdentity(t *testing.T) *age.X25519Identity {
	t.Helper()

	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	return identity
}

func writeOptions(pool *pgxpool.Pool, store *kago.Store) *backup.WriteOptions {
	return &backup.WriteOptions{
		DB: pool, Store: store, SecretKey: testSecretKey, Clock: clocktest.NewFake(testNow()), Logger: slog.New(slog.DiscardHandler),
	}
}

func restoreOptions(pool *pgxpool.Pool, store *kago.Store) *backup.RestoreOptions {
	return &backup.RestoreOptions{
		DB: pool, Store: store, Logger: slog.New(slog.DiscardHandler),
		CheckSecretKey: func(secretKey string) error {
			if secretKey != testSecretKey {
				return errors.New("unexpected secret key")
			}

			return nil
		},
	}
}

// listTables は、今のスキーマの表の名前を返す。
func listTables(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()

	rows, err := pool.Query(t.Context(), "SELECT tablename FROM pg_tables WHERE schemaname = current_schema() ORDER BY tablename")
	require.NoError(t, err)
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)

	return names
}

// TestTablesAreClassified は、マイグレーションを当てた DB のどの表も、バックアップに入れるか外すかを決めてあり、
// 書き出しが止まらないことを確かめる。表を足して分け忘れると、書き出しが ErrUnclassifiedTable で止まる。
func TestTablesAreClassified(t *testing.T) {
	t.Parallel()

	pool := newDB(t)
	var buf bytes.Buffer
	result, err := backup.Write(t.Context(), writeOptions(pool, kago.New(nil, nil)), &buf, []age.Recipient{newIdentity(t).Recipient()})
	require.NoError(t, err)

	written := make([]string, 0, len(result.Manifest.Tables))
	for _, table := range result.Manifest.Tables {
		written = append(written, table.Name)
	}
	excluded := []string{}
	for _, name := range listTables(t, pool) {
		if !slices.Contains(written, name) {
			excluded = append(excluded, name)
		}
	}
	// 外すのは、番組表とその変更の記録、セッションとそれを指す表、River の表とジョブの進み具合、マイグレーションの表だけ。
	for _, name := range excluded {
		if strings.HasPrefix(name, "river_") {
			continue
		}
		assert.Contains(t, []string{
			"programs", "epg_changes", "change_versions", "anime_match_cursor", "drama_match_cursor",
			"sessions", "players", "player_commands", "oidc_logins", "anime_record_authorizations", "drama_record_authorizations", "google_authorizations",
			"job_progress", "goose_db_version", "goose_lock", "schema_scripts",
		}, name)
	}
	assert.Contains(t, written, "program_archives")
	assert.Contains(t, written, "recordings")
	assert.Equal(t, database.SchemaVersion(), result.Manifest.SchemaVersion)
}

// seed は、戻すことを確かめる行を入れる。
type seed struct {
	userID, captureID, recordingID uuid.UUID
	image                          []byte
}

func insertSeed(t *testing.T, pool *pgxpool.Pool, dataDir string) *seed {
	t.Helper()

	s := &seed{userID: uuid.New(), captureID: uuid.New(), recordingID: uuid.New(), image: []byte("\x89PNG\r\n\x1a\nbackup-test-image")}
	ruleID, reservationID, sessionID := uuid.New(), uuid.New(), uuid.New()
	ctx := t.Context()
	statements := []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO users (id, username, display_name, role, password_hash, created_at, updated_at) VALUES ($1, 'admin', 'Admin', 'admin', 'hash', $2, $2)", []any{s.userID, testNow()}},
		{"INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上', 0)", nil},
		{"INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at) VALUES (3273601024, 32736, 1024, 'NHK', 'nhk', 1, 'GR', '27', $1, $1)", []any{testNow()}},
		{"INSERT INTO system_settings (id, recording_defaults, archive_retention_days, mcp_enabled, updated_at) VALUES (1, '{}', 30, true, $1)", []any{testNow()}},
		{"INSERT INTO rules (id, name, memo, enabled, conditions, settings, avoid_duplicates, follow_event_relay, created_at, updated_at) VALUES ($1, 'ニュース', '', $2, '{}', '{}', $2, $2, $3, $3)", []any{ruleID, true, testNow()}},
		{`INSERT INTO reservations (id, kind, service_id, event_id, start_at, end_at, name, description, extended, genres, settings, created_at, updated_at)
VALUES ($1, 'rule', 3273601024, 100, $2, $3, 'ニュース', '', '{}', '[]', '{}', $4, $4)`, []any{reservationID, testNow().Add(time.Hour), testNow().Add(2 * time.Hour), testNow()}},
		{"INSERT INTO reservation_rules (reservation_id, rule_id) VALUES ($1, $2)", []any{reservationID, ruleID}},
		{`INSERT INTO recordings (id, api_id, service_id, event_id, name, description, name_normalized, description_normalized, start_at, end_at, state,
recorded_from, created_at, updated_at)
VALUES ($1, 41, 3273601024, 99, 'きのうのニュース', '', 'きのうのニュース', '', $2, $3, 'completed', $2, $2, $2)`, []any{s.recordingID, testNow().Add(-2 * time.Hour), testNow().Add(-time.Hour)}},
		{`INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, created_at)
VALUES ($1, $2, 'segment', 'main', 'news.m2ts', 1024, $3, $3)`, []any{uuid.New(), s.recordingID, testNow().Add(-2 * time.Hour)}},
		{"INSERT INTO watch_progress (user_id, recording_id, position_ms, updated_at) VALUES ($1, $2, 60000, $3)", []any{s.userID, s.recordingID, testNow()}},
		{`INSERT INTO captures (id, user_id, recording_id, position_ms, service_name, service_name_normalized, program_name_normalized, caption_normalized, broadcast_at, taken_at, size, created_at)
VALUES ($1, $2, $3, 1000, 'NHK', 'nhk', '', '', $4, $4, $5, $4)`, []any{s.captureID, s.userID, s.recordingID, testNow(), len(s.image)}},
		// 外す表とそれを指す行。
		{`INSERT INTO sessions (id, token_hash, user_id, created_at, authenticated_at, last_seen_at, expires_at, user_agent)
VALUES ($1, '\x01', $2, $3, $3, $3, $4, 'test')`, []any{sessionID, s.userID, testNow(), testNow().Add(24 * time.Hour)}},
		{`INSERT INTO web_push_subscriptions (id, user_id, endpoint, p256dh, auth, user_agent, created_at, updated_at, session_id)
VALUES ($1, $2, 'https://push.example/1', '\x02', '\x03', 'test', $3, $3, $4)`, []any{uuid.New(), s.userID, testNow(), sessionID}},
		{`INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free, name, description, name_normalized,
description_normalized, extended, genres, related_items, created_at, updated_at)
VALUES (3273601024, 100, 1, $1, $2, false, true, 'ニュース', '', 'ニュース', '', '{}', '[]', '[]', $3, $3)`, []any{testNow().Add(time.Hour), testNow().Add(2 * time.Hour), testNow()}},
		// COPY の出力を区切る大きさ (4 MiB) より大きくし、1 つの表を複数のエントリーに分けて書き、つないで戻すことを確かめる。
		// 暗号化の 1 つの塊 (64 KiB) よりも大きいので、途中で切れたファイルは、流し込みの途中で読めなくなる。
		{`INSERT INTO program_rewrites (service_id, event_id, start_at, field, before, after, observed_at)
SELECT 3273601024, n, $1, 'name', md5(n::text || 'before'), md5(n::text || 'after'), $1 FROM generate_series(1, 60000) AS n`, []any{testNow()}},
		{"INSERT INTO anime_match_cursor (id, epg_version, updated_at) VALUES (1, 500, $1)", []any{testNow()}},
		{"INSERT INTO epg_changes (version, service_id, start_at, end_at, created_at) VALUES (500, 3273601024, $1, $2, $1)", []any{testNow(), testNow().Add(time.Hour)}},
	}
	for _, statement := range statements {
		_, err := pool.Exec(ctx, statement.sql, statement.args...)
		require.NoError(t, err, statement.sql)
	}
	for range 500 {
		_, err := notify.Publish(ctx, pool, clocktest.NewFake(testNow()), epg.NotifyChannel)
		require.NoError(t, err)
	}

	location := kago.CaptureLocation(s.userID, s.captureID)
	require.NoError(t, kago.New(nil, nil).WithDataDir(dataDir).WriteFile(location.PoolID, location.Path, s.image))

	return s
}

// tableContents は、表のすべての行を JSON の文字列にし、並べ替えて返す。表どうしを比べるのに使う。
func tableContents(t *testing.T, pool *pgxpool.Pool, table string) []string {
	t.Helper()

	rows, err := pool.Query(t.Context(), "SELECT to_jsonb(t)::text FROM "+pgx.Identifier{table}.Sanitize()+" t")
	require.NoError(t, err)
	values, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	slices.Sort(values)

	return values
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()

	var n int
	require.NoError(t, pool.QueryRow(t.Context(), "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&n))

	return n
}

// TestRestoreRoundTrip は、書き出したバックアップを空の DB に戻すと、予約、ルール、録画、視聴の記録、設定、キャプチャが戻り、
// 外した表が空で作り直され、戻した DB でロールが起動できる (スキーマの確かめを通る) ことを確かめる。
func TestRestoreRoundTrip(t *testing.T) {
	t.Parallel()

	source := newDB(t)
	sourceDir := t.TempDir()
	s := insertSeed(t, source, sourceDir)
	identity := newIdentity(t)

	var buf bytes.Buffer
	result, err := backup.Write(t.Context(), writeOptions(source, kago.New(nil, nil).WithDataDir(sourceDir)), &buf, []age.Recipient{identity.Recipient()})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Manifest.Captures)
	assert.Zero(t, result.MissingCaptures)
	assert.Equal(t, int64(buf.Len()), result.Bytes)
	// 暗号化しているので、行の中身がそのまま現れない。
	assert.NotContains(t, buf.String(), "きのうのニュース")
	assert.NotContains(t, buf.String(), testSecretKey)

	target := newEmptyDB(t)
	targetDir := t.TempDir()
	manifest, err := backup.Restore(t.Context(), restoreOptions(target, kago.New(nil, nil).WithDataDir(targetDir)), bytes.NewReader(buf.Bytes()), []age.Identity{identity})
	require.NoError(t, err)
	assert.Equal(t, result.Manifest.SchemaVersion, manifest.SchemaVersion)

	// 戻した DB で、ロールが起動できる。
	require.NoError(t, database.CheckSchema(t.Context(), target))
	require.NoError(t, jobs.CheckSchema(t.Context(), target))

	// 入れた表は、行がそのまま戻る。Web Push の購読は、セッションとの結びつきだけを外す。
	for _, table := range manifest.Tables {
		switch table.Name {
		case "web_push_subscriptions", "schema_compatibility":
			continue
		}
		assert.Equal(t, tableContents(t, source, table.Name), tableContents(t, target, table.Name), table.Name)
	}
	var sessionID *uuid.UUID
	require.NoError(t, target.QueryRow(t.Context(), "SELECT session_id FROM web_push_subscriptions").Scan(&sessionID))
	assert.Nil(t, sessionID)
	for _, table := range []string{"users", "rules", "reservations", "reservation_rules", "recordings", "recording_files", "watch_progress", "system_settings", "captures"} {
		assert.Equal(t, 1, countRows(t, target, table), table)
	}

	// 外した表は、空で作り直される。
	for _, table := range []string{"sessions", "programs", "epg_changes", "change_versions", "anime_match_cursor"} {
		assert.Zero(t, countRows(t, target, table), table)
	}

	// キャプチャの画像は、戻す先のデータディレクトリに書く。
	location := kago.CaptureLocation(s.userID, s.captureID)
	image, err := os.ReadFile(filepath.Join(targetDir, filepath.FromSlash(location.Path)))
	require.NoError(t, err)
	assert.Equal(t, s.image, image)

	// 録画の API の ID は、戻した値の次から振る。
	var apiID int64
	require.NoError(t, target.QueryRow(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
VALUES ($1, 3273601024, 'new', '', $2, $3, 'completed', $2, $2, $2) RETURNING api_id`, uuid.New(), testNow(), testNow().Add(time.Minute)).Scan(&apiID))
	assert.Equal(t, int64(42), apiID)

	// 変更の通知の版数は 1 から数え直し、見張る側は、戻した後の最初の変更に気づく。
	watcher, err := notify.NewWatcher(t.Context(), target, slog.New(slog.DiscardHandler), []string{epg.NotifyChannel}, notify.WatcherOptions{UseListen: false, PollInterval: 10 * time.Millisecond})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() { _ = watcher.Run(ctx) }()
	version, err := notify.Publish(t.Context(), target, clocktest.NewFake(testNow()), epg.NotifyChannel)
	require.NoError(t, err)
	assert.Equal(t, int64(1), version)
	select {
	case <-watcher.Changes():
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher did not notice the first change after the restore")
	}
	assert.Equal(t, int64(1), watcher.Version(epg.NotifyChannel))
}

// TestRestoreRejects は、戻せないバックアップや戻す先を、DB を変えずに拒むことを確かめる。
func TestRestoreRejects(t *testing.T) {
	t.Parallel()

	source := newDB(t)
	sourceDir := t.TempDir()
	insertSeed(t, source, sourceDir)
	identity := newIdentity(t)
	var buf bytes.Buffer
	_, err := backup.Write(t.Context(), writeOptions(source, kago.New(nil, nil).WithDataDir(sourceDir)), &buf, []age.Recipient{identity.Recipient()})
	require.NoError(t, err)
	data := buf.Bytes()

	tests := []struct {
		name       string
		data       []byte
		identity   age.Identity
		target     func(t *testing.T) *pgxpool.Pool
		secretKey  string
		wantErr    func(t *testing.T, err error)
		wantTables bool
	}{
		{
			name: "another private key cannot decrypt the backup", data: data, identity: newIdentity(t),
			target: newEmptyDB, secretKey: testSecretKey, wantErr: func(t *testing.T, err error) {
				t.Helper()
				var noMatch *age.NoIdentityMatchError
				require.ErrorAs(t, err, &noMatch)
			}, wantTables: false,
		},
		{
			name: "the database is not empty", data: data, identity: identity,
			target: newDB, secretKey: testSecretKey, wantErr: func(t *testing.T, err error) {
				t.Helper()
				require.ErrorIs(t, err, backup.ErrDatabaseNotEmpty)
			}, wantTables: true,
		},
		{
			name: "the secret key is rejected", data: data, identity: identity,
			target: newEmptyDB, secretKey: "another", wantErr: func(t *testing.T, err error) { t.Helper(); require.Error(t, err) }, wantTables: false,
		},
		{
			name: "a truncated backup is not committed", data: data[:len(data)*3/4], identity: identity,
			target: newEmptyDB, secretKey: testSecretKey, wantErr: func(t *testing.T, err error) { t.Helper(); require.Error(t, err) }, wantTables: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			target := tt.target(t)
			opts := restoreOptions(target, kago.New(nil, nil).WithDataDir(t.TempDir()))
			opts.CheckSecretKey = func(secretKey string) error {
				if secretKey != tt.secretKey {
					return errors.New("unexpected secret key")
				}

				return nil
			}
			_, err := backup.Restore(t.Context(), opts, bytes.NewReader(tt.data), []age.Identity{tt.identity})
			tt.wantErr(t, err)
			// 拒んだら、行を流し込まない。途中で切れたファイルは、マイグレーションの後の流し込みを確定させない。
			if tt.wantTables {
				assert.Zero(t, countRows(t, target, "users"))
			} else {
				assert.Empty(t, slices.DeleteFunc(listTables(t, target), func(name string) bool { return name == "goose_lock" }))
			}
		})
	}
}

// TestWriteCaptures は、キャプチャの行があるのにデータディレクトリがなければ書き出さず、画像だけがなければ、行だけを書き出して数えることを確かめる。
func TestWriteCaptures(t *testing.T) {
	t.Parallel()

	pool := newDB(t)
	dataDir := t.TempDir()
	s := insertSeed(t, pool, dataDir)
	recipients := []age.Recipient{newIdentity(t).Recipient()}

	_, err := backup.Write(t.Context(), writeOptions(pool, kago.New(nil, nil)), new(bytes.Buffer), recipients)
	require.ErrorIs(t, err, backup.ErrNoDataDir)

	location := kago.CaptureLocation(s.userID, s.captureID)
	require.NoError(t, os.Remove(filepath.Join(dataDir, filepath.FromSlash(location.Path))))
	result, err := backup.Write(t.Context(), writeOptions(pool, kago.New(nil, nil).WithDataDir(dataDir)), new(bytes.Buffer), recipients)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Manifest.Captures)
	assert.Equal(t, 1, result.MissingCaptures)
}
