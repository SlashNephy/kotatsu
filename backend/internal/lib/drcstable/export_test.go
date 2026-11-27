package drcstable

import "io/fs"

// LoadFrom は load をテストから使う。
func LoadFrom(fsys fs.FS) (map[string]string, error) {
	return load(fsys)
}
