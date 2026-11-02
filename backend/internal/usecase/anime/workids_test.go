package anime_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/workids"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
)

const (
	// secondPage は、すべての作品の 2 ページ目の cursor。annictToken は、管理者が設定した Annict のトークン。
	secondPage  = "page-2"
	annictToken = "annict-token"
	autumn2026  = "2026-autumn"
	winter2027  = "2027-winter"
)

// fakeWorkIDs は、Annict と AniList の偽物。今期と来期の作品は 1 ページ、すべての作品は cursor でつながる 2 ページを返す。
type fakeWorkIDs struct {
	mu sync.Mutex
	// seasonal は、今期と来期の作品。
	seasonal []*workids.AnnictWork
	// pages は、すべての作品を cursor ごとに返すページ。
	pages map[string]*workids.AnnictPage
	// aniList は、MyAnimeList の ID から AniList の ID への表。
	aniList map[int32]int32
	// fail なら、Annict への問い合わせに失敗する。
	fail bool
	// seasons と cursors は、Annict に問い合わせた時期と cursor。malBatches は、AniList に問い合わせた ID。
	seasons    [][]string
	cursors    []string
	malBatches [][]int32
}

var _ anime.WorkIDFetcher = new(fakeWorkIDs)

func newFakeWorkIDs() *fakeWorkIDs {
	return &fakeWorkIDs{
		mu: sync.Mutex{},
		seasonal: []*workids.AnnictWork{
			// arm の対応がある TID 274 は、Annict の対応で置き換える。
			{AnnictID: 1586, SyobocalTID: new(int32(274)), MALAnimeID: new(int32(318))},
			{AnnictID: 9999, SyobocalTID: new(int32(274)), MALAnimeID: nil},
		},
		pages: map[string]*workids.AnnictPage{
			"": {Works: []*workids.AnnictWork{
				{AnnictID: 100, SyobocalTID: new(int32(7000)), MALAnimeID: new(int32(700))},
				// TID のない作品は持たない。
				{AnnictID: 101, SyobocalTID: nil, MALAnimeID: new(int32(701))},
			}, Next: secondPage},
			secondPage: {Works: []*workids.AnnictWork{
				// AniList にない作品。
				{AnnictID: 102, SyobocalTID: new(int32(7001)), MALAnimeID: new(int32(799))},
			}, Next: ""},
		},
		aniList:    map[int32]int32{318: 318, 700: 70000},
		fail:       false,
		seasons:    [][]string{},
		cursors:    []string{},
		malBatches: [][]int32{},
	}
}

func (f *fakeWorkIDs) AnnictWorks(_ context.Context, token string, seasons []string, cursor string) (*workids.AnnictPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if token != annictToken {
		return nil, workids.ErrUnauthorized
	}
	if f.fail {
		return nil, errors.New("connection refused")
	}
	if seasons != nil {
		f.seasons = append(f.seasons, seasons)

		return &workids.AnnictPage{Works: f.seasonal, Next: ""}, nil
	}
	f.cursors = append(f.cursors, cursor)

	return f.pages[cursor], nil
}

func (f *fakeWorkIDs) AniListIDsByMAL(_ context.Context, malIDs []int32) (map[int32]int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.malBatches = append(f.malBatches, slices.Sorted(slices.Values(malIDs)))
	found := map[int32]int32{}
	for _, id := range malIDs {
		if anilist, ok := f.aniList[id]; ok {
			found[id] = anilist
		}
	}

	return found, nil
}

func (f *fakeWorkIDs) calls() (int, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.seasons), len(f.cursors), len(f.malBatches)
}

func (f *fakeWorkIDs) setFail(fail bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = fail
}

// fakeTokens は、管理者が設定した Annict のトークンを返す偽物。
type fakeTokens struct{ token string }

var _ anime.TokenSource = new(fakeTokens)

func (f *fakeTokens) AnnictImportToken(context.Context, sqlc.DBTX) (string, error) {
	return f.token, nil
}

// TestImportAnnictWorksWithoutToken は、管理者が Annict のトークンを設定していなければ、Annict にも AniList にも問い合わせないことを確かめる。
func TestImportAnnictWorksWithoutToken(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	enable(t, db, clk)
	fetcher := newFakeWorkIDs()
	importer := anime.NewImporter(db, clk, newFakeFetcher(), newFakeTitles(), nil, slog.New(slog.DiscardHandler)).
		WithWorkIDs(&anime.WorkIDOptions{Fetcher: fetcher, Tokens: &fakeTokens{token: ""}, AnnictInterval: 0, AniListInterval: 0})

	result, err := importer.Import(t.Context(), true)
	require.NoError(t, err)
	assert.NotContains(t, result.Imported, anime.SourceAnnictWorks)
	seasonal, full, aniList := fetcher.calls()
	assert.Zero(t, seasonal+full+aniList)
}

// TestImportAnnictWorks は、Annict と AniList から作った対応を、arm の対応より優先して使うことを確かめる (#1223)。
// 今期と来期は 1 日に 1 回、すべての作品は引き終えてから 30 日後に引き直し、AniList の ID は 30 日たつまで引き直さない。
func TestImportAnnictWorks(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	enable(t, db, clk)
	fetcher := newFakeWorkIDs()
	importer := anime.NewImporter(db, clk, newFakeFetcher(), newFakeTitles(), nil, slog.New(slog.DiscardHandler)).
		WithWorkIDs(&anime.WorkIDOptions{Fetcher: fetcher, Tokens: &fakeTokens{token: annictToken}, AnnictInterval: 0, AniListInterval: 0})

	result, err := importer.Import(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, 4, result.Imported[anime.SourceAnnictWorks])
	assert.Equal(t, [][]string{{autumn2026, winter2027}}, fetcher.seasons)
	assert.Equal(t, []string{"", secondPage}, fetcher.cursors)
	assert.Equal(t, [][]int32{{318, 700, 799}}, fetcher.malBatches)

	works, err := anime.ListWorks(t.Context(), db, []int32{274, 1575, 7000, 7001})
	require.NoError(t, err)
	assert.Equal(t, map[int32][]*anime.Work{
		// Annict の対応がある TID は、arm の対応を使わない。
		274: {
			{AnnictWorkID: new(int32(1586)), AniListID: new(int32(318)), MALID: new(int32(318))},
			{AnnictWorkID: new(int32(9999)), AniListID: nil, MALID: nil},
		},
		// Annict の対応がない TID は、arm の対応を使う。
		1575: {{AnnictWorkID: new(int32(1745)), AniListID: new(int32(5114)), MALID: new(int32(5114))}},
		7000: {{AnnictWorkID: new(int32(100)), AniListID: new(int32(70000)), MALID: new(int32(700))}},
		7001: {{AnnictWorkID: new(int32(102)), AniListID: nil, MALID: new(int32(799))}},
	}, works)

	// 1 時間後は、今期と来期も、引き終えたすべての作品も、AniList も問い合わせない。
	clk.Advance(time.Hour)
	result, err = importer.Import(t.Context(), false)
	require.NoError(t, err)
	assert.Contains(t, result.Unchanged, anime.SourceAnnictWorks)
	seasonal, full, aniList := fetcher.calls()
	assert.Equal(t, []int{1, 2, 1}, []int{seasonal, full, aniList})

	// 1 日後は、今期と来期だけを引き直す。
	clk.Advance(24 * time.Hour)
	_, err = importer.Import(t.Context(), false)
	require.NoError(t, err)
	seasonal, full, aniList = fetcher.calls()
	assert.Equal(t, []int{2, 2, 1}, []int{seasonal, full, aniList})

	// 30 日後は、すべての作品を初めから引き直し、AniList の ID も引き直す。Annict で TID が外れた作品は、対応からも外す。
	fetcher.mu.Lock()
	fetcher.pages[""].Works[0].SyobocalTID = nil
	fetcher.mu.Unlock()
	clk.Advance(30 * 24 * time.Hour)
	_, err = importer.Import(t.Context(), false)
	require.NoError(t, err)
	seasonal, full, aniList = fetcher.calls()
	assert.Equal(t, []int{3, 4, 2}, []int{seasonal, full, aniList})
	works, err = anime.ListWorks(t.Context(), db, []int32{7000})
	require.NoError(t, err)
	assert.Empty(t, works)
}

// TestImportAnnictWorksFailure は、Annict に問い合わせられなければ、理由を残し、ほかの取り込み元は取り込み、
// 1 時間後に問い合わせ直して対応を作ることを確かめる。
func TestImportAnnictWorksFailure(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	enable(t, db, clk)
	fetcher := newFakeWorkIDs()
	fetcher.setFail(true)
	importer := anime.NewImporter(db, clk, newFakeFetcher(), newFakeTitles(), nil, slog.New(slog.DiscardHandler)).
		WithWorkIDs(&anime.WorkIDOptions{Fetcher: fetcher, Tokens: &fakeTokens{token: annictToken}, AnnictInterval: 0, AniListInterval: 0})

	result, err := importer.Import(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{anime.SourceAnnictWorks: "failed to fetch the works from Annict: connection refused"}, result.Failed)
	assert.Contains(t, result.Imported, anime.SourceArm)

	// 失敗してから 1 時間たつまでは、問い合わせ直さない。
	clk.Advance(30 * time.Minute)
	fetcher.setFail(false)
	_, err = importer.Import(t.Context(), false)
	require.NoError(t, err)
	seasonal, _, _ := fetcher.calls()
	assert.Zero(t, seasonal)

	clk.Advance(30 * time.Minute)
	result, err = importer.Import(t.Context(), false)
	require.NoError(t, err)
	assert.Equal(t, 4, result.Imported[anime.SourceAnnictWorks])
	imports, err := anime.ListImports(t.Context(), db)
	require.NoError(t, err)
	for _, status := range imports {
		assert.Nil(t, status.Error, status.Source)
	}
}

func TestSeasonsOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		now  time.Time
		want []string
	}{
		{name: "秋", now: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), want: []string{autumn2026, winter2027}},
		{name: "日本時間で年が明けている", now: time.Date(2026, 12, 31, 15, 0, 0, 0, time.UTC), want: []string{winter2027, "2027-spring"}},
		{name: "日本時間でまだ夏", now: time.Date(2026, 9, 30, 14, 59, 0, 0, time.UTC), want: []string{"2026-summer", autumn2026}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, anime.SeasonsOf(tt.now))
		})
	}
}
