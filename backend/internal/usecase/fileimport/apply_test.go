package fileimport_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/jobs"
	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/fileimport"
)

const (
	poolID = "main"
	// knownServiceID は、テストの TS のサービス (network_id 0x7FE0、service_id 0x0400) の Kotatsu のサービス ID。
	knownServiceID = 3273601024
	// otherServiceID は、同じネットワークの、テストの TS にないサービス。
	otherServiceID = 3273601025
	importDir      = "import"
	// fileA は、取り込むテストの TS の、プールの中のパス。
	fileA = "import/a.ts"
	// countRecordings は、録画の数を数える。
	countRecordings = "SELECT count(*) FROM recordings"
)

type applyFixture struct {
	pool     *pgxpool.Pool
	clock    *clocktest.Fake
	dir      string
	userID   uuid.UUID
	importer *fileimport.Importer
}

// newApplyFixture は、サービス serviceIDs を持つ Kotatsu と、空のプールを作る。
func newApplyFixture(t *testing.T, serviceIDs ...int64) *applyFixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	require.NoError(t, jobs.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	_, err = pool.Exec(t.Context(), "INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 10)")
	require.NoError(t, err)
	for _, serviceID := range serviceIDs {
		_, err = pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
			VALUES ($1, $2, $3, 'こたつテスト1', 'こたつテスト1', 1, 'GR', '27', $4, $4)`, serviceID, serviceID/100000, serviceID%100000, clk.Now())
		require.NoError(t, err)
	}
	userID := id.New()
	require.NoError(t, sqlc.New(pool).InsertUser(t.Context(), sqlc.InsertUserParams{
		ID: userID, Username: "mikan", DisplayName: "みかん", Role: "admin", PasswordHash: nil, CreatedAt: clk.Now(),
	}))
	inserter, err := jobs.NewClient(pool, slog.New(slog.DiscardHandler), &jobs.Options{PollOnly: false, Workers: nil, PeriodicJobs: nil})
	require.NoError(t, err)
	dir := t.TempDir()
	store := kago.New(map[string]string{poolID: dir}, nil).WithDataDir(t.TempDir())

	return &applyFixture{
		pool: pool, clock: clk, dir: dir, userID: userID,
		importer: fileimport.NewImporter(clk, audit.NewRecorder(clk), inserter, store),
	}
}

// write は、プールの rel にファイルを書き、更新日時を今の age 前にする。
func (f *applyFixture) write(t *testing.T, rel string, data []byte, age time.Duration) {
	t.Helper()

	path := filepath.Join(f.dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o600))
	modified := f.clock.Now().Add(-age)
	require.NoError(t, os.Chtimes(path, modified, modified))
}

func (f *applyFixture) apply(t *testing.T, dryRun bool) *fileimport.Result {
	t.Helper()

	userID := f.userID
	if dryRun {
		userID = uuid.Nil()
	}
	result, err := f.importer.Apply(t.Context(), f.pool, &fileimport.Options{
		PoolID: poolID, Dir: importDir, UserID: userID, MinAge: fileimport.DefaultMinAge, DryRun: dryRun,
	})
	require.NoError(t, err)

	return result
}

func (f *applyFixture) count(t *testing.T, query string) int {
	t.Helper()

	var n int
	require.NoError(t, f.pool.QueryRow(t.Context(), query).Scan(&n))

	return n
}

// TestApply は、プールのディレクトリの TS のうち、Kotatsu にあるサービスのものを取り込み、取り込めないものは理由を付けて飛ばすことを確認する (#1734)。
func TestApply(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	tests := []struct {
		name     string
		services []int64
		// files は、プールの中のパスから、中身と、最後に書いてからの時間への対応。
		files        map[string]time.Duration
		bdav         []string
		wantImported []string
		wantSkipped  map[string]string
	}{
		{
			name:     "Kotatsu にあるサービスの TS",
			services: []int64{knownServiceID},
			files: map[string]time.Duration{
				fileA: time.Hour, "import/sub/b.M2TS": time.Hour,
				// ディレクトリの外と、TS でない拡張子のファイルは見ない。
				"other/c.ts": time.Hour, "import/d.mp4": time.Hour,
			},
			bdav:         nil,
			wantImported: []string{fileA, "import/sub/b.M2TS"},
			wantSkipped:  map[string]string{},
		},
		{
			name:         "Kotatsu にないサービスの TS",
			services:     []int64{otherServiceID},
			files:        map[string]time.Duration{fileA: time.Hour},
			bdav:         nil,
			wantImported: []string{},
			wantSkipped:  map[string]string{fileA: `the service is not in Kotatsu (network 32736, service 1024 "こたつテスト１")`},
		},
		{
			name:         "192 バイトの BDAV",
			services:     []int64{knownServiceID},
			files:        map[string]time.Duration{},
			bdav:         []string{"import/bd.m2ts"},
			wantImported: []string{},
			wantSkipped:  map[string]string{"import/bd.m2ts": fileimport.ErrBDAV.Error()},
		},
		{
			name:         "書いている途中かもしれない TS",
			services:     []int64{knownServiceID},
			files:        map[string]time.Duration{fileA: time.Minute},
			bdav:         nil,
			wantImported: []string{},
			wantSkipped:  map[string]string{fileA: "modified within the last 10m0s; it may still be being written"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newApplyFixture(t, tt.services...)
			for rel, age := range tt.files {
				f.write(t, rel, data, age)
			}
			for _, rel := range tt.bdav {
				f.write(t, rel, bdav(data), time.Hour)
			}

			result := f.apply(t, false)
			imported := []string{}
			for _, file := range result.Imported {
				imported = append(imported, file.Path)
			}
			assert.ElementsMatch(t, tt.wantImported, imported)
			skipped := map[string]string{}
			for _, s := range result.Skipped {
				skipped[s.Path] = s.Reason
			}
			assert.Equal(t, tt.wantSkipped, skipped)
			assert.Equal(t, len(tt.wantImported), f.count(t, countRecordings))
		})
	}
}

// TestApplyRecording は、取り込んだ録画に、TS から読んだ番組と録った時刻を記録し、ファイルを Kotatsu が書いていないものとして記録し、
// 解析するジョブを投入して、監査ログに残すことを確認する (#1734)。
func TestApplyRecording(t *testing.T) {
	t.Parallel()

	f := newApplyFixture(t, knownServiceID)
	f.write(t, fileA, loadTestTS(t), time.Hour)
	result := f.apply(t, false)
	require.Len(t, result.Imported, 1)

	var (
		recordingID                  uuid.UUID
		serviceID                    int64
		eventID                      *int64
		name, description, state     string
		startAt, endAt, recordedFrom time.Time
		recordedUntil                *time.Time
		path                         string
		external                     bool
		kind                         string
	)
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT r.id, r.service_id, r.event_id, r.name, r.description, r.state, r.start_at, r.end_at,
		r.recorded_from, r.recorded_until, f.path, f.external, f.kind
		FROM recordings r JOIN recording_files f ON f.recording_id = r.id`).Scan(
		&recordingID, &serviceID, &eventID, &name, &description, &state, &startAt, &endAt, &recordedFrom, &recordedUntil, &path, &external, &kind))
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, jst())
	assert.Equal(t, int64(knownServiceID), serviceID)
	assert.Equal(t, new(int64(1)), eventID)
	assert.Equal(t, "テスト番組", name)
	assert.Equal(t, "合成した映像と音声です。", description)
	assert.Equal(t, "completed", state)
	assert.True(t, start.Equal(startAt), startAt)
	assert.True(t, start.Add(30*time.Minute).Equal(endAt), endAt)
	assert.True(t, start.Equal(recordedFrom), recordedFrom)
	require.NotNil(t, recordedUntil)
	assert.True(t, start.Equal(*recordedUntil), *recordedUntil)
	assert.Equal(t, fileA, path)
	assert.True(t, external)
	assert.Equal(t, "segment", kind)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM river_job WHERE kind = 'recording_analyze'"))
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM import_sources WHERE source = 'files' AND kind = 'recorded'"))
	var action string
	var actorUserID uuid.UUID
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT action, actor_user_id FROM audit_events WHERE target_id = $1", recordingID.String()).Scan(&action, &actorUserID))
	assert.Equal(t, "recording.imported", action)
	assert.Equal(t, f.userID, actorUserID)
}

// TestApplyIdempotent は、同じファイルを二重に取り込まず、Kotatsu が録った録画のファイルも取り込まないことを確認する (#1734)。
// 取り込んだ録画を消しても、取り込んだ記録が残るので、取り込み直さない。
func TestApplyIdempotent(t *testing.T) {
	t.Parallel()

	f := newApplyFixture(t, knownServiceID)
	data := loadTestTS(t)
	f.write(t, fileA, data, time.Hour)
	f.write(t, "import/recorded.m2ts", data, time.Hour)
	recordingID := id.New()
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, '録った録画', '', $3, $3, 'completed', $3, $3, $3)`, recordingID, knownServiceID, f.clock.Now())
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, created_at)
		VALUES ($1, $2, 'segment', $3, 'import/recorded.m2ts', 1, $4, $4)`, id.New(), recordingID, poolID, f.clock.Now())
	require.NoError(t, err)

	first := f.apply(t, false)
	assert.Len(t, first.Imported, 1)
	assert.Equal(t, 1, first.AlreadyRecorded)

	second := f.apply(t, false)
	assert.Empty(t, second.Imported)
	assert.Equal(t, 1, second.AlreadyImported)
	assert.Equal(t, 1, second.AlreadyRecorded)
	assert.Equal(t, 2, f.count(t, countRecordings))

	_, err = f.pool.Exec(t.Context(), "DELETE FROM recordings WHERE id <> $1", recordingID)
	require.NoError(t, err)
	third := f.apply(t, false)
	assert.Empty(t, third.Imported)
	assert.Equal(t, 1, third.AlreadyImported)
}

// TestApplyDryRun は、dry run では取り込む計画を返し、DB に何も書かないことを確認する (#1734)。
func TestApplyDryRun(t *testing.T) {
	t.Parallel()

	f := newApplyFixture(t, knownServiceID)
	f.write(t, fileA, loadTestTS(t), time.Hour)

	result := f.apply(t, true)
	require.Len(t, result.Imported, 1)
	assert.Equal(t, "テスト番組", result.Imported[0].Name)
	for _, query := range []string{
		countRecordings, "SELECT count(*) FROM recording_files", "SELECT count(*) FROM import_sources",
		"SELECT count(*) FROM river_job", "SELECT count(*) FROM audit_events",
	} {
		assert.Zero(t, f.count(t, query), query)
	}

	// 取り込めば、dry run の計画どおりに取り込む。
	assert.Len(t, f.apply(t, false).Imported, 1)
}

// TestApplyInvalidDirectory は、プールの外を指すディレクトリを拒むことを確認する (#1734)。
func TestApplyInvalidDirectory(t *testing.T) {
	t.Parallel()

	f := newApplyFixture(t, knownServiceID)
	tests := []struct {
		name string
		dir  string
	}{
		{name: "親のディレクトリ", dir: "../outside"},
		{name: "絶対パス", dir: "/etc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := f.importer.Apply(t.Context(), f.pool, &fileimport.Options{
				PoolID: poolID, Dir: tt.dir, UserID: f.userID, MinAge: fileimport.DefaultMinAge, DryRun: true,
			})
			require.ErrorIs(t, err, fileimport.ErrInvalidDirectory)
		})
	}
}
