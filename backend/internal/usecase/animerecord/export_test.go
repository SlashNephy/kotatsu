package animerecord

import "time"

// MergeSpans は、mergeSpans をテストから使う。区間は [始まり, 終わり) の UNIX ミリ秒。
// limit は、まとめた区間の数の上限。
func MergeSpans(spans [][2]int64, limit int) [][2]int64 {
	converted := make([]span, 0, len(spans))
	for _, s := range spans {
		converted = append(converted, span(s))
	}
	merged := capSpans(mergeSpans(converted), limit)
	result := make([][2]int64, 0, len(merged))
	for _, s := range merged {
		result = append(result, [2]int64(s))
	}

	return result
}

// ReachedThreshold は、reachedThreshold をテストから使う。
func ReachedThreshold(played int64, length time.Duration, percent int) bool {
	return reachedThreshold(played, length, percent)
}

// NormalizeSpans は、normalizeSpans をテストから使う。
func NormalizeSpans(spans [][2]int64) [][2]int64 {
	converted := make([]span, 0, len(spans))
	for _, s := range spans {
		converted = append(converted, span(s))
	}
	normalized := normalizeSpans(converted)
	result := make([][2]int64, 0, len(normalized))
	for _, s := range normalized {
		result = append(result, [2]int64(s))
	}

	return result
}

// MaxStoredSpans は、maxStoredSpans をテストから使う。
const MaxStoredSpans = maxStoredSpans
