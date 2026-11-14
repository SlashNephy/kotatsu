package streamapi

import "time"

// SetRecheckInterval は、署名付き URL で配信している間に、続けてよいかを確かめ直す間隔を変える。テストで待つ時間を短くするのに使う。
func (a *Authorizer) SetRecheckInterval(interval time.Duration) {
	a.recheck = interval
}
