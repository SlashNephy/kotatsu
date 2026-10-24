package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/graph"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/update"
)

// fakeUpdates は、決まったリリースを返す graph.Updates。
type fakeUpdates struct{ release *update.Release }

var _ graph.Updates = new(fakeUpdates)

func (f *fakeUpdates) Latest() *update.Release { return f.release }

// TestLatestRelease は、新しいリリースを管理者にだけ返し、確かめない設定なら返さないことを確かめる (#506)。
func TestLatestRelease(t *testing.T) {
	t.Parallel()

	release := &update.Release{Version: "1.3.0", URL: "https://github.com/SlashNephy/kotatsu/releases/tag/v1.3.0", PublishedAt: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)}
	tests := []struct {
		name    string
		updates graph.Updates
		role    auth.Role
		want    bool
	}{
		{name: "管理者には返す", updates: &fakeUpdates{release: release}, role: auth.RoleAdmin, want: true},
		{name: "管理者でなければ返さない", updates: &fakeUpdates{release: release}, role: auth.RoleMember, want: false},
		{name: "確かめない設定なら返さない", updates: nil, role: auth.RoleAdmin, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, &graph.Options{Updates: tt.updates})
			var got struct {
				Version struct {
					LatestRelease *struct {
						Version string `json:"version"`
						URL     string `json:"url"`
					} `json:"latestRelease"`
				} `json:"version"`
			}
			require.Empty(t, f.as(t, "someone", tt.role).query(t, `{ version { latestRelease { version url } } }`, nil, &got))
			if !tt.want {
				assert.Nil(t, got.Version.LatestRelease)

				return
			}
			require.NotNil(t, got.Version.LatestRelease)
			assert.Equal(t, release.Version, got.Version.LatestRelease.Version)
			assert.Equal(t, release.URL, got.Version.LatestRelease.URL)
		})
	}
}
