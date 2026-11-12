// Package procprio は、急がない子プロセス (録画の後処理の FFmpeg など) を、OS の最低の優先度で動かす (#1511、設計書 11.7)。
// ライブ視聴の変換と CPU を取り合っても、ライブが先に CPU を使えるようにする。ほかに何も動いていなければ、速さは変わらない。
package procprio

import (
	"errors"
	"log/slog"
	"os/exec"
	"sync/atomic"
)

// Lowerer は、子プロセスを起動して、OS の最低の優先度に下げる。下げられなかったときの警告は、最初の 1 回だけ出す。
type Lowerer struct {
	logger *slog.Logger
	warned atomic.Bool
}

// New は Lowerer を返す。logger には、下げられなかったときの警告を出す。
func New(logger *slog.Logger) *Lowerer {
	return &Lowerer{logger: logger, warned: atomic.Bool{}}
}

// Start は、cmd を起動し、OS の最低の優先度に下げる。
// 下げられなくても止めない。プロセスがもう終わっていれば何もせず、権限がないなどで下げられなければ、
// 最初の 1 回だけ警告して、普段の優先度で続ける。l が nil なら、優先度を変えずに起動する。
func (l *Lowerer) Start(cmd *exec.Cmd) error {
	if l == nil {
		return cmd.Start()
	}
	prepare(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := lower(cmd.Process.Pid); err != nil && !errors.Is(err, errProcessGone) && l.warned.CompareAndSwap(false, true) {
		l.logger.Warn("failed to lower the priority of a child process; it runs with the normal priority",
			slog.String("command", cmd.Path), slog.Any("error", err))
	}

	return nil
}

// Run は、Start で起動し、終わるまで待つ。
func (l *Lowerer) Run(cmd *exec.Cmd) error {
	if err := l.Start(cmd); err != nil {
		return err
	}

	return cmd.Wait()
}
