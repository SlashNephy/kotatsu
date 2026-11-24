package epg

import "github.com/SlashNephy/kotatsu/internal/domain/tunerserver"

// ServiceIDBase は serviceIDBase をテストから使う。
const ServiceIDBase = serviceIDBase

// BatchEvents は、events をまとめたときに、番組を照合するサービスと、サービスの一覧を照合するかどうかと、
// 全量を照合するかどうかを返す。
func BatchEvents(events []*tunerserver.Event) (map[int64]struct{}, bool, bool) {
	batch := newEventBatch()
	for _, event := range events {
		batch.add(event)
	}

	return batch.serviceIDs, batch.services, batch.all
}
