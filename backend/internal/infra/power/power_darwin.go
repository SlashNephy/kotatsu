//go:build darwin

package power

import (
	"fmt"
	"sync"
	"time"

	"github.com/ebitengine/purego"
)

const (
	iokitPath          = "/System/Library/Frameworks/IOKit.framework/IOKit"
	coreFoundationPath = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
	// kCFStringEncodingASCII。名前と種類は ASCII だけで書く。
	cfStringEncodingASCII = 0x0600
	// kIOPMAssertionLevelOn。
	assertionLevelOn = 255
	// kIOPMAssertionTypePreventUserIdleSystemSleep。操作が無いことで眠るのだけを止める。蓋を閉じることと、メニューからのスリープは止めない。
	assertionTypePreventUserIdleSystemSleep = "PreventUserIdleSystemSleep"
	// kIOPMAutoWake。
	powerEventAutoWake = "wake"
	// kIOReturnSuccess。IOReturn は符号付きだが、エラーの値を 16 進で読めるよう、符号なしで受け取る。
	ioReturnSuccess = 0
)

// iokit は、purego で読み込んだ IOKit と CoreFoundation の関数。
type iokit struct {
	cfStringCreateWithCString func(alloc uintptr, s string, encoding uint32) uintptr
	cfDateCreate              func(alloc uintptr, at float64) uintptr
	cfRelease                 func(ref uintptr)
	assertionCreateWithName   func(assertionType uintptr, level uint32, name uintptr, id *uint32) uint32
	assertionRelease          func(id uint32) uint32
	schedulePowerEvent        func(at, id, eventType uintptr) uint32
	cancelScheduledPowerEvent func(at, id, eventType uintptr) uint32
}

// loadIOKit は、IOKit と CoreFoundation を開き、使う関数を読む。
func loadIOKit() (*iokit, error) {
	iokitLib, err := purego.Dlopen(iokitPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("failed to open IOKit: %w", err)
	}
	cfLib, err := purego.Dlopen(coreFoundationPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("failed to open CoreFoundation: %w", err)
	}
	k := new(iokit)
	for _, f := range []struct {
		lib  uintptr
		name string
		fptr any
	}{
		{cfLib, "CFStringCreateWithCString", &k.cfStringCreateWithCString},
		{cfLib, "CFDateCreate", &k.cfDateCreate},
		{cfLib, "CFRelease", &k.cfRelease},
		{iokitLib, "IOPMAssertionCreateWithName", &k.assertionCreateWithName},
		{iokitLib, "IOPMAssertionRelease", &k.assertionRelease},
		{iokitLib, "IOPMSchedulePowerEvent", &k.schedulePowerEvent},
		{iokitLib, "IOPMCancelScheduledPowerEvent", &k.cancelScheduledPowerEvent},
	} {
		// RegisterLibFunc は、関数がないと panic するので、先に Dlsym で確かめる。
		sym, err := purego.Dlsym(f.lib, f.name)
		if err != nil {
			return nil, fmt.Errorf("failed to find %s: %w", f.name, err)
		}
		purego.RegisterFunc(f.fptr, sym)
	}

	return k, nil
}

// cfString は、s の CFString を作る。使い終えたら CFRelease で放す。
func (k *iokit) cfString(s string) (uintptr, error) {
	ref := k.cfStringCreateWithCString(0, s, cfStringEncodingASCII)
	if ref == 0 {
		return 0, fmt.Errorf("failed to create a CFString for %q", s)
	}

	return ref, nil
}

// powerEvent は、at と名前の起こす予約を、schedule か cancel で伝える。
func (k *iokit) powerEvent(call func(at, id, eventType uintptr) uint32, at time.Time) error {
	date := k.cfDateCreate(0, cfAbsoluteTime(at))
	if date == 0 {
		return fmt.Errorf("failed to create a CFDate for %s", at)
	}
	defer k.cfRelease(date)
	id, err := k.cfString(Name)
	if err != nil {
		return err
	}
	defer k.cfRelease(id)
	eventType, err := k.cfString(powerEventAutoWake)
	if err != nil {
		return err
	}
	defer k.cfRelease(eventType)
	if ret := call(date, id, eventType); ret != ioReturnSuccess {
		return fmt.Errorf("IOKit returned %#x", ret)
	}

	return nil
}

// System は、macOS のスリープの抑止と起こす予約を扱う。
//
// 抑止は、IOPMAssertionCreateWithName で名前を付けて取り、IOPMAssertionRelease で外す。プロセスが落ちれば、OS が外す。
// 起こす予約は IOPMSchedulePowerEvent で置き、root が要る。
// 予約は OS が覚えていて、プロセスを起動し直しても残るので、置いた時刻をデータディレクトリのファイルに書く。
type System struct {
	k    *iokit
	file *wakeFile

	mu sync.Mutex
	// assertion は、取った抑止の ID。取っていなければ 0。
	assertion uint32
	// scheduled は、このプロセスが置いた予約の時刻。置いていなければゼロ。データディレクトリがなくても取り消せるよう、覚えておく。
	scheduled time.Time
}

// New は System を返す。
func New(opts *Options) (*System, error) {
	k, err := loadIOKit()
	if err != nil {
		return nil, err
	}

	return &System{k: k, file: newWakeFile(opts.DataDir), mu: sync.Mutex{}, assertion: 0, scheduled: time.Time{}}, nil
}

// SetInhibit は、操作の無いことで眠るのを止めるか、止めるのをやめる。
func (s *System) SetInhibit(on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on == (s.assertion != 0) {
		return nil
	}
	if !on {
		if ret := s.k.assertionRelease(s.assertion); ret != ioReturnSuccess {
			return fmt.Errorf("IOPMAssertionRelease returned %#x", ret)
		}
		s.assertion = 0

		return nil
	}
	assertionType, err := s.k.cfString(assertionTypePreventUserIdleSystemSleep)
	if err != nil {
		return err
	}
	defer s.k.cfRelease(assertionType)
	name, err := s.k.cfString(Name)
	if err != nil {
		return err
	}
	defer s.k.cfRelease(name)
	var id uint32
	if ret := s.k.assertionCreateWithName(assertionType, assertionLevelOn, name, &id); ret != ioReturnSuccess {
		return fmt.Errorf("IOPMAssertionCreateWithName returned %#x", ret)
	}
	s.assertion = id

	return nil
}

// ScheduleWake は、前の予約を取り消してから、at に起こす予約を置き、時刻をファイルに書く。
func (s *System) ScheduleWake(at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.cancelLocked(); err != nil {
		return err
	}
	// 取り消しは、置いたときと同じ時刻でないと効かないので、秒に切り捨ててそろえる。
	at = at.Truncate(time.Second)
	if err := s.k.powerEvent(s.k.schedulePowerEvent, at); err != nil {
		return fmt.Errorf("failed to schedule a wake (root is required): %w", err)
	}
	s.scheduled = at
	if err := s.file.write(at); err != nil {
		return fmt.Errorf("failed to record the scheduled wake: %w", err)
	}

	return nil
}

// CancelWake は、ファイルに書いた予約を取り消し、ファイルを消す。
func (s *System) CancelWake() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.cancelLocked()
}

// Close は、取った抑止を外す。
func (s *System) Close() error {
	return s.SetInhibit(false)
}

func (s *System) cancelLocked() error {
	at := s.scheduled
	if at.IsZero() {
		// 前のプロセスが置いたまま残した予約は、ファイルから読む。
		var err error
		if at, err = s.file.read(); err != nil {
			return err
		}
	}
	if at.IsZero() {
		return nil
	}
	// 時刻を過ぎた予約は、OS が使い終えて消しているので、取り消さない。
	if at.After(time.Now()) {
		if err := s.k.powerEvent(s.k.cancelScheduledPowerEvent, at); err != nil {
			return fmt.Errorf("failed to cancel the wake at %s: %w", at.Format(time.RFC3339), err)
		}
	}
	s.scheduled = time.Time{}

	return s.file.remove()
}
