package ts

// NIT と SDT (ARIB STD-B10 5.2.4、5.2.6)。放送が送る局の顔ぶれ (ネットワーク、トランスポートストリーム、サービス) を読む (#1519)。
const (
	tableIDNITActual = 0x40
	tableIDNITOther  = 0x41
	tableIDSDTActual = 0x42
	tableIDSDTOther  = 0x46

	// 記述子のタグ (ARIB STD-B10 6.1)。
	descriptorServiceList         = 0x41
	descriptorSatelliteDelivery   = 0x43
	descriptorService             = 0x48
	serviceListEntrySize          = 3
	satelliteDeliveryFrequencyLen = 4
)

// NIT は、NIT の 1 つのセクションで読んだ、ネットワークの構成。
type NIT struct {
	// Actual は、自ネットワークの NIT (NIT[actual]) かどうか。
	Actual    bool
	NetworkID uint16
	Version   uint8
	// SectionNumber と LastSectionNumber は、このセクションの番号と、最後のセクションの番号。
	SectionNumber     uint8
	LastSectionNumber uint8
	TransportStreams  []*NITTransportStream
}

// NITTransportStream は、NIT のトランスポートストリームのループの 1 つ。
type NITTransportStream struct {
	TransportStreamID uint16
	OriginalNetworkID uint16
	// FrequencyKHz は、衛星分配システム記述子の周波数 (kHz)。記述子がなければ 0。
	FrequencyKHz int
	// Services は、サービスリスト記述子に並ぶサービス。
	Services []NITService
}

// NITService は、サービスリスト記述子の 1 つのサービス。
type NITService struct {
	ServiceID   uint16
	ServiceType uint8
}

// SDT は、SDT の 1 つのセクションで読んだ、トランスポートストリームのサービス。
type SDT struct {
	// Actual は、自ストリームの SDT (SDT[actual]) かどうか。
	Actual            bool
	TransportStreamID uint16
	OriginalNetworkID uint16
	Version           uint8
	SectionNumber     uint8
	LastSectionNumber uint8
	Services          []*SDTService
}

// SDTService は、SDT のサービスのループの 1 つ。
type SDTService struct {
	ServiceID uint16
	// HasServiceDescriptor は、サービス記述子があったかどうか。ロゴの対応だけを載せた SDT には、サービス記述子がない。
	HasServiceDescriptor bool
	ServiceType          uint8
	ProviderName         string
	Name                 string
}

// parseNIT は、NIT のセクションを読む。NIT でないか、壊れていれば false を返す。
func parseNIT(section []byte) (*NIT, bool) {
	header := parseSectionHeader(section)
	if header == nil || !header.currentNext || (header.tableID != tableIDNITActual && header.tableID != tableIDNITOther) {
		return nil, false
	}
	body := header.body
	networkDescriptors, ok := lengthPrefixed(body)
	if !ok {
		return nil, false
	}
	body = body[2+len(networkDescriptors):]
	loop, ok := lengthPrefixed(body)
	if !ok {
		return nil, false
	}
	nit := &NIT{
		Actual: header.tableID == tableIDNITActual, NetworkID: header.tableIDExtension, Version: header.version,
		SectionNumber: section[6], LastSectionNumber: section[7], TransportStreams: []*NITTransportStream{},
	}
	for len(loop) > 0 {
		if len(loop) < 6 {
			return nil, false
		}
		descriptors, ok := lengthPrefixed(loop[4:])
		if !ok {
			return nil, false
		}
		stream := &NITTransportStream{
			TransportStreamID: uint16(loop[0])<<8 | uint16(loop[1]),
			OriginalNetworkID: uint16(loop[2])<<8 | uint16(loop[3]),
			FrequencyKHz:      0,
			Services:          []NITService{},
		}
		if !eachDescriptor(descriptors, func(tag byte, data []byte) bool {
			switch tag {
			case descriptorServiceList:
				for ; len(data) >= serviceListEntrySize; data = data[serviceListEntrySize:] {
					stream.Services = append(stream.Services, NITService{ServiceID: uint16(data[0])<<8 | uint16(data[1]), ServiceType: data[2]})
				}
			case descriptorSatelliteDelivery:
				if len(data) < satelliteDeliveryFrequencyLen {
					return false
				}
				frequency, ok := bcdDigits(data[:satelliteDeliveryFrequencyLen])
				if !ok {
					return false
				}
				// 周波数は、10 kHz 単位の BCD 8 桁。
				stream.FrequencyKHz = frequency * 10
			}

			return true
		}) {
			return nil, false
		}
		nit.TransportStreams = append(nit.TransportStreams, stream)
		loop = loop[6+len(descriptors):]
	}

	return nit, true
}

// parseSDT は、SDT のセクションを読む。SDT でないか、壊れていれば false を返す。
func parseSDT(section []byte) (*SDT, bool) {
	header := parseSectionHeader(section)
	if header == nil || !header.currentNext || (header.tableID != tableIDSDTActual && header.tableID != tableIDSDTOther) {
		return nil, false
	}
	body := header.body
	// original_network_id の 2 バイトと、reserved_future_use の 1 バイト。
	if len(body) < 3 {
		return nil, false
	}
	sdt := &SDT{
		Actual: header.tableID == tableIDSDTActual, TransportStreamID: header.tableIDExtension,
		OriginalNetworkID: uint16(body[0])<<8 | uint16(body[1]), Version: header.version,
		SectionNumber: section[6], LastSectionNumber: section[7], Services: []*SDTService{},
	}
	loop := body[3:]
	for len(loop) > 0 {
		// service_id の 2 バイト、EIT のフラグの 1 バイト、running_status から descriptors_loop_length までの 2 バイト。
		if len(loop) < 5 {
			return nil, false
		}
		descriptors, ok := lengthPrefixed(loop[3:])
		if !ok {
			return nil, false
		}
		service := &SDTService{ServiceID: uint16(loop[0])<<8 | uint16(loop[1]), HasServiceDescriptor: false, ServiceType: 0, ProviderName: "", Name: ""}
		if !eachDescriptor(descriptors, func(tag byte, data []byte) bool {
			if tag != descriptorService {
				return true
			}
			provider, name, ok := parseServiceDescriptor(data)
			if !ok {
				return false
			}
			service.HasServiceDescriptor = true
			service.ServiceType = data[0]
			service.ProviderName, service.Name = provider, name

			return true
		}) {
			return nil, false
		}
		sdt.Services = append(sdt.Services, service)
		loop = loop[5+len(descriptors):]
	}

	return sdt, true
}

// parseServiceDescriptor は、サービス記述子の中身 (service_type の後) から、事業者名とサービス名を読む。
func parseServiceDescriptor(data []byte) (string, string, bool) {
	if len(data) < 2 {
		return "", "", false
	}
	providerLength := int(data[1])
	if len(data) < 2+providerLength+1 {
		return "", "", false
	}
	provider := data[2 : 2+providerLength]
	rest := data[2+providerLength:]
	nameLength := int(rest[0])
	if len(rest) < 1+nameLength {
		return "", "", false
	}

	return DecodeARIBString(provider), DecodeARIBString(rest[1 : 1+nameLength]), true
}

// lengthPrefixed は、4 ビットの予約と 12 ビットの長さの後に続く、その長さの部分を返す。収まらなければ false を返す。
func lengthPrefixed(b []byte) ([]byte, bool) {
	if len(b) < 2 {
		return nil, false
	}
	length := int(b[0]&0x0F)<<8 | int(b[1])
	if len(b) < 2+length {
		return nil, false
	}

	return b[2 : 2+length], true
}

// eachDescriptor は、記述子の並びを 1 つずつ fn に渡す。並びが壊れているか、fn が false を返したら false を返す。
func eachDescriptor(b []byte, fn func(tag byte, data []byte) bool) bool {
	for len(b) > 0 {
		if len(b) < 2 || len(b) < 2+int(b[1]) {
			return false
		}
		if !fn(b[0], b[2:2+int(b[1])]) {
			return false
		}
		b = b[2+int(b[1]):]
	}

	return true
}

// bcdDigits は、BCD の並びを 10 進の数にする。BCD でなければ false を返す。
func bcdDigits(b []byte) (int, bool) {
	n := 0
	for _, c := range b {
		v, ok := bcd(c)
		if !ok {
			return 0, false
		}
		n = n*100 + v
	}

	return n, true
}
