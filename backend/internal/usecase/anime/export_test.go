package anime

import "time"

// CacheRetention と OrphanGrace は、cacheRetention と orphanGrace をテストから使う。
const (
	CacheRetention = cacheRetention
	OrphanGrace    = orphanGrace
)

// SeasonsOf は、seasonsOf をテストから使う。
func SeasonsOf(now time.Time) []string { return seasonsOf(now) }
