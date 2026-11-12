//go:build linux || darwin

package procprio

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

// lowestNice は、Linux と macOS の最も低い優先度の nice 値。
const lowestNice = 19

// errProcessGone は、優先度を下げる前に、プロセスがもう終わっていたこと。
var errProcessGone = errors.New("the process has already exited")

// prepare は、起動する前の準備をする。Linux と macOS では、起動した後に下げるので、何もしない。
func prepare(*exec.Cmd) {}

// lower は、プロセス pid の nice 値を 19 にする。その後にプロセスが作るスレッドは、この値を引き継ぐ。
// 起動してから下げるまでに FFmpeg が作ったスレッドは、元の優先度のままになり得る (推測)。
func lower(pid int) error {
	err := syscall.Setpriority(syscall.PRIO_PROCESS, pid, lowestNice)
	if errors.Is(err, syscall.ESRCH) {
		return errProcessGone
	}
	if err != nil {
		return fmt.Errorf("failed to set the nice value of process %d: %w", pid, err)
	}

	return nil
}
