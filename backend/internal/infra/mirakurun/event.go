package mirakurun

import (
	"encoding/json"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
)

// serviceIDBase は、Mirakurun のサービス ID と番組 ID を組み立てる基数 (networkId × 100000 + serviceId)。
const serviceIDBase = 100_000

// toEvent は、Mirakurun、Mahiron と mirakc のイベントを、何が変わったかの知らせに読み替える。
// Kotatsu が使わないイベントと、形の分からないイベントでは false を返す。取りこぼしは全量の突き合わせで埋まる (設計書 3.3)。
//
// Mirakurun と Mahiron の program のイベントは、create と update では番組を、remove では番組の ID だけを送る。
// mirakc の epg.programs-updated と onair.program-changed は、番組が変わったサービスの ID を送る。
// service と tuner のイベントは、種類を問わず、サービスかチューナーが変わったことにする。
func toEvent(raw *RawEvent) (*tunerserver.Event, bool) {
	switch {
	case raw.Resource == "program":
		serviceID, ok := programServiceID(raw.Data)
		if !ok {
			return nil, false
		}

		return &tunerserver.Event{Type: programEventType(raw.Type), ServiceID: serviceID}, true
	// mirakc の onair.program-changed は、放送中か次の番組 (EIT[p/f]) が変わったサービスの ID を送る (設計書 5.1)。
	case raw.Resource == "epg" && raw.Type == "programs-updated",
		raw.Resource == "onair" && raw.Type == "program-changed":
		var data struct {
			ServiceID int64 `json:"serviceId"`
		}
		if err := json.Unmarshal(raw.Data, &data); err != nil || data.ServiceID == 0 {
			return nil, false
		}

		return &tunerserver.Event{Type: tunerserver.EventTypeProgramUpdated, ServiceID: data.ServiceID}, true
	case raw.Resource == "service":
		return &tunerserver.Event{Type: tunerserver.EventTypeServiceChanged, ServiceID: 0}, true
	case raw.Resource == "tuner":
		return &tunerserver.Event{Type: tunerserver.EventTypeTunerChanged, ServiceID: 0}, true
	default:
		return nil, false
	}
}

// programEventType は、Mirakurun の program のイベントの種類を読み替える。知らない種類は、番組が変わったことにする。
func programEventType(eventType string) tunerserver.EventType {
	switch eventType {
	case "create":
		return tunerserver.EventTypeProgramCreated
	case "remove":
		return tunerserver.EventTypeProgramRemoved
	default:
		return tunerserver.EventTypeProgramUpdated
	}
}

// programServiceID は、Mirakurun の program のイベントの中身から、番組が変わったサービスの ID を求める。
func programServiceID(data json.RawMessage) (int64, bool) {
	var program struct {
		ID        int64 `json:"id"`
		NetworkID int64 `json:"networkId"`
		ServiceID int64 `json:"serviceId"`
	}
	if err := json.Unmarshal(data, &program); err != nil {
		return 0, false
	}
	// サービスの番号は networkId と組でしか一意にならないので、片方しか無ければ番組 ID から求める。
	switch {
	case program.NetworkID != 0 && program.ServiceID != 0:
		return program.NetworkID*serviceIDBase + program.ServiceID, true
	case program.ID != 0:
		// 番組 ID は (networkId × 100000 + serviceId) × 100000 + eventId なので、下 5 桁を除くとサービス ID になる。
		return program.ID / serviceIDBase, true
	default:
		return 0, false
	}
}
