package ts

import (
	"encoding/json"
	"slices"
)

const (
	// tableIDEITPresentFollowingActual は、自 TS の EIT[p/f] の table_id。
	tableIDEITPresentFollowingActual = 0x4E
	// descriptorTagAudioComponent は、音声コンポーネント記述子 (ARIB STD-B10) のタグ。
	descriptorTagAudioComponent = 0xC4
	// eitEventsOffset は、EIT の本体 (ヘッダーの後) のうち、イベントの並びより前の部分の長さ。
	// transport_stream_id、original_network_id、segment_last_section_number、last_table_id。
	eitEventsOffset = 6
	// eitEventHeaderLength は、イベントの、記述子より前の部分の長さ。
	eitEventHeaderLength = 12
	// audioComponentMinLength は、音声コンポーネント記述子の、ISO_639_language_code までの長さ。
	audioComponentMinLength = 9
	// audioComponentMultiLingualLength は、2 つ目の ISO_639_language_code までの長さ。
	audioComponentMultiLingualLength = 12
)

// AudioComponent は、音声コンポーネント記述子が表す、番組の音声 (ARIB STD-B10)。
type AudioComponent struct {
	// ComponentType は component_type。1 はシングルモノ、2 はデュアルモノ、3 はステレオ、9 は 5.1ch など。
	ComponentType uint8
	ComponentTag  uint8
	IsMain        bool
	// SamplingRate は、標本化周波数 (Hz)。分からなければ 0。
	SamplingRate int
	// Languages は、言語のコード (ISO 639-2)。デュアルモノなどの多言語では 2 つになる。
	Languages []string
}

// EventAudio は、TS の EIT[p/f] で放送中とされた番組と、その音声の構成。
type EventAudio struct {
	// ServiceID は、番組のサービス ID (program_number)。
	ServiceID uint16
	EventID   uint16
	Audios    []*AudioComponent
	// Seen は、放送中として読んだ回数。EIT[p/f] は一定の間隔で送られるので、長く放送された番組ほど多い。
	Seen int
}

// eitTable は、EIT[p/f] の放送中の番組から、音声の構成を集める。
type eitTable struct {
	events map[[2]uint16]*EventAudio
}

func newEITTable() *eitTable {
	return &eitTable{events: map[[2]uint16]*EventAudio{}}
}

// handle は、EIT[p/f] (自 TS) のセクションを読む。放送中の番組 (section_number 0) だけを見る。
func (t *eitTable) handle(section []byte) {
	header := parseSectionHeader(section)
	// section_number はヘッダーの 7 バイト目にある。
	if header == nil || header.tableID != tableIDEITPresentFollowingActual || !header.currentNext || section[6] != 0 {
		return
	}
	body := header.body
	if len(body) < eitEventsOffset+eitEventHeaderLength {
		return
	}
	event := body[eitEventsOffset:]
	eventID := uint16(event[0])<<8 | uint16(event[1])
	loopLength := int(event[10]&0x0F)<<8 | int(event[11])
	if eitEventHeaderLength+loopLength > len(event) {
		return
	}
	audios := audioComponents(event[eitEventHeaderLength : eitEventHeaderLength+loopLength])
	key := [2]uint16{header.tableIDExtension, eventID}
	if current := t.events[key]; current != nil {
		current.Seen++
		// 構成は、番組の途中で変わることがあるので、最後に読んだものにする。
		current.Audios = audios

		return
	}
	t.events[key] = &EventAudio{ServiceID: header.tableIDExtension, EventID: eventID, Audios: audios, Seen: 1}
}

// snapshot は、集めた番組を、サービス ID、eventId の順に返す。
func (t *eitTable) snapshot() []*EventAudio {
	events := make([]*EventAudio, 0, len(t.events))
	for _, event := range t.events {
		copied := *event
		copied.Audios = slices.Clone(event.Audios)
		events = append(events, &copied)
	}
	slices.SortFunc(events, func(a, b *EventAudio) int {
		if a.ServiceID != b.ServiceID {
			return int(a.ServiceID) - int(b.ServiceID)
		}

		return int(a.EventID) - int(b.EventID)
	})

	return events
}

// audioComponents は、記述子の並びから、音声コンポーネント記述子を読む。
func audioComponents(descriptors []byte) []*AudioComponent {
	audios := []*AudioComponent{}
	for len(descriptors) >= 2 {
		tag, length := descriptors[0], int(descriptors[1])
		if 2+length > len(descriptors) {
			break
		}
		if body := descriptors[2 : 2+length]; tag == descriptorTagAudioComponent && len(body) >= audioComponentMinLength {
			multiLingual := body[5]&0x80 != 0
			audio := &AudioComponent{
				ComponentType: body[1],
				ComponentTag:  body[2],
				IsMain:        body[5]&0x40 != 0,
				SamplingRate:  samplingRate(body[5] >> 1 & 0x07),
				Languages:     []string{string(body[6:9])},
			}
			if multiLingual && len(body) >= audioComponentMultiLingualLength {
				audio.Languages = append(audio.Languages, string(body[9:12]))
			}
			audios = append(audios, audio)
		}
		descriptors = descriptors[2+length:]
	}

	return audios
}

// samplingRate は、音声コンポーネント記述子の sampling_rate を Hz にする。
func samplingRate(code uint8) int {
	switch code {
	case 1:
		return 16000
	case 2:
		return 22050
	case 3:
		return 24000
	case 5:
		return 32000
	case 6:
		return 44100
	case 7:
		return 48000
	default:
		return 0
	}
}

// PresentAudios は、録画したサービス serviceID の、放送中だった番組の音声の構成を返す。
// eventID があれば、その番組のものを返す。なければ (時刻予約など)、最も長く放送中だった番組のものを返す。見つからなければ nil。
func PresentAudios(events []*EventAudio, serviceID uint16, eventID *uint16) []*AudioComponent {
	var best *EventAudio
	for _, event := range events {
		if event.ServiceID != serviceID || len(event.Audios) == 0 {
			continue
		}
		if eventID != nil {
			if event.EventID == *eventID {
				return event.Audios
			}

			continue
		}
		if best == nil || event.Seen > best.Seen {
			best = event
		}
	}
	if best == nil {
		return nil
	}

	return best.Audios
}

// audioJSON は、Mirakurun の番組の audios の要素と同じ形。録画の音声の構成は、EPG から写したものと同じ形で保存する。
type audioJSON struct {
	ComponentType int      `json:"componentType"`
	ComponentTag  int      `json:"componentTag"`
	IsMain        bool     `json:"isMain"`
	SamplingRate  int      `json:"samplingRate"`
	Langs         []string `json:"langs"`
}

// EncodeAudios は、音声の構成を、Mirakurun の番組の audios と同じ形の JSON にする。
func EncodeAudios(audios []*AudioComponent) ([]byte, error) {
	encoded := make([]audioJSON, 0, len(audios))
	for _, a := range audios {
		encoded = append(encoded, audioJSON{
			ComponentType: int(a.ComponentType), ComponentTag: int(a.ComponentTag), IsMain: a.IsMain, SamplingRate: a.SamplingRate, Langs: a.Languages,
		})
	}

	return json.Marshal(encoded)
}

// serviceIDBase は、Mirakurun のサービス ID (networkId * 100000 + serviceId) から、放送のサービス ID を取り出す基数。
const serviceIDBase = 100000

// RecordingAudios は、Mirakurun のサービス ID serviceKey の録画の、放送中だった番組の音声の構成を、EPG と同じ形の JSON で返す。
// eventID は、番組予約なら番組の eventId で、時刻予約なら nil。見つからなければ、false を返す。
func RecordingAudios(events []*EventAudio, serviceKey int64, eventID *int64) ([]byte, bool, error) {
	var event *uint16
	if eventID != nil && *eventID >= 0 && *eventID <= 0xFFFF {
		event = new(uint16(*eventID))
	}
	sid := serviceKey % serviceIDBase
	if sid < 0 || sid > 0xFFFF {
		return nil, false, nil
	}
	audios := PresentAudios(events, uint16(sid), event)
	if audios == nil {
		return nil, false, nil
	}
	encoded, err := EncodeAudios(audios)
	if err != nil {
		return nil, false, err
	}

	return encoded, true, nil
}
