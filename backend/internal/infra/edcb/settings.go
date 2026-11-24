package edcb

import (
	"bufio"
	"bytes"
	"context"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
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

// next は、now 以後で最も近い取得の時刻を返す。時刻は JST で数える。秒以下は切り捨てた分で比較する (ReserveManager.cpp の
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
	// Loaded は、EpgTimerSrv.ini を読めたかどうか。偽なら、ほかの項目は既定値。
	Loaded bool
	// EPGCaptureTimes は、有効にした EPG の取得の時刻。
	EPGCaptureTimes []EPGCaptureTime
	// StartMargin と EndMargin は、マージンを個別に指定していない予約のマージン。
	StartMargin, EndMargin time.Duration
	// EPGCaptureAnyBonDriver は、[SET] の NGEpgCapTime が 0 かどうか (#1856)。0 なら、EpgTimerSrv は、取得に使う BonDriver の
	// どれかに空きチューナーがあれば取得する。0 でなければ (既定は 20 分)、取得に使う BonDriver ごとに、EPGCount の数の空きチューナーが要る。
	EPGCaptureAnyBonDriver bool
	// EPGCaptureBonDrivers は、BonDriver の節 (小文字にした名前) ごとの、GetEpg と EPGCount (#1856)。
	EPGCaptureBonDrivers map[string]tunerserver.EPGCaptureBonDriver
}

// defaultTimerSettings は、EpgTimerSrv.ini を読めないときの設定。EPG の取得の時刻は分からないので空にする。
func defaultTimerSettings() *TimerSettings {
	return &TimerSettings{
		Loaded: false, EPGCaptureTimes: []EPGCaptureTime{}, StartMargin: defaultStartMargin, EndMargin: defaultEndMargin,
		EPGCaptureAnyBonDriver: false, EPGCaptureBonDrivers: map[string]tunerserver.EPGCaptureBonDriver{},
	}
}

// isBonDriverSection は、EpgTimerSrv.ini の節 (小文字にした名前) が、BonDriver の節になりうるかを返す。
func isBonDriverSection(name string) bool {
	switch name {
	case "set", "epg_cap", "tvtest":
		return false
	default:
		return true
	}
}

// timerSettingsCache は、読んだ EpgTimerSrv.ini の設定と、読んだ時刻。
type timerSettingsCache struct {
	mu       sync.Mutex
	settings *TimerSettings
	loadedAt time.Time
}

// timerSettings は、EpgTimerSrv.ini の設定を返す。timerSettingsTTL の間は、前に読んだものを使う。
// EpgTimerSrv が返さなければ (互換動作を有効にしていない)、既定の設定 (EPG の取得の時刻は分からないので空) を返す。
// 読み直せなければ (EDCB に届かないなど)、前に読んだものがあれば使い続ける。設定は利用者が変えたときだけ変わるので、
// 一時的に届かないだけで、EPG の取得の時刻が分からないものとして扱いを変えないためである (#1856)。
func (s *TunerServer) timerSettings(ctx context.Context) (*TimerSettings, error) {
	s.settings.mu.Lock()
	defer s.settings.mu.Unlock()
	if s.settings.settings != nil && s.now().Sub(s.settings.loadedAt) < timerSettingsTTL {
		return s.settings.settings, nil
	}
	files, err := s.client.Files(ctx, []string{timerSettingsFile})
	if err != nil {
		if s.settings.settings != nil {
			return s.settings.settings, nil
		}

		return nil, err
	}
	settings := defaultTimerSettings()
	if len(files) > 0 && len(files[0].Data) > 0 {
		settings = ParseTimerSettings(files[0].Data)
	}
	s.settings.settings, s.settings.loadedAt = settings, s.now()

	return settings, nil
}

// ParseTimerSettings は、EpgTimerSrv.ini の [SET] のマージンと NGEpgCapTime、[EPG_CAP] の EPG の取得の時刻、
// BonDriver の節の GetEpg と EPGCount を読む (EpgTimerSrvSetting.cpp、ReserveManager.cpp)。
//   - [EPG_CAP] の Count がなければ、毎日 23:00 に取得する (EpgTimerSrv の既定)。
//   - NGEpgCapTime の既定は 20 (分)。GetEpg の既定は 1 で、EPGCount の既定は 0 (その BonDriver のすべてのチューナー)。
//   - 時刻は「時:分」で、曜日を指定するときは「w1」(月) 〜「w7」(日) を続ける。{n}Select が 0 なら無効。
//
// 文字コードは、UTF-16 (BOM 付き。Windows の EDCB が書く)、UTF-8 (Linux の EDCB) のどちらも読む。キーと値は ASCII だけを読む。
func ParseTimerSettings(data []byte) *TimerSettings {
	sections := parseINI(data)
	settings := defaultTimerSettings()
	settings.Loaded = true
	set := sections["set"]
	if v, ok := atoi(set["ngepgcaptime"]); ok {
		settings.EPGCaptureAnyBonDriver = v == 0
	}
	for name, section := range sections {
		if !isBonDriverSection(name) {
			continue
		}
		bonDriver := tunerserver.EPGCaptureBonDriver{Disabled: false, Tuners: 0}
		if v, ok := atoi(section["getepg"]); ok && v == 0 {
			bonDriver.Disabled = true
		}
		if v, ok := atoi(section["epgcount"]); ok {
			bonDriver.Tuners = v
		}
		settings.EPGCaptureBonDrivers[name] = bonDriver
	}
	if v, ok := atoi(set["startmargin"]); ok {
		settings.StartMargin = time.Duration(v) * time.Second
	}
	if v, ok := atoi(set["endmargin"]); ok {
		settings.EndMargin = time.Duration(v) * time.Second
	}
	capture := sections["epg_cap"]
	count, ok := atoi(capture["count"])
	// EpgTimerSrv は、Count を既定値 INT_MAX で読み、INT_MAX なら未設定とみなす (EpgTimerSrvSetting.cpp)。
	if !ok || count == math.MaxInt32 {
		settings.EPGCaptureTimes = []EPGCaptureTime{{Weekday: time.Sunday, Daily: true, Minute: defaultEPGCaptureMinute}}

		return settings
	}
	// 番号を 0 から Count まで数えず、節にある番号のキーだけを見る。大きな Count で、何億回も回らないためである (#1981)。
	for _, i := range captureIndexes(capture, count) {
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

// captureIndexes は、[EPG_CAP] にある時刻の番号のうち、Count より小さいものを小さい順に返す。
func captureIndexes(capture map[string]string, count int) []int {
	indexes := []int{}
	for key := range capture {
		i, err := strconv.Atoi(key)
		// 「01」のような書き方は、EpgTimerSrv が読む「1」のキーとは別のキーなので数えない。
		if err != nil || i < 0 || i >= count || strconv.Itoa(i) != key {
			continue
		}
		indexes = append(indexes, i)
	}
	slices.Sort(indexes)

	return indexes
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
