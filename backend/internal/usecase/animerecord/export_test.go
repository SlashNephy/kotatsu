package animerecord

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

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

// PickWishlistWorks は、pickWishlistWorks をテストから使う。reserved は、予約のある番組の (サービス, eventId)。
// rules は、今あるルールの (キーワード, サービス)。
func PickWishlistWorks(
	sources map[int32][]*WishlistSource, programs []sqlc.ListAnimeWishlistProgramsRow, reserved [][2]int64, rules map[string][]int64,
) []*WishlistWork {
	keys := map[programKey]bool{}
	for _, r := range reserved {
		keys[programKey{serviceID: r[0], eventID: r[1]}] = true
	}

	identities := map[string]bool{}
	for keyword, services := range rules {
		identities[ruleIdentity(keyword, services)] = true
	}

	return pickWishlistWorks(sources, programs, keys, identities)
}

// AdvanceWishlist は、advanceWishlist をテストから使う。
func (s *Service) AdvanceWishlist(ctx context.Context, db sqlc.DBTX, userID uuid.UUID, name string, workIDs []int32, number *float64) {
	s.advanceWishlist(ctx, db, userID, name, workIDs, number)
}
