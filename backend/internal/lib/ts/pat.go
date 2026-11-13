package ts

// PAT は、Program Association Table の中身。
type PAT struct {
	// TransportStreamID は transport_stream_id。
	TransportStreamID uint16
	// Programs は、program_number の一覧。NIT を指す 0 は含めない。
	Programs []uint16
}

// PATWatcher は、TS を流しながら、PAT を読むたびに知らせる (#1534)。
// EDCB の NetworkTV は、EDCB の予約にチューナーを取られると、接続を保ったまま別の局の TS を送るので、PAT の変化で気づく。
type PATWatcher struct {
	onPAT     func(*PAT)
	assembler *sectionAssembler
	// rest は、パケットの区切りに届かなかった残り。
	rest []byte
}

// NewPATWatcher は PATWatcher を返す。onPAT は、CRC の正しい、現在有効な PAT を読むたびに呼ぶ。
func NewPATWatcher(onPAT func(*PAT)) *PATWatcher {
	return &PATWatcher{onPAT: onPAT, assembler: newSectionAssembler(), rest: make([]byte, 0, PacketSize)}
}

// Write は、TS を読む。パケットの区切りは、同期バイトで合わせ直す。読み始めがパケットの途中でも、
// 小さな塊に分けて渡されても、届いた順につないでから区切る。エラーは返さない。
func (w *PATWatcher) Write(p []byte) (int, error) {
	w.rest = append(w.rest, p...)
	buf := w.rest
	for len(buf) >= PacketSize {
		if buf[0] != syncByte {
			// 区切りがずれたら、次の同期バイトまで読み飛ばす。
			w.assembler.reset()
			buf = buf[1:]

			continue
		}
		w.packet(buf[:PacketSize])
		buf = buf[PacketSize:]
	}
	// 残りを先頭に寄せる。読むたびに、残りの分だけを持つ。
	w.rest = w.rest[:copy(w.rest, buf)]

	return len(p), nil
}

func (w *PATWatcher) packet(b []byte) {
	if b[0] != syncByte {
		w.assembler.reset()

		return
	}
	pkt := parsePacket(b)
	if pkt.pid != 0 || pkt.malformed || pkt.transportError || !pkt.hasPayload {
		return
	}
	w.assembler.feed(pkt.payloadUnitStart, pkt.payload, func(section []byte) {
		header := parseSectionHeader(section)
		if header == nil || header.tableID != 0 || !header.currentNext {
			return
		}
		pat := &PAT{TransportStreamID: header.tableIDExtension, Programs: []uint16{}}
		for i := 0; i+4 <= len(header.body); i += 4 {
			if program := uint16(header.body[i])<<8 | uint16(header.body[i+1]); program != 0 {
				pat.Programs = append(pat.Programs, program)
			}
		}
		w.onPAT(pat)
	})
}
