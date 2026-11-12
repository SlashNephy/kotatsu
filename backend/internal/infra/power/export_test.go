package power

import "time"

// テストから、OS に依らない部分を呼べるようにする。
const WakeFileName = wakeFileName

// WakeTaskXML は wakeTaskXML を呼ぶ。
func WakeTaskXML(at time.Time, timeLimit time.Duration) []byte { return wakeTaskXML(at, timeLimit) }

// CFAbsoluteTime は cfAbsoluteTime を呼ぶ。
func CFAbsoluteTime(t time.Time) float64 { return cfAbsoluteTime(t) }

// WakeFile は、置いた予約の時刻を書くファイル。
type WakeFile = wakeFile

// NewWakeFile は newWakeFile を呼ぶ。
func NewWakeFile(dataDir string) *WakeFile { return newWakeFile(dataDir) }

// Read は read を呼ぶ。
func (f *wakeFile) Read() (time.Time, error) { return f.read() }

// Write は write を呼ぶ。
func (f *wakeFile) Write(at time.Time) error { return f.write(at) }

// Remove は remove を呼ぶ。
func (f *wakeFile) Remove() error { return f.remove() }
