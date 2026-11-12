//go:build !windows && !darwin

package power

import "time"

// System は、スリープを扱わない OS の口。New は ErrUnsupported を返すので、作られない。
type System struct{}

// New は、この OS ではスリープを扱わないので、ErrUnsupported を返す。Linux とコンテナでは、何もしない。
func New(*Options) (*System, error) {
	return nil, ErrUnsupported
}

// SetInhibit は ErrUnsupported を返す。
func (*System) SetInhibit(bool) error { return ErrUnsupported }

// ScheduleWake は ErrUnsupported を返す。
func (*System) ScheduleWake(time.Time) error { return ErrUnsupported }

// CancelWake は ErrUnsupported を返す。
func (*System) CancelWake() error { return ErrUnsupported }

// Close は何もしない。
func (*System) Close() error { return nil }
