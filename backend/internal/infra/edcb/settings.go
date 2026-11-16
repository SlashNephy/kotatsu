package edcb

import (
	"bufio"
	"bytes"
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

const (
	// timerSettingsFile は、EpgTimerSrv の設定のファイル。EpgTimerSrv は、互換動作 (EpgTimerSrv.ini の [SET] の
	// CompatFlags の 0x80) を有効にしたときしか、FILE_COPY2 で返さない (EpgTimerSrvMain.cpp)。
	timerSettingsFile = "EpgTimerSrv.ini"
	// timerSettingsTTL は、EpgTimerSrv.ini を読み直す間隔。EPG の取得の時刻とマージンは、利用者が設定を変えたときだけ変わる。
	timerSettingsTTL = 10 * time.Minute
	// defaultStartMargin と defaultEndMargin は、EpgTimerSrv.ini を読めないときの、予約のマージンの既定
	// (EpgTimerSrvSetting.cpp の StartMargin と EndMargin の既定値)。
	defaultStartMargin = 5 * time.Second
	defaultEndMargin   = 2 * time.Second
	// defaultEPGCaptureMinute は、[EPG_CAP] の Count がないときの、EPG の取得の時刻 (毎日 23:00)。
	defaultEPGCaptureMinute = 23 * 60
	daysPerWeek             = 7
)

// EPGCaptureTime は、EpgTimerSrv が EPG を取得する時刻 (EpgTimerSrv.ini の [EPG_CAP])。
type EPGCaptureTime struct {
	// Weekday は曜日。Daily が真なら使わない。
	Weekday time.Weekday
	// Daily は、毎日取得するかどうか。
	Daily bool
	// Minute は、0 時からの分。
	Minute int
}

// noCaptureTime は、読めなかった EPG の取得の時刻を返す。
func noCaptureTime() EPGCaptureTime {
	return EPGCaptureTime{Weekday: time.Sunday, Daily: false, Minute: 0}
}

// next は、now 以後で最も近い取得の時刻を返す。時刻は JST で数える。秒以下は切り捨てた分で比べる (ReserveManager.cpp の
// GetNextEpgCapTime と同じ)。
func (t EPGCaptureTime) next(now time.Time) time.Time {
	wall := now.In(jst()).Truncate(time.Minute)
	midnight := time.Date(wall.Year(), wall.Month(), wall.Day(), 0, 0, 0, 0, jst())
	candidate := midnight.Add(time.Duration(t.Minute) * time.Minute)
	if !t.Daily {
		days := (int(t.Weekday) - int(wall.Weekday()) + daysPerWeek) % daysPerWeek
		candidate = candidate.AddDate(0, 0, days)
		if candidate.Before(wall) {
			candidate = candidate.AddDate(0, 0, daysPerWeek)
		}

		return candidate
	}
	if candidate.Before(wall) {
		candidate = candidate.AddDate(0, 0, 1)
	}

	return candidate
}

// TimerSettings は、EpgTimerSrv.ini のうち、チューナーの割り当てに使う設定 (#1536)。
type TimerSettings struct {
	// EPGCaptureTimes は、有効にした EPG の取得の時刻。
	EPGCaptureTimes []EPGCaptureTime
	// StartMargin と EndMargin は、マージンを個別に指定していない予約のマージン。
	StartMargin, EndMargin time.Duration
}

// defaultTimerSettings は、EpgTimerSrv.ini を読めないときの設定。EPG の取得の時刻は分からないので空にする。
func defaultTimerSettings() *TimerSettings {
	return &TimerSettings{EPGCaptureTimes: []EPGCaptureTime{}, StartMargin: defaultStartMargin, EndMargin: defaultEndMargin}
}

// timerSettingsCache は、読んだ EpgTimerSrv.ini の設定と、読んだ時刻。
type timerSettingsCache struct {
	mu       sync.Mutex
	settings *TimerSettings
	loadedAt time.Time
}

// timerSettings は、EpgTimerSrv.ini の設定を返す。timerSettingsTTL の間は、前に読んだものを使う。
// EpgTimerSrv が返さなければ (互換動作を有効にしていない)、既定の設定 (EPG の取得の時刻は分からないので空) を返す。
func (s *TunerServer) timerSettings(ctx context.Context) (*TimerSettings, error) {
	s.settings.mu.Lock()
	defer s.settings.mu.Unlock()
	if s.settings.settings != nil && time.Since(s.settings.loadedAt) < timerSettingsTTL {
		return s.settings.settings, nil
	}
	files, err := s.client.Files(ctx, []string{timerSettingsFile})
	if err != nil {
		return nil, err
	}
	settings := defaultTimerSettings()
	if len(files) > 0 && len(files[0].Data) > 0 {
		settings = ParseTimerSettings(files[0].Data)
	}
	s.settings.settings, s.settings.loadedAt = settings, time.Now()

	return settings, nil
}

// ParseTimerSettings は、EpgTimerSrv.ini の [SET] のマージンと、[EPG_CAP] の EPG の取得の時刻を読む (EpgTimerSrvSetting.cpp)。
//   - [EPG_CAP] の Count がなければ、毎日 23:00 に取得する (EpgTimerSrv の既定)。
//   - 時刻は「時:分」で、曜日を指定するときは「w1」(月) 〜「w7」(日) を続ける。{n}Select が 0 なら無効。
//
// 文字コードは、UTF-16 (BOM 付き。Windows の EDCB が書く)、UTF-8 (Linux の EDCB) のどちらも読む。キーと値は ASCII だけを読む。
func ParseTimerSettings(data []byte) *TimerSettings {
	sections := parseINI(data)
	settings := defaultTimerSettings()
	set := sections["set"]
	if v, ok := atoi(set["startmargin"]); ok {
		settings.StartMargin = time.Duration(v) * time.Second
	}
	if v, ok := atoi(set["endmargin"]); ok {
		settings.EndMargin = time.Duration(v) * time.Second
	}
	capture := sections["epg_cap"]
	count, ok := atoi(capture["count"])
	if !ok {
		settings.EPGCaptureTimes = []EPGCaptureTime{{Weekday: time.Sunday, Daily: true, Minute: defaultEPGCaptureMinute}}

		return settings
	}
	for i := range max(0, count) {
		key := strconv.Itoa(i)
		if selected, _ := atoi(capture[key+"select"]); selected == 0 {
			continue
		}
		if t, ok := parseCaptureTime(capture[key]); ok {
			settings.EPGCaptureTimes = append(settings.EPGCaptureTimes, t)
		}
	}

	return settings
}

// parseCaptureTime は、「23:00」や「4:30w1」の形の EPG の取得の時刻を読む。曜日は w1 が月曜、w7 が日曜。
func parseCaptureTime(value string) (EPGCaptureTime, bool) {
	hourText, rest, ok := strings.Cut(value, ":")
	if !ok {
		return noCaptureTime(), false
	}
	minuteText, weekdayText, weekly := strings.Cut(rest, "w")
	hour, err1 := strconv.Atoi(hourText)
	minute, err2 := strconv.Atoi(minuteText)
	if err1 != nil || err2 != nil || hour < 0 || minute < 0 {
		return noCaptureTime(), false
	}
	t := EPGCaptureTime{Weekday: time.Sunday, Daily: true, Minute: hour%24*60 + minute%60}
	if weekly {
		weekday, err := strconv.Atoi(weekdayText)
		// w0 や数字でない曜日は、EpgTimerSrv と同じく、毎日とみなす。
		if err == nil && weekday%8 != 0 {
			t.Daily, t.Weekday = false, time.Weekday(weekday%8%daysPerWeek)
		}
	}

	return t, true
}

// parseINI は、INI ファイルを、小文字にしたセクションとキーの対応にする。
func parseINI(data []byte) map[string]map[string]string {
	text := decodeINIText(data)
	sections := map[string]map[string]string{}
	section := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if sections[name] == nil {
				sections[name] = map[string]string{}
			}
			section = sections[name]

			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		section[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}

	return sections
}

// decodeINIText は、BOM 付きの UTF-16LE と UTF-8 を文字列にする。BOM がなければ、そのまま読む (ASCII の部分だけを使う)。
func decodeINIText(data []byte) string {
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) {
		data = data[2:]
		units := make([]uint16, 0, len(data)/2)
		for i := 0; i+1 < len(data); i += 2 {
			units = append(units, uint16(data[i])|uint16(data[i+1])<<8)
		}

		return string(utf16.Decode(units))
	}

	return string(bytes.TrimPrefix(data, []byte(utf8BOM)))
}

func atoi(value string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(value))

	return v, err == nil
}
