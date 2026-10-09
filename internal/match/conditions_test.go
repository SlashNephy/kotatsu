package match_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
	"github.com/SlashNephy/kotatsu/internal/match"
)

func fullConditions() *match.Conditions {
	return &match.Conditions{
		Keywords: []*match.Keyword{
			{Text: "ゆるキャン", Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldName}},
			{Text: `第\d+話`, Regexp: true, CaseSensitive: true, Fields: []match.Field{match.FieldName, match.FieldDescription, match.FieldExtended}},
		},
		ExcludeKeywords:    []*match.Keyword{{Text: "再放送", Regexp: false, CaseSensitive: false, Fields: []match.Field{match.FieldName}}},
		ChannelTypes:       []string{channelGR, "BS4K"},
		ServiceIDs:         []int64{serviceBS},
		Genres:             []*match.Genre{{Lv1: 7, Lv2: nil}},
		ExcludeGenres:      []*match.Genre{{Lv1: 0, Lv2: new(1)}},
		TimeRanges:         []*match.TimeRange{{Weekdays: []int{0, 6}, StartMinute: 1380, DurationMinutes: 180}},
		MinDurationSeconds: new(600),
		MaxDurationSeconds: new(7200),
		FreeOnly:           true,
	}
}

func TestValidateRejectsInvalidConditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modify  func(c *match.Conditions)
		wantErr string
	}{
		{
			name:    "invalid regular expression",
			modify:  func(c *match.Conditions) { c.Keywords[1].Text = `第(\d+話` },
			wantErr: "keywords[1]: invalid regular expression",
		},
		{
			name:    "empty keyword",
			modify:  func(c *match.Conditions) { c.ExcludeKeywords[0].Text = "" },
			wantErr: "excludeKeywords[0]: text must be 1 to 1000 characters",
		},
		{
			name:    "keyword without fields",
			modify:  func(c *match.Conditions) { c.Keywords[0].Fields = []match.Field{} },
			wantErr: "keywords[0]: fields must not be empty",
		},
		{
			name:    "unknown field",
			modify:  func(c *match.Conditions) { c.Keywords[0].Fields = []match.Field{"title"} },
			wantErr: `keywords[0]: unknown field "title"`,
		},
		{
			name:    "genre out of range",
			modify:  func(c *match.Conditions) { c.Genres[0].Lv1 = 16 },
			wantErr: "genres[0] must have lv1 and lv2 between 0 and 15",
		},
		{
			name:    "weekday out of range",
			modify:  func(c *match.Conditions) { c.TimeRanges[0].Weekdays = []int{7} },
			wantErr: "timeRanges[0]: weekdays must be between 0 (Sunday) and 6 (Saturday)",
		},
		{
			name:    "time range longer than a week",
			modify:  func(c *match.Conditions) { c.TimeRanges[0].DurationMinutes = 7*24*60 + 1 },
			wantErr: "timeRanges[0]: durationMinutes must be between 1 and 10080",
		},
		{
			name:    "minimum duration exceeds maximum",
			modify:  func(c *match.Conditions) { c.MinDurationSeconds = new(7201) },
			wantErr: "minDurationSeconds must not exceed maxDurationSeconds",
		},
		{
			name: "too many time ranges",
			modify: func(c *match.Conditions) {
				for range 1000 {
					c.TimeRanges = append(c.TimeRanges, c.TimeRanges[0])
				}
			},
			wantErr: "timeRanges must have at most 1000 items",
		},
		{
			name:    "empty channel type",
			modify:  func(c *match.Conditions) { c.ChannelTypes = []string{""} },
			wantErr: "channelTypes must not contain an empty string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := fullConditions()
			tt.modify(c)

			_, err := match.Encode(c)
			require.ErrorIs(t, err, match.ErrInvalidConditions)
			require.ErrorContains(t, err, tt.wantErr)
			_, err = match.Compile(c, time.UTC)
			require.ErrorIs(t, err, match.ErrInvalidConditions)
		})
	}
}

func TestNilInputsAreRejected(t *testing.T) {
	t.Parallel()

	_, err := match.Encode(nil)
	require.ErrorIs(t, err, match.ErrInvalidConditions)
	_, err = match.Compile(nil, time.UTC)
	require.ErrorIs(t, err, match.ErrInvalidConditions)
	_, err = match.Compile(fullConditions(), nil)
	require.ErrorContains(t, err, "location must not be nil")
}

func TestDecodeRejectsUnknownData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json string
	}{
		{name: "unknown version", json: `{"version":2,"conditions":{}}`},
		{name: "missing version", json: `{"conditions":{}}`},
		{name: "unknown field", json: `{"version":1,"conditions":{"keyword":"x"}}`},
		{name: "invalid conditions", json: `{"version":1,"conditions":{"keywords":[{"text":"(","regexp":true,"fields":["name"]}]}}`},
		{name: "not an object", json: `[]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := match.Decode([]byte(tt.json))
			require.ErrorIs(t, err, match.ErrInvalidConditions)
		})
	}
}

// TestConditionsRoundTripThroughJSONB は、条件を JSONB に保存して読み戻しても、同じ条件になることを確かめる。
// PostgreSQL は JSONB を正規化して返す (キーの順や空白が変わる) ので、実際の DB を通す。
func TestConditionsRoundTripThroughJSONB(t *testing.T) {
	t.Parallel()

	conn, err := pgx.Connect(t.Context(), databasetest.New(t).URL.Value())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close(t.Context())
	})

	encoded, err := match.Encode(fullConditions())
	require.NoError(t, err)
	var stored []byte
	require.NoError(t, conn.QueryRow(t.Context(), "SELECT $1::jsonb", encoded).Scan(&stored))
	var version int
	require.NoError(t, conn.QueryRow(t.Context(), "SELECT ($1::jsonb ->> 'version')::int", encoded).Scan(&version))
	assert.Equal(t, match.SchemaVersion, version)

	decoded, err := match.Decode(stored)
	require.NoError(t, err)
	assert.Equal(t, fullConditions(), decoded)
}
