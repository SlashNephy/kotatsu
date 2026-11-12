package logreport

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/logring"
)

var (
	// mirakurunLine は、Mirakurun の 1 行 (ISO 8601 の時刻、重さ、コロン、本文)。
	mirakurunLine = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}\S*) ([A-Za-z]+): ?(.*)$`)
	// logfmtTime と logfmtLevel は、Mahiron が出す logfmt (Go の slog の TextHandler) の時刻と重さ。
	logfmtTime  = regexp.MustCompile(`^time=("[^"]*"|\S+)`)
	logfmtLevel = regexp.MustCompile(`(?:^|\s)level=("[^"]*"|\S+)`)
	// leadingTime は、重さの無い行の、先頭の時刻。
	leadingTime = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)\s+(.*)$`)

	// errorWords と warnWords は、重さの無い行の重さを、本文の言葉から推し量るのに使う。
	errorWords = regexp.MustCompile(`(?i)\b(?:fail(?:s|ed|ure|ures|ing)?|errors?|exceptions?|fatal|critical|panic(?:ked)?)\b|失敗|エラー|例外|できません|できない`)
	warnWords  = regexp.MustCompile(`(?i)\bwarn(?:s|ing|ings)?\b|警告|注意`)
)

// ParseTunerLog は、チューナーサーバーの /api/log の本文を、古い順の行にする (#1521)。
// 形式の違う行 (時刻の読めない行) は捨てる。重さが無ければ、本文の言葉から推し量り、Inferred を立てる。
// 時刻にタイムゾーンが無ければ、location の時刻として読む。
func ParseTunerLog(body string, location *time.Location) []Line {
	lines := []Line{}
	for raw := range strings.SplitSeq(body, "\n") {
		raw = strings.TrimRight(raw, "\r")
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if line, ok := parseLine(raw, location); ok {
			lines = append(lines, line)
		}
	}

	return lines
}

func parseLine(raw string, location *time.Location) (Line, bool) {
	switch {
	case strings.HasPrefix(raw, "{"):
		return parseJSONLine(raw, location)
	case strings.HasPrefix(raw, "time="):
		return parseLogfmtLine(raw, location)
	}
	if m := mirakurunLine.FindStringSubmatch(raw); m != nil {
		at, ok := parseTime(m[1], location)
		if !ok {
			return noLine(), false
		}
		if level, ok := levelOf(m[2]); ok {
			return Line{Time: at, Level: level, Inferred: false, Text: m[3]}, true
		}
	}
	if m := leadingTime.FindStringSubmatch(raw); m != nil {
		if at, ok := parseTime(m[1], location); ok {
			return inferred(at, m[2]), true
		}
	}

	return noLine(), false
}

// parseLogfmtLine は、time=... level=... msg=... の行を読む。本文は、時刻と重さを除いた残り。
func parseLogfmtLine(raw string, location *time.Location) (Line, bool) {
	m := logfmtTime.FindStringSubmatch(raw)
	if m == nil {
		return noLine(), false
	}
	at, ok := parseTime(strings.Trim(m[1], `"`), location)
	if !ok {
		return noLine(), false
	}
	rest := strings.TrimSpace(raw[len(m[0]):])
	if lm := logfmtLevel.FindStringSubmatchIndex(rest); lm != nil {
		word := strings.Trim(rest[lm[2]:lm[3]], `"`)
		text := strings.TrimSpace(rest[:lm[0]] + " " + rest[lm[1]:])
		if level, ok := levelOf(word); ok {
			return Line{Time: at, Level: level, Inferred: false, Text: text}, true
		}

		return inferred(at, text), true
	}

	return inferred(at, rest), true
}

// parseJSONLine は、{"time":...,"level":...,"msg":...} の行を読む。本文は、msg と、残りの属性の JSON。
func parseJSONLine(raw string, location *time.Location) (Line, bool) {
	var fields map[string]any
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return noLine(), false
	}
	rawTime, _ := fields["time"].(string)
	at, ok := parseTime(rawTime, location)
	if !ok {
		return noLine(), false
	}
	message, _ := fields["msg"].(string)
	word, _ := fields["level"].(string)
	for _, key := range []string{"time", "level", "msg"} {
		delete(fields, key)
	}
	text := message
	if len(fields) > 0 {
		if attrs, err := json.Marshal(fields); err == nil {
			text = strings.TrimSpace(text + " " + string(attrs))
		}
	}
	if level, ok := levelOf(word); ok {
		return Line{Time: at, Level: level, Inferred: false, Text: text}, true
	}

	return inferred(at, text), true
}

func parseTime(value string, location *time.Location) (int64, bool) {
	if location == nil {
		location = time.UTC
	}
	// 行の時刻として読む形。
	for _, layout := range [...]string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.999999999Z0700",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05.999999999",
	} {
		if at, err := time.ParseInLocation(layout, value, location); err == nil {
			return at.UnixMilli(), true
		}
	}

	return 0, false
}

// levelOf は、行に書かれた重さの語を、3 段階の重さにする。知らない語なら false を返す。
func levelOf(word string) (logring.Level, bool) {
	switch strings.ToLower(word) {
	case "trace", "debug", "info", "notice":
		return logring.LevelInfo, true
	case "warn", "warning":
		return logring.LevelWarn, true
	case "error", "fatal", "critical", "panic", "crit", "err":
		return logring.LevelError, true
	default:
		return logring.LevelInfo, false
	}
}

// inferred は、重さの無い行の重さを、本文の言葉から推し量った行を返す。
func inferred(at int64, text string) Line {
	return Line{Time: at, Level: InferLevel(text), Inferred: true, Text: text}
}

// InferLevel は、本文の言葉から重さを推し量る。
// 失敗やエラーの類の語を含めば error、警告の類の語を含めば warn、どちらも無ければ info にする。
func InferLevel(text string) logring.Level {
	switch {
	case errorWords.MatchString(text):
		return logring.LevelError
	case warnWords.MatchString(text):
		return logring.LevelWarn
	default:
		return logring.LevelInfo
	}
}

// lastLines は、lines の最後の limit 行を返す。
func lastLines(lines []Line, limit int) []Line {
	if limit > 0 && len(lines) > limit {
		return slices.Clone(lines[len(lines)-limit:])
	}

	return lines
}

// noLine は、読めなかったときに返す空の行。
func noLine() Line {
	return Line{Time: 0, Level: logring.LevelInfo, Inferred: false, Text: ""}
}
