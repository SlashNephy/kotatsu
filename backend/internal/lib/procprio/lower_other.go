//go:build !linux && !darwin && !windows

package procprio

import (
	"errors"
	"os/exec"
)

// errProcessGone は、ほかの OS では使わない。
var errProcessGone = errors.New("the process has already exited")

// prepare は、ほかの OS では何もしない。
func prepare(*exec.Cmd) {}

// lower は、ほかの OS では何もしない。優先度を下げずに動かす。
func lower(int) error { return nil }
