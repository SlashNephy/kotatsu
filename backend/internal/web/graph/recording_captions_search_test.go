package graph_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const listRecordingCaptionMatches = `query($first: Int!, $after: String, $filter: RecordingFilter) {
	recordings(first: $first, after: $after, filter: $filter) {
		edges { node { name recordedFrom } captionMatches { source from to text } }
		pageInfo { hasNextPage endCursor }
	} }`

type captionMatchPage struct {
	Recordings struct {
		Edges []struct {
			Node struct {
				Name         string    `json:"name"`
				RecordedFrom time.Time `json:"recordedFrom"`
			} `json:"node"`
			CaptionMatches []struct {
				Source string    `json:"source"`
				From   time.Time `json:"from"`
				To     time.Time `json:"to"`
				Text   string    `json:"text"`
			} `json:"captionMatches"`
		} `json:"edges"`
		PageInfo struct {
			HasNextPage bool    `json:"hasNextPage"`
			EndCursor   *string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"recordings"`
}

func (p *captionMatchPage) names() []string {
	names := make([]string, 0, len(p.Recordings.Edges))
	for _, edge := range p.Recordings.Edges {
		names = append(names, edge.Node.Name)
	}

	return names
}

// texts は、録画ごとに、当たった字幕の文を返す。
func (p *captionMatchPage) texts() map[string][]string {
	out := map[string][]string{}
	for _, edge := range p.Recordings.Edges {
		texts := make([]string, 0, len(edge.CaptionMatches))
		for _, m := range edge.CaptionMatches {
			texts = append(texts, m.Text)
		}
		out[edge.Node.Name] = texts
	}

	return out
}

type captionKeywordVar struct {
	Text          string `json:"text"`
	Regexp        bool   `json:"regexp"`
	CaseSensitive bool   `json:"caseSensitive"`
}

type testCue struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Text    string `json:"text"`
}

const (
	varCaptionKeywords = "captionKeywords"
	captionHarbor      = "港の朝"
	captionLighthouse  = "灯台の夜"
	captionNone        = "字幕のない録画"
	captionStale       = "古い gram の録画"
	cueShip            = "ＮＨＫの船が港に着きました"
	cueGull            = "カモメが鳴いています"
	cueLight           = "灯台の光が見えます"
	cuePreview         = "第12話の予告です"
)

// addAutoCaptions は、録画 globalID に自動生成の字幕 cues を書く。fresh なら、字幕を書くときと同じく gram も書き、
// そうでなければ、カラムを知らない古いバイナリが書いた字幕と同じく gram を書かない。
func (f *fixture) addAutoCaptions(t *testing.T, globalID string, cues []testCue, fresh bool) {
	t.Helper()

	_, rawID, err := graph.ParseGlobalID(globalID)
	require.NoError(t, err)
	data, err := json.Marshal(cues)
	require.NoError(t, err)
	var grams []string
	var gramsAt *time.Time
	if fresh {
		texts := make([]string, 0, len(cues))
		for _, c := range cues {
			texts = append(texts, domain.Normalize(c.Text))
		}
		grams, gramsAt = match.TextGrams(texts), new(testNow())
	}
	_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_auto_captions
		(recording_id, model, generator_version, generated_at, cues, search_grams, search_grams_generated_at)
		VALUES ($1, 'ggml-test.bin', 1, $2, $3, $4, $5)`, rawID, testNow(), data, grams, gramsAt)
	require.NoError(t, err)
}

func (f *fixture) captionMatches(t *testing.T, first int, after *string, filter map[string]any) *captionMatchPage {
	t.Helper()

	var page captionMatchPage
	require.Empty(t, f.query(t, listRecordingCaptionMatches, map[string]any{varFirst: first, varAfter: after, varFilter: filter}, &page))

	return &page
}

// TestRecordingsByCaptionKeywords は、録画を自動生成の字幕の文で絞り、当たった字幕を返すことを確認する (#2214)。
func TestRecordingsByCaptionKeywords(t *testing.T) {
	t.Parallel()

	f, dir := newRecordingFixture(t)
	// 新しい順に、古い gram の録画、字幕のない録画、灯台の夜、港の朝。
	harbor := f.addRecording(t, dir, captionHarbor, "completed", 4*time.Hour)
	lighthouse := f.addRecording(t, dir, captionLighthouse, "completed", 3*time.Hour)
	f.addRecording(t, dir, captionNone, "completed", 2*time.Hour)
	stale := f.addRecording(t, dir, captionStale, "completed", time.Hour)
	f.addAutoCaptions(t, harbor, []testCue{
		{StartMs: 1000, EndMs: 2500, Text: cueShip},
		{StartMs: 3000, EndMs: 4000, Text: cueGull},
	}, true)
	f.addAutoCaptions(t, lighthouse, []testCue{
		{StartMs: 5000, EndMs: 6000, Text: cueLight},
		{StartMs: 7000, EndMs: 8000, Text: cuePreview},
	}, true)
	f.addAutoCaptions(t, stale, []testCue{{StartMs: 0, EndMs: 1000, Text: "港の灯台"}}, false)

	literal := func(text string) captionKeywordVar {
		return captionKeywordVar{Text: text, Regexp: false, CaseSensitive: false}
	}
	tests := []struct {
		name   string
		filter map[string]any
		want   map[string][]string
	}{
		{
			// 全角の英字と、大文字と小文字は、番組の検索と同じくそろえる。
			name:   "a literal keyword",
			filter: map[string]any{varCaptionKeywords: []captionKeywordVar{literal("nhkの船")}},
			want:   map[string][]string{captionHarbor: {cueShip}},
		},
		{
			name:   "case sensitive",
			filter: map[string]any{varCaptionKeywords: []captionKeywordVar{{Text: "nhkの船", Regexp: false, CaseSensitive: true}}},
			want:   map[string][]string{},
		},
		{
			name:   "all of the keywords in different captions",
			filter: map[string]any{varCaptionKeywords: []captionKeywordVar{literal("船"), literal("カモメ")}},
			want:   map[string][]string{captionHarbor: {cueShip, cueGull}},
		},
		{
			name:   "one of the keywords is missing",
			filter: map[string]any{varCaptionKeywords: []captionKeywordVar{literal("船"), literal("灯台")}},
			want:   map[string][]string{},
		},
		{
			name: "any of the keywords",
			filter: map[string]any{
				varCaptionKeywords: []captionKeywordVar{literal("カモメ"), literal("予告")}, "captionKeywordMatch": "ANY",
			},
			want: map[string][]string{captionLighthouse: {cuePreview}, captionHarbor: {cueGull}},
		},
		{
			name:   "a regular expression",
			filter: map[string]any{varCaptionKeywords: []captionKeywordVar{{Text: `第\d+話`, Regexp: true, CaseSensitive: false}}},
			want:   map[string][]string{captionLighthouse: {cuePreview}},
		},
		{
			// gram を書いていない字幕も、候補に残して照らす。
			name:   "captions without the search grams",
			filter: map[string]any{varCaptionKeywords: []captionKeywordVar{literal("灯台")}},
			want:   map[string][]string{captionStale: {"港の灯台"}, captionLighthouse: {cueLight}},
		},
		{
			name: "with the name keywords",
			filter: map[string]any{
				varCaptionKeywords: []captionKeywordVar{literal("灯台")},
				varKeywords:        []keywordVar{{Text: "夜", Regexp: false, CaseSensitive: false, Fields: []string{fieldName}}},
			},
			want: map[string][]string{captionLighthouse: {cueLight}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, f.captionMatches(t, 10, nil, tt.filter).texts())
		})
	}

	t.Run("returns the time of the captions", func(t *testing.T) {
		t.Parallel()

		page := f.captionMatches(t, 10, nil, map[string]any{varCaptionKeywords: []captionKeywordVar{literal("カモメ")}})
		require.Len(t, page.Recordings.Edges, 1)
		edge := page.Recordings.Edges[0]
		require.Len(t, edge.CaptionMatches, 1)
		assert.Equal(t, "AUTO", edge.CaptionMatches[0].Source)
		assert.True(t, edge.Node.RecordedFrom.Add(3*time.Second).Equal(edge.CaptionMatches[0].From))
		assert.True(t, edge.Node.RecordedFrom.Add(4*time.Second).Equal(edge.CaptionMatches[0].To))
	})

	t.Run("pages through the matched recordings", func(t *testing.T) {
		t.Parallel()

		filter := map[string]any{varCaptionKeywords: []captionKeywordVar{literal("灯台")}}
		page := f.captionMatches(t, 1, nil, filter)
		assert.Equal(t, []string{captionStale}, page.names())
		assert.True(t, page.Recordings.PageInfo.HasNextPage)
		next := f.captionMatches(t, 1, page.Recordings.PageInfo.EndCursor, filter)
		assert.Equal(t, []string{captionLighthouse}, next.names())
		assert.False(t, next.Recordings.PageInfo.HasNextPage)
	})

	t.Run("returns no captions without the caption keywords", func(t *testing.T) {
		t.Parallel()

		page := f.captionMatches(t, 10, nil, nil)
		require.Len(t, page.Recordings.Edges, 4)
		for _, edge := range page.Recordings.Edges {
			assert.Empty(t, edge.CaptionMatches)
			assert.NotNil(t, edge.CaptionMatches)
		}
	})

	t.Run("rejects an invalid regular expression", func(t *testing.T) {
		t.Parallel()

		filter := map[string]any{varCaptionKeywords: []captionKeywordVar{{Text: "(", Regexp: true, CaseSensitive: false}}}
		assert.Equal(t, []string{codeBadUserInput}, f.query(t, listRecordingCaptionMatches, map[string]any{varFirst: 10, varFilter: filter}, nil))
	})
}

// TestRecordingCaptionMatchesAreLimited は、録画 1 つについて返す当たった字幕を、始まりの順に 20 件までにすることを確認する (#2214)。
func TestRecordingCaptionMatchesAreLimited(t *testing.T) {
	t.Parallel()

	f, dir := newRecordingFixture(t)
	recording := f.addRecording(t, dir, captionHarbor, "completed", time.Hour)
	cues := make([]testCue, 0, 25)
	for i := range 25 {
		cues = append(cues, testCue{StartMs: int64(i) * 1000, EndMs: int64(i)*1000 + 500, Text: fmt.Sprintf("カモメ %d", i)})
	}
	f.addAutoCaptions(t, recording, cues, true)

	page := f.captionMatches(t, 10, nil, map[string]any{varCaptionKeywords: []captionKeywordVar{{Text: "カモメ", Regexp: false, CaseSensitive: false}}})
	texts := page.texts()[captionHarbor]
	require.Len(t, texts, 20)
	assert.Equal(t, "カモメ 0", texts[0])
	assert.Equal(t, "カモメ 19", texts[19])
}
