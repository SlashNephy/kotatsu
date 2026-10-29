package stream

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
)

// errNegativePosition は、読む位置を、ファイルの始まりより前に動かそうとしたことを表す。
var errNegativePosition = errors.New("seek to a negative position")

// contentPart は、Content を作る範囲の 1 つ。ファイルの範囲か、PAT と PMT のようにメモリーにあるバイト列のどちらか。
type contentPart struct {
	file *os.File
	// from は、file の読み始めの位置。
	from int64
	data []byte
	size int64
}

// Content は、録画を終えた録画を、引き継ぎで切り詰めた後の流れの、1 本のファイルとして読む (#535)。
// 外部プレイヤーが HTTP の Range でシークできるよう、http.ServeContent に渡す。
type Content struct {
	parts []*contentPart
	size  int64
	// offset は、次に読む位置。
	offset int64
}

var _ io.ReadSeekCloser = new(Content)

// Content は、録画の始まりから終わりまでを、1 本のファイルとして開く。録画中の録画は、長さが決まらないので開けない。
// 途中から流すセグメントの前には、Write と同じく、その手前の PAT と PMT を置く。
func (r *Recording) Content() (*Content, error) {
	if r.InProgress {
		return nil, errors.New("the recording is still in progress")
	}
	pieces, _ := plan(r.segments, 0)
	c := &Content{parts: make([]*contentPart, 0, len(pieces)*2), size: 0, offset: 0}
	for i, p := range pieces {
		if err := c.add(r.streamer.store, p, i > 0); err != nil {
			_ = c.Close()

			return nil, err
		}
	}

	return c, nil
}

// Size は、Content の長さ (バイト)。
func (c *Content) Size() int64 {
	return c.size
}

func (c *Content) Read(p []byte) (int, error) {
	start := int64(0)
	for _, part := range c.parts {
		end := start + part.size
		if c.offset >= end {
			start = end

			continue
		}
		within := c.offset - start
		limit := min(int64(len(p)), part.size-within)
		var n int
		var err error
		if part.file == nil {
			n = copy(p[:limit], part.data[within:])
		} else {
			n, err = part.file.ReadAt(p[:limit], part.from+within)
			if errors.Is(err, io.EOF) && n > 0 {
				err = nil
			}
		}
		c.offset += int64(n)
		if err != nil {
			return n, fmt.Errorf("failed to read the segment: %w", err)
		}

		return n, nil
	}

	return 0, io.EOF
}

func (c *Content) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += c.offset
	case io.SeekEnd:
		offset += c.size
	default:
		return 0, fmt.Errorf("invalid whence %d", whence)
	}
	if offset < 0 {
		return 0, errNegativePosition
	}
	c.offset = offset

	return offset, nil
}

// Close は、開いたセグメントのファイルを閉じる。
func (c *Content) Close() error {
	errs := []error{}
	for _, part := range c.parts {
		if part.file != nil {
			errs = append(errs, part.file.Close())
		}
	}

	return errors.Join(errs...)
}

// add は、範囲 p を、Content の終わりに足す。
func (c *Content) add(store *kago.Store, p *piece, joined bool) error {
	file, err := store.Open(p.segment.poolID, p.segment.path)
	if err != nil {
		return fmt.Errorf("failed to open the segment: %w", err)
	}
	to, err := servedEnd(file, p)
	if err != nil {
		_ = file.Close()

		return err
	}
	if p.from > 0 {
		var prelude bytes.Buffer
		if err := writePrelude(&prelude, file, p.from); err != nil {
			_ = file.Close()

			return err
		}
		c.parts = append(c.parts, &contentPart{file: nil, from: 0, data: prelude.Bytes(), size: int64(prelude.Len())})
		c.size += int64(prelude.Len())
	}
	from := p.from
	// 前のセグメントにつなぐなら、頭の前から続く PES の残りを落とす (writeHead と同じ)。
	if joined && !p.segment.continues {
		head, consumed, err := completeHead(file, p.from, to)
		if err != nil {
			_ = file.Close()

			return err
		}
		c.parts = append(c.parts, &contentPart{file: nil, from: 0, data: head, size: int64(len(head))})
		c.size += int64(len(head))
		from += consumed
	}
	c.parts = append(c.parts, &contentPart{file: file, from: from, data: nil, size: max(to-from, 0)})
	c.size += max(to-from, 0)

	return nil
}
