package ts

import "bytes"

const (
	// cdtPID は、CDT (全受信機共通データ) を運ぶ。
	cdtPID = 0x0029
	// tableIDCDT は、CDT の table_id。
	tableIDCDT = 0xC8
	// cdtDataTypeLogo は、CDT の data_type のうち、ロゴのデータを表す値。
	cdtDataTypeLogo = 0x01
	// logoHeaderSize は、ロゴのデータモジュールの、logo_type から data_size までの大きさ。
	logoHeaderSize = 7
	// pngSignature は、PNG の先頭の 8 バイト。
	pngSignature = "\x89PNG\r\n\x1a\n"
)

// Logo は、CDT で送られるサービスのロゴ (#1308、ARIB STD-B21、TR-B15)。
type Logo struct {
	// NetworkID は、ロゴを送ったネットワークの original_network_id。サービスの logo_id と組んで、ロゴを指す。
	NetworkID uint16
	// LogoID は、ロゴの番号。SDT の logo_transmission_descriptor で、サービスと結ばれる。
	LogoID uint16
	// Type は、ロゴの種類 (logo_type)。大きさを表す。
	Type uint8
	// Version は、ロゴの版 (logo_version)。
	Version uint16
	// PNG は、送られたままの PNG。パレットを持たないことがある (FixPalette)。
	PNG []byte
}

// parseCDTLogo は、CDT のセクションから、ロゴのデータを読む。ロゴでなければ、壊れていれば false を返す。
// セクションのバッファは使い回されるので、PNG は写して返す。
func parseCDTLogo(section []byte) (*Logo, bool) {
	header := parseSectionHeader(section)
	if header == nil || header.tableID != tableIDCDT || !header.currentNext {
		return nil, false
	}
	// original_network_id の 2 バイト、data_type の 1 バイト、descriptors_loop_length の 2 バイト。
	body := header.body
	if len(body) < 5 || body[2] != cdtDataTypeLogo {
		return nil, false
	}
	networkID := uint16(body[0])<<8 | uint16(body[1])
	descriptorsLength := int(body[3]&0x0F)<<8 | int(body[4])
	if len(body) < 5+descriptorsLength+logoHeaderSize {
		return nil, false
	}
	module := body[5+descriptorsLength:]
	size := int(module[5])<<8 | int(module[6])
	data := module[logoHeaderSize:]
	if size > len(data) || !bytes.HasPrefix(data[:size], []byte(pngSignature)) {
		return nil, false
	}

	return &Logo{
		NetworkID: networkID,
		LogoID:    uint16(module[1]&0x01)<<8 | uint16(module[2]),
		Type:      module[0],
		Version:   uint16(module[3]&0x0F)<<8 | uint16(module[4]),
		PNG:       bytes.Clone(data[:size]),
	}, true
}
