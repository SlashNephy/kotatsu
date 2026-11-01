package domain

import "time"

// TimeshiftChannel は、タイムシフトで録るチャンネルと、その今の状態。
type TimeshiftChannel struct {
	// ServiceID は、Mirakurun のサービス ID。
	ServiceID        int64
	RetentionSeconds int32
	// BufferedFrom と BufferedUntil は、バッファにある範囲。チャンクがなければ nil。
	BufferedFrom  *time.Time
	BufferedUntil *time.Time
	// Recording は、mikan が割り振りを持ち、録っているかどうか。
	Recording bool
}
