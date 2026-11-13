package edcb_test

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
)

// payload は、テストで CtrlCmd の本文を組む。
type payload []byte

func (p payload) u8(v uint8) payload   { return append(p, v) }
func (p payload) u16(v uint16) payload { return binary.LittleEndian.AppendUint16(p, v) }
func (p payload) u32(v uint32) payload { return binary.LittleEndian.AppendUint32(p, v) }

// str は、文字列を、全体の大きさと UTF-16LE の文字と終わりの 0 で書く。
func (p payload) str(s string) payload {
	units := utf16.Encode([]rune(s))
	p = p.u32(dword(4 + 2*(len(units)+1)))
	for _, u := range units {
		p = p.u16(u)
	}

	return p.u16(0)
}

// sized は、body を、全体の大きさを先頭に置いた構造体にする。
func sized(body payload) payload {
	return payload{}.u32(dword(4 + len(body))).append(body)
}

// vector は、items を、全体の大きさと数を先頭に置いた配列にする。
func vector(items ...payload) payload {
	body := payload{}
	for _, item := range items {
		body = body.append(item)
	}

	return payload{}.u32(dword(8 + len(body))).u32(dword(len(items))).append(body)
}

func (p payload) append(b payload) payload { return append(p, b...) }

// dword は、テストで組む小さな長さを DWORD にする。
func dword(n int) uint32 {
	if n < 0 || n > 1<<20 {
		panic("too large for a test payload")
	}

	return uint32(n)
}

// service は、EPGDB_SERVICE_INFO を組む。extra は、新しい版が末尾に足した項目を真似る。
func service(name string, extra payload) payload {
	body := payload{}.u16(1).u16(2).u16(3).u8(1).u8(0).str("").str(name).str("").str("").u8(5).append(extra)

	return sized(body)
}

func TestServiceNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "empty", in: ""},
		{name: "japanese", in: "ＮＨＫ総合１・東京"},
		// BMP の外の文字は、UTF-16 のサロゲートペアになる。
		{name: "surrogate pair", in: "𠮷野家🈑"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newFakeServer(t, map[uint32]reply{1021: {status: 1, payload: vector(service(tt.in, nil)), stream: nil, hold: false, drop: false}})
			got, err := s.client(t).Services(t.Context())
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, tt.in, got[0].Name)
		})
	}
}

func TestServicesSkipUnknownTrailingFields(t *testing.T) {
	t.Parallel()

	// 新しい版の EpgTimerSrv が構造体の末尾に項目を足しても、知っている項目だけを読み、次の構造体から続けて読む。
	extra := payload{}.u32(0xDEADBEEF).str("unknown")
	s := newFakeServer(t, map[uint32]reply{1021: {status: 1, payload: vector(service("A", extra), service("B", nil)), stream: nil, hold: false, drop: false}})
	got, err := s.client(t).Services(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "A", got[0].Name)
	assert.Equal(t, uint8(5), got[0].RemoteControlKeyID)
	assert.Equal(t, "B", got[1].Name)
}

func TestServicesRejectMalformed(t *testing.T) {
	t.Parallel()

	whole := vector(service("A", nil))
	tests := []struct {
		name string
		in   payload
	}{
		{name: "truncated", in: whole[:len(whole)-3]},
		{name: "without the vector count", in: payload{}.u32(4)},
		// 数が残りのバイト数に収まらない配列は、要素を読む前に止める。
		{name: "vector count beyond the payload", in: payload{}.u32(12).u32(1_000_000).u32(4)},
		{name: "struct larger than the payload", in: vector(payload{}.u32(1000))},
		{name: "string smaller than its own size", in: vector(sized(payload{}.u16(1).u16(2).u16(3).u8(1).u8(0).u32(3)))},
		// 構造体の中身が、項目を読み終える前に尽きる。
		{name: "struct shorter than its fields", in: vector(sized(payload{}.u16(1)))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := newFakeServer(t, map[uint32]reply{1021: {status: 1, payload: tt.in, stream: nil, hold: false, drop: false}})
			_, err := s.client(t).Services(t.Context())
			require.ErrorIs(t, err, edcb.ErrMalformed)
		})
	}
}
