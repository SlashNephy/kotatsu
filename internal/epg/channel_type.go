package epg

// channelTypeDefaults は、チャンネル種別の表示名と並び順の既定値。
type channelTypeDefaults struct {
	displayName string
	sortOrder   int32
}

// otherChannelTypeOrder は、既定値の無い種別の並び順。既定値のある種別より後ろに並べる。
const otherChannelTypeOrder = 1000

// defaultChannelType は、種別の表示名と並び順の既定値を返す (設計書 3.1)。
// GR、BS、CS、SKY のほかは、種別の文字列をそのまま表示名にする。
func defaultChannelType(channelType string) channelTypeDefaults {
	switch channelType {
	case "GR":
		return channelTypeDefaults{displayName: "地上波", sortOrder: 10}
	case "BS":
		return channelTypeDefaults{displayName: "BS", sortOrder: 20}
	case "CS":
		return channelTypeDefaults{displayName: "CS", sortOrder: 30}
	case "SKY":
		return channelTypeDefaults{displayName: "スカパー！", sortOrder: 40}
	default:
		return channelTypeDefaults{displayName: channelType, sortOrder: otherChannelTypeOrder}
	}
}
