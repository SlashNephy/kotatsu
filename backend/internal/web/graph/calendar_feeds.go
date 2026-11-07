package graph

import (
	"errors"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// errCalendarFeedsUnavailable は、購読用の URL を扱う設定がないことを表す。
var errCalendarFeedsUnavailable = errors.New("calendar feeds are not configured")

func calendarFeedOf(feed *auth.CalendarFeed) *model.CalendarFeed {
	return &model.CalendarFeed{CreatedAt: feed.CreatedAt, LastUsedAt: feed.LastUsedAt}
}

// calendarFeedPath は、秘密の値 token から、予約の iCalendar のパスを作る。
func calendarFeedPath(token string) string {
	return "/calendar/" + token + "/reservations.ics"
}
