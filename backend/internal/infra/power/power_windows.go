//go:build windows

package power

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

const (
	// SetThreadExecutionState に渡すフラグ。画面を消すのは止めないので、ES_DISPLAY_REQUIRED は付けない。
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
	// commandTimeout は、schtasks を待つ時間の上限。
	commandTimeout = 30 * time.Second
)

// System は、Windows のスリープの抑止と起こすタスクを扱う。
//
// SetThreadExecutionState は呼んだスレッドにだけ効くので、OS のスレッドに固定した専用の goroutine から、いつも呼ぶ。
// プロセスが落ちれば、OS が抑止を外す。
type System struct {
	setThreadExecutionState *windows.LazyProc
	requests                chan inhibitRequest
	timeLimit               time.Duration
}

type inhibitRequest struct {
	on     bool
	result chan error
}

// New は System を返す。止めるときは Close を呼ぶ。
func New(opts *Options) (*System, error) {
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadExecutionState")
	if err := proc.Find(); err != nil {
		return nil, fmt.Errorf("failed to load SetThreadExecutionState: %w", err)
	}
	timeLimit := opts.TaskTimeLimit
	if timeLimit <= 0 {
		timeLimit = DefaultTaskTimeLimit
	}
	s := &System{setThreadExecutionState: proc, requests: make(chan inhibitRequest), timeLimit: timeLimit}
	go s.loop()

	return s, nil
}

// SetInhibit は、操作の無いことで眠るのを止めるか、止めるのをやめる。
func (s *System) SetInhibit(on bool) error {
	result := make(chan error, 1)
	s.requests <- inhibitRequest{on: on, result: result}

	return <-result
}

// ScheduleWake は、at に「スリープを解除して実行」するタスクを登録する。同じ名前のタスクは置き換える。
// 実行者を SYSTEM にするので、管理者の権限が要る。
func (s *System) ScheduleWake(at time.Time) error {
	dir, err := os.MkdirTemp("", "kotatsu-wake-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "task.xml")
	if err := os.WriteFile(path, wakeTaskXML(at, s.timeLimit), 0o600); err != nil {
		return err
	}

	return schtasks(func(ctx context.Context, name string) *exec.Cmd {
		return exec.CommandContext(ctx, name, "/Create", "/TN", WakeTaskName, "/XML", path, "/F")
	})
}

// CancelWake は、起こすタスクを消す。消す前に、あるかを確かめ、なければ何もしない。
func (s *System) CancelWake() error {
	if err := schtasks(func(ctx context.Context, name string) *exec.Cmd {
		return exec.CommandContext(ctx, name, "/Query", "/TN", WakeTaskName)
	}); err != nil {
		if _, ok := errors.AsType[*exec.ExitError](err); ok {
			// 見つからないときも、終了コード 1 で終わる。
			return nil
		}

		return err
	}

	return schtasks(func(ctx context.Context, name string) *exec.Cmd {
		return exec.CommandContext(ctx, name, "/Delete", "/TN", WakeTaskName, "/F")
	})
}

// Close は、抑止の goroutine を止める。止めると、そのスレッドの抑止も外れる。
func (s *System) Close() error {
	close(s.requests)

	return nil
}

// loop は、OS のスレッドに固定したまま、抑止の要求を順に処理する。抜けるときは固定を解かず、スレッドごと終わらせる。
func (s *System) loop() {
	runtime.LockOSThread()
	for req := range s.requests {
		flags := uintptr(esContinuous)
		if req.on {
			flags |= esSystemRequired
		}
		// 戻り値は前の状態で、0 なら失敗である。
		ret, _, err := s.setThreadExecutionState.Call(flags)
		if ret == 0 {
			req.result <- fmt.Errorf("SetThreadExecutionState failed: %w", err)

			continue
		}
		req.result <- nil
	}
}

// schtasks は、command が作る、システムのディレクトリの schtasks.exe のコマンドを動かす。失敗したら、出力をエラーに含める。
// 引数は、呼び出し側に固定の値で書く。
func schtasks(command func(ctx context.Context, name string) *exec.Cmd) error {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return fmt.Errorf("failed to find the system directory: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := command(ctx, filepath.Join(dir, "schtasks.exe"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks %s: %w: %s", cmd.Args[1], err, strings.TrimSpace(string(out)))
	}

	return nil
}
