package ts

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// 索引ファイルの形式 (設計書 5.3)。
//
// 先頭の 8 バイトがヘッダーで、その後に 16 バイトのレコードが I フレームの順に並ぶ。数値はすべてリトルエンディアン。
//
//	ヘッダー:   magic "KTIX" (4 バイト) | 版 uint16 | レコードの大きさ uint16
//	レコード:   I フレームを含む PES の先頭パケットの位置 int64 | その PES の PTS int64
//
// レコードは録画と同時に追記していくので、書き込みの途中で止まったファイルは、末尾のレコードが欠けていることがある。
// 読むときは、欠けたレコードを捨てる。2 時間の録画で、I フレームが 0.5 秒ごとなら 230 KB ほどになる。
const (
	indexMagic      = "KTIX"
	indexVersion    = 1
	indexHeaderSize = 8
	indexRecordSize = 16
)

// ptsWrap は PTS が一周する値 (33 ビット)。
const ptsWrap = 1 << 33

// PTSClock は PTS の 1 秒あたりの刻み (90 kHz)。
const PTSClock = 90_000

// IndexEntry は、索引の 1 件。
type IndexEntry struct {
	// Offset は、I フレームを含む PES の先頭パケットの位置 (ファイルの先頭からのバイト数)。
	// シーケンスヘッダーもこのパケットから始まるので、ここから流せばデコーダーがすぐに絵を出せる。
	Offset int64
	// PTS は、その PES の PTS (90 kHz、33 ビット)。一周したときの補正はしていない。
	// 同じ放送を録画した別のセグメントとも比べられるよう、放送局が付けた値のまま持つ。
	PTS int64
}

// IndexWriter は、索引ファイルにレコードを追記する。
type IndexWriter struct {
	w   io.Writer
	buf []byte
}

// NewIndexWriter は、w にヘッダーを書き、IndexWriter を作る。w は空のファイルであること。
func NewIndexWriter(w io.Writer) (*IndexWriter, error) {
	header := make([]byte, 0, indexHeaderSize)
	header = append(header, indexMagic...)
	header = binary.LittleEndian.AppendUint16(header, indexVersion)
	header = binary.LittleEndian.AppendUint16(header, indexRecordSize)
	if _, err := w.Write(header); err != nil {
		return nil, fmt.Errorf("failed to write the index header: %w", err)
	}

	return &IndexWriter{w: w, buf: make([]byte, 0, indexRecordSize)}, nil
}

// Append は 1 件を追記する。レコードは 1 回の Write で書くので、ファイルに直接書けば、他のプロセスからすぐに読める。
func (w *IndexWriter) Append(entry IndexEntry) error {
	record, err := binary.Append(w.buf[:0], binary.LittleEndian, [2]int64{entry.Offset, entry.PTS})
	if err != nil {
		return fmt.Errorf("failed to encode an index entry: %w", err)
	}
	if _, err := w.w.Write(record); err != nil {
		return fmt.Errorf("failed to append to the index: %w", err)
	}

	return nil
}

// ErrInvalidIndex は、索引ファイルの形式が正しくないことを表す。
var ErrInvalidIndex = errors.New("invalid index file")

// ReadIndex は索引ファイルを読む。末尾の欠けたレコードは捨てる。
func ReadIndex(r io.Reader) ([]IndexEntry, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read the index: %w", err)
	}
	if len(b) < indexHeaderSize || !bytes.Equal(b[:4], []byte(indexMagic)) {
		return nil, fmt.Errorf("%w: missing header", ErrInvalidIndex)
	}
	if v := binary.LittleEndian.Uint16(b[4:6]); v != indexVersion {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrInvalidIndex, v)
	}
	if size := binary.LittleEndian.Uint16(b[6:8]); size != indexRecordSize {
		return nil, fmt.Errorf("%w: unexpected record size %d", ErrInvalidIndex, size)
	}

	records := b[indexHeaderSize:]
	entries := make([]IndexEntry, 0, len(records)/indexRecordSize)
	for ; len(records) >= indexRecordSize; records = records[indexRecordSize:] {
		var record [2]int64
		if _, err := binary.Decode(records[:indexRecordSize], binary.LittleEndian, &record); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidIndex, err)
		}
		entries = append(entries, IndexEntry{Offset: record[0], PTS: record[1]})
	}

	return entries, nil
}

// PTSDiff は、PTS の from から to までの刻みの数を返す。PTS が一周していても、差が半周より小さければ正しく求まる。
func PTSDiff(from, to int64) int64 {
	d := (to - from) % ptsWrap
	if d < 0 {
		d += ptsWrap
	}
	if d >= ptsWrap/2 {
		d -= ptsWrap
	}

	return d
}
