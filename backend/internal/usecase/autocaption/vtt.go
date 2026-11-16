// Package autocaption は、録画の音声から字幕を自動生成する (設計書 6.1、#1682)。
//
// 録画を終えた後のジョブとして、FFmpeg で録画の音声を取り出し、whisper.cpp の whisper-cli を子プロセスとして動かして、
// 出力した WebVTT を読む。字幕は、CM の区間と同じく、録画を始めた日時からの時間にそろえて DB に残し、プレイヤーが「自動生成」の字幕として出す。
// whisper-cli は -ffmpeg のイメージに入れ、モデルは管理者が画面からダウンロードするか (usecase/captionmodel)、手で置く (設定の captions)。
package autocaption

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Cue は、自動生成した字幕の 1 つ。Start と End は、録画を始めた日時からの時間 (ParseVTT が返すものは、音声の始まりからの時間)。
type Cue struct {
	Start time.Duration
	End   time.Duration
	Text  string
}

// errNoHeader は、WebVTT の最初の行が WEBVTT でないことを表す。
var errNoHeader = errors.New("the file is not WebVTT")

// timingSeparator は、WebVTT の字幕の時刻の行の、始まりと終わりの区切り。
const timingSeparator = "-->"

// tagPattern は、字幕の本文の中のタグ (<c>、<i>、単語ごとの時刻 <00:00:01.000> など)。プレイヤーは本文だけを出すので取り除く。
var tagPattern = regexp.MustCompile(`<[^>]*>`)

// ParseVTT は、WebVTT を読み、本文のある字幕を、出てくる順に返す。
// 時刻は、時の欄のある形 (00:01:02.345) とない形 (01:02.345) のどちらも読む。字幕の ID の行、時刻の後の設定、NOTE と STYLE の塊は読み飛ばす。
// 本文が空の字幕 (whisper が無音の区間に出すもの) と、終わりが始まりより後でない字幕は捨てる。本文の改行は残す。
func ParseVTT(r io.Reader) ([]Cue, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read the WebVTT: %w", err)
		}

		return nil, errNoHeader
	}
	if header := strings.TrimPrefix(scanner.Text(), "\uFEFF"); header != "WEBVTT" && !strings.HasPrefix(header, "WEBVTT ") && !strings.HasPrefix(header, "WEBVTT\t") {
		return nil, errNoHeader
	}
	cues := []Cue{}
	var (
		current *Cue
		lines   []string
	)
	flush := func() {
		if current == nil {
			return
		}
		text := cueText(lines)
		if text != "" && current.End > current.Start {
			current.Text = text
			cues = append(cues, *current)
		}
		current, lines = nil, nil
	}
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			flush()

			continue
		}
		if current != nil {
			lines = append(lines, line)

			continue
		}
		// 時刻の行より前の行は、字幕の ID か、NOTE や STYLE の塊。時刻の行でなければ読み飛ばす。
		if !strings.Contains(line, timingSeparator) {
			continue
		}
		start, end, err := parseTiming(line)
		if err != nil {
			return nil, err
		}
		current = &Cue{Start: start, End: end, Text: ""}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the WebVTT: %w", err)
	}
	flush()

	return cues, nil
}

// cueText は、字幕の本文の行から、タグを取り除き、前後の空白を落として、改行でつなぐ。空になった行は捨てる。
func cueText(lines []string) string {
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if text := strings.TrimSpace(tagPattern.ReplaceAllString(line, "")); text != "" {
			kept = append(kept, text)
		}
	}

	return strings.Join(kept, "\n")
}

// parseTiming は、「00:00:01.000 --> 00:00:02.500 align:start」のような時刻の行を読む。
func parseTiming(line string) (time.Duration, time.Duration, error) {
	from, rest, _ := strings.Cut(line, timingSeparator)
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0, 0, fmt.Errorf("the WebVTT cue timing has no end: %q", line)
	}
	start, err := parseTimestamp(strings.TrimSpace(from))
	if err != nil {
		return 0, 0, err
	}
	end, err := parseTimestamp(fields[0])
	if err != nil {
		return 0, 0, err
	}

	return start, end, nil
}

// parseTimestamp は、WebVTT の時刻 (hh:mm:ss.ttt か mm:ss.ttt) を読む。
func parseTimestamp(s string) (time.Duration, error) {
	clock, millis, ok := strings.Cut(s, ".")
	if !ok || len(millis) != 3 {
		return 0, fmt.Errorf("invalid WebVTT timestamp: %q", s)
	}
	parts := strings.Split(clock, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return 0, fmt.Errorf("invalid WebVTT timestamp: %q", s)
	}
	units := []time.Duration{time.Hour, time.Minute, time.Second}[3-len(parts):]
	var d time.Duration
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || (i > 0 && n > 59) {
			return 0, fmt.Errorf("invalid WebVTT timestamp: %q", s)
		}
		d += time.Duration(n) * units[i]
	}
	ms, err := strconv.Atoi(millis)
	if err != nil || ms < 0 {
		return 0, fmt.Errorf("invalid WebVTT timestamp: %q", s)
	}

	return d + time.Duration(ms)*time.Millisecond, nil
}
