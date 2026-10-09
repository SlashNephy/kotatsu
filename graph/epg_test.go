package graph_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/tuner/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

// syncEPG は、偽チューナーサーバーの EPG (12:10 の時点、11:00 から 30 分ごとの番組) を DB に取り込む。
func (f *fixture) syncEPG(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(testNow(), nil)))
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)
	_, err = epg.NewReconciler(f.pool, client, clocktest.NewFake(testNow()), func(context.Context, pgx.Tx) error { return nil }).SyncAll(t.Context())
	require.NoError(t, err)
}

const (
	noon             = "2026-10-10T12:00:00Z"
	onePM            = "2026-10-10T13:00:00Z"
	codeBadUserInput = "BAD_USER_INPUT"
	codeNotFound     = "NOT_FOUND"
	codeForbidden    = "FORBIDDEN"
	// fieldName は、番組名を照合するキーワードの項目。
	fieldName = "NAME"
	// testUsername は、newFixture がログインさせるユーザーの名前。
	testUsername = "kotatsu"
	// GraphQL の変数の名前。
	varInput   = "input"
	varFirst   = "first"
	varEnabled = "enabled"
	// testUserAgent は、テストのリクエストの User-Agent。監査ログに残ることを確かめるのに使う。
	testUserAgent = "kotatsu-test/1.0"
)

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message    string `json:"message"`
		Extensions struct {
			Code string `json:"code"`
		} `json:"extensions"`
	} `json:"errors"`
}

// query は、ログイン済みのユーザーとして GraphQL を叩き、data を v に読む。エラーがあれば、そのエラーコードを返す。
func (f *fixture) query(t *testing.T, query string, variables any, v any) []string {
	t.Helper()

	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.server.URL+"/graphql", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentTypeJSON)
	req.Header.Set("Cookie", auth.CookieName+"="+f.token)
	req.Header.Set("User-Agent", testUserAgent)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()

	var decoded gqlResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
	codes := make([]string, 0, len(decoded.Errors))
	for _, e := range decoded.Errors {
		codes = append(codes, e.Extensions.Code)
	}
	if len(codes) == 0 && v != nil {
		require.NoError(t, json.Unmarshal(decoded.Data, v))
	}

	return codes
}

func TestChannelTypesAndServices(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	_, err := f.pool.Exec(t.Context(), "UPDATE services SET hidden = true WHERE name = 'こたつテスト1 サブ'")
	require.NoError(t, err)

	var got struct {
		ChannelTypes []struct {
			Type        string `json:"type"`
			DisplayName string `json:"displayName"`
		} `json:"channelTypes"`
		Visible []struct {
			Name        string `json:"name"`
			ChannelType struct {
				Type string `json:"type"`
			} `json:"channelType"`
		} `json:"visible"`
		All []struct {
			Name string `json:"name"`
		} `json:"all"`
	}
	codes := f.query(t, `{
		channelTypes { type displayName }
		visible: services { name channelType { type } }
		all: services(includeHidden: true) { name }
	}`, nil, &got)
	require.Empty(t, codes)

	assert.Equal(t, []string{"GR", "BS"}, []string{got.ChannelTypes[0].Type, got.ChannelTypes[1].Type})
	assert.Equal(t, "地上波", got.ChannelTypes[0].DisplayName)
	// 地上波を先に、サービス ID の順に並べる。非表示のサービスは、includeHidden を指定したときだけ返す。
	names := make([]string, 0, len(got.Visible))
	for _, s := range got.Visible {
		names = append(names, s.Name+"/"+s.ChannelType.Type)
	}
	assert.Equal(t, []string{"こたつテスト1/GR", "こたつテスト2/GR", "こたつテスト3/GR", "こたつ BS/BS"}, names)
	assert.Len(t, got.All, 5)
}

type guideInput struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Services []string `json:"services"`
}

// guideVariables は、guide の query の変数を作る。
func guideVariables(from, to string, services []string) any {
	return guideInput{From: from, To: to, Services: services}
}

func TestGuide(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)

	type program struct {
		Name    string    `json:"name"`
		StartAt time.Time `json:"startAt"`
		Service struct {
			Name string `json:"name"`
		} `json:"service"`
	}
	var services struct {
		Services []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"services"`
	}
	require.Empty(t, f.query(t, `{ services { id name } }`, nil, &services))

	tests := []struct {
		name      string
		variables any
		wantRows  []string
		// wantPrograms は、最初の列の番組名。
		wantPrograms []string
		wantCodes    []string
	}{
		{
			name:         "all services between 12:00 and 13:00",
			variables:    guideVariables(noon, onePM, nil),
			wantRows:     []string{"こたつテスト1", "こたつテスト1 サブ", "こたつテスト2", "こたつテスト3", "こたつ BS"},
			wantPrograms: []string{programName(3), programName(4)},
		},
		{
			name:         "a program overlapping the start is included",
			variables:    guideVariables("2026-10-10T12:15:00Z", "2026-10-10T12:20:00Z", []string{services.Services[4].ID}),
			wantRows:     []string{"こたつ BS"},
			wantPrograms: []string{"こたつ BS テスト番組 3"},
		},
		{
			name:      "empty services",
			variables: guideVariables(noon, onePM, []string{}),
			wantRows:  []string{},
		},
		{
			name:      "range longer than 7 days",
			variables: guideVariables("2026-10-10T00:00:00Z", "2026-10-17T00:00:01Z", nil),
			wantCodes: []string{codeBadUserInput},
		},
		{
			name:      "from after to",
			variables: guideVariables(onePM, noon, nil),
			wantCodes: []string{codeBadUserInput},
		},
		{
			name:      "invalid service ID",
			variables: guideVariables(noon, onePM, []string{"bogus"}),
			wantCodes: []string{codeBadUserInput},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got struct {
				Guide []struct {
					Service struct {
						Name string `json:"name"`
					} `json:"service"`
					Programs []program `json:"programs"`
				} `json:"guide"`
			}
			codes := f.query(t, `query($from: Time!, $to: Time!, $services: [ID!]) {
				guide(from: $from, to: $to, services: $services) { service { name } programs { name startAt service { name } } }
			}`, tt.variables, &got)
			if tt.wantCodes != nil {
				assert.Equal(t, tt.wantCodes, codes)

				return
			}
			require.Empty(t, codes)
			rows := []string{}
			for _, row := range got.Guide {
				rows = append(rows, row.Service.Name)
			}
			assert.Equal(t, tt.wantRows, rows)
			if len(got.Guide) == 0 {
				return
			}
			programs := []string{}
			for _, p := range got.Guide[0].Programs {
				programs = append(programs, p.Name)
				assert.Equal(t, got.Guide[0].Service.Name, p.Service.Name)
			}
			assert.Equal(t, tt.wantPrograms, programs)
		})
	}
}

func TestNode(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	var guide struct {
		Guide []struct {
			Service struct {
				ID string `json:"id"`
			} `json:"service"`
			Programs []struct {
				ID string `json:"id"`
			} `json:"programs"`
		} `json:"guide"`
	}
	require.Empty(t, f.query(t, `{ guide(from: "2026-10-10T12:00:00Z", to: "2026-10-10T12:30:00Z") { service { id } programs { id } } }`, nil, &guide))

	var got struct {
		Program struct {
			Typename string `json:"__typename"`
			Name     string `json:"name"`
			EventID  int    `json:"eventId"`
		} `json:"program"`
		Service struct {
			Typename string `json:"__typename"`
			Name     string `json:"name"`
		} `json:"service"`
	}
	codes := f.query(t, `query($program: ID!, $service: ID!) {
		program: node(id: $program) { __typename ... on Program { name eventId } }
		service: node(id: $service) { __typename ... on Service { name } }
	}`, map[string]any{"program": guide.Guide[0].Programs[0].ID, "service": guide.Guide[0].Service.ID}, &got)
	require.Empty(t, codes)
	assert.Equal(t, "Program", got.Program.Typename)
	assert.Equal(t, programName(3), got.Program.Name)
	assert.Equal(t, 3, got.Program.EventID)
	assert.Equal(t, "Service", got.Service.Typename)
	assert.Equal(t, "こたつテスト1", got.Service.Name)

	assert.Equal(t, []string{codeNotFound}, f.query(t, `query($id: ID!) { node(id: $id) { id } }`,
		map[string]any{"id": "UHJvZ3JhbTozMjc2MzAxMDI0Ljk5OTk"}, nil))
}
