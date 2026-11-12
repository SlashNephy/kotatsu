package power_test

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/power"
)

// TestWakeTaskXML は、Windows の起こすタスクの定義が、schtasks の読む UTF-16 (BOM 付き) で、時刻を UTC で書くことを確かめる。
func TestWakeTaskXML(t *testing.T) {
	t.Parallel()

	jst := time.FixedZone("JST", 9*60*60)
	tests := []struct {
		name          string
		at            time.Time
		timeLimit     time.Duration
		wantStart     string
		wantTimeLimit string
	}{
		{
			name: "local time is written in UTC", at: time.Date(2026, 11, 12, 21, 55, 0, 0, jst), timeLimit: time.Minute,
			wantStart: "2026-11-12T12:55:00Z", wantTimeLimit: "PT60S",
		},
		{
			name: "a limit under a second is rounded up to a second", at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), timeLimit: time.Millisecond,
			wantStart: "2026-01-01T00:00:00Z", wantTimeLimit: "PT1S",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data := power.WakeTaskXML(tt.at, tt.timeLimit)
			require.Equal(t, []byte{0xff, 0xfe}, data[:2], "UTF-16LE BOM")
			units := make([]uint16, (len(data)-2)/2)
			require.NoError(t, binary.Read(bytes.NewReader(data[2:]), binary.LittleEndian, units))
			var task struct {
				Start     string `xml:"Triggers>TimeTrigger>StartBoundary"`
				UserID    string `xml:"Principals>Principal>UserId"`
				RunLevel  string `xml:"Principals>Principal>RunLevel"`
				WakeToRun bool   `xml:"Settings>WakeToRun"`
				Policy    string `xml:"Settings>MultipleInstancesPolicy"`
				StopOnBat bool   `xml:"Settings>StopIfGoingOnBatteries"`
				NoBattery bool   `xml:"Settings>DisallowStartIfOnBatteries"`
				TimeLimit string `xml:"Settings>ExecutionTimeLimit"`
			}
			decoder := xml.NewDecoder(strings.NewReader(string(utf16.Decode(units))))
			// 宣言の encoding="UTF-16" は、デコードした後の文字列には当たらないので、そのまま読む。
			decoder.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
			require.NoError(t, decoder.Decode(&task))
			assert.Equal(t, tt.wantStart, task.Start)
			assert.Equal(t, "S-1-5-18", task.UserID)
			assert.Equal(t, "LeastPrivilege", task.RunLevel)
			assert.True(t, task.WakeToRun)
			assert.Equal(t, "IgnoreNew", task.Policy)
			assert.False(t, task.StopOnBat)
			assert.False(t, task.NoBattery)
			assert.Equal(t, tt.wantTimeLimit, task.TimeLimit)
		})
	}
}

// TestCFAbsoluteTime は、Unix 時刻を、2001-01-01 UTC からの秒にすることを確かめる。
func TestCFAbsoluteTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		at   time.Time
		want float64
	}{
		{name: "epoch", at: time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC), want: 0},
		{name: "a day later in another zone", at: time.Date(2001, 1, 2, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60)), want: 86400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.InDelta(t, tt.want, power.CFAbsoluteTime(tt.at), 0)
		})
	}
}

// TestWakeFile は、置いた予約の時刻を秒で書き、読み戻し、消せることを確かめる。データディレクトリがなければ、何もしない。
func TestWakeFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// withDataDir は、データディレクトリを渡すかどうか。
		withDataDir bool
		want        time.Time
	}{
		{name: "writes and reads back in seconds", withDataDir: true, want: time.Date(2026, 11, 12, 3, 55, 0, 0, time.UTC)},
		{name: "does nothing without the data directory", withDataDir: false, want: time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dataDir := ""
			if tt.withDataDir {
				dataDir = t.TempDir()
			}
			f := power.NewWakeFile(dataDir)
			at, err := f.Read()
			require.NoError(t, err)
			assert.True(t, at.IsZero(), "nothing is written yet")
			require.NoError(t, f.Write(time.Date(2026, 11, 12, 3, 55, 0, 0, time.UTC)))
			at, err = f.Read()
			require.NoError(t, err)
			assert.True(t, tt.want.Equal(at), "read %s, want %s", at, tt.want)
			require.NoError(t, f.Remove())
			at, err = f.Read()
			require.NoError(t, err)
			assert.True(t, at.IsZero(), "removed")
			require.NoError(t, f.Remove(), "removing twice is fine")
		})
	}
}

// TestWakeFileBroken は、壊れたファイルを読めないときに、エラーにすることを確かめる。
func TestWakeFileBroken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, power.WakeFileName), []byte("not a time"), 0o600))
	_, err := power.NewWakeFile(dir).Read()
	require.Error(t, err)
}
