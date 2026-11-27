// Command captiondump は、録画の TS から ARIB 字幕の字幕文を取り出し、1 行に 1 つずつ JSON で出力する (#2216、設計書 7.4)。
// 取り出した文を、ブラウザの字幕の履歴 (aribb24.js) の文と比較して確認するのに使う。
//
//	go run ./internal/cmd/captiondump -drcs ../frontend/vendor/libaribcaption/drcs.json 録画.m2ts
//
// 録画のパスに - を渡すと、標準入力から読む。-drcs には、外字の既定のテーブル (frontend/scripts/fetch-libaribcaption-drcs.mjs が作るもの) を渡す。省くと、外字はすべて「〓」にする。
//
// 開発と検証にだけ使うエントリーポイントなので、配布するものには入れない (設計書 11.5)。
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// line は、出力する字幕文 1 つ。
type line struct {
	PTS     int64  `json:"pts"`
	Text    string `json:"text"`
	Cleared bool   `json:"cleared"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	drcsPath := flag.String("drcs", "", "the default DRCS replacement table (drcs.json)")
	serviceID := flag.Uint("service", 0, "the service ID (program_number); 0 reads the first program with video")
	flag.Parse()
	if flag.NArg() != 1 {
		return errors.New("usage: captiondump [-drcs drcs.json] [-service id] recording.m2ts")
	}
	if *serviceID > math.MaxUint16 {
		return fmt.Errorf("the service ID must be at most %d", math.MaxUint16)
	}
	replacements, err := readReplacements(*drcsPath)
	if err != nil {
		return err
	}
	var file io.Reader = os.Stdin
	if flag.Arg(0) != "-" {
		opened, err := os.Open(flag.Arg(0))
		if err != nil {
			return fmt.Errorf("failed to open the recording: %w", err)
		}
		defer opened.Close()
		file = opened
	}

	scanner := ts.NewCaptionScanner(uint16(*serviceID), func(md5 string) (string, bool) {
		text, ok := replacements[md5]

		return text, ok
	})
	if _, err := io.Copy(scanner, bufio.NewReaderSize(file, 1<<20)); err != nil {
		return fmt.Errorf("failed to read the recording: %w", err)
	}
	out := bufio.NewWriter(os.Stdout)
	encoder := json.NewEncoder(out)
	for _, caption := range scanner.Captions() {
		if err := encoder.Encode(line{PTS: caption.PTS, Text: caption.Text, Cleared: caption.Cleared}); err != nil {
			return fmt.Errorf("failed to write a caption: %w", err)
		}
	}
	if err := out.Flush(); err != nil {
		return fmt.Errorf("failed to write the captions: %w", err)
	}

	return nil
}

// readReplacements は、外字の既定のテーブルを読む。path が空なら、空のテーブルを返す。
func readReplacements(path string) (map[string]string, error) {
	if path == "" {
		return map[string]string{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read the DRCS table: %w", err)
	}
	var table struct {
		Replacements map[string]string `json:"replacements"`
	}
	if err := json.Unmarshal(data, &table); err != nil {
		return nil, fmt.Errorf("failed to decode the DRCS table: %w", err)
	}

	return table.Replacements, nil
}
