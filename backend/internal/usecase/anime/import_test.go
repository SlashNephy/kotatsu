package anime_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/animedata"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/syobocal"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// 地上波は地域でネットワーク ID が違う。saya-definitions は 15 を入れている。
	grNetworkID = 0x7FE0
	// grService は、ネットワーク ID 0x7FE0、サービス ID 1024 の地上波のサービス。
	grService = grNetworkID*100000 + 1024
	// bsService は、ネットワーク ID 4、サービス ID 101 の BS のサービス。
	bsService = 4*100000 + 101
	// csService は、saya-definitions にないサービス。
	csService = 7*100000 + 999
)

func testNow() time.Time {
	return time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)
}

func newDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	_, err = pool.Exec(t.Context(), `INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 0), ('BS', 'BS', 1), ('CS', 'CS', 2)`)
	require.NoError(t, err)
	for _, service := range []struct {
		id, networkID, sid int64
		channelType        string
	}{
		{id: grService, networkID: grNetworkID, sid: 1024, channelType: "GR"},
		{id: bsService, networkID: 4, sid: 101, channelType: "BS"},
		{id: csService, networkID: 7, sid: 999, channelType: "CS"},
	} {
		_, err := pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
			VALUES ($1, $2, $3, 'テスト', 'テスト', 1, $4, '27', now(), now())`, service.id, service.networkID, service.sid, service.channelType)
		require.NoError(t, err)
	}

	return pool
}

// enable は、連携を有効にする。
func enable(t *testing.T, db *pgxpool.Pool, clk *clocktest.Fake) {
	t.Helper()

	_, err := settings.NewStore(clk, audit.NewRecorder(clk)).Update(t.Context(), db, audit.System("test"), &settings.Changes{
		Recording: nil, ArchiveRetentionDays: nil, NotificationRetentionDays: nil, Rule: nil, MCPEnabled: nil,
		AnimeLinkingEnabled: new(true), DRCSReplacements: nil,
	})
	require.NoError(t, err)
}

// fakeFetcher は、取り込み元を返す偽物。etags にある ETag と同じ ETag で問い合わせたら、ErrNotModified を返す。
type fakeFetcher struct {
	mu       sync.Mutex
	channels []*animedata.Channel
	arm      []*animedata.Work
	// failArm なら、arm の取得に失敗する。
	failArm bool
	// calls は、取り込み元ごとの問い合わせの回数。
	calls map[string]int
	// barriers にある取り込み元は、問い合わせがそろうまで返さない。取り込みが重なったときを作るのに使う。
	barriers map[string]*sync.WaitGroup
}

var _ anime.Fetcher = new(fakeFetcher)

func newFakeFetcher() *fakeFetcher {
	return &fakeFetcher{
		mu: sync.Mutex{},
		channels: []*animedata.Channel{
			{Name: "NHK 総合", Type: "GR", ServiceIDs: []int64{1024}, NetworkID: 15, SyobocalID: new(int32(1)), AnnictID: new(int32(1))},
			// 同じサービスが 2 度出てきたら、先のものを使う。
			{Name: "別の地域", Type: "GR", ServiceIDs: []int64{1024}, NetworkID: 15, SyobocalID: new(int32(99)), AnnictID: nil},
			{Name: "NHKBS1", Type: "BS", ServiceIDs: []int64{101, 102}, NetworkID: 4, SyobocalID: new(int32(9)), AnnictID: new(int32(9))},
			// 対応のないチャンネルは取り込まない。
			{Name: "対応なし", Type: "CS", ServiceIDs: []int64{999}, NetworkID: 7, SyobocalID: nil, AnnictID: nil},
		},
		arm: []*animedata.Work{
			{SyobocalTID: new(int32(1575)), AnnictID: new(int32(1745)), AniListID: new(int32(5114)), MALID: new(int32(5114))},
			// 1 つの TID に複数の作品が対応することがある。
			{SyobocalTID: new(int32(274)), AnnictID: new(int32(1586)), AniListID: new(int32(318)), MALID: new(int32(318))},
			{SyobocalTID: new(int32(274)), AnnictID: new(int32(2922)), AniListID: new(int32(1234)), MALID: nil},
			// 同じ対応の重複は 1 つにする。
			{SyobocalTID: new(int32(274)), AnnictID: new(int32(2922)), AniListID: new(int32(1234)), MALID: nil},
			// TID のない作品は使わない。
			{SyobocalTID: nil, AnnictID: new(int32(2)), AniListID: nil, MALID: nil},
		},
		failArm:  false,
		calls:    map[string]int{},
		barriers: map[string]*sync.WaitGroup{},
	}
}

func (f *fakeFetcher) SayaDefinitions(_ context.Context, etag string) ([]*animedata.Channel, string, error) {
	f.wait(anime.SourceSayaDefinitions)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[anime.SourceSayaDefinitions]++
	if etag == "saya-1" {
		return nil, "", animedata.ErrNotModified
	}

	return f.channels, "saya-1", nil
}

func (f *fakeFetcher) Arm(_ context.Context, etag string) ([]*animedata.Work, string, error) {
	f.wait(anime.SourceArm)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[anime.SourceArm]++
	if f.failArm {
		return nil, "", errors.New("connection refused")
	}
	if etag == "arm-1" {
		return nil, "", animedata.ErrNotModified
	}

	return f.arm, "arm-1", nil
}

// wait は、取り込み元 source の問い合わせがそろうまで待つ。
func (f *fakeFetcher) wait(source string) {
	f.mu.Lock()
	barrier := f.barriers[source]
	f.mu.Unlock()
	if barrier != nil {
		barrier.Done()
		barrier.Wait()
	}
}

// fakeTitles は、しょぼいカレンダーのタイトルの一覧を返す偽物。前に取り込んだ後の更新だけを返す。
type fakeTitles struct {
	mu     sync.Mutex
	titles []*syobocal.Title
	since  []time.Time
}

var _ anime.TitleFetcher = new(fakeTitles)

// newFakeTitles は、次のタイトルを持つ偽物を返す。「同名作品」は 2 つあり、どちらか決められない。
func newFakeTitles() *fakeTitles {
	updated := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	return &fakeTitles{
		mu: sync.Mutex{},
		titles: []*syobocal.Title{
			{TID: 10, Title: testTitle, LastUpdate: updated},
			{TID: 30, Title: "リピート 作品", LastUpdate: updated},
			{TID: 31, Title: "同名作品", LastUpdate: updated},
			{TID: 32, Title: "同名作品", LastUpdate: updated.Add(time.Hour)},
		},
		since: []time.Time{},
	}
}

func (f *fakeTitles) TitleLookup(_ context.Context, since time.Time) ([]*syobocal.Title, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.since = append(f.since, since)
	found := []*syobocal.Title{}
	for _, title := range f.titles {
		if since.IsZero() || title.LastUpdate.After(since) {
			found = append(found, title)
		}
	}

	return found, nil
}

func (f *fakeFetcher) setFailArm(fail bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failArm = fail
}

func (f *fakeFetcher) callsOf(source string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls[source]
}

// TestImportDisabled は、連携を有効にしていなければ、外部へ問い合わせないことを確かめる (設計書 0.3)。
func TestImportDisabled(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	fetcher := newFakeFetcher()
	result, err := anime.NewImporter(db, clocktest.NewFake(testNow()), fetcher, newFakeTitles(), nil, slog.New(slog.DiscardHandler)).Import(t.Context(), true)
	require.NoError(t, err)
	assert.True(t, result.Disabled)
	for _, source := range []string{anime.SourceSayaDefinitions, anime.SourceArm} {
		assert.Zero(t, fetcher.callsOf(source), source)
	}
}

// TestImport は、取り込んだ対応から、サービスごとのチャンネルと、TID ごとの作品を引けることを確かめる。
// 地上波は種別とサービス ID で、BS はネットワーク ID とサービス ID で照らし合わせる。
func TestImport(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	enable(t, db, clk)
	fetcher := newFakeFetcher()
	importer := anime.NewImporter(db, clk, fetcher, newFakeTitles(), nil, slog.New(slog.DiscardHandler))

	result, err := importer.Import(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{
		anime.SourceSayaDefinitions: 3, anime.SourceArm: 3, anime.SourceSyobocalTitles: 4,
	}, result.Imported)

	channels, err := anime.ListChannels(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, []*anime.Channel{
		{
			ServiceID: bsService, DefinitionName: new("NHKBS1"), ImportedSyobocalChannelID: new(int32(9)), ImportedAnnictChannelID: new(int32(9)),
			OverrideSyobocalChannelID: nil, OverrideAnnictChannelID: nil, SyobocalChannelID: new(int32(9)), AnnictChannelID: new(int32(9)),
		},
		{
			ServiceID: csService, DefinitionName: nil, ImportedSyobocalChannelID: nil, ImportedAnnictChannelID: nil,
			OverrideSyobocalChannelID: nil, OverrideAnnictChannelID: nil, SyobocalChannelID: nil, AnnictChannelID: nil,
		},
		{
			ServiceID: grService, DefinitionName: new("NHK 総合"), ImportedSyobocalChannelID: new(int32(1)), ImportedAnnictChannelID: new(int32(1)),
			OverrideSyobocalChannelID: nil, OverrideAnnictChannelID: nil, SyobocalChannelID: new(int32(1)), AnnictChannelID: new(int32(1)),
		},
	}, channels)

	works, err := anime.ListWorks(t.Context(), db, []int32{274, 1575, 1})
	require.NoError(t, err)
	assert.Equal(t, map[int32][]*anime.Work{
		274: {
			{AnnictWorkID: new(int32(1586)), AniListID: new(int32(318)), MALID: new(int32(318))},
			{AnnictWorkID: new(int32(2922)), AniListID: new(int32(1234)), MALID: nil},
		},
		1575: {{AnnictWorkID: new(int32(1745)), AniListID: new(int32(5114)), MALID: new(int32(5114))}},
	}, works)

	imports, err := anime.ListImports(t.Context(), db)
	require.NoError(t, err)
	require.Len(t, imports, 3)
	for _, status := range imports {
		assert.Nil(t, status.Error)
		require.NotNil(t, status.ImportedAt)
		assert.True(t, status.ImportedAt.Equal(testNow()))
	}
}

// TestImportConcurrently は、取り込みのジョブが重なって、同じ取り込み元を同時に入れ替えても、取り込めなかったと記録しないことを確かめる。
// 連携を有効にしたときのジョブと、定期ジョブが重なることがある。
func TestImportConcurrently(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	enable(t, db, clk)
	fetcher := newFakeFetcher()
	for _, source := range []string{anime.SourceSayaDefinitions, anime.SourceArm} {
		barrier := &sync.WaitGroup{}
		barrier.Add(2)
		fetcher.barriers[source] = barrier
	}
	importer := anime.NewImporter(db, clk, fetcher, newFakeTitles(), nil, slog.New(slog.DiscardHandler))

	results := make([]*anime.ImportResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for k := range results {
		wg.Go(func() {
			results[k], errs[k] = importer.Import(t.Context(), true)
		})
	}
	wg.Wait()
	for k := range results {
		require.NoError(t, errs[k])
		assert.Empty(t, results[k].Failed)
	}
	imports, err := anime.ListImports(t.Context(), db)
	require.NoError(t, err)
	for _, status := range imports {
		assert.Nil(t, status.Error, status.Source)
	}
	channels, err := anime.ListChannels(t.Context(), db)
	require.NoError(t, err)
	assert.Len(t, channels, 3)
}

// TestImportSchedule は、1 日たつまで問い合わせ直さず、変わっていなければ前の内容を残すことを確かめる。
// 取り込めなかった取り込み元は、理由を残し、1 時間後に問い合わせ直す。ほかの取り込み元は取り込む。
func TestImportSchedule(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	enable(t, db, clk)
	fetcher := newFakeFetcher()
	importer := anime.NewImporter(db, clk, fetcher, newFakeTitles(), nil, slog.New(slog.DiscardHandler))
	_, err := importer.Import(t.Context(), false)
	require.NoError(t, err)

	clk.Advance(time.Hour)
	result, err := importer.Import(t.Context(), false)
	require.NoError(t, err)
	assert.Empty(t, result.Imported)
	assert.Equal(t, 1, fetcher.callsOf(anime.SourceArm), "not checked again within a day")

	clk.Advance(anime.ImportInterval)
	fetcher.setFailArm(true)
	result, err = importer.Import(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, []string{anime.SourceSayaDefinitions}, result.Unchanged)
	assert.Equal(t, map[string]int{anime.SourceSyobocalTitles: 4}, result.Imported)
	assert.Equal(t, map[string]string{anime.SourceArm: "connection refused"}, result.Failed)
	works, err := anime.ListWorks(t.Context(), db, []int32{1575})
	require.NoError(t, err)
	assert.Len(t, works[1575], 1, "the works imported before are kept")

	clk.Advance(time.Hour)
	fetcher.setFailArm(false)
	result, err = importer.Import(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, []string{anime.SourceArm}, result.Unchanged, "a failed source is checked again after an hour")
	imports, err := anime.ListImports(t.Context(), db)
	require.NoError(t, err)
	counts := map[string]int{}
	for _, status := range imports {
		assert.Nil(t, status.Error, status.Source)
		counts[status.Source] = status.ItemCount
	}
	assert.Equal(t, map[string]int{
		anime.SourceSayaDefinitions: 3, anime.SourceArm: 3, anime.SourceSyobocalTitles: 4,
	}, counts,
		"unchanged sources keep the counts imported before")
}
