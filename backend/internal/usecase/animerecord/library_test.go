package animerecord_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
)

const (
	// titleWorkA は、AniList の作品名。stateCompleted は、録り終えた録画の状態。
	titleWorkA     = "Work A"
	stateCompleted = "completed"
)

// TestLibrary は、視聴管理の画面の作品を、TID ごとにまとめ、次の話、次の放送、録画のある話を求めることを確かめる (#1231)。
func TestLibrary(t *testing.T) {
	t.Parallel()
	// unmapped は、作品 ID の対応のない作品。番組も録画も引けないので、後ろに回る。
	const unmapped = "-|作品Z|wanna_watch|progress=-|next=-|broadcast=-|"

	tests := []struct {
		name string
		// annictProgress と annictNext は、Annict の作品の見終えた話数と次の話数。aniListProgress は、AniList の見終えた話数。
		annictProgress  *int32
		annictNext      *int32
		aniListProgress *int32
		want            []string
	}{
		{
			name:           "the larger progress wins and a next episode already watched elsewhere is replaced",
			annictProgress: new(int32(2)), annictNext: new(int32(3)), aniListProgress: new(int32(3)),
			want: []string{
				"6776|作品A|watching|progress=3|next=4:|broadcast=12:00|ep1:watched:2,ep2:watched:1,ep3:watched:1,ep5:unwatched:1",
				unmapped,
			},
		},
		{
			name:           "the next broadcast skips a broadcast of episodes already watched",
			annictProgress: new(int32(4)), annictNext: new(int32(5)), aniListProgress: nil,
			want: []string{
				"6776|作品A|watching|progress=4|next=5:第5話|broadcast=13:00|ep1:watched:2,ep2:watched:1,ep3:watched:1,ep5:unwatched:1",
				unmapped,
			},
		},
		{
			name:           "without progress, nothing is claimed as unwatched",
			annictProgress: nil, annictNext: nil, aniListProgress: nil,
			want: []string{
				"6776|作品A|watching|progress=-|next=-|broadcast=12:00|ep1:unknown:2,ep2:unknown:1,ep3:unknown:1,ep5:unknown:1",
				unmapped,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordFixture(t)
			f.connect(t, f.alice, animerecord.DestinationAnnict)
			f.connect(t, f.alice, animerecord.DestinationAniList)
			var nextID, nextText *string
			if tt.annictNext != nil {
				nextID, nextText = new(fmt.Sprintf("E%d", *tt.annictNext)), new(fmt.Sprintf("第%d話", *tt.annictNext))
			}
			for _, entry := range []struct {
				destination string
				workID      int32
				status      string
				title       string
				nextID      *string
				next        *int32
				nextText    *string
				progress    *int32
			}{
				{animerecord.DestinationAnnict, annictWorkID, "watching", titleA, nextID, tt.annictNext, nextText, tt.annictProgress},
				{animerecord.DestinationAniList, aniListID, "wanna_watch", titleWorkA, nil, nil, nil, tt.aniListProgress},
				// 作品 ID の対応のない作品は、番組も録画も引けないので、後ろに回る。
				{animerecord.DestinationAnnict, 99, "wanna_watch", "作品Z", nil, nil, nil, nil},
			} {
				_, err := f.pool.Exec(t.Context(), `INSERT INTO anime_wishlist_entries
  (user_id, destination, work_id, status, title, next_episode_id, next_episode_number, next_episode_number_text, progress, fetched_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())`,
					f.alice.UserID, entry.destination, entry.workID, entry.status, entry.title, entry.nextID, entry.next, entry.nextText, entry.progress)
				require.NoError(t, err)
			}
			// 12:00 の番組 (第 3 話と第 4 話) に加え、13:00 の番組に第 5 話を結びつける。
			linkProgram(t, f, 13, 5)
			// 第 1 話は 2 回録り、録れなかった第 6 話の録画は数えない。
			for i, episode := range []struct {
				count float64
				state string
			}{{1, stateCompleted}, {1, stateCompleted}, {2, stateCompleted}, {3, "partial"}, {5, "recording"}, {6, "failed"}} {
				insertEpisodeRecording(t, f, uuid.MustParse(fmt.Sprintf("0192f000-0000-7000-8000-%012d", i+1)), i, episode.count, episode.state)
			}

			library, err := f.service.Library(t.Context(), f.pool, f.alice.UserID)
			require.NoError(t, err)
			got := make([]string, 0, len(library.Works))
			for _, work := range library.Works {
				got = append(got, describeLibraryWork(work))
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// linkProgram は、hour 時からの番組に、作品 testTID の第 episode 話を結びつける。
func linkProgram(t *testing.T, f *recordFixture, hour int, episode float64) {
	t.Helper()
	for _, statement := range []string{
		`INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at) SELECT service_id, start_at, event_id, now() FROM programs WHERE service_id = $1 AND start_at = $2`,
		`INSERT INTO program_anime_episodes (service_id, start_at, position, source, syobocal_tid, count_start, count_end, subtitle, is_first, is_last)
			VALUES ($1, $2, 0, 'syobocal', 6776, $3, $3, NULL, false, false)`,
	} {
		args := []any{serviceID(), at(hour, 0)}
		if strings.Contains(statement, "program_anime_episodes") {
			args = append(args, episode)
		}
		_, err := f.pool.Exec(t.Context(), statement, args...)
		require.NoError(t, err)
	}
}

// insertEpisodeRecording は、作品 testTID の第 episode 話の録画を足す。i 番目の録画ほど、新しく録ったものにする。
func insertEpisodeRecording(t *testing.T, f *recordFixture, id uuid.UUID, i int, episode float64, state string) {
	t.Helper()
	recordedFrom := at(1, 0).Add(-24 * 7 * time.Hour).Add(time.Duration(i) * time.Hour)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
VALUES ($1, $2, 'テスト', '', $3, $4, $5, $3, now(), now())`, id, serviceID(), recordedFrom, recordedFrom.Add(30*time.Minute), state)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_anime_matches (recording_id, matched_at) VALUES ($1, now())`, id)
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_anime_episodes (recording_id, position, source, syobocal_tid, count_start, count_end, is_first, is_last)
VALUES ($1, 0, 'syobocal', 6776, $2, $2, false, false)`, id, episode)
	require.NoError(t, err)
}

// describeLibraryWork は、作品を、比べやすい 1 行にする。NULL は「-」にする。
func describeLibraryWork(work *animerecord.LibraryWork) string {
	tid, progress, next, broadcast := "-", "-", "-", "-"
	if work.TID != nil {
		tid = strconv.Itoa(int(*work.TID))
	}
	if work.Progress != nil {
		progress = strconv.Itoa(int(*work.Progress))
	}
	if work.NextEpisode != nil && work.NextEpisode.Number != nil {
		next = fmt.Sprintf("%d:%s", *work.NextEpisode.Number, work.NextEpisode.NumberText)
	}
	if work.NextBroadcast != nil {
		broadcast = work.NextBroadcast.StartAt.UTC().Format("15:04")
	}
	status := animerecord.WishlistWannaWatch
	if work.Watching() {
		status = animerecord.WishlistWatching
	}
	episodes := make([]string, 0, len(work.RecordedEpisodes))
	for _, episode := range work.RecordedEpisodes {
		watched := "unknown"
		if episode.Watched != nil {
			watched = map[bool]string{true: "watched", false: "unwatched"}[*episode.Watched]
		}
		episodes = append(episodes, fmt.Sprintf("ep%g:%s:%d", episode.CountStart, watched, len(episode.RecordingIDs)))
	}

	return fmt.Sprintf("%s|%s|%s|progress=%s|next=%s|broadcast=%s|%s", tid, work.Title, status, progress, next, broadcast, strings.Join(episodes, ","))
}

// TestLibraryBroadcastMarks は、放送の再放送と最終話の印と、この 1 週間に始まった本放送と再放送の最も新しい放送を返すことを確かめる (#1233)。
func TestLibraryBroadcastMarks(t *testing.T) {
	t.Parallel()
	f := newRecordFixture(t)
	f.connect(t, f.alice, animerecord.DestinationAnnict)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO anime_wishlist_entries (user_id, destination, work_id, status, title, fetched_at)
VALUES ($1, 'annict', $2, 'watching', '作品A', now())`, f.alice.UserID, annictWorkID)
	require.NoError(t, err)
	// 11:00 の番組は第 2 話の再放送、12:00 の番組 (放送中) は第 3 話と第 4 話、13:00 の番組は最終話の第 5 話。
	linkProgram(t, f, 11, 2)
	linkProgram(t, f, 13, 5)
	_, err = f.pool.Exec(t.Context(), `UPDATE program_anime_episodes SET is_rerun = (start_at = $1), is_last = (start_at = $2)`, at(11, 0), at(13, 0))
	require.NoError(t, err)

	library, err := f.service.Library(t.Context(), f.pool, f.alice.UserID)
	require.NoError(t, err)
	require.Len(t, library.Works, 1)
	work := library.Works[0]
	broadcasts := make([]string, 0, len(work.Broadcasts))
	for _, broadcast := range work.Broadcasts {
		broadcasts = append(broadcasts, fmt.Sprintf("%s rerun=%t last=%t", broadcast.StartAt.UTC().Format("15:04"), broadcast.Rerun, broadcast.Last))
	}
	assert.Equal(t, []string{"12:00 rerun=false last=false", "13:00 rerun=false last=true"}, broadcasts, "only broadcasts not yet ended are upcoming")
	recent := make([]string, 0, len(work.RecentBroadcasts))
	for _, past := range work.RecentBroadcasts {
		recent = append(recent, fmt.Sprintf("%s rerun=%t", past.StartAt.UTC().Format("15:04"), past.Rerun))
	}
	assert.Equal(t, []string{"12:00 rerun=false", "11:00 rerun=true"}, recent, "the latest broadcast and the latest rerun, newest first")
}
