package settings_test

import (
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
}

func newDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))

	return pool
}

func newStore() *settings.Store {
	clk := clocktest.NewFake(testNow())

	return settings.NewStore(clk, audit.NewRecorder(clk))
}

// TestLoadDefaults は、一度も変えていなければ、既定値を返すことを確かめる。
func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	got, err := settings.Load(t.Context(), newDB(t))
	require.NoError(t, err)
	assert.Equal(t, reserve.DefaultSettings(), got.Recording)
	assert.Equal(t, settings.DefaultArchiveRetentionDays, got.ArchiveRetentionDays)
	assert.Equal(t, 90*24*time.Hour, got.ArchiveRetention())
	assert.Nil(t, got.UpdatedAt)
}

// TestUpdate は、変えた項目だけを、既定値や前の変更に重ねることを確かめる。
func TestUpdate(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	store := newStore()
	actor := audit.System("test")

	margin, priority := 30, 3
	_, err := store.Update(t.Context(), db, actor, &settings.Changes{
		Recording:            &reserve.Overrides{PoolID: nil, FileNameTemplate: nil, StartMarginSeconds: &margin, EndMarginSeconds: nil, Priority: nil},
		ArchiveRetentionDays: nil,
	})
	require.NoError(t, err)
	zero := 0
	updated, err := store.Update(t.Context(), db, actor, &settings.Changes{
		Recording:            &reserve.Overrides{PoolID: nil, FileNameTemplate: nil, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: &priority},
		ArchiveRetentionDays: &zero,
	})
	require.NoError(t, err)

	loaded, err := settings.Load(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, updated, loaded)
	want := reserve.DefaultSettings()
	want.StartMarginSeconds, want.Priority = margin, priority
	assert.Equal(t, want, loaded.Recording, "the earlier change is kept and the rest stays at the defaults")
	assert.Equal(t, 0, loaded.ArchiveRetentionDays)
	assert.Negative(t, loaded.ArchiveRetention(), "0 days keeps archives forever")
	require.NotNil(t, loaded.UpdatedAt)
	assert.True(t, loaded.UpdatedAt.Equal(testNow()))

	// ファイル名のテンプレートとプールも、ほかの項目を保ったまま変えられる。プールは空にすると、自動で選ぶのに戻る。
	template, pool, empty := `{{.Name}}`, "main", ""
	_, err = store.Update(t.Context(), db, actor, &settings.Changes{
		Recording:            &reserve.Overrides{PoolID: &pool, FileNameTemplate: &template, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: nil},
		ArchiveRetentionDays: nil,
	})
	require.NoError(t, err)
	loaded, err = settings.Load(t.Context(), db)
	require.NoError(t, err)
	want.PoolID, want.FileNameTemplate = pool, template
	assert.Equal(t, want, loaded.Recording)
	_, err = store.Update(t.Context(), db, actor, &settings.Changes{
		Recording:            &reserve.Overrides{PoolID: &empty, FileNameTemplate: nil, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: nil},
		ArchiveRetentionDays: nil,
	})
	require.NoError(t, err)
	loaded, err = settings.Load(t.Context(), db)
	require.NoError(t, err)
	want.PoolID = ""
	assert.Equal(t, want, loaded.Recording)

	var audited int
	require.NoError(t, db.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE action = 'settings.updated'").Scan(&audited))
	assert.Equal(t, 4, audited)
}

// TestUpdateRejectsInvalidSettings は、正しくない値を保存しないことを確かめる。
func TestUpdateRejectsInvalidSettings(t *testing.T) {
	t.Parallel()

	negative, tooLong, badTemplate := -1, 3651, "{{.Unknown}}"
	tests := []struct {
		name    string
		changes *settings.Changes
		wantErr error
	}{
		{name: "nil", changes: nil, wantErr: settings.ErrInvalidSettings},
		{name: "negative retention", changes: &settings.Changes{Recording: nil, ArchiveRetentionDays: &negative}, wantErr: settings.ErrInvalidSettings},
		{name: "too long retention", changes: &settings.Changes{Recording: nil, ArchiveRetentionDays: &tooLong}, wantErr: settings.ErrInvalidSettings},
		{
			name: "negative margin", wantErr: reserve.ErrInvalidSettings,
			changes: &settings.Changes{
				Recording:            &reserve.Overrides{PoolID: nil, FileNameTemplate: nil, StartMarginSeconds: &negative, EndMarginSeconds: nil, Priority: nil},
				ArchiveRetentionDays: nil,
			},
		},
		{
			name: "unknown template field", wantErr: reserve.ErrInvalidSettings,
			changes: &settings.Changes{
				Recording:            &reserve.Overrides{PoolID: nil, FileNameTemplate: &badTemplate, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: nil},
				ArchiveRetentionDays: nil,
			},
		},
	}
	db := newDB(t)
	for _, tt := range tests {
		_, err := newStore().Update(t.Context(), db, audit.System("test"), tt.changes)
		require.ErrorIs(t, err, tt.wantErr, tt.name)
	}
	loaded, err := settings.Load(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, reserve.DefaultSettings(), loaded.Recording)
	assert.Equal(t, settings.DefaultArchiveRetentionDays, loaded.ArchiveRetentionDays)
}

// TestUpdateConcurrently は、違う項目を同時に変えても、どちらの変更も失わないことを確かめる。
func TestUpdateConcurrently(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	store := newStore()
	start, end := 20, 40
	var wg sync.WaitGroup
	for _, change := range []*reserve.Overrides{
		{PoolID: nil, FileNameTemplate: nil, StartMarginSeconds: &start, EndMarginSeconds: nil, Priority: nil},
		{PoolID: nil, FileNameTemplate: nil, StartMarginSeconds: nil, EndMarginSeconds: &end, Priority: nil},
	} {
		wg.Go(func() {
			_, err := store.Update(t.Context(), db, audit.System("test"), &settings.Changes{Recording: change, ArchiveRetentionDays: nil})
			assert.NoError(t, err)
		})
	}
	wg.Wait()

	loaded, err := settings.Load(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, start, loaded.Recording.StartMarginSeconds)
	assert.Equal(t, end, loaded.Recording.EndMarginSeconds)
}

// TestUpdateRuleDefaults は、ルールを作るときの既定値を、変えた項目だけ重ねて保存することを確かめる (#470)。
func TestUpdateRuleDefaults(t *testing.T) {
	t.Parallel()

	days, zero, tooLong := 30, 0, 3651
	tests := []struct {
		name    string
		changes []*settings.RuleChanges
		want    *settings.RuleDefaults
		wantErr bool
	}{
		{
			name:    "変えていなければ、イベントリレーへの追従だけが有効",
			changes: nil,
			want:    &settings.RuleDefaults{AvoidDuplicates: false, DuplicateWindowDays: nil, FollowEventRelay: true},
		},
		{
			name: "変えた項目だけを重ねる",
			changes: []*settings.RuleChanges{
				{AvoidDuplicates: new(true), DuplicateWindowDays: &days, ClearDuplicateWindowDays: false, FollowEventRelay: nil},
				{AvoidDuplicates: nil, DuplicateWindowDays: nil, ClearDuplicateWindowDays: false, FollowEventRelay: new(false)},
			},
			want: &settings.RuleDefaults{AvoidDuplicates: true, DuplicateWindowDays: &days, FollowEventRelay: false},
		},
		{
			name: "期間を区切らないように戻せる",
			changes: []*settings.RuleChanges{
				{AvoidDuplicates: new(true), DuplicateWindowDays: &days, ClearDuplicateWindowDays: false, FollowEventRelay: nil},
				{AvoidDuplicates: nil, DuplicateWindowDays: nil, ClearDuplicateWindowDays: true, FollowEventRelay: nil},
			},
			want: &settings.RuleDefaults{AvoidDuplicates: true, DuplicateWindowDays: nil, FollowEventRelay: true},
		},
		{
			name:    "期間は 1 日から",
			changes: []*settings.RuleChanges{{AvoidDuplicates: nil, DuplicateWindowDays: &zero, ClearDuplicateWindowDays: false, FollowEventRelay: nil}},
			wantErr: true,
		},
		{
			name:    "期間は 3650 日まで",
			changes: []*settings.RuleChanges{{AvoidDuplicates: nil, DuplicateWindowDays: &tooLong, ClearDuplicateWindowDays: false, FollowEventRelay: nil}},
			wantErr: true,
		},
		{
			name:    "期間を入れながら区切らないようにはできない",
			changes: []*settings.RuleChanges{{AvoidDuplicates: nil, DuplicateWindowDays: &days, ClearDuplicateWindowDays: true, FollowEventRelay: nil}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := newDB(t)
			store := newStore()
			for _, changes := range tt.changes {
				_, err := store.Update(t.Context(), db, audit.System("test"), &settings.Changes{
					Recording: nil, ArchiveRetentionDays: nil, NotificationRetentionDays: nil, Rule: changes,
				})
				if tt.wantErr {
					require.ErrorIs(t, err, settings.ErrInvalidSettings)

					return
				}
				require.NoError(t, err)
			}
			loaded, err := settings.Load(t.Context(), db)
			require.NoError(t, err)
			assert.Equal(t, tt.want, loaded.Rule)
		})
	}
}
