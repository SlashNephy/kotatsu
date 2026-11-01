package animerecord_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
)

// TestMergeSpans は、見た区間を重ねずにまとめ、送り直しや順番の入れ替わり、複数のタブで多く数えないことを確かめる (#927)。
func TestMergeSpans(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		spans [][2]int64
		want  [][2]int64
	}{
		{name: "empty", spans: [][2]int64{}, want: [][2]int64{}},
		{name: "separate spans stay apart", spans: [][2]int64{{30, 40}, {0, 10}}, want: [][2]int64{{0, 10}, {30, 40}}},
		{name: "overlapping spans from two tabs count once", spans: [][2]int64{{0, 60}, {30, 90}}, want: [][2]int64{{0, 90}}},
		{name: "the same report sent twice counts once", spans: [][2]int64{{0, 60}, {0, 60}}, want: [][2]int64{{0, 60}}},
		{name: "touching spans join", spans: [][2]int64{{10, 20}, {0, 10}}, want: [][2]int64{{0, 20}}},
		{name: "a span inside another is absorbed", spans: [][2]int64{{0, 100}, {20, 30}}, want: [][2]int64{{0, 100}}},
		{name: "empty and reversed spans are dropped", spans: [][2]int64{{10, 10}, {20, 5}, {0, 1}}, want: [][2]int64{{0, 1}}},
		{name: "a span over the cap is coalesced with its nearest neighbor", spans: [][2]int64{{0, 10}, {11, 20}, {40, 50}}, want: [][2]int64{{0, 20}, {40, 50}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, animerecord.MergeSpans(tt.spans, 2))
		})
	}
}

// TestReachedThreshold は、番組の長さのうち決めた割合を見たら、記録することを確かめる。境目のちょうどの長さも含める。
func TestReachedThreshold(t *testing.T) {
	t.Parallel()

	length := 30 * time.Minute
	tests := []struct {
		name    string
		played  time.Duration
		percent int
		want    bool
	}{
		{name: "just below 20%", played: 6*time.Minute - time.Millisecond, percent: 20, want: false},
		{name: "exactly 20%", played: 6 * time.Minute, percent: 20, want: true},
		{name: "the whole program at 100%", played: length, percent: 100, want: true},
		{name: "almost the whole program at 100%", played: length - time.Second, percent: 100, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, animerecord.ReachedThreshold(tt.played.Milliseconds(), length, tt.percent))
		})
	}
	assert.False(t, animerecord.ReachedThreshold(1000, 0, 20), "a program without a length is never recorded")
}

// TestNextStatus は、作品の状態を変える条件が、TVTestAnnictRecorder と同じであることを確かめる。
func TestNextStatus(t *testing.T) {
	t.Parallel()

	all := &animerecord.StatusRules{WatchingOnFirst: true, WatchingOnEpisode: true, WatchedOnLast: true}
	tests := []struct {
		name        string
		rules       *animerecord.StatusRules
		first, last bool
		current     animerecord.WorkStatus
		want        animerecord.WorkStatus
		wantChange  bool
	}{
		{name: "rules are off by default", rules: &animerecord.StatusRules{}, first: true, last: false, want: animerecord.WorkStatusNone},
		{name: "the first episode sets watching", rules: &animerecord.StatusRules{WatchingOnFirst: true}, first: true, want: animerecord.WorkStatusWatching, wantChange: true},
		{name: "a middle episode sets watching", rules: &animerecord.StatusRules{WatchingOnEpisode: true}, want: animerecord.WorkStatusWatching, wantChange: true},
		{name: "the last episode sets watched", rules: all, last: true, current: animerecord.WorkStatusWatching, want: animerecord.WorkStatusWatched, wantChange: true},
		{name: "the last episode does not set watching", rules: &animerecord.StatusRules{WatchingOnEpisode: true}, last: true, want: animerecord.WorkStatusNone},
		{name: "a watched work stays watched", rules: all, current: animerecord.WorkStatusWatched, want: animerecord.WorkStatusNone},
		{name: "a new season reopens a watched work", rules: all, first: true, current: animerecord.WorkStatusWatched, want: animerecord.WorkStatusWatching, wantChange: true},
		{name: "no change when already watching", rules: all, current: animerecord.WorkStatusWatching, want: animerecord.WorkStatusNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, changed := animerecord.NextStatus(tt.rules, tt.first, tt.last, tt.current)
			assert.Equal(t, tt.wantChange, changed)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestAnnictRecordEpisode は、Annict のエピソードを、話数、話数の文字列、サブタイトルの順で照らし合わせて記録し、
// エピソードのない作品は作品を「見た」にすることを確かめる (#927)。
func TestAnnictRecordEpisode(t *testing.T) {
	t.Parallel()

	off := &animerecord.StatusRules{}
	tests := []struct {
		name        string
		work        *fakeWork
		episode     *animerecord.Episode
		rules       *animerecord.StatusRules
		wantURL     string
		wantRecords []string
		wantStatus  []string
		wantErr     bool
	}{
		{
			name:    "by number",
			work:    &fakeWork{AnnictID: 10, Episodes: []fakeEpisode{{ID: "E1", AnnictID: 101, Number: new(1)}, {ID: "E2", AnnictID: 102, Number: new(2)}}},
			episode: &animerecord.Episode{Number: new(2.0)}, rules: off,
			wantURL: "/works/10/episodes/102", wantRecords: []string{"E2"}, wantStatus: []string{},
		},
		{
			name:    "by number text when the number is missing",
			work:    &fakeWork{AnnictID: 10, Episodes: []fakeEpisode{{ID: "E3", AnnictID: 103, NumberText: new("第３話")}}},
			episode: &animerecord.Episode{Number: new(3.0)}, rules: off,
			wantURL: "/works/10/episodes/103", wantRecords: []string{"E3"}, wantStatus: []string{},
		},
		{
			name:    "by subtitle",
			work:    &fakeWork{AnnictID: 10, Episodes: []fakeEpisode{{ID: "E4", AnnictID: 104, NumberText: new("特別編"), Title: new("はじまりの日")}}},
			episode: &animerecord.Episode{Number: new(4.0), Subtitle: new("はじまりの日")}, rules: off,
			wantURL: "/works/10/episodes/104", wantRecords: []string{"E4"}, wantStatus: []string{},
		},
		{
			name:    "a work without episodes is marked watched",
			work:    &fakeWork{AnnictID: 20, NoEpisodes: true},
			episode: &animerecord.Episode{}, rules: off,
			wantURL: "/works/20", wantRecords: []string{}, wantStatus: []string{"W20:WATCHED"},
		},
		{
			name:    "the last episode marks the work watched when chosen",
			work:    &fakeWork{AnnictID: 10, Status: new("WATCHING"), Episodes: []fakeEpisode{{ID: "E12", AnnictID: 112, Number: new(12)}}},
			episode: &animerecord.Episode{Number: new(12.0), Last: true}, rules: &animerecord.StatusRules{WatchedOnLast: true},
			wantURL: "/works/10/episodes/112", wantRecords: []string{"E12"}, wantStatus: []string{"W10:WATCHED"},
		},
		{
			name:    "an unknown episode is not recorded",
			work:    &fakeWork{AnnictID: 10, Episodes: []fakeEpisode{{ID: "E1", AnnictID: 101, Number: new(1)}}},
			episode: &animerecord.Episode{Number: new(5.0)}, rules: off,
			wantErr: true, wantRecords: []string{}, wantStatus: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeService(t)
			fake.tokens["token"] = true
			fake.works[tt.work.AnnictID] = tt.work
			annict := animerecord.NewAnnict(animerecord.NewHTTPClient(), fake.server.URL)

			recorded, err := annict.RecordEpisode(t.Context(), "token", []int32{tt.work.AnnictID}, tt.episode, tt.rules)
			// 作品の状態は、記録を付けたことを残した後に、ジョブが変える。
			if err == nil && recorded.FollowUp != nil {
				require.NoError(t, recorded.FollowUp(t.Context()))
			}
			records, statuses, _ := fake.snapshot()
			assert.Equal(t, tt.wantRecords, records)
			assert.Equal(t, tt.wantStatus, statuses)
			if tt.wantErr {
				require.ErrorIs(t, err, animerecord.ErrPermanent, "a missing episode is not retried")

				return
			}
			require.NoError(t, err)
			assert.Equal(t, fake.server.URL+tt.wantURL, recorded.URL)
		})
	}
}

// TestAniListRecordEpisode は、AniList の何話まで見たか (progress) を、今の値より大きいときだけ進め、見直しで巻き戻さないことを確かめる (#927)。
func TestAniListRecordEpisode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		entry     *fakeEntry
		episodes  *int
		episode   *animerecord.Episode
		rules     *animerecord.StatusRules
		wantSaves []map[string]any
		wantErr   bool
	}{
		{
			name: "a new entry starts at the watched episode", entry: nil, episodes: new(12),
			episode: &animerecord.Episode{Number: new(3.0)}, rules: &animerecord.StatusRules{},
			wantSaves: []map[string]any{{keyMediaID: 600.0, keyProgress: 3.0}},
		},
		{
			name: "progress moves forward", entry: &fakeEntry{Progress: 2}, episodes: new(12),
			episode: &animerecord.Episode{Number: new(3.0)}, rules: &animerecord.StatusRules{},
			wantSaves: []map[string]any{{keyMediaID: 600.0, keyProgress: 3.0}},
		},
		{
			name: "rewatching an earlier episode does not move progress back", entry: &fakeEntry{Progress: 8}, episodes: new(12),
			episode: &animerecord.Episode{Number: new(3.0)}, rules: &animerecord.StatusRules{},
			wantSaves: []map[string]any{},
		},
		{
			name: "rewatching changes only the status", entry: &fakeEntry{Progress: 8, Status: new("PLANNING")}, episodes: new(12),
			episode: &animerecord.Episode{Number: new(3.0)}, rules: &animerecord.StatusRules{WatchingOnEpisode: true},
			wantSaves: []map[string]any{{keyMediaID: 600.0, keyStatus: "CURRENT"}},
		},
		{
			name: "the last episode completes the entry", entry: &fakeEntry{Progress: 11, Status: new("CURRENT")}, episodes: new(12),
			episode: &animerecord.Episode{Number: new(12.0), Last: true}, rules: &animerecord.StatusRules{WatchedOnLast: true},
			wantSaves: []map[string]any{{keyMediaID: 600.0, keyProgress: 12.0, keyStatus: "COMPLETED"}},
		},
		{
			name: "a movie without an episode number counts as one", entry: nil, episodes: new(1),
			episode: &animerecord.Episode{}, rules: &animerecord.StatusRules{},
			wantSaves: []map[string]any{{keyMediaID: 600.0, keyProgress: 1.0}},
		},
		{
			name: "a series without an episode number is not recorded", entry: nil, episodes: new(12),
			episode: &animerecord.Episode{}, rules: &animerecord.StatusRules{},
			wantSaves: []map[string]any{}, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeService(t)
			fake.tokens["token"] = true
			fake.media[600] = tt.episodes
			if tt.entry != nil {
				fake.entries[600] = tt.entry
			}
			anilist := animerecord.NewAniList(animerecord.NewHTTPClient(), fake.server.URL)

			recorded, err := anilist.RecordEpisode(t.Context(), "token", []int32{600}, tt.episode, tt.rules)
			_, _, saves := fake.snapshot()
			assert.Equal(t, tt.wantSaves, saves)
			if tt.wantErr {
				require.ErrorIs(t, err, animerecord.ErrPermanent)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, fake.server.URL+"/anime/600", recorded.URL)
		})
	}
}

// TestGraphQLErrors は、記録先のエラーを、送り直すものと送り直さないものに分け、エラーにトークンを入れないことを確かめる。
func TestGraphQLErrors(t *testing.T) {
	t.Parallel()

	fake := newFakeService(t)
	fake.tokens["token"] = true
	fake.failures["viewer"] = 1
	annict := animerecord.NewAnnict(animerecord.NewHTTPClient(), fake.server.URL)

	_, err := annict.AccountName(t.Context(), "token")
	require.Error(t, err)
	require.NotErrorIs(t, err, animerecord.ErrPermanent, "a 5xx is retried")

	_, err = annict.AccountName(t.Context(), "leaked-token")
	require.ErrorIs(t, err, animerecord.ErrPermanent, "an unusable token is not retried")
	assert.NotContains(t, err.Error(), "leaked-token")

	name, err := annict.AccountName(t.Context(), "token")
	require.NoError(t, err)
	assert.Equal(t, "annict-user", name)
}

// TestNormalizeSpans は、報告された区間の端を秒にそろえ、1 秒に満たない区間を捨てることを確かめる。
// 細かくばらけた区間をたくさん送られても、DB に置く区間が増えすぎないようにするため。
func TestNormalizeSpans(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		spans [][2]int64
		want  [][2]int64
	}{
		{name: "edges are aligned to seconds", spans: [][2]int64{{1_500, 4_700}}, want: [][2]int64{{2_000, 4_000}}},
		{name: "spans shorter than a second are dropped", spans: [][2]int64{{1_001, 1_999}, {3_000, 3_999}, {5_000, 6_000}}, want: [][2]int64{{5_000, 6_000}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, animerecord.NormalizeSpans(tt.spans))
		})
	}
}

// TestMergeSpansCap は、まとめた区間の数を上限までに抑えることを確かめる。
func TestMergeSpansCap(t *testing.T) {
	t.Parallel()

	spans := make([][2]int64, 0, animerecord.MaxStoredSpans+50)
	for i := range int64(animerecord.MaxStoredSpans + 50) {
		spans = append(spans, [2]int64{i * 10_000, i*10_000 + 1_000})
	}
	merged := animerecord.MergeSpans(spans, animerecord.MaxStoredSpans)
	assert.Len(t, merged, animerecord.MaxStoredSpans)
	assert.Equal(t, int64(0), merged[0][0])
	assert.Equal(t, spans[len(spans)-1][1], merged[len(merged)-1][1])
}
