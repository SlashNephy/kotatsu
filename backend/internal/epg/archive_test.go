package epg_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
)

func noGuard(context.Context, pgx.Tx) error { return nil }

// TestArchiverMovesProgramsAfterGrace は、放送の終わりから猶予が過ぎた番組だけを program_archives へ移すことを確かめる。
// 同期した時点 (12:10) で、eventId 1 (11:00〜11:30) と 2 (11:30〜12:00) は放送が終わっている。
func TestArchiverMovesProgramsAfterGrace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// elapsed は、同期してから Archiver を動かすまでの時間。
		elapsed      time.Duration
		wantArchived int
	}{
		{name: "before the grace period", elapsed: 23 * time.Hour, wantArchived: 0},
		// 翌日 11:30 を過ぎると eventId 1 が、12:00 を過ぎると eventId 2 も移る。
		{name: "after the first program's grace period", elapsed: 23*time.Hour + 30*time.Minute, wantArchived: 1 * fixtureServices},
		{name: "after the grace period", elapsed: 24 * time.Hour, wantArchived: 2 * fixtureServices},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			_, err := f.reconciler.SyncAll(t.Context())
			require.NoError(t, err)

			archiver := epg.NewArchiver(f.pool, clocktest.NewFake(testNow().Add(tt.elapsed)), noGuard, slog.New(slog.DiscardHandler), nil)
			result, err := archiver.RunOnce(t.Context())
			require.NoError(t, err)

			assert.Equal(t, int64(tt.wantArchived), result.Archived)
			assert.Equal(t, tt.wantArchived, f.count(t, "SELECT count(*) FROM program_archives"))
			// 検索で正規化し直さないよう、詳細の正規化した列も移す (#852)。
			assert.Zero(t, f.count(t, "SELECT count(*) FROM program_archives WHERE extended_normalized IS NULL"))
			// 移した番組は programs に残らない。
			assert.Equal(t, fixturePrograms-tt.wantArchived, f.count(t, "SELECT count(*) FROM programs"))
		})
	}
}

// TestArchiverSkipsProgramsRemovedBeforeBroadcast は、放送前に予定から消えた番組をアーカイブしないことを確かめる。
func TestArchiverSkipsProgramsRemovedBeforeBroadcast(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	require.NoError(t, f.fake.ForgetProgram(programID(5)))
	_, err = f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)

	// 全番組の放送が終わり、猶予も過ぎた時刻。
	archiver := epg.NewArchiver(f.pool, clocktest.NewFake(testNow().Add(48*time.Hour)), noGuard, slog.New(slog.DiscardHandler), nil)
	result, err := archiver.RunOnce(t.Context())
	require.NoError(t, err)

	assert.Equal(t, int64(fixturePrograms-1), result.Archived)
	assert.Zero(t, f.count(t, "SELECT count(*) FROM program_archives WHERE service_id = $1 AND event_id = 5", serviceID()))
}

// TestArchiverArchivesProgramsWithUndecidedEnd は、終了時刻が未定のまま残った番組も、開始から猶予が過ぎたら移すことを確かめる。
func TestArchiverArchivesProgramsWithUndecidedEnd(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.fake.AddProgram(&faketuner.Program{
		ID: programID(200), EventID: 200, ServiceID: grServiceID, NetworkID: grNetworkID,
		// 終了時刻が未定の番組は、Mirakurun が長さを 1 ミリ秒にして返す。
		StartAt: testNow().Add(-time.Hour).UnixMilli(), Duration: 1,
		IsFree: true, Name: "延長中の番組", Description: "",
	})
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)

	archiver := epg.NewArchiver(f.pool, clocktest.NewFake(testNow().Add(23*time.Hour+time.Minute)), noGuard, slog.New(slog.DiscardHandler), nil)
	_, err = archiver.RunOnce(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, f.count(t, "SELECT count(*) FROM program_archives WHERE event_id = 200 AND end_undecided"))
}

// TestArchiverPrunesInBatches は、保持期間を過ぎたアーカイブを、少しずつ (BatchSize 件ずつ) すべて消すことを確かめる。
func TestArchiverPrunesInBatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		retention time.Duration
		// fromFunc なら、保持期間を RetentionFunc から返し、Retention には残す期間 (7 日) を入れる。
		// 画面から変えた保持期間が、起動時の値より優先されることを確かめる。
		fromFunc   bool
		wantPruned int
	}{
		// 3 日後の時点で、保持期間 1 日なら全番組が期限切れになる。
		{name: "expired archives are pruned", retention: 24 * time.Hour, wantPruned: fixturePrograms},
		{name: "archives within retention are kept", retention: 7 * 24 * time.Hour, wantPruned: 0},
		{name: "negative retention keeps archives forever", retention: -1, wantPruned: 0},
		{name: "retention from the function", retention: 24 * time.Hour, fromFunc: true, wantPruned: fixturePrograms},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			_, err := f.reconciler.SyncAll(t.Context())
			require.NoError(t, err)
			opts := &epg.ArchiverOptions{
				Retention: tt.retention,
				// 番組の数より小さくして、何回かに分けて移し、消すことを確かめる。
				BatchSize: 7,
			}
			if tt.fromFunc {
				opts.Retention = 7 * 24 * time.Hour
				opts.RetentionFunc = func(context.Context) (time.Duration, error) { return tt.retention, nil }
			}
			archiver := epg.NewArchiver(f.pool, clocktest.NewFake(testNow().Add(72*time.Hour)), noGuard, slog.New(slog.DiscardHandler), opts)

			result, err := archiver.RunOnce(t.Context())
			require.NoError(t, err)
			assert.Equal(t, int64(fixturePrograms), result.Archived)
			assert.Equal(t, int64(tt.wantPruned), result.Pruned)
			assert.Equal(t, fixturePrograms-tt.wantPruned, f.count(t, "SELECT count(*) FROM program_archives"))
		})
	}
}

// TestArchiverRetentionError は、保持期間を引けなければ、アーカイブを消さずにエラーを返すことを確かめる。
func TestArchiverRetentionError(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	_, err := f.reconciler.SyncAll(t.Context())
	require.NoError(t, err)
	errLoad := errors.New("load failed")
	archiver := epg.NewArchiver(f.pool, clocktest.NewFake(testNow().Add(72*time.Hour)), noGuard, slog.New(slog.DiscardHandler), &epg.ArchiverOptions{
		Retention:     24 * time.Hour,
		RetentionFunc: func(context.Context) (time.Duration, error) { return 0, errLoad },
	})
	_, err = archiver.RunOnce(t.Context())
	require.ErrorIs(t, err, errLoad)
	assert.Equal(t, fixturePrograms, f.count(t, "SELECT count(*) FROM program_archives"))
}
