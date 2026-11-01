package domain

import (
	"encoding/json"
	"fmt"
)

const (
	// relatedShared は、同じイベントを複数のサービスで放送する関係 (イベント共有)。
	relatedShared = "shared"
	// serviceIDBase は、Mirakurun のサービス ID を組み立てる基数 (networkId × 100000 + serviceId)。
	serviceIDBase = 100_000
)

// SharedFrom は、イベント共有の元のサービスの Mirakurun のサービス ID を返す。番組が元であるか、共有していなければ 0 を返す。
// relatedItems は、上流の番組の relatedItems の JSON。
// 元は、同じネットワークで共有しているサービス (自分を含む) のうち、サービス ID が最も小さいものにする。
// 地上波では、メインチャンネルの番組は共有するサブチャンネルを全て挙げ、サブチャンネルの番組はメインチャンネルだけを挙げる。
func SharedFrom(serviceID int64, relatedItems []byte) (int64, error) {
	if len(relatedItems) == 0 {
		return 0, nil
	}
	var related []*RelatedItem
	if err := json.Unmarshal(relatedItems, &related); err != nil {
		return 0, fmt.Errorf("failed to decode the related items: %w", err)
	}
	networkID := serviceID / serviceIDBase
	from := serviceID
	for _, item := range related {
		if item.Type != relatedShared || (item.NetworkID != 0 && int64(item.NetworkID) != networkID) {
			continue
		}
		from = min(from, networkID*serviceIDBase+int64(item.ServiceID))
	}
	if from == serviceID {
		return 0, nil
	}

	return from, nil
}

// SharedProgram は、同じイベントを共有している番組。
type SharedProgram struct {
	// ServiceID は、Mirakurun のサービス ID。
	ServiceID int64
	EventID   int64
}

// SharedPrograms は、番組と同じイベントを共有している、ほかのサービスの番組を返す。共有していなければ空を返す。
// relatedItems は、上流の番組の relatedItems の JSON。ネットワーク ID を省いた項目は、番組と同じネットワークとみなす。
func SharedPrograms(serviceID int64, relatedItems []byte) ([]*SharedProgram, error) {
	programs := []*SharedProgram{}
	if len(relatedItems) == 0 {
		return programs, nil
	}
	var related []*RelatedItem
	if err := json.Unmarshal(relatedItems, &related); err != nil {
		return nil, fmt.Errorf("failed to decode the related items: %w", err)
	}
	networkID := serviceID / serviceIDBase
	for _, item := range related {
		if item.Type != relatedShared {
			continue
		}
		itemNetwork := int64(item.NetworkID)
		if itemNetwork == 0 {
			itemNetwork = networkID
		}
		other := itemNetwork*serviceIDBase + int64(item.ServiceID)
		if other == serviceID {
			continue
		}
		programs = append(programs, &SharedProgram{ServiceID: other, EventID: int64(item.EventID)})
	}

	return programs, nil
}
