package epg

import "github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"

// ServiceIDBase は serviceIDBase をテストから使う。
const ServiceIDBase = serviceIDBase

// BatchEvents は、events をまとめたときに、番組を突き合わせるサービスと、全量を突き合わせるかどうかを返す。
func BatchEvents(events []*mirakurun.Event) (map[int64]struct{}, bool) {
	batch := newEventBatch()
	for _, event := range events {
		batch.add(event)
	}

	return batch.serviceIDs, batch.services
}
