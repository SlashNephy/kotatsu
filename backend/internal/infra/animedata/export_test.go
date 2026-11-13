package animedata

// ParseChannelDefinitions は、テストから、埋め込んだもの以外の定義を読ませる。
func ParseChannelDefinitions(data []byte, etag string) ([]*Channel, string, error) {
	return parseChannelDefinitions(data, etag)
}
