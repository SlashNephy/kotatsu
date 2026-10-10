package cli

// AdvertiseAddress は、テストから advertiseAddress を呼ぶために公開する。
func AdvertiseAddress(listen, advertiseHost string) (string, error) {
	return advertiseAddress(listen, advertiseHost)
}
