package graph_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/jikkyoclient"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/jikkyo"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

// TestServiceJikkyo は、サービスの実況のチャンネルを、対応表と NX-Jikkyo のチャンネルの一覧から返すことを確かめる。
func TestServiceJikkyo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// channels は、NX-Jikkyo のチャンネルの一覧の本文。空なら NX-Jikkyo に届かないことにする。
		channels string
		// want は、実況のチャンネルを返すサービスの、放送のサービス ID からの対応。
		want map[int]string
	}{
		{
			name:     "対応表で見つかり、NX-Jikkyo が扱うチャンネルだけを返す",
			channels: `[{"id":"jk1","name":"NHK総合"},{"id":"jk101","name":"NHK BS"}]`,
			// サブチャンネル (0x0401) は、1 つ前のメインチャンネルに寄せる。0x0408 (jk2) と 0x0410 (jk4) は、NX-Jikkyo が扱わない。
			want: map[int]string{0x0400: "jk1", 0x0401: "jk1", 101: "jk101"},
		},
		{
			name:     "NX-Jikkyo に届かなければ、エラーにせずどれも null にする",
			channels: "",
			want:     map[int]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.channels == "" {
					w.WriteHeader(http.StatusServiceUnavailable)

					return
				}
				_, _ = w.Write([]byte(tt.channels))
			}))
			t.Cleanup(nx.Close)
			table, err := jikkyo.NewTable(nil)
			require.NoError(t, err)
			client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{NXJikkyoURL: nx.URL, KakologURL: nx.URL, HTTPClient: httpclienttest.New(t), Clock: nil})
			f := newFixture(t, &graph.Options{JikkyoTable: table, JikkyoClient: client})
			f.syncEPG(t)
			enableJikkyo(t, f)

			var got struct {
				Services []struct {
					ServiceID int `json:"serviceId"`
					Jikkyo    *struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"jikkyo"`
				} `json:"services"`
			}
			require.Empty(t, f.query(t, `{ services(includeHidden: true) { serviceId jikkyo { id name } } }`, nil, &got))
			require.Len(t, got.Services, 5)
			ids := map[int]string{}
			for _, service := range got.Services {
				if service.Jikkyo != nil {
					ids[service.ServiceID] = service.Jikkyo.ID
				}
			}
			assert.Equal(t, tt.want, ids)
		})
	}
}

// enableJikkyo は、全体の設定とユーザーの設定の両方で実況を有効にする。
func enableJikkyo(t *testing.T, f *fixture) {
	t.Helper()

	require.Empty(t, f.as(t, "jikkyo-admin", auth.RoleAdmin).query(t, `mutation { updateSystemSettings(input: {jikkyoEnabled: true}) { jikkyoEnabled } }`, nil, nil))
	require.Empty(t, f.query(t, `mutation { updatePlayerSettings(input: {jikkyoEnabled: true}) { jikkyoEnabled } }`, nil, nil))
}

// TestServiceJikkyoOptIn は、全体の設定とユーザーの設定の両方で有効なときだけ NX-Jikkyo へ問い合わせ、
// ID は全体の設定だけで決まることを確かめる (設計書 0.3、7.4)。
func TestServiceJikkyoOptIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		system       bool
		user         bool
		wantJikkyo   bool
		wantChannel  bool
		wantRequests bool
	}{
		{name: "どちらも無効", system: false, user: false, wantJikkyo: false, wantChannel: false, wantRequests: false},
		{name: "全体の設定だけが有効なら、ボタンのための ID だけを返す", system: true, user: false, wantJikkyo: false, wantChannel: true, wantRequests: false},
		{name: "ユーザーの設定だけが有効", system: false, user: true, wantJikkyo: false, wantChannel: false, wantRequests: false},
		{name: "両方が有効", system: true, user: true, wantJikkyo: true, wantChannel: true, wantRequests: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int64
			nx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				_, _ = w.Write([]byte(`[{"id":"jk1","name":"NHK総合"}]`))
			}))
			t.Cleanup(nx.Close)
			table, err := jikkyo.NewTable(nil)
			require.NoError(t, err)
			client := jikkyoclient.NewClient(&jikkyoclient.ClientOptions{NXJikkyoURL: nx.URL, KakologURL: nx.URL, HTTPClient: httpclienttest.New(t), Clock: nil})
			f := newFixture(t, &graph.Options{JikkyoTable: table, JikkyoClient: client})
			f.syncEPG(t)
			if tt.system {
				require.Empty(t, f.as(t, "jikkyo-admin", auth.RoleAdmin).query(t, `mutation { updateSystemSettings(input: {jikkyoEnabled: true}) { jikkyoEnabled } }`, nil, nil))
			}
			if tt.user {
				require.Empty(t, f.query(t, `mutation { updatePlayerSettings(input: {jikkyoEnabled: true}) { jikkyoEnabled } }`, nil, nil))
			}

			var got struct {
				Services []struct {
					ServiceID       int     `json:"serviceId"`
					JikkyoChannelID *string `json:"jikkyoChannelId"`
					Jikkyo          *struct {
						ID string `json:"id"`
					} `json:"jikkyo"`
				} `json:"services"`
			}
			require.Empty(t, f.query(t, `{ services(includeHidden: true) { serviceId jikkyoChannelId jikkyo { id } } }`, nil, &got))
			for _, service := range got.Services {
				if service.ServiceID != 0x0400 {
					continue
				}
				assert.Equal(t, tt.wantJikkyo, service.Jikkyo != nil)
				assert.Equal(t, tt.wantChannel, service.JikkyoChannelID != nil)
			}
			assert.Equal(t, tt.wantRequests, requests.Load() > 0)
		})
	}
}
