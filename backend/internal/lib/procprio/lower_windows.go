//go:build windows

package procprio

import (
	"errors"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// errProcessGone は、Windows では使わない。起動するときに優先度クラスを決めるので、下げる前に終わることがない。
var errProcessGone = errors.New("the process has already exited")

// prepare は、IDLE_PRIORITY_CLASS で起動させる。起動の時点から低いので、起動直後の隙がない。
func prepare(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.IDLE_PRIORITY_CLASS
}

// lower は、Windows では何もしない。prepare で、起動するときに下げている。
func lower(int) error { return nil }
