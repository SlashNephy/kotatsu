// Package clock は、現在時刻の取得を差し替えられるようにする。
// テストでは clocktest.Fake を渡し、時刻を進めて期限切れなどを再現する。
package clock

import "time"

// Clock は現在時刻を返す。
type Clock interface {
	Now() time.Time
}

// Real は実際の時刻を UTC で返す。
type Real struct{}

var _ Clock = new(Real)

func (Real) Now() time.Time {
	return time.Now().UTC()
}
