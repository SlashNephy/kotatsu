package buildinfo_test

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/SlashNephy/kotatsu/internal/buildinfo"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	const (
		embeddedRevision = "fedcba9876543210fedcba9876543210fedcba98"
		vcsRevision      = "0123456789abcdef0123456789abcdef01234567"
		moduleVersion    = "v1.0.0"
		devel            = "dev"
	)
	vcs := &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: vcsRevision},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	tests := []struct {
		name     string
		embedded string
		build    *debug.BuildInfo
		want     *buildinfo.Info
		wantStr  string
	}{
		{
			name: "埋め込んだ版とコミットを使う", embedded: "1.2.3+" + embeddedRevision, build: vcs,
			want:    &buildinfo.Info{Version: "1.2.3", Revision: embeddedRevision, Modified: false},
			wantStr: "1.2.3 (fedcba9)",
		},
		{
			name: "埋め込んだ版にコミットがなければ、VCS の情報で補う", embedded: "edge", build: vcs,
			want:    &buildinfo.Info{Version: "edge", Revision: vcsRevision, Modified: true},
			wantStr: "edge (0123456-dirty)",
		},
		{
			name: "埋め込まれていなければ、VCS の情報で補う", embedded: "", build: vcs,
			want:    &buildinfo.Info{Version: devel, Revision: vcsRevision, Modified: true},
			wantStr: "dev (0123456-dirty)",
		},
		{
			name: "手元のビルドの擬似版は使わない", embedded: "",
			build: &debug.BuildInfo{
				Main:     debug.Module{Version: "v0.0.0-20261014140841-0123456789ab+dirty"},
				Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: vcsRevision}, {Key: "vcs.modified", Value: "true"}},
			},
			want:    &buildinfo.Info{Version: devel, Revision: vcsRevision, Modified: true},
			wantStr: "dev (0123456-dirty)",
		},
		{
			name: "go install したモジュールの版を使う", embedded: "", build: &debug.BuildInfo{Main: debug.Module{Version: moduleVersion}},
			want:    &buildinfo.Info{Version: moduleVersion, Revision: "", Modified: false},
			wantStr: moduleVersion,
		},
		{
			name: "ビルドの情報がなければ dev", embedded: "", build: nil,
			want:    &buildinfo.Info{Version: devel, Revision: "", Modified: false},
			wantStr: devel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildinfo.Resolve(tt.embedded, tt.build)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantStr, got.String())
		})
	}
}
