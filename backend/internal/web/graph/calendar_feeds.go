package graph

import (
	"errors"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

var (
	// errCalendarFeedsUnavailable は、購読用の URL を扱う設定がないことを表す。
	errCalendarFeedsUnavailable = errors.New("calendar feeds are not configured")
	// errIPTVFeedsUnavailable は、IPTV の配信の URL を扱う設定がないことを表す。
	errIPTVFeedsUnavailable = errors.New("IPTV feeds are not configured")
)

func calendarFeedOf(feed *auth.Feed) *model.CalendarFeed {
	return &model.CalendarFeed{CreatedAt: feed.CreatedAt, LastUsedAt: feed.LastUsedAt}
}

// calendarFeedPath は、秘密の値 token から、予約の iCalendar のパスを作る。
func calendarFeedPath(token string) string {
	return "/calendar/" + token + "/reservations.ics"
}

func iptvFeedOf(feed *auth.Feed) *model.IptvFeed {
	return &model.IptvFeed{CreatedAt: feed.CreatedAt, LastUsedAt: feed.LastUsedAt}
}

// iptvFeedPaths は、秘密の値 token から、ライブ視聴の M3U と番組表の XMLTV のパスを作る (#1324)。
func iptvFeedPaths(token string) (string, string) {
	return "/iptv/" + token + "/channels.m3u", "/iptv/" + token + "/guide.xml"
}
