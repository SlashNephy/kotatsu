package licenses

import "io/fs"

// LoadFrom は load をテストから使う。
func LoadFrom(fsys fs.FS, linked map[string]bool) (*List, error) {
	return load(fsys, linked)
}
