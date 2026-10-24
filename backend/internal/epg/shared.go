package epg

import (
	"encoding/json"
	"fmt"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
)

// relatedShared は、同じイベントを複数のサービスで放送する関係 (イベント共有)。
const relatedShared = "shared"

// SharedFrom は、イベント共有の元のサービスの Mirakurun のサービス ID を返す。番組が元であるか、共有していなければ 0 を返す。
// relatedItems は、上流の番組の relatedItems の JSON。
// 元は、同じネットワークで共有しているサービス (自分を含む) のうち、サービス ID が最も小さいものにする。
// 地上波では、メインチャンネルの番組は共有するサブチャンネルを全て挙げ、サブチャンネルの番組はメインチャンネルだけを挙げる。
func SharedFrom(serviceID int64, relatedItems []byte) (int64, error) {
	if len(relatedItems) == 0 {
		return 0, nil
	}
	var related []*mirakurun.RelatedItem
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
