package animerecord_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/workids"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

// fakeArtworks は、AniList と Jikan の代わり。引かれた MyAnimeList の ID を数える。
type fakeArtworks struct {
	mu     sync.Mutex
	covers map[int32]string
	// jikan は、Jikan が返す画像の URL。jikanErr にある ID は、そのエラーを返す。
	jikan    map[int32]string
	jikanErr map[int32]error
	aniList  [][]int32
	jikanIDs []int32
}

func (f *fakeArtworks) AniListCoversByMAL(_ context.Context, malIDs []int32) (map[int32]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aniList = append(f.aniList, append([]int32{}, malIDs...))
	result := map[int32]string{}
	for _, id := range malIDs {
		if url, ok := f.covers[id]; ok {
			result[id] = url
		}
	}

	return result, nil
}

func (f *fakeArtworks) JikanImage(_ context.Context, malID int32) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jikanIDs = append(f.jikanIDs, malID)
	if err, ok := f.jikanErr[malID]; ok {
		return "", err
	}

	return f.jikan[malID], nil
}

const (
	// jikanWorkID と missingWorkID は、AniList にない Annict の作品。jikanWorkID は Jikan にあり、missingWorkID はどちらにもない。
	jikanWorkID   = 11
	missingWorkID = 12
	aniListCover  = "https://anilist.example/xl-100.jpg"
	jikanImage    = "https://cdn.example/200l.jpg"
	annictImageA  = "https://example.com/a.png"
	annictImageC  = "https://example.com/c.png"
	aniListImage  = "https://anilist.example/600.jpg"
)

// newArtworkFixture は、Annict の作品 3 つ (MyAnimeList の 100、200、300) と AniList の作品 1 つを見たいリストに持つフィクスチャと、
// 高画質の画像を引く Service を返す。
func newArtworkFixture(t *testing.T, artworks *fakeArtworks) (*recordFixture, *animerecord.Service) {
	t.Helper()

	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	f.connect(t, f.alice, animerecord.DestinationAniList)
	for _, statement := range []string{
		`UPDATE anime_works SET mal_id = 100 WHERE annict_work_id = 10`,
		`INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm', 7001, 0, 11, NULL, 200), ('arm', 7002, 0, 12, NULL, 300)`,
	} {
		_, err := f.pool.Exec(t.Context(), statement)
		require.NoError(t, err)
	}
	for _, entry := range []struct {
		destination string
		workID      int32
		image       *string
	}{
		{animerecord.DestinationAnnict, annictWorkID, new(annictImageA)},
		{animerecord.DestinationAnnict, jikanWorkID, nil},
		{animerecord.DestinationAnnict, missingWorkID, new(annictImageC)},
		{animerecord.DestinationAniList, aniListID, new(aniListImage)},
	} {
		_, err := f.pool.Exec(t.Context(), `INSERT INTO anime_wishlist_entries (user_id, destination, work_id, status, title, image_url, fetched_at)
VALUES ($1, $2, $3, 'watching', $4, $5, now())`, f.alice.UserID, entry.destination, entry.workID, titleA, entry.image)
		require.NoError(t, err)
	}
	service, err := animerecord.NewService(&animerecord.Options{
		Clock: f.clock, Recorder: audit.NewRecorder(f.clock), AppSecret: strings.Repeat("k", 32), Destinations: nil,
		Logger: slog.New(slog.DiscardHandler), Inserter: nil, WishlistInterval: 0, Artworks: artworks,
	})
	require.NoError(t, err)

	return f, service
}

// images は、見たいリストの作品の画像を、"記録先:作品 ID" ごとに返す。
func images(t *testing.T, f *recordFixture) map[string]string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), `SELECT destination || ':' || work_id, COALESCE(image_url, '') FROM anime_wishlist_entries`)
	require.NoError(t, err)
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var key, url string
		require.NoError(t, rows.Scan(&key, &url))
		result[key] = url
	}
	require.NoError(t, rows.Err())

	return result
}

// TestRefreshArtworks は、Annict の作品の画像を、AniList の高画質の画像か、なければ Jikan の画像にすることを確かめる (#1315)。
// どちらにもなければ Annict の画像のままにし、引いた作品は、引き直す時期まで問い合わせない。
func TestRefreshArtworks(t *testing.T) {
	t.Parallel()

	artworks := &fakeArtworks{
		mu: sync.Mutex{}, covers: map[int32]string{100: aniListCover}, jikan: map[int32]string{200: jikanImage},
		jikanErr: map[int32]error{300: workids.ErrNotFound}, aniList: nil, jikanIDs: nil,
	}
	f, service := newArtworkFixture(t, artworks)

	require.NoError(t, service.RefreshArtworks(t.Context(), f.pool))
	assert.Equal(t, map[string]string{
		"annict:10": aniListCover, "annict:11": jikanImage, "annict:12": annictImageC,
		// AniList の作品は、取り込んだときの画像 (extraLarge) のまま。
		"anilist:600": aniListImage,
	}, images(t, f))
	assert.Equal(t, [][]int32{{100, 200, 300}}, artworks.aniList)
	assert.Equal(t, []int32{200, 300}, artworks.jikanIDs)

	// 引き直す時期までは、問い合わせない。
	require.NoError(t, service.RefreshArtworks(t.Context(), f.pool))
	assert.Len(t, artworks.aniList, 1)

	// 取り込み直しても、引いた画像を使う。
	_, err := f.pool.Exec(t.Context(), `DELETE FROM anime_wishlist_entries WHERE destination = 'annict'`)
	require.NoError(t, err)
	require.NoError(t, sqlc.New(f.pool).InsertAnimeWishlistEntries(t.Context(), sqlc.InsertAnimeWishlistEntriesParams{
		UserID: f.alice.UserID, Destination: animerecord.DestinationAnnict, Now: f.clock.Now(),
		WorkIds: []int32{annictWorkID}, Statuses: []string{"watching"}, Titles: []string{titleA}, NextEpisodeIds: []string{""},
		NextEpisodeNumbers: []int32{-1}, NextEpisodeNumberTexts: []string{""}, NextEpisodeTitles: []string{""}, Progresses: []int32{-1},
		EpisodesCounts: []int32{-1}, SeasonYears: []int32{-1}, SeasonNames: []string{""}, ImageURLs: []string{annictImageA},
	}))
	assert.Equal(t, aniListCover, images(t, f)["annict:10"])

	// 引き直す時期が来たら、引き直す。
	f.clock.Advance(animerecord.ArtworkRefreshInterval + time.Hour)
	require.NoError(t, service.RefreshArtworks(t.Context(), f.pool))
	assert.Equal(t, [][]int32{{100, 200, 300}, {100}}, artworks.aniList)
}

// TestRefreshArtworksJikanUnavailable は、Jikan に届かなければ、AniList で見つけた画像だけを書き、残りは次の機会に引き直すことを確かめる (#1315)。
func TestRefreshArtworksJikanUnavailable(t *testing.T) {
	t.Parallel()

	artworks := &fakeArtworks{
		mu: sync.Mutex{}, covers: map[int32]string{100: aniListCover}, jikan: map[int32]string{},
		jikanErr: map[int32]error{200: errors.New("connection timed out"), 300: errors.New("connection timed out")}, aniList: nil, jikanIDs: nil,
	}
	f, service := newArtworkFixture(t, artworks)

	require.NoError(t, service.RefreshArtworks(t.Context(), f.pool))
	got := images(t, f)
	assert.Equal(t, aniListCover, got["annict:10"])
	assert.Empty(t, got["annict:11"])
	// 届かなければ、残りの作品には問い合わせない。
	assert.Equal(t, []int32{200}, artworks.jikanIDs)

	artworks.jikanErr = map[int32]error{300: workids.ErrNotFound}
	artworks.jikan = map[int32]string{200: jikanImage}
	require.NoError(t, service.RefreshArtworks(t.Context(), f.pool))
	assert.Equal(t, [][]int32{{100, 200, 300}, {200, 300}}, artworks.aniList)
	assert.Equal(t, jikanImage, images(t, f)["annict:11"])
}

// TestRefreshArtworksDisabled は、管理者がアニメの連携を無効にしていれば、画像を引かないことを確かめる (#1315)。
func TestRefreshArtworksDisabled(t *testing.T) {
	t.Parallel()

	artworks := &fakeArtworks{
		mu: sync.Mutex{}, covers: map[int32]string{100: aniListCover}, jikan: map[int32]string{}, jikanErr: map[int32]error{}, aniList: nil, jikanIDs: nil,
	}
	f, service := newArtworkFixture(t, artworks)
	_, err := settings.NewStore(f.clock, audit.NewRecorder(f.clock)).Update(t.Context(), f.pool, audit.User(f.bob.UserID), &settings.Changes{
		Recording: nil, ArchiveRetentionDays: nil, NotificationRetentionDays: nil, Rule: nil, MCPEnabled: nil, DataBroadcastIPEnabled: nil,
		AnimeLinkingEnabled: new(false), DRCSReplacements: nil,
	})
	require.NoError(t, err)

	require.NoError(t, service.RefreshArtworks(t.Context(), f.pool))
	assert.Empty(t, artworks.aniList)
	assert.Equal(t, annictImageA, images(t, f)["annict:10"])
}
