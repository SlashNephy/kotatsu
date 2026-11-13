package edcb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
	"unicode/utf16"
)

// ErrMalformed は、EpgTimerSrv の応答が、CtrlCmd の形に合わないことを表す。
var ErrMalformed = errors.New("malformed CtrlCmd payload")

// jstOffset は、日本標準時の UTC からのずれ (秒)。
const jstOffset = 9 * 60 * 60

// jst は、EDCB の時刻のタイムゾーンを返す。SYSTEMTIME はタイムゾーンを持たず、放送の時刻 (日本標準時) で書かれる。
func jst() *time.Location {
	return time.FixedZone("JST", jstOffset)
}

// encoder は、CtrlCmd の本文を書く。数はすべてリトルエンディアンで書く (EDCB の Common/CtrlCmdUtil.cpp)。
type encoder struct {
	buf []byte
}

func (e *encoder) u16(v uint16) {
	e.buf = binary.LittleEndian.AppendUint16(e.buf, v)
}

func (e *encoder) u32(v uint32) {
	e.buf = binary.LittleEndian.AppendUint32(e.buf, v)
}

func (e *encoder) i32(v int32) {
	e.buf, _ = binary.Append(e.buf, binary.LittleEndian, v)
}

func (e *encoder) i64(v int64) {
	e.buf, _ = binary.Append(e.buf, binary.LittleEndian, v)
}

// size は、書いた長さを DWORD にする。CtrlCmd の本文は DWORD に収まる大きさしか送らない。
func size(n int) uint32 {
	if n < 0 || n > math.MaxUint32 {
		panic(fmt.Sprintf("CtrlCmd payload size %d does not fit in a DWORD", n))
	}

	return uint32(n)
}

// str は、文字列を、全体の大きさ (DWORD、大きさ自身を含む) と、UTF-16LE の文字と、終わりの 0 で書く。
func (e *encoder) str(s string) {
	units := utf16.Encode([]rune(s))
	e.u32(size(4 + 2*(len(units)+1)))
	for _, u := range units {
		e.u16(u)
	}
	e.u16(0)
}

// sized は、先頭に全体の大きさ (DWORD、大きさ自身を含む) を置く構造体を書く。
func (e *encoder) sized(body func(*encoder)) {
	start := len(e.buf)
	e.u32(0)
	body(e)
	binary.LittleEndian.PutUint32(e.buf[start:], size(len(e.buf)-start))
}

// vector は、先頭に全体の大きさ (DWORD、大きさと数を含む) と数 (DWORD) を置く配列を書く。
func (e *encoder) vector(n int, item func(*encoder, int)) {
	start := len(e.buf)
	e.u32(0)
	e.u32(size(n))
	for i := range n {
		item(e, i)
	}
	binary.LittleEndian.PutUint32(e.buf[start:], size(len(e.buf)-start))
}

// decoder は、CtrlCmd の本文を読む。読めなければ、それ以降の読み取りはゼロ値を返し、err に最初の誤りを残す。
// 構造体と配列は、先頭の大きさまでで読み終えたことにする。新しい版の EpgTimerSrv が末尾に項目を足しても読めるようにするため。
type decoder struct {
	b   []byte
	pos int
	err error
}

func newDecoder(b []byte) *decoder {
	return &decoder{b: b, pos: 0, err: nil}
}

func (d *decoder) failf(format string, args ...any) {
	if d.err == nil {
		d.err = fmt.Errorf("%w: "+format, append([]any{ErrMalformed}, args...)...)
	}
}

// remaining は、まだ読んでいないバイト数を返す。版によって後ろに項目が増える構造体で、項目があるかを確かめるのに使う。
func (d *decoder) remaining() int {
	if d.err != nil {
		return 0
	}

	return len(d.b) - d.pos
}

// take は、n バイトを読む。足りなければ nil を返す。
func (d *decoder) take(n int) []byte {
	if d.err != nil {
		return nil
	}
	if n < 0 || len(d.b)-d.pos < n {
		d.failf("want %d bytes at %d, have %d", n, d.pos, len(d.b)-d.pos)

		return nil
	}
	v := d.b[d.pos : d.pos+n]
	d.pos += n

	return v
}

func (d *decoder) u8() uint8 {
	if v := d.take(1); v != nil {
		return v[0]
	}

	return 0
}

func (d *decoder) u16() uint16 {
	if v := d.take(2); v != nil {
		return binary.LittleEndian.Uint16(v)
	}

	return 0
}

func (d *decoder) u32() uint32 {
	if v := d.take(4); v != nil {
		return binary.LittleEndian.Uint32(v)
	}

	return 0
}

func (d *decoder) i32() int32 {
	var v int32
	if b := d.take(4); b != nil {
		_, _ = binary.Decode(b, binary.LittleEndian, &v)
	}

	return v
}

func (d *decoder) u64() uint64 {
	if v := d.take(8); v != nil {
		return binary.LittleEndian.Uint64(v)
	}

	return 0
}

func (d *decoder) f32() float32 {
	return math.Float32frombits(d.u32())
}

// region は、先頭の大きさ (大きさ自身を含む) で区切った範囲を読むための decoder を返し、この decoder をその後ろへ進める。
// header は、大きさの後に続く、範囲に含まれる固定の項目のバイト数 (配列の数の 4 など)。
func (d *decoder) region(header int) *decoder {
	start := d.pos
	size := int(d.u32())
	if d.err != nil {
		return &decoder{b: nil, pos: 0, err: d.err}
	}
	if size < 4+header || size-4 > len(d.b)-d.pos {
		d.failf("invalid size %d at %d", size, start)

		return &decoder{b: nil, pos: 0, err: d.err}
	}
	inner := &decoder{b: d.b[d.pos : start+size], pos: 0, err: nil}
	d.pos = start + size

	return inner
}

// str は、文字列を読む。終わりの 0 か、大きさの終わりまでを文字とする。
func (d *decoder) str() string {
	r := d.region(0)
	if r.err != nil {
		d.failf("%v", r.err)

		return ""
	}
	units := make([]uint16, 0, len(r.b)/2)
	for len(r.b)-r.pos >= 2 {
		u := binary.LittleEndian.Uint16(r.b[r.pos:])
		r.pos += 2
		if u == 0 {
			break
		}
		units = append(units, u)
	}

	return string(utf16.Decode(units))
}

// systemTime は、SYSTEMTIME (年、月、曜日、日、時、分、秒、ミリ秒の WORD) を、日本標準時の時刻として読む。
func (d *decoder) systemTime() time.Time {
	year, month := int(d.u16()), int(d.u16())
	_ = d.u16() // 曜日は、日付から決まるので読まない。
	day, hour, minute, second, millisecond := int(d.u16()), int(d.u16()), int(d.u16()), int(d.u16()), int(d.u16())
	if d.err != nil {
		return time.Time{}
	}

	return time.Date(year, time.Month(month), day, hour, minute, second, millisecond*int(time.Millisecond), jst())
}

// structure は、構造体を読む。読み終えたら、構造体の終わりへ進む。
func (d *decoder) structure(body func(*decoder)) {
	r := d.region(0)
	if r.err == nil {
		body(r)
	}
	if r.err != nil {
		d.failf("%v", r.err)
	}
}

// optionalStructure は、構造体を読む。中身のない構造体 (大きさが 4) なら body を呼ばず、false を返す。
// EPGDB_EVENT_INFO の基本情報や拡張情報などは、持たないときに中身のない構造体を置く。
func (d *decoder) optionalStructure(body func(*decoder)) bool {
	r := d.region(0)
	if r.err != nil {
		d.failf("%v", r.err)

		return false
	}
	if len(r.b) == 0 {
		return false
	}
	body(r)
	if r.err != nil {
		d.failf("%v", r.err)
	}

	return true
}

// vector は、配列を読み、要素ごとに item を呼ぶ。数は、残りのバイト数で収まる分までしか信じない。
// 要素はどれも 4 バイト以上 (構造体の大きさか DWORD) なので、4 で割った数を超えれば壊れた応答とする。
// 壊れた応答の大きな数で、先に大きく確保しないためである。
func (d *decoder) vector(item func(*decoder)) {
	r := d.region(4)
	if r.err != nil {
		d.failf("%v", r.err)

		return
	}
	n := int(r.u32())
	if n > (len(r.b)-r.pos)/minItemSize {
		d.failf("vector count %d exceeds the remaining %d bytes", n, len(r.b)-r.pos)

		return
	}
	for range n {
		item(r)
		if r.err != nil {
			d.failf("%v", r.err)

			return
		}
	}
}
