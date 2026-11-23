package settings_test

import (
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
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
	assert.Equal(t, domain.DefaultRecordingSettings(), got.Recording)
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
		Recording:            &domain.RecordingOverrides{PoolID: nil, FileNameTemplate: nil, StartMarginSeconds: &margin, EndMarginSeconds: nil, Priority: nil},
		ArchiveRetentionDays: nil,
	})
	require.NoError(t, err)
	zero := 0
	updated, err := store.Update(t.Context(), db, actor, &settings.Changes{
		Recording:            &domain.RecordingOverrides{PoolID: nil, FileNameTemplate: nil, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: &priority},
		ArchiveRetentionDays: &zero,
	})
	require.NoError(t, err)

	loaded, err := settings.Load(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, updated, loaded)
	want := domain.DefaultRecordingSettings()
	want.StartMarginSeconds, want.Priority = margin, priority
	assert.Equal(t, want, loaded.Recording, "the earlier change is kept and the rest stays at the defaults")
	assert.Equal(t, 0, loaded.ArchiveRetentionDays)
	assert.Negative(t, loaded.ArchiveRetention(), "0 days keeps archives forever")
	require.NotNil(t, loaded.UpdatedAt)
	assert.True(t, loaded.UpdatedAt.Equal(testNow()))

	// ファイル名のテンプレートとプールも、ほかの項目を保ったまま変えられる。プールは空にすると、自動で選ぶのに戻る。
	template, pool, empty := `{{.Name}}`, "main", ""
	_, err = store.Update(t.Context(), db, actor, &settings.Changes{
		Recording:            &domain.RecordingOverrides{PoolID: &pool, FileNameTemplate: &template, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: nil},
		ArchiveRetentionDays: nil,
	})
	require.NoError(t, err)
	loaded, err = settings.Load(t.Context(), db)
	require.NoError(t, err)
	want.PoolID, want.FileNameTemplate = pool, template
	assert.Equal(t, want, loaded.Recording)
	_, err = store.Update(t.Context(), db, actor, &settings.Changes{
		Recording:            &domain.RecordingOverrides{PoolID: &empty, FileNameTemplate: nil, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: nil},
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
		{name: "too short timeshift retention", changes: &settings.Changes{TimeshiftWhileWatchingRetentionSeconds: new(599)}, wantErr: settings.ErrInvalidSettings},
		{name: "too long timeshift retention", changes: &settings.Changes{TimeshiftWhileWatchingRetentionSeconds: new(86401)}, wantErr: settings.ErrInvalidSettings},
		{name: "unknown CM detector", changes: &settings.Changes{CMDetector: new("join_logo_scp")}, wantErr: settings.ErrInvalidSettings},
		{name: "negative history max drops", changes: &settings.Changes{HistoryMaxDrops: new(-1)}, wantErr: settings.ErrInvalidSettings},
		{name: "too many history max drops", changes: &settings.Changes{HistoryMaxDrops: new(1001)}, wantErr: settings.ErrInvalidSettings},
		// 監査ログは、ほかの保持期間と違い、無期限 (0) にできない (#1869)。
		{name: "unlimited audit retention", changes: &settings.Changes{AuditRetentionDays: new(0)}, wantErr: settings.ErrInvalidSettings},
		{name: "too long audit retention", changes: &settings.Changes{AuditRetentionDays: new(3651)}, wantErr: settings.ErrInvalidSettings},
		{name: "unlimited audit client info retention", changes: &settings.Changes{AuditClientInfoRetentionDays: new(0)}, wantErr: settings.ErrInvalidSettings},
		{name: "web push subject without a scheme", changes: &settings.Changes{WebPushSubject: new("admin@kotatsu.example")}, wantErr: settings.ErrInvalidSettings},
		{name: "web push subject over http", changes: &settings.Changes{WebPushSubject: new("http://kotatsu.example")}, wantErr: settings.ErrInvalidSettings},
		{name: "web push subject with a space", changes: &settings.Changes{WebPushSubject: new("mailto:admin @kotatsu.example")}, wantErr: settings.ErrInvalidSettings},
		{name: "web push subject with a no-break space", changes: &settings.Changes{WebPushSubject: new("mailto:admin\u00a0@kotatsu.example")}, wantErr: settings.ErrInvalidSettings},
		{name: "web push subject with only the scheme", changes: &settings.Changes{WebPushSubject: new("mailto:")}, wantErr: settings.ErrInvalidSettings},
		{name: "negative caption threads", changes: &settings.Changes{CaptionThreads: new(-1)}, wantErr: settings.ErrInvalidSettings},
		{name: "too many caption threads", changes: &settings.Changes{CaptionThreads: new(257)}, wantErr: settings.ErrInvalidSettings},
		{
			name: "negative margin", wantErr: reserve.ErrInvalidSettings,
			changes: &settings.Changes{
				Recording:            &domain.RecordingOverrides{PoolID: nil, FileNameTemplate: nil, StartMarginSeconds: &negative, EndMarginSeconds: nil, Priority: nil},
				ArchiveRetentionDays: nil,
			},
		},
		{
			name: "unknown template field", wantErr: reserve.ErrInvalidSettings,
			changes: &settings.Changes{
				Recording:            &domain.RecordingOverrides{PoolID: nil, FileNameTemplate: &badTemplate, StartMarginSeconds: nil, EndMarginSeconds: nil, Priority: nil},
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
	assert.Equal(t, domain.DefaultRecordingSettings(), loaded.Recording)
	assert.Equal(t, settings.DefaultArchiveRetentionDays, loaded.ArchiveRetentionDays)
}

// TestUpdateHistoryMaxDrops は、録画済みとみなすドロップの上限が、行を作ったときも既定値 (0 件) で、変えられることを確かめる。
func TestUpdateHistoryMaxDrops(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	store := newStore()
	actor := audit.System("test")

	defaults, err := settings.Load(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, settings.DefaultHistoryMaxDrops, defaults.HistoryMaxDrops)

	// ほかの項目を変えて行を作っても、DB の既定値は Go の既定値と同じ。
	_, err = store.Update(t.Context(), db, actor, &settings.Changes{MCPEnabled: new(true)})
	require.NoError(t, err)
	loaded, err := settings.Load(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, settings.DefaultHistoryMaxDrops, loaded.HistoryMaxDrops)

	_, err = store.Update(t.Context(), db, actor, &settings.Changes{HistoryMaxDrops: new(4)})
	require.NoError(t, err)
	loaded, err = settings.Load(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, 4, loaded.HistoryMaxDrops)
	assert.True(t, loaded.MCPEnabled, "the other change is kept")
}

// TestUpdateMovedSettings は、設定ファイルから画面に移した項目 (#1869) が、行がなくても、ほかの項目で行を作っても、
// 設定ファイルの既定値と同じで、変えられ、変えたことを監査ログに残すことを確かめる。
func TestUpdateMovedSettings(t *testing.T) {
	t.Parallel()

	// wantDefaults は、設定ファイルで決めていたときの既定値 (付録 A)。
	wantDefaults := func(t *testing.T, got *settings.System) {
		t.Helper()
		assert.Equal(t, 365*24*time.Hour, got.AuditRetention())
		assert.Equal(t, 90*24*time.Hour, got.AuditClientInfoRetention())
		assert.False(t, got.HIBPEnabled)
		assert.Equal(t, settings.DefaultWebPushSubject, got.WebPushSubjectOrDefault())
		assert.True(t, got.ThumbnailPreviewsEnabled)
		assert.True(t, got.SleepControlEnabled)
		assert.Zero(t, got.CaptionThreads)
	}
	tests := []struct {
		name    string
		changes *settings.Changes
		check   func(t *testing.T, got *settings.System)
		// wantAudit は、監査ログの変更の要約に残る、変える前と後の値。
		wantAudit map[string]map[string]any
	}{
		{
			name:    "a row made by another change keeps the defaults",
			changes: &settings.Changes{MCPEnabled: new(true)},
			check:   wantDefaults,
		},
		{
			name: "every moved setting",
			changes: &settings.Changes{
				AuditRetentionDays: new(30), AuditClientInfoRetentionDays: new(7), HIBPEnabled: new(true),
				WebPushSubject: new("mailto:admin@kotatsu.example"), ThumbnailPreviewsEnabled: new(false), SleepControlEnabled: new(false), CaptionThreads: new(4),
			},
			check: func(t *testing.T, got *settings.System) {
				t.Helper()
				assert.Equal(t, 30*24*time.Hour, got.AuditRetention())
				assert.Equal(t, 7*24*time.Hour, got.AuditClientInfoRetention())
				assert.True(t, got.HIBPEnabled)
				assert.Equal(t, "mailto:admin@kotatsu.example", got.WebPushSubjectOrDefault())
				assert.False(t, got.ThumbnailPreviewsEnabled)
				assert.False(t, got.SleepControlEnabled)
				assert.Equal(t, 4, got.CaptionThreads)
			},
			wantAudit: map[string]map[string]any{
				"auditRetentionDays":           fromTo(float64(365), float64(30)),
				"auditClientInfoRetentionDays": fromTo(float64(90), float64(7)),
				"hibpEnabled":                  fromTo(false, true),
				"webPushSubject":               fromTo("", "mailto:admin@kotatsu.example"),
				"thumbnailPreviewsEnabled":     fromTo(true, false),
				"sleepControlEnabled":          fromTo(true, false),
				"captionThreads":               fromTo(float64(0), float64(4)),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := newDB(t)
			before, err := settings.Load(t.Context(), db)
			require.NoError(t, err)
			wantDefaults(t, before)

			updated, err := newStore().Update(t.Context(), db, audit.System("test"), tt.changes)
			require.NoError(t, err)
			loaded, err := settings.Load(t.Context(), db)
			require.NoError(t, err)
			assert.Equal(t, updated, loaded)
			tt.check(t, loaded)

			var summary map[string]map[string]any
			require.NoError(t, db.QueryRow(t.Context(), "SELECT summary FROM audit_events WHERE action = 'settings.updated'").Scan(&summary))
			for key, want := range tt.wantAudit {
				assert.Equal(t, want, summary[key], key)
			}
		})
	}
}

// fromTo は、監査ログの変更の要約に残る、変える前と後の値。
func fromTo(from, to any) map[string]any {
	return map[string]any{"from": from, "to": to}
}

// TestUpdateWebPushSubjectBackToDefault は、Web Push の送り手の連絡先を空にすると、既定の URL に戻ることを確かめる (#1869)。
func TestUpdateWebPushSubjectBackToDefault(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	store := newStore()
	_, err := store.Update(t.Context(), db, audit.System("test"), &settings.Changes{WebPushSubject: new("https://kotatsu.example/contact")})
	require.NoError(t, err)
	updated, err := store.Update(t.Context(), db, audit.System("test"), &settings.Changes{WebPushSubject: new("")})
	require.NoError(t, err)
	assert.Equal(t, settings.DefaultWebPushSubject, updated.WebPushSubjectOrDefault())
}

// TestUpdateJikkyoChannels は、実況のチャンネルの対応を、書いたサービスだけ変え、10 進のキーにそろえ、正しくない対応を拒むことを確かめる (#1869)。
func TestUpdateJikkyoChannels(t *testing.T) {
	t.Parallel()

	// terrestrialKey は、地上波のサービスの、10 進のキー。
	const terrestrialKey = "32736:1024"
	// bsKey は、BS のサービスのキー。
	const bsKey = "4:101"

	tests := []struct {
		name    string
		changes []map[string]*string
		want    map[string]string
		wantErr error
	}{
		{
			name:    "足して、書いていないサービスは変えない",
			changes: []map[string]*string{{bsKey: new("jk101")}, {terrestrialKey: new("jk1")}},
			want:    map[string]string{bsKey: "jk101", terrestrialKey: "jk1"},
		},
		{
			name:    "nil で消す",
			changes: []map[string]*string{{bsKey: new("jk101"), "4:103": new("jk103")}, {bsKey: nil}},
			want:    map[string]string{"4:103": "jk103"},
		},
		{
			name:    "16 進のキーは 10 進にそろえる",
			changes: []map[string]*string{{"0x7FE0:0x400": new("jk1")}},
			want:    map[string]string{terrestrialKey: "jk1"},
		},
		{name: "キーの区切りがない", changes: []map[string]*string{{"4-101": new("jk101")}}, wantErr: settings.ErrInvalidSettings},
		{name: "ID が範囲の外", changes: []map[string]*string{{"65536:101": new("jk101")}}, wantErr: settings.ErrInvalidSettings},
		{name: "チャンネルが jk で始まらない", changes: []map[string]*string{{bsKey: new("101")}}, wantErr: settings.ErrInvalidSettings},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := newDB(t)
			var err error
			for _, changes := range tt.changes {
				if _, err = newStore().Update(t.Context(), db, audit.System("test"), &settings.Changes{JikkyoChannels: changes}); err != nil {
					break
				}
			}
			loaded, loadErr := settings.Load(t.Context(), db)
			require.NoError(t, loadErr)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, loaded.JikkyoChannels, "rejected changes are not saved")

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, loaded.JikkyoChannels)
		})
	}
}

// TestUpdateConcurrently は、違う項目を同時に変えても、どちらの変更も失わないことを確かめる。
func TestUpdateConcurrently(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	store := newStore()
	start, end := 20, 40
	var wg sync.WaitGroup
	for _, change := range []*domain.RecordingOverrides{
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
			name:    "変えていなければ、イベントリレーへの追従と同じ回の束ねだけが有効",
			changes: nil,
			want:    &settings.RuleDefaults{AvoidDuplicates: false, DuplicateWindowDays: nil, FollowEventRelay: true, BundleSameEpisodes: true},
		},
		{
			name: "変えた項目だけを重ねる",
			changes: []*settings.RuleChanges{
				{AvoidDuplicates: new(true), DuplicateWindowDays: &days, ClearDuplicateWindowDays: false, FollowEventRelay: nil},
				{AvoidDuplicates: nil, DuplicateWindowDays: nil, ClearDuplicateWindowDays: false, FollowEventRelay: new(false)},
				{BundleSameEpisodes: new(false)},
			},
			want: &settings.RuleDefaults{AvoidDuplicates: true, DuplicateWindowDays: &days, FollowEventRelay: false, BundleSameEpisodes: false},
		},
		{
			name: "期間を区切らないように戻せる",
			changes: []*settings.RuleChanges{
				{AvoidDuplicates: new(true), DuplicateWindowDays: &days, ClearDuplicateWindowDays: false, FollowEventRelay: nil},
				{AvoidDuplicates: nil, DuplicateWindowDays: nil, ClearDuplicateWindowDays: true, FollowEventRelay: nil},
			},
			want: &settings.RuleDefaults{AvoidDuplicates: true, DuplicateWindowDays: nil, FollowEventRelay: true, BundleSameEpisodes: true},
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

// TestUpdateNetworkActivity は、外部との通信の記録を有効にした日時を残し、無効に戻すと記録を消すことを確かめる (#1369)。
func TestUpdateNetworkActivity(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	store := newStore()
	on, off := true, false

	enabled, err := store.Update(t.Context(), db, audit.System("test"), &settings.Changes{NetworkActivityEnabled: &on})
	require.NoError(t, err)
	assert.True(t, enabled.NetworkActivityEnabled)
	require.NotNil(t, enabled.NetworkActivityEnabledAt)
	assert.Equal(t, testNow(), enabled.NetworkActivityEnabledAt.UTC())
	_, err = db.Exec(t.Context(), `INSERT INTO network_activity (hour, purpose, host, initiator, success, count, sent_bytes, received_bytes)
		VALUES (now(), 'UPDATE_CHECK', '', 'SCHEDULED', true, 1, 0, 0)`)
	require.NoError(t, err)

	disabled, err := store.Update(t.Context(), db, audit.System("test"), &settings.Changes{NetworkActivityEnabled: &off})
	require.NoError(t, err)
	assert.False(t, disabled.NetworkActivityEnabled)
	assert.Nil(t, disabled.NetworkActivityEnabledAt)
	var count int
	require.NoError(t, db.QueryRow(t.Context(), `SELECT count(*) FROM network_activity`).Scan(&count))
	assert.Zero(t, count, "無効に戻したら、それまでの記録を消す")
}

// TestUpdateCMDetector は、CM の区間の見分け方が既定で ffmpeg で、logo に替えて戻せることを確かめる (#1505)。
func TestUpdateCMDetector(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	store := newStore()
	actor := audit.System("test")

	loaded, err := settings.Load(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, settings.CMDetectorFFmpeg, loaded.CMDetector)
	for _, want := range []string{settings.CMDetectorLogo, settings.CMDetectorFFmpeg} {
		updated, err := store.Update(t.Context(), db, actor, &settings.Changes{CMDetector: new(want)})
		require.NoError(t, err)
		assert.Equal(t, want, updated.CMDetector)
		loaded, err := settings.Load(t.Context(), db)
		require.NoError(t, err)
		assert.Equal(t, want, loaded.CMDetector)
	}
}
