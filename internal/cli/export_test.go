package cli

// AdvertiseAddress は、テストから advertiseAddress を呼ぶために公開する。
func AdvertiseAddress(listen string) (string, error) {
	return advertiseAddress(listen)
}
