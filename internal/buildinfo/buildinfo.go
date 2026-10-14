// Package buildinfo は、動いている Kotatsu の版を返す。
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// version は、ビルドのときに ldflags の -X で埋め込む (Dockerfile、.goreleaser.yaml)。
// SemVer のビルドメタデータの形で、版とコミットを 1.2.3+<コミット> のようにつなげる。
var version string

// develVersion は、版を埋め込まず、モジュールの版もないビルド (go run など) の版。
const develVersion = "dev"

// Info は、Kotatsu の版。
type Info struct {
	// Version は、リリースのタグ (v1.2.3 なら 1.2.3)、main のビルドなら edge。
	Version string
	// Revision は、ビルドしたコミット。分からなければ空。
	Revision string
	// Modified は、コミットしていない変更を含むビルドかどうか。埋め込んだときは分からないので false。
	Modified bool
}

// Get は、埋め込んだ版を返す。埋め込まれていない項目は、Go が埋め込む VCS の情報で補う。
func Get() *Info {
	build, ok := debug.ReadBuildInfo()
	if !ok {
		build = nil
	}

	return resolve(version, build)
}

func resolve(embedded string, build *debug.BuildInfo) *Info {
	v, revision, _ := strings.Cut(embedded, "+")
	info := &Info{Version: v, Revision: revision, Modified: false}
	if build == nil {
		build = &debug.BuildInfo{}
	}
	fromVCS := false
	if info.Revision == "" {
		for _, s := range build.Settings {
			switch s.Key {
			case "vcs.revision":
				info.Revision = s.Value
				fromVCS = true
			case "vcs.modified":
				info.Modified = s.Value == "true"
			}
		}
	}
	if info.Version == "" {
		info.Version = develVersion
		// 手元でビルドしたときの Go の擬似版 (v0.0.0-<日時>-<コミット>) は、コミットと重なるので使わない。
		// go install でモジュールの版から入れたときだけ使う。
		if m := build.Main.Version; !fromVCS && m != "" && m != "(devel)" {
			info.Version = m
		}
	}

	return info
}

// String は、版とコミットの先頭 7 文字を、1.2.3 (abcdef0) のように返す。
func (i *Info) String() string {
	if i.Revision == "" {
		return i.Version
	}
	short := i.Revision
	if len(short) > 7 {
		short = short[:7]
	}
	if i.Modified {
		short += "-dirty"
	}

	return i.Version + " (" + short + ")"
}
