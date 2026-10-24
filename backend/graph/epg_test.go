package graph_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

// syncEPG は、偽チューナーサーバーの EPG (12:10 の時点、11:00 から 30 分ごとの番組) を DB に取り込む。
func (f *fixture) syncEPG(tb testing.TB) {
	tb.Helper()

	server := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(testNow(), nil)))
	tb.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(tb, err)
	_, err = epg.NewReconciler(f.pool, client, clocktest.NewFake(testNow()), func(context.Context, pgx.Tx) error { return nil }).SyncAll(tb.Context())
	require.NoError(tb, err)
}

const (
	noon             = "2026-10-10T12:00:00Z"
	onePM            = "2026-10-10T13:00:00Z"
	codeBadUserInput = "BAD_USER_INPUT"
	codeNotFound     = "NOT_FOUND"
	codeForbidden    = "FORBIDDEN"
	// fieldName は、番組名を照合するキーワードの項目。
	fieldName = "NAME"
	// keywordMatchAny は、いずれかのキーワードに一致すればよい照合のしかた。
	keywordMatchAny = "ANY"
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
			// Reason は、パスワードの規定に当たった理由。
			Reason string `json:"reason"`
		} `json:"extensions"`
	} `json:"errors"`
}

// query は、ログイン済みのユーザーとして GraphQL を叩き、data を v に読む。エラーがあれば、そのエラーコードを返す。
// extensions.reason があれば、エラーコードに「:理由」を付ける。
func (f *fixture) query(tb testing.TB, query string, variables any, v any) []string {
	tb.Helper()

	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	require.NoError(tb, err)
	req, err := http.NewRequestWithContext(tb.Context(), http.MethodPost, f.server.URL+"/graphql", bytes.NewReader(body))
	require.NoError(tb, err)
	req.Header.Set("Content-Type", contentTypeJSON)
	req.Header.Set("Cookie", auth.CookieName+"="+f.token)
	req.Header.Set("User-Agent", testUserAgent)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(tb, err)
	defer func() {
		_ = resp.Body.Close()
	}()

	var decoded gqlResponse
	require.NoError(tb, json.NewDecoder(resp.Body).Decode(&decoded))
	codes := make([]string, 0, len(decoded.Errors))
	for _, e := range decoded.Errors {
		code := e.Extensions.Code
		if e.Extensions.Reason != "" {
			code += ":" + e.Extensions.Reason
		}
		codes = append(codes, code)
	}
	if len(codes) == 0 && v != nil {
		require.NoError(tb, json.Unmarshal(decoded.Data, v))
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

func TestServiceLogoURL(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)

	var got struct {
		Services []struct {
			NetworkID int64   `json:"networkId"`
			ServiceID int64   `json:"serviceId"`
			LogoURL   *string `json:"logoUrl"`
		} `json:"services"`
	}
	require.Empty(t, f.query(t, `{ services(includeHidden: true) { networkId serviceId logoUrl } }`, nil, &got))
	logos := map[int64]*string{}
	for _, service := range got.Services {
		logos[service.NetworkID*100000+service.ServiceID] = service.LogoURL
	}
	// 偽チューナーサーバーは、こたつテスト1 と BS にロゴの番号 0 を、こたつテスト2 に番号 1 を付ける。
	// 番号が 0 でもロゴはある。こたつテスト2 は、画像をまだ受信していないが、番号はあるので URL を返す。
	// 番号がなければ、URL も返さない。
	assert.Equal(t, map[int64]*string{
		3273601024: new("/media/services/3273601024/logo"),
		3273601025: nil,
		3273701032: new("/media/services/3273701032/logo"),
		3273801040: nil,
		400101:     new("/media/services/400101/logo"),
	}, logos)
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
			ID        string `json:"id"`
			Name      string `json:"name"`
			NetworkID int64  `json:"networkId"`
			ServiceID int64  `json:"serviceId"`
			StreamURL string `json:"streamUrl"`
		} `json:"services"`
	}
	require.Empty(t, f.query(t, `{ services { id name networkId serviceId streamUrl } }`, nil, &services))
	// ライブ視聴の URL は、Mirakurun のサービス ID (network_id * 100000 + service_id) で表す。
	for _, service := range services.Services {
		assert.Equal(t, fmt.Sprintf("/streams/services/%d", service.NetworkID*100000+service.ServiceID), service.StreamURL)
	}

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
			wantRows:     []string{"こたつテスト1", "こたつテスト1 サブ", "こたつテスト2", "こたつテスト3", bsServiceName},
			wantPrograms: []string{programName(3), programName(4)},
		},
		{
			name:         "a program overlapping the start is included",
			variables:    guideVariables("2026-10-10T12:15:00Z", "2026-10-10T12:20:00Z", []string{services.Services[4].ID}),
			wantRows:     []string{bsServiceName},
			wantPrograms: []string{"こたつ BS テスト番組 3"},
		},
		{
			name:      "empty services",
			variables: guideVariables(noon, onePM, []string{}),
			wantRows:  []string{},
		},
		{
			// 1 つのチャンネルの番組表で、EPG が届く 8 日分を一度に引く (#482)。
			name:         "8 days of one service",
			variables:    guideVariables("2026-10-10T00:00:00Z", "2026-10-18T00:00:00Z", []string{services.Services[4].ID}),
			wantRows:     []string{bsServiceName},
			wantPrograms: bsProgramNames(14),
		},
		{
			name:      "range longer than 8 days and 1 hour",
			variables: guideVariables("2026-10-10T00:00:00Z", "2026-10-18T01:00:01Z", nil),
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

// TestProgramAudios は、番組の音声の構成を、主音声を先にして返すことを確かめる (#284)。
func TestProgramAudios(t *testing.T) {
	t.Parallel()

	type audio struct {
		ComponentType int      `json:"componentType"`
		IsMain        bool     `json:"isMain"`
		Languages     []string `json:"languages"`
		SamplingRate  int      `json:"samplingRate"`
	}
	tests := []struct {
		name   string
		stored string
		want   []audio
	}{
		{name: "unknown", stored: `[]`, want: []audio{}},
		{
			// mirakc は、主音声でない音声を先に返すことがあり、componentTag も返さない。
			name: "main audio comes first",
			stored: `[{"componentType":3,"isMain":false,"samplingRate":48000,"langs":["eng"]},` +
				`{"componentType":3,"isMain":true,"samplingRate":48000,"langs":["jpn"]}]`,
			want: []audio{
				{ComponentType: 3, IsMain: true, Languages: []string{"jpn"}, SamplingRate: 48000},
				{ComponentType: 3, IsMain: false, Languages: []string{"eng"}, SamplingRate: 48000},
			},
		},
		{
			name:   "dual mono",
			stored: `[{"componentType":2,"componentTag":16,"isMain":true,"samplingRate":48000,"langs":["jpn","eng"]}]`,
			want:   []audio{{ComponentType: 2, IsMain: true, Languages: []string{"jpn", "eng"}, SamplingRate: 48000}},
		},
		{
			name:   "no languages",
			stored: `[{"componentType":3,"isMain":true,"samplingRate":48000}]`,
			want:   []audio{{ComponentType: 3, IsMain: true, Languages: []string{}, SamplingRate: 48000}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			f.syncEPG(t)
			_, err := f.pool.Exec(t.Context(), `UPDATE programs SET audios = $1 WHERE event_id = 3`, tt.stored)
			require.NoError(t, err)
			var got struct {
				Guide []struct {
					Programs []struct {
						Audios []audio `json:"audios"`
					} `json:"programs"`
				} `json:"guide"`
			}
			require.Empty(t, f.query(t, `{ guide(from: "2026-10-10T12:00:00Z", to: "2026-10-10T12:30:00Z") { programs { audios {
				componentType isMain languages samplingRate } } } }`, nil, &got))
			require.NotEmpty(t, got.Guide)
			require.NotEmpty(t, got.Guide[0].Programs)
			assert.Equal(t, tt.want, got.Guide[0].Programs[0].Audios)
		})
	}
}

func TestProgramSharedFrom(t *testing.T) {
	t.Parallel()

	// こたつテスト1 (0x0400) とそのサブチャンネル (0x0401) の、Mirakurun のサービス ID。
	const (
		mainKey = 0x7FE0*100000 + 0x0400
		subKey  = 0x7FE0*100000 + 0x0401
	)
	tests := []struct {
		name string
		// mainItems と subItems は、メインとサブの番組に入れる関連する番組。
		mainItems string
		subItems  string
		// want は、メインとサブの番組の sharedFrom のサービス名。null なら空。
		want []string
	}{
		{name: "not shared", mainItems: `[]`, subItems: `[]`, want: []string{"", ""}},
		{
			// メインは共有する全てのサービスを挙げ、サブはメインだけを挙げる。
			name:      "sub channel shares the event of the main channel",
			mainItems: `[{"type":"shared","serviceId":1024,"eventId":3},{"type":"shared","serviceId":1025,"eventId":3}]`,
			subItems:  `[{"type":"shared","serviceId":1024,"eventId":3}]`,
			want:      []string{"", "こたつテスト1"},
		},
		{
			name:      "shared with another network",
			mainItems: `[]`,
			subItems:  `[{"type":"shared","networkId":4,"serviceId":101,"eventId":3}]`,
			want:      []string{"", ""},
		},
		{
			name:      "relay is not sharing",
			mainItems: `[]`,
			subItems:  `[{"type":"relay","serviceId":1024,"eventId":3}]`,
			want:      []string{"", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			f.syncEPG(t)
			for key, items := range map[int64]string{mainKey: tt.mainItems, subKey: tt.subItems} {
				_, err := f.pool.Exec(t.Context(), `UPDATE programs SET related_items = $1 WHERE service_id = $2`, items, key)
				require.NoError(t, err)
			}
			var got struct {
				Guide []struct {
					Programs []struct {
						SharedFrom *struct {
							Name string `json:"name"`
						} `json:"sharedFrom"`
					} `json:"programs"`
				} `json:"guide"`
			}
			require.Empty(t, f.query(t, `query($services: [ID!]) {
				guide(from: "2026-10-10T12:00:00Z", to: "2026-10-10T12:30:00Z", services: $services) { programs { sharedFrom { name } } }
			}`, map[string]any{"services": []string{
				graph.GlobalID("Service", strconv.FormatInt(mainKey, 10)), graph.GlobalID("Service", strconv.FormatInt(subKey, 10)),
			}}, &got))
			names := make([]string, 0, len(got.Guide))
			for _, row := range got.Guide {
				require.NotEmpty(t, row.Programs)
				name := ""
				if from := row.Programs[0].SharedFrom; from != nil {
					name = from.Name
				}
				names = append(names, name)
			}
			assert.Equal(t, tt.want, names)
		})
	}
}

func TestServicesExcludeNonMediaServices(t *testing.T) {
	t.Parallel()

	// こたつテスト1 サブ (0x0401) を、ワンセグのようなデータ放送のサービス (0xC0) にする。
	const subKey = 0x7FE0*100000 + 0x0401
	subID := graph.GlobalID("Service", strconv.FormatInt(subKey, 10))
	tests := []struct {
		name      string
		query     string
		variables any
		want      bool
	}{
		{name: "services", query: `{ services { id } }`, want: false},
		{name: "services including hidden ones", query: `{ services(includeHidden: true) { id } }`, want: true},
		{
			name:      "guide",
			query:     `query { services: guide(from: "2026-10-10T12:00:00Z", to: "2026-10-10T12:30:00Z") { service { id } } }`,
			variables: nil,
			want:      false,
		},
		{
			name:      "guide of the specified service",
			query:     `query($ids: [ID!]) { services: guide(from: "2026-10-10T12:00:00Z", to: "2026-10-10T12:30:00Z", services: $ids) { service { id } } }`,
			variables: map[string]any{"ids": []string{subID}},
			want:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, nil)
			f.syncEPG(t)
			_, err := f.pool.Exec(t.Context(), `UPDATE services SET service_type = 192 WHERE id = $1`, subKey)
			require.NoError(t, err)
			var got struct {
				Services []struct {
					ID      string `json:"id"`
					Service struct {
						ID string `json:"id"`
					} `json:"service"`
				} `json:"services"`
			}
			require.Empty(t, f.query(t, tt.query, tt.variables, &got))
			require.NotEmpty(t, got.Services)
			found := false
			for _, s := range got.Services {
				if s.ID == subID || s.Service.ID == subID {
					found = true
				}
			}
			assert.Equal(t, tt.want, found)
		})
	}
}

// bsServiceName は、偽チューナーの BS のサービスの名前。
const bsServiceName = "こたつ BS"

// bsProgramNames は、偽チューナーの こたつ BS の、eventId 1 から n までの番組名を返す。
func bsProgramNames(n int) []string {
	names := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		names = append(names, fmt.Sprintf("%s テスト番組 %d", bsServiceName, i))
	}

	return names
}

const (
	serviceName1 = "こたつテスト1"
	serviceName2 = "こたつテスト2"
	varPinned    = "pinned"
)

// TestPinnedServices は、サービスのピン留めを、ユーザーごとに、ピン留めした順に覚えることを確かめる (#485)。
func TestPinnedServices(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.syncEPG(t)
	services := f.serviceIDs(t)
	first, second := services[serviceName2], services[serviceName1]
	const (
		setPinned  = `mutation($id: ID!, $pinned: Boolean!) { setServicePinned(id: $id, pinned: $pinned) { name } }`
		listPinned = `{ pinnedServices { name } }`
	)
	type service struct {
		Name string `json:"name"`
	}
	names := func(services []service) []string {
		out := make([]string, 0, len(services))
		for _, s := range services {
			out = append(out, s.Name)
		}

		return out
	}
	var set struct {
		SetServicePinned []service `json:"setServicePinned"`
	}
	for _, id := range []string{first, second, first} {
		require.Empty(t, f.query(t, setPinned, map[string]any{varID: id, varPinned: true}, &set))
		f.clock.Advance(time.Second)
	}
	assert.Equal(t, []string{serviceName2, serviceName1}, names(set.SetServicePinned), "pinning twice keeps the first position")

	// ほかのユーザーのピン留めは、別に持つ。
	var other struct {
		PinnedServices []service `json:"pinnedServices"`
	}
	require.Empty(t, f.as(t, "other", auth.RoleViewer).query(t, listPinned, nil, &other))
	assert.Empty(t, other.PinnedServices)

	require.Empty(t, f.query(t, setPinned, map[string]any{varID: first, varPinned: false}, &set))
	var got struct {
		PinnedServices []service `json:"pinnedServices"`
	}
	require.Empty(t, f.query(t, listPinned, nil, &got))
	assert.Equal(t, []string{serviceName1}, names(got.PinnedServices))

	assert.Equal(t, []string{codeNotFound}, f.query(t, setPinned, map[string]any{varID: graph.GlobalID("Service", "999"), varPinned: true}, nil))
}
