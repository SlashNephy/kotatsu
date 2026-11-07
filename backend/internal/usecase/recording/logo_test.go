package recording_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/servicelogo"
)

// cdtPackets は、ネットワーク networkID のロゴ logoID (種類 logoType) を送る CDT を、TS のパケットにする。
func cdtPackets(t *testing.T, networkID, logoID uint16, logoType byte, data []byte) []byte {
	t.Helper()

	size := len(data)
	if size > 4000 {
		panic("the logo is too large")
	}
	body := binary.BigEndian.AppendUint16(nil, networkID)
	// data_type はロゴ、記述子はない。
	body = append(body, 0x01, 0xF0, 0x00, logoType)
	body = binary.BigEndian.AppendUint16(body, 0xFE00|logoID)
	body = binary.BigEndian.AppendUint16(body, 0xF001)
	body = binary.BigEndian.AppendUint16(body, uint16(size))
	body = append(body, data...)
	// section_length は、download_data_id から CRC までの長さ。ヘッダーの残りが 5 バイト、ロゴのデータの前が 12 バイト。
	sectionLength := 5 + 12 + size + 4
	if sectionLength > 0x0FFF {
		panic("the section is too long")
	}
	section := []byte{0xC8}
	section = binary.BigEndian.AppendUint16(section, 0xB000|uint16(sectionLength))
	section = append(section, 0x00, 0x01, 0xC3, 0x00, 0x00)
	section = append(section, body...)
	section = binary.BigEndian.AppendUint32(section, crcMPEG(section))

	packets := []byte{}
	payload := append([]byte{0x00}, section...)
	for i := 0; len(payload) > 0; i++ {
		p := bytes.Repeat([]byte{0xFF}, ts.PacketSize)
		p[0], p[1], p[2], p[3] = 0x47, 0x00, 0x29, 0x10|byte(i&0x0F)
		if i == 0 {
			p[1] |= 0x40
		}
		n := copy(p[4:], payload)
		payload = payload[n:]
		packets = append(packets, p...)
	}

	return packets
}

// TestRecorderCollectsLogo は、録画している TS の CDT からロゴを集め、サービスのロゴとして返せるようにすることを確かめる (#1308)。
// 録画の様子を書く間隔より短い録画でも、セグメントを閉じるときに書く。
func TestRecorderCollectsLogo(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	// 放送のロゴは、パレットを持たない 8 ビットのパレットの PNG。
	img := image.NewPaletted(image.Rect(0, 0, 64, 36), make(color.Palette, 128))
	for i := range img.Palette {
		img.Palette[i] = color.Black
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	logo := buf.Bytes()
	broadcast := slices.Clone(logo[:8])
	for rest := logo[8:]; len(rest) > 0; {
		length := int(binary.BigEndian.Uint32(rest))
		if name := string(rest[4:8]); name != "PLTE" && name != "tRNS" {
			broadcast = append(broadcast, rest[:12+length]...)
		}
		rest = rest[12+length:]
	}
	// 地上波の 0 番目のサービスのロゴの番号は 0。
	stream := slices.Concat(data, cdtPackets(t, grNetworkID, 0, 5, broadcast))

	f := newFixture(t)
	store := servicelogo.NewStore(f.pool)
	_, err = store.ServiceLogo(t.Context(), grService(0))
	require.ErrorIs(t, err, servicelogo.ErrNoLogo)

	reservationID := f.reserve(t, 0)
	recorder, _ := f.newRecorder(t, onceStream{data: stream}, nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool { return len(f.files(t, reservationID)) == 1 }, waitTimeout, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	f.clock.Advance(time.Hour)
	require.NoError(t, wait(t, done))

	got, err := store.ServiceLogo(t.Context(), grService(0))
	require.NoError(t, err)
	want, err := ts.LogoPNG(broadcast)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
