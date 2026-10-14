package buildinfo

import "runtime/debug"

// Resolve は、テストから resolve を呼ぶために公開する。
func Resolve(embedded string, build *debug.BuildInfo) *Info {
	return resolve(embedded, build)
}
