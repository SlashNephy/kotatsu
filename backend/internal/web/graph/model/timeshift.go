package model

import (
	"net/url"
	"strconv"
	"time"
)

// TimeshiftChannel は、タイムシフトで録るチャンネル。サービスは、リゾルバが dataloader で引く。
type TimeshiftChannel struct {
	RetentionSeconds int        `json:"retentionSeconds"`
	BufferedFrom     *time.Time `json:"bufferedFrom,omitempty"`
	BufferedUntil    *time.Time `json:"bufferedUntil,omitempty"`
	Recording        bool       `json:"recording"`
	// ServiceKey は、Mirakurun のサービス ID。スキーマには出さない。
	ServiceKey int64 `json:"-"`
}

// TimeshiftProgram は、タイムシフトのバッファから再生できる番組。サービスは、リゾルバが dataloader で引く。
type TimeshiftProgram struct {
	EventID      int             `json:"eventId"`
	Name         string          `json:"name"`
	StartAt      time.Time       `json:"startAt"`
	EndAt        *time.Time      `json:"endAt,omitempty"`
	PlayableFrom time.Time       `json:"playableFrom"`
	Gaps         []*RecordingGap `json:"gaps"`
	// ServiceKey は、Mirakurun のサービス ID。スキーマには出さない。
	ServiceKey int64 `json:"-"`
}

// StreamURL は、バッファの TS を、PlayableFrom を起点に流す URL のパス。tenban の GET /streams/timeshift/{id} で、Mirakurun のサービス ID を使う。
func (p *TimeshiftProgram) StreamURL() string {
	return "/streams/timeshift/" + strconv.FormatInt(p.ServiceKey, 10) + "?from=" + url.QueryEscape(p.PlayableFrom.UTC().Format(time.RFC3339Nano))
}
