package ts_test

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/ts"
)

const (
	testServiceID = 1024
	langJapanese  = "jpn"
	langEnglish   = "eng"
)

// audioDescriptor は、音声コンポーネント記述子を作る。languages が 2 つなら、多言語 (ES_multi_lingual_flag) にする。
func audioDescriptor(componentType, tag byte, main bool, languages ...string) []byte {
	flags := byte(7 << 1) // sampling_rate は 48kHz。
	if len(languages) > 1 {
		flags |= 0x80
	}
	if main {
		flags |= 0x40
	}
	body := []byte{0xF2, componentType, tag, 0x0F, 0x00, flags}
	for _, language := range languages {
		body = append(body, language...)
	}

	length := len(body)
	if length > 0xFF {
		panic("the descriptor is too long")
	}

	return append([]byte{0xC4, byte(length)}, body...)
}

// eitSection は、サービス testServiceID の EIT[p/f] (自 TS) のセクションを作る。sectionNumber 0 が放送中の番組。
func eitSection(sectionNumber byte, eventID uint16, descriptors ...[]byte) []byte {
	loop := []byte{}
	for _, d := range descriptors {
		loop = append(loop, d...)
	}
	// transport_stream_id、original_network_id、segment_last_section_number、last_table_id
	body := []byte{0x7F, 0xE0, 0x7F, 0xE0, 0x01, 0x4E}
	body = binary.BigEndian.AppendUint16(body, eventID)
	// start_time と duration は読まないので、適当な値にする。
	body = append(body, 0xE8, 0x1F, 0x23, 0x00, 0x00, 0x00, 0x30, 0x00)
	length := len(loop)
	if length > 0x0FFF {
		panic("the descriptors are too long")
	}
	body = binary.BigEndian.AppendUint16(body, 0x8000|uint16(length))
	body = append(body, loop...)
	s := section(0x4E, testServiceID, 1, body)
	// section() は section_number を 0 にするので、ここで変えて CRC を付け直す。
	s = s[:len(s)-4]
	s[6] = sectionNumber
	s[7] = 1

	return binary.BigEndian.AppendUint32(s, crcMPEG(s))
}

// TestAnalyzerEventAudios は、EIT[p/f] の放送中の番組の音声コンポーネント記述子から、音声の構成を読むことを確かめる (#311)。
func TestAnalyzerEventAudios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sections [][]byte
		want     []*ts.EventAudio
	}{
		{
			name: "二か国語放送 (音声が 2 つ)",
			sections: [][]byte{
				eitSection(0, 100, audioDescriptor(3, 0x10, true, langJapanese), audioDescriptor(3, 0x11, false, langEnglish)),
			},
			want: []*ts.EventAudio{{ServiceID: testServiceID, EventID: 100, Seen: 1, Audios: []*ts.AudioComponent{
				{ComponentType: 3, ComponentTag: 0x10, IsMain: true, SamplingRate: 48000, Languages: []string{langJapanese}},
				{ComponentType: 3, ComponentTag: 0x11, IsMain: false, SamplingRate: 48000, Languages: []string{langEnglish}},
			}}},
		},
		{
			name:     "デュアルモノ",
			sections: [][]byte{eitSection(0, 100, audioDescriptor(2, 0x10, true, langJapanese, langEnglish))},
			want: []*ts.EventAudio{{ServiceID: testServiceID, EventID: 100, Seen: 1, Audios: []*ts.AudioComponent{
				{ComponentType: 2, ComponentTag: 0x10, IsMain: true, SamplingRate: 48000, Languages: []string{langJapanese, langEnglish}},
			}}},
		},
		{
			name: "放送中の番組を数え、次の番組は読まない",
			sections: [][]byte{
				eitSection(0, 100, audioDescriptor(3, 0x10, true, langJapanese)),
				eitSection(1, 101, audioDescriptor(2, 0x10, true, langJapanese, langEnglish)),
				eitSection(0, 100, audioDescriptor(3, 0x10, true, langJapanese)),
			},
			want: []*ts.EventAudio{{ServiceID: testServiceID, EventID: 100, Seen: 2, Audios: []*ts.AudioComponent{
				{ComponentType: 3, ComponentTag: 0x10, IsMain: true, SamplingRate: 48000, Languages: []string{langJapanese}},
			}}},
		},
		{
			name:     "音声コンポーネント記述子がない",
			sections: [][]byte{eitSection(0, 100, []byte{0x4D, 0x02, 0x00, 0x00})},
			want:     []*ts.EventAudio{{ServiceID: testServiceID, EventID: 100, Seen: 1, Audios: []*ts.AudioComponent{}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			analyzer := ts.NewAnalyzer(nil)
			cc := byte(0)
			for _, p := range psiPackets(0x12, &cc, tt.sections...) {
				_, err := analyzer.Write(p)
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, analyzer.Stats().Events)
		})
	}
}

// TestRecordingAudios は、録画した番組の音声の構成を選ぶことを確かめる。番組予約は eventId で、時刻予約は最も長く放送中だった番組で選ぶ。
func TestRecordingAudios(t *testing.T) {
	t.Parallel()

	stereo := []*ts.AudioComponent{{ComponentType: 3, ComponentTag: 0x10, IsMain: true, SamplingRate: 48000, Languages: []string{langJapanese}}}
	dualMono := []*ts.AudioComponent{{ComponentType: 2, ComponentTag: 0x10, IsMain: true, SamplingRate: 48000, Languages: []string{langJapanese, langEnglish}}}
	events := []*ts.EventAudio{
		{ServiceID: testServiceID, EventID: 100, Audios: stereo, Seen: 3},
		{ServiceID: testServiceID, EventID: 101, Audios: dualMono, Seen: 10},
		{ServiceID: 1025, EventID: 200, Audios: stereo, Seen: 50},
	}
	const serviceKey = 32736*100000 + testServiceID
	tests := []struct {
		name    string
		service int64
		eventID *int64
		want    string
	}{
		{
			name: "番組予約は eventId で選ぶ", service: serviceKey, eventID: new(int64(100)),
			want: `[{"componentType":3,"componentTag":16,"isMain":true,"samplingRate":48000,"langs":["jpn"]}]`,
		},
		{
			name: "時刻予約は最も長く放送中だった番組で選ぶ", service: serviceKey, eventID: nil,
			want: `[{"componentType":2,"componentTag":16,"isMain":true,"samplingRate":48000,"langs":["jpn","eng"]}]`,
		},
		{name: "番組が見つからない", service: serviceKey, eventID: new(int64(999)), want: ""},
		{name: "サービスが違う", service: 32736*100000 + 1032, eventID: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok, err := ts.RecordingAudios(events, tt.service, tt.eventID)
			require.NoError(t, err)
			if tt.want == "" {
				assert.False(t, ok)

				return
			}
			require.True(t, ok)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}
