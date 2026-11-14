package ts_test

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// sectionOf は、table_id と table_id_extension と section_number を指定して、セクションを作る。
func sectionOf(tableID byte, extension uint16, version, number, last byte, body []byte) []byte {
	s := section(tableID, extension, version, body)
	s[6], s[7] = number, last
	// section_number を変えたので、CRC を付け直す。
	s = s[:len(s)-4]

	return binary.BigEndian.AppendUint32(s, crcMPEG(s))
}

// withLength は、4 ビットの予約と 12 ビットの長さを頭に付ける。
func withLength(b []byte) []byte {
	return append(binary.BigEndian.AppendUint16(nil, 0xF000|uint16(len(b)&0x0FFF)), b...)
}

func descriptor(tag byte, data ...byte) []byte {
	return append([]byte{tag, byte(len(data) & 0xFF)}, data...)
}

// testName は、testNameBytes (サービス名の ARIB の文字列) を読んだ文。
const testName = "テスト"

func testNameBytes() []byte {
	return []byte{0x25, 0x46, 0x25, 0x39, 0x25, 0x48}
}

func TestParseNIT(t *testing.T) {
	t.Parallel()

	stream := func(tsid uint16, descriptors ...[]byte) []byte {
		b := binary.BigEndian.AppendUint16(nil, tsid)
		b = binary.BigEndian.AppendUint16(b, 4)
		all := []byte{}
		for _, d := range descriptors {
			all = append(all, d...)
		}

		return append(b, withLength(all)...)
	}
	body := withLength(descriptor(0x40, 0x41, 0x42)) // ネットワーク名記述子は読まない
	loop := append(
		stream(0x4010,
			// サービスリスト記述子: BS 101 と 102 (テレビ)。
			descriptor(0x41, 0x00, 0x65, 0x01, 0x00, 0x66, 0x01),
			// 衛星分配システム記述子: 11.72748 GHz (10 kHz 単位の BCD)。後ろの項目は読まない。
			descriptor(0x43, 0x01, 0x17, 0x27, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00),
		),
		stream(0x4011)...,
	)
	body = append(body, withLength(loop)...)

	nit, ok := ts.ParseNITForTest(sectionOf(0x40, 4, 3, 1, 1, body))
	require.True(t, ok)
	assert.True(t, nit.Actual)
	assert.Equal(t, uint16(4), nit.NetworkID)
	assert.Equal(t, uint8(3), nit.Version)
	assert.Equal(t, uint8(1), nit.SectionNumber)
	assert.Equal(t, uint8(1), nit.LastSectionNumber)
	require.Len(t, nit.TransportStreams, 2)
	assert.Equal(t, &ts.NITTransportStream{
		TransportStreamID: 0x4010, OriginalNetworkID: 4, FrequencyKHz: 11_727_480,
		Services: []ts.NITService{{ServiceID: 101, ServiceType: 1}, {ServiceID: 102, ServiceType: 1}},
	}, nit.TransportStreams[0])
	assert.Equal(t, &ts.NITTransportStream{TransportStreamID: 0x4011, OriginalNetworkID: 4, FrequencyKHz: 0, Services: []ts.NITService{}}, nit.TransportStreams[1])

	// NIT[other] も読む。
	other, ok := ts.ParseNITForTest(sectionOf(0x41, 6, 0, 0, 0, append(withLength(nil), withLength(nil)...)))
	require.True(t, ok)
	assert.False(t, other.Actual)
	assert.Empty(t, other.TransportStreams)
}

func TestParseSDT(t *testing.T) {
	t.Parallel()

	service := func(id uint16, descriptors ...[]byte) []byte {
		b := binary.BigEndian.AppendUint16(nil, id)
		b = append(b, 0xFC)
		all := []byte{}
		for _, d := range descriptors {
			all = append(all, d...)
		}
		length := withLength(all)
		// running_status と free_CA_mode は、長さの上位 4 ビットに入る。
		length[0] = length[0]&0x0F | 0x80

		return append(b, length...)
	}
	name := testNameBytes()
	// 事業者名も、サービス名と同じ文字列にする。
	serviceDescriptor := append([]byte{0x01, byte(len(name) & 0xFF)}, name...)
	serviceDescriptor = append(append(serviceDescriptor, byte(len(name)&0xFF)), name...)
	body := make([]byte, 0, 64)
	body = append(body, 0x7F, 0xE0, 0xFF)
	body = append(body, service(0x0400, descriptor(0x48, serviceDescriptor...))...)
	// ロゴの対応だけを載せたサービス (サービス記述子がない)。
	body = append(body, service(0x0401, descriptor(0xCF, 0x01, 0x02))...)

	sdt, ok := ts.ParseSDTForTest(sectionOf(0x42, 0x7FE0, 2, 0, 0, body))
	require.True(t, ok)
	assert.True(t, sdt.Actual)
	assert.Equal(t, uint16(0x7FE0), sdt.TransportStreamID)
	assert.Equal(t, uint16(0x7FE0), sdt.OriginalNetworkID)
	require.Len(t, sdt.Services, 2)
	assert.Equal(t, &ts.SDTService{ServiceID: 0x0400, HasServiceDescriptor: true, ServiceType: 1, ProviderName: testName, Name: testName}, sdt.Services[0])
	assert.False(t, sdt.Services[1].HasServiceDescriptor)

	other, ok := ts.ParseSDTForTest(sectionOf(0x46, 0x7FE1, 0, 0, 0, []byte{0x7F, 0xE0, 0xFF}))
	require.True(t, ok)
	assert.False(t, other.Actual)
}

// TestParseNITSDTRejects は、ほかの表、CRC の誤り、長さが収まらないセクションを捨てることを確かめる。
func TestParseNITSDTRejects(t *testing.T) {
	t.Parallel()

	brokenCRC := sectionOf(0x42, 1, 0, 0, 0, []byte{0x7F, 0xE0, 0xFF})
	brokenCRC[len(brokenCRC)-1] ^= 0xFF
	// サービスのループの記述子の長さが、セクションに収まらない。
	overflow := sectionOf(0x42, 1, 0, 0, 0, []byte{0x7F, 0xE0, 0xFF, 0x04, 0x00, 0xFC, 0x80, 0x20})
	tests := []struct {
		name    string
		section []byte
		nit     bool
	}{
		{name: "PAT as NIT", section: sectionOf(0x00, 1, 0, 0, 0, nil), nit: true},
		{name: "PAT as SDT", section: sectionOf(0x00, 1, 0, 0, 0, nil), nit: false},
		{name: "broken CRC", section: brokenCRC, nit: false},
		{name: "descriptors overflow", section: overflow, nit: false},
		{name: "NIT without loops", section: sectionOf(0x40, 1, 0, 0, 0, []byte{0xF0}), nit: true},
		// 衛星分配システム記述子の周波数が BCD でない。
		{
			name: "frequency is not BCD",
			section: sectionOf(0x40, 4, 0, 0, 0, append(withLength(nil), withLength(append(
				[]byte{0x40, 0x10, 0x00, 0x04}, withLength(descriptor(0x43, 0xFF, 0xFF, 0xFF, 0xFF))...))...)),
			nit: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.nit {
				_, ok := ts.ParseNITForTest(tt.section)
				assert.False(t, ok)

				return
			}
			_, ok := ts.ParseSDTForTest(tt.section)
			assert.False(t, ok)
		})
	}
}

// TestAnalyzerNITSDT は、解析器が NIT と SDT のセクションを読み、OnNIT と OnSDT に渡すことを確かめる (#1519)。
// SDT の PID が運ぶ BAT は渡さない。
func TestAnalyzerNITSDT(t *testing.T) {
	t.Parallel()

	var nits []*ts.NIT
	var sdts []*ts.SDT
	analyzer := ts.NewAnalyzer(&ts.AnalyzerOptions{
		Index: nil, CutoffPTS: nil, OnTOT: nil, OnLogo: nil,
		OnNIT: func(n *ts.NIT) { nits = append(nits, n) },
		OnSDT: func(s *ts.SDT) { sdts = append(sdts, s) },
	})
	var nitCC, sdtCC byte
	nit := sectionOf(0x40, 4, 1, 0, 0, append(withLength(nil), withLength(nil)...))
	sdt := sectionOf(0x42, 0x40F1, 1, 0, 0, []byte{0x00, 0x04, 0xFF})
	bat := sectionOf(0x4A, 1, 0, 0, 0, append(withLength(nil), withLength(nil)...))
	for _, p := range append(psiPackets(0x0010, &nitCC, nit), psiPackets(0x0011, &sdtCC, sdt, bat)...) {
		_, err := analyzer.Write(p)
		require.NoError(t, err)
	}

	require.Len(t, nits, 1)
	assert.Equal(t, uint16(4), nits[0].NetworkID)
	require.Len(t, sdts, 1)
	assert.Equal(t, uint16(0x40F1), sdts[0].TransportStreamID)
}
