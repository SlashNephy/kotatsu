package model

import "time"

// TimeshiftChannel は、タイムシフトで録るチャンネル。サービスは、リゾルバが dataloader で引く。
type TimeshiftChannel struct {
	RetentionSeconds int        `json:"retentionSeconds"`
	BufferedFrom     *time.Time `json:"bufferedFrom,omitempty"`
	BufferedUntil    *time.Time `json:"bufferedUntil,omitempty"`
	Recording        bool       `json:"recording"`
	// ServiceKey は、Mirakurun のサービス ID。スキーマには出さない。
	ServiceKey int64 `json:"-"`
}
