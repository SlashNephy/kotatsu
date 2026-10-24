package stream

import (
	"math"
	"net/http"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// serviceIDBase は、Mirakurun のサービス ID を組み立てる基数 (networkId × 100000 + serviceId)。
	serviceIDBase = 100_000
	// syncByte は、TS パケットの先頭のバイト。
	syncByte = 0x47
)

// dataBroadcastWriter は、書かれた TS から、データ放送の描画に要るパケットだけを応答に書く (設計書 7.4)。
// Unwrap で元の応答を返すので、http.ResponseController で送り出せる。
type dataBroadcastWriter struct {
	http.ResponseWriter

	filter *ts.DataBroadcastFilter
	// pending は、前の Write で余った、1 パケットに満たないバイト。
	pending []byte
	// out は、1 回の Write で残すパケットをまとめて書くためのバッファ。
	out []byte
}

var _ http.ResponseWriter = new(dataBroadcastWriter)

// programNumber は、Mirakurun のサービス ID (networkId × 100000 + serviceId) から、TS の program_number を取り出す。
// program_number は 16 ビットなので、収まらなければ false を返す。
func programNumber(serviceID int64) (uint16, bool) {
	number := serviceID % serviceIDBase
	if number < 0 || number > math.MaxUint16 {
		return 0, false
	}

	return uint16(number), true
}

// newDataBroadcastWriter は、program_number が number のサービスの dataBroadcastWriter を返す。
func newDataBroadcastWriter(w http.ResponseWriter, number uint16) *dataBroadcastWriter {
	return &dataBroadcastWriter{
		ResponseWriter: w,
		filter:         ts.NewDataBroadcastFilter(number),
		pending:        make([]byte, 0, ts.PacketSize),
		out:            make([]byte, 0, copyBufferSize),
	}
}

// Write は、p をパケットに切り分け、残すパケットだけを応答に書く。
// パケットの境目がずれていれば、次の sync byte まで読み飛ばす。
func (d *dataBroadcastWriter) Write(p []byte) (int, error) {
	d.out = d.out[:0]
	data := p
	if len(d.pending) > 0 {
		need := min(ts.PacketSize-len(d.pending), len(data))
		d.pending = append(d.pending, data[:need]...)
		data = data[need:]
		if len(d.pending) < ts.PacketSize {
			return len(p), nil
		}
		d.keep(d.pending)
		d.pending = d.pending[:0]
	}
	for len(data) > 0 {
		if data[0] != syncByte {
			data = data[1:]

			continue
		}
		if len(data) < ts.PacketSize {
			d.pending = append(d.pending, data...)

			break
		}
		d.keep(data[:ts.PacketSize])
		data = data[ts.PacketSize:]
	}
	if len(d.out) == 0 {
		return len(p), nil
	}
	if _, err := d.ResponseWriter.Write(d.out); err != nil {
		return 0, err
	}

	return len(p), nil
}

// Unwrap は、元の応答を返す。
func (d *dataBroadcastWriter) Unwrap() http.ResponseWriter {
	return d.ResponseWriter
}

// keep は、packet を残すなら out に足す。
func (d *dataBroadcastWriter) keep(packet []byte) {
	if kept, ok := d.filter.Filter(packet); ok {
		d.out = append(d.out, kept...)
	}
}
