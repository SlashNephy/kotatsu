package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

// TestSkippedEpisodes は、同じ回の束ねで見送った番組を、理由と、録った局か代わりに録る放送と一緒に返すことと、
// ルールの同じ回の束ねの設定を保存して返すことを確かめる (#1498)。
func TestSkippedEpisodes(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	const (
		episode3 = "灯台の地図 #3「霧笛」"
		episode4 = "灯台の地図 #4"
	)
	grService := faketuner.ServiceItemID(grNetworkID, grServiceID)
	otherService := faketuner.ServiceItemID(grNetworkID+1, 0x0408)
	bsService := faketuner.ServiceItemID(4, 101)
	// こたつテスト1 の 13:00 と、こたつテスト2 の 13:30 に同じ回を、こたつテスト1 の 14:00 に、BS で録った回を流す。
	for _, p := range []struct {
		serviceID, eventID int64
		name               string
	}{{grService, 5, episode3}, {otherService, 6, "灯台の地図 #3"}, {grService, 7, episode4}} {
		_, err := f.pool.Exec(t.Context(), "UPDATE programs SET name = $3, name_normalized = $3 WHERE service_id = $1 AND event_id = $2", p.serviceID, p.eventID, p.name)
		require.NoError(t, err)
	}
	recordedEnd := testNow().Add(-24 * time.Hour)
	_, err := f.pool.Exec(t.Context(),
		"INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $6)",
		id.New(), bsService, rule.TitleKey(episode4), episode4, recordedEnd.Add(-time.Hour), recordedEnd)
	require.NoError(t, err)

	var created struct {
		CreateRule struct {
			BundleSameEpisodes bool `json:"bundleSameEpisodes"`
		} `json:"createRule"`
	}
	input := map[string]any{
		"name": "灯台の地図", "bundleSameEpisodes": true,
		"conditions": map[string]any{
			"keywords":     []map[string]any{{"text": "灯台の地図", "regexp": false, "caseSensitive": false, "fields": []string{fieldName}}},
			"channelTypes": []string{"GR"},
		},
	}
	require.Empty(t, f.query(t, `mutation($input: RuleInput!) { createRule(input: $input) { bundleSameEpisodes } }`,
		map[string]any{varInput: input}, &created))
	assert.True(t, created.CreateRule.BundleSameEpisodes)

	type program struct {
		Name    string `json:"name"`
		Service struct {
			Name string `json:"name"`
		} `json:"service"`
	}
	type skipped struct {
		Program        program    `json:"program"`
		Reason         string     `json:"reason"`
		LastRecordedAt *time.Time `json:"lastRecordedAt"`
		Recorded       *struct {
			Service *struct {
				Name string `json:"name"`
			} `json:"service"`
			Name  string    `json:"name"`
			EndAt time.Time `json:"endAt"`
		} `json:"recorded"`
		Instead *program `json:"instead"`
	}
	var got struct {
		SkippedDuplicates []skipped `json:"skippedDuplicates"`
	}
	require.Empty(t, f.query(t, `{ skippedDuplicates {
		program { name service { name } } reason lastRecordedAt
		recorded { service { name } name endAt }
		instead { name service { name } }
	} }`, nil, &got))
	require.Len(t, got.SkippedDuplicates, 2)

	other := got.SkippedDuplicates[0]
	assert.Equal(t, "OTHER_BROADCAST", other.Reason)
	assert.Equal(t, "こたつテスト2", other.Program.Service.Name)
	require.NotNil(t, other.Instead)
	assert.Equal(t, episode3, other.Instead.Name)
	assert.Equal(t, "こたつテスト1", other.Instead.Service.Name)
	assert.Nil(t, other.LastRecordedAt)
	assert.Nil(t, other.Recorded)

	recorded := got.SkippedDuplicates[1]
	assert.Equal(t, "RECORDED_EPISODE", recorded.Reason)
	assert.Equal(t, episode4, recorded.Program.Name)
	require.NotNil(t, recorded.Recorded)
	require.NotNil(t, recorded.Recorded.Service)
	assert.Equal(t, "こたつ BS", recorded.Recorded.Service.Name)
	assert.Equal(t, episode4, recorded.Recorded.Name)
	assert.True(t, recordedEnd.Equal(recorded.Recorded.EndAt))
	assert.Nil(t, recorded.Instead)
}
