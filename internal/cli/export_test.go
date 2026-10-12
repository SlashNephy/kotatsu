package cli

// AdvertiseAddress は、テストから advertiseAddress を呼ぶために公開する。
func AdvertiseAddress(listen, advertiseHost string) (string, error) {
	return advertiseAddress(listen, advertiseHost)
}

// SourceWithPassword は、テストから sourceWithPassword を呼ぶために公開する。
func SourceWithPassword(source string, env map[string]string) (string, error) {
	return sourceWithPassword(source, func(key string) (string, bool) {
		value, ok := env[key]

		return value, ok
	})
}
