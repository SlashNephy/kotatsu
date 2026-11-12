// Package power は、OS のスリープの抑止と、スリープから起こす予約を OS に伝える (#1522、設計書 11.7)。
//
// Windows では kernel32 の SetThreadExecutionState とタスク スケジューラ (schtasks) を、
// macOS では IOKit の電源管理の API を、cgo を使わずに purego で読み込んで呼ぶ。配るバイナリを CGO_ENABLED=0 でクロスコンパイルするためである。
// ほかの OS では、New が ErrUnsupported を返す。
package power

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// ErrUnsupported は、この OS ではスリープを扱わないことを表す。
var ErrUnsupported = errors.New("sleep control is not supported on this OS")

// Name は、抑止と起こす予約に付ける名前。pmset -g assertions は ASCII でない名前を空で出すので、ASCII だけにする。
const Name = "Kotatsu recording"

const (
	// DefaultTaskTimeLimit は、Windows の起こすタスクの実行の上限 (付録 A)。中身は何もせずに終わるコマンドなので、短くする。
	DefaultTaskTimeLimit = time.Minute
	// WakeTaskName は、Windows のタスク スケジューラに登録する、起こすタスクの名前。
	WakeTaskName = `\Kotatsu\Wake`
	// wakeFileName は、macOS で置いた予約の時刻を書く、データディレクトリの下のファイルの名前。
	wakeFileName = "sleep-wake"
)

// Options は New の設定。
type Options struct {
	// DataDir は、macOS で置いた予約の時刻を書くデータディレクトリ。空なら書かない。
	DataDir string
	// TaskTimeLimit は、Windows の起こすタスクの実行の上限。0 なら DefaultTaskTimeLimit にする。
	TaskTimeLimit time.Duration
}

// cfAbsoluteTimeEpoch は、CFAbsoluteTime の起点 (2001-01-01 UTC) の Unix 時刻。
const cfAbsoluteTimeEpoch = 978307200

// cfAbsoluteTime は、t を CFAbsoluteTime (2001-01-01 UTC からの秒) にする。秒に切り捨てた時刻を渡す。
func cfAbsoluteTime(t time.Time) float64 {
	return float64(t.Unix() - cfAbsoluteTimeEpoch)
}

// wakeFile は、置いた予約の時刻を書くファイル。macOS の予約は OS が覚えていて、プロセスを起動し直しても残り、
// 置いたときと同じ時刻でないと取り消せないので、置いた時刻を残す。
type wakeFile struct {
	// path が空なら、読みも書きもしない。
	path string
}

func newWakeFile(dataDir string) *wakeFile {
	if dataDir == "" {
		return &wakeFile{path: ""}
	}

	return &wakeFile{path: filepath.Join(dataDir, wakeFileName)}
}

// read は、書いた時刻を返す。ファイルがなければ、ゼロと nil を返す。
func (f *wakeFile) read() (time.Time, error) {
	if f.path == "" {
		return time.Time{}, nil
	}
	data, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse %s: %w", f.path, err)
	}

	return time.Unix(sec, 0).UTC(), nil
}

// write は、at を Unix 時刻の秒で書く。
func (f *wakeFile) write(at time.Time) error {
	if f.path == "" {
		return nil
	}

	return os.WriteFile(f.path, []byte(strconv.FormatInt(at.Unix(), 10)+"\n"), 0o600)
}

// remove は、ファイルを消す。なければ何もしない。
func (f *wakeFile) remove() error {
	if f.path == "" {
		return nil
	}
	if err := os.Remove(f.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// wakeTaskXML は、at に「スリープを解除して実行」するタスクの定義を、schtasks /XML が読む UTF-16 (BOM 付き) で返す。
//
// 実行者は SYSTEM (S-1-5-18) にし、権限は最小にする。バッテリーでも止めず、多重起動は無視する。
// 中身は、何もせずに終わるコマンドである。時刻は UTC で書く。
func wakeTaskXML(at time.Time, timeLimit time.Duration) []byte {
	doc := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Wakes the computer before the next Kotatsu recording.</Description>
  </RegistrationInfo>
  <Triggers>
    <TimeTrigger>
      <StartBoundary>` + at.UTC().Format("2006-01-02T15:04:05Z") + `</StartBoundary>
      <Enabled>true</Enabled>
    </TimeTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>S-1-5-18</UserId>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <WakeToRun>true</WakeToRun>
    <ExecutionTimeLimit>PT` + strconv.Itoa(max(1, int(timeLimit/time.Second))) + `S</ExecutionTimeLimit>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>cmd.exe</Command>
      <Arguments>/c exit 0</Arguments>
    </Exec>
  </Actions>
</Task>
`
	var buf bytes.Buffer
	// UTF-16 の BOM (リトルエンディアン)。
	buf.Write([]byte{0xff, 0xfe})
	for _, u := range utf16.Encode([]rune(doc)) {
		_ = binary.Write(&buf, binary.LittleEndian, u)
	}

	return buf.Bytes()
}
