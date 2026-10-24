package graph_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			client := jikkyo.NewClient(&jikkyo.ClientOptions{NXJikkyoURL: nx.URL, KakologURL: nx.URL, HTTPClient: nil, Clock: nil})
			f := newFixture(t, &graph.Options{JikkyoTable: table, JikkyoClient: client})
			f.syncEPG(t)

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
