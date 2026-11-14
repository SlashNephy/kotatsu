package iptvapi

import "time"

// SetRecheckInterval は、ストリームを流している間に、続けてよいかを確かめ直す間隔を変える。テストで待つ時間を短くするのに使う。
func (h *Handler) SetRecheckInterval(interval time.Duration) {
	h.recheck = interval
}
