// Package ffprogress は、FFmpeg の -progress の出力を読み、録画の後処理のジョブの進み具合 (割合と残り時間) にする (#1511、設計書 6.1)。
//
// 割合の分子は、エンコードしたコマ数 (frame) にする。out_time_us は mux が最後に書いたパケットの時刻で、疎な字幕で先に飛んだり、
// 壊れた副音声で止まったりする。入力の読み位置は、デマクサが先に読み切って終わり近くに貼り付く。
package ffprogress

import (
	"bytes"
	"strconv"
	"strings"
)

// Block は、-progress の 1 まとまり。FFmpeg は、key=value の行を並べ、progress=continue か progress=end で区切る。
type Block struct {
	// Frame は、エンコードしたコマ数。まとまりになければ -1。
	Frame int64
	// TotalSize は、書いた出力の大きさ (バイト)。分からなければ -1。
	TotalSize int64
	// DropFrames は、捨てたコマ数。分からなければ -1。
	DropFrames int64
	// Bitrate と Speed は、FFmpeg が出したままの文字列 (例: 2963.4kbits/s、1.02x)。ログに出すだけなので、読み替えない。
	Bitrate string
	Speed   string
	// End は、progress=end で終わったまとまりかどうか。FFmpeg が書き終えたときにだけ届く。
	End bool
}

func emptyBlock() Block {
	return Block{Frame: -1, TotalSize: -1, DropFrames: -1, Bitrate: "", Speed: "", End: false}
}

// Parser は、-progress の出力を行ごとに読み、まとまりごとに onBlock を呼ぶ io.Writer。
// key=value の形でない行は、onLine に渡す。標準エラー出力に混ぜて出させたとき (CM の区間の見分け) に、ほかのログを読むのに使う。
// 書き込みは 1 つのゴルーチンから行う (exec.Cmd の標準出力や標準エラー出力に置く)。
type Parser struct {
	onBlock func(*Block)
	onLine  func(string)
	buf     []byte
	block   Block
}

// NewParser は Parser を返す。onLine が nil なら、ほかの行は捨てる。
func NewParser(onBlock func(*Block), onLine func(string)) *Parser {
	return &Parser{onBlock: onBlock, onLine: onLine, buf: []byte{}, block: emptyBlock()}
}

func (p *Parser) Write(b []byte) (int, error) {
	p.buf = append(p.buf, b...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			break
		}
		p.line(strings.TrimRight(string(p.buf[:i]), "\r"))
		p.buf = p.buf[i+1:]
	}

	return len(b), nil
}

// Flush は、改行で終わっていない最後の行を読む。書き込みが終わった後に呼ぶ。
func (p *Parser) Flush() {
	if len(p.buf) > 0 {
		p.line(strings.TrimRight(string(p.buf), "\r"))
		p.buf = p.buf[:0]
	}
}

func (p *Parser) line(line string) {
	key, value, ok := progressLine(line)
	if !ok {
		if p.onLine != nil {
			p.onLine(line)
		}

		return
	}
	switch key {
	case "frame":
		p.block.Frame = parseCount(value, p.block.Frame)
	case "total_size":
		p.block.TotalSize = parseCount(value, p.block.TotalSize)
	case "drop_frames":
		p.block.DropFrames = parseCount(value, p.block.DropFrames)
	case "bitrate":
		p.block.Bitrate = value
	case "speed":
		p.block.Speed = value
	case "progress":
		p.block.End = value == "end"
		block := p.block
		p.block = emptyBlock()
		p.onBlock(&block)
	}
}

// progressLine は、行が -progress の key=value の行なら、キーと値を返す。キーは英小文字、数字、_ だけからなる。
// FFmpeg のほかのログ (「[silencedetect @ 0x...] silence_start: 1.2」など) は、キーに当たるところに空白や記号を含むので、見分けられる。
func progressLine(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, "=")
	if !ok || key == "" {
		return "", "", false
	}
	for _, r := range key {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return "", "", false
		}
	}

	return key, strings.TrimSpace(value), true
}

// parseCount は、数を読む。N/A や、読めない値 (終わりに出る統計の行など) なら、prev のままにする。
func parseCount(value string, prev int64) int64 {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 0 {
		return prev
	}

	return n
}
