package ts

// packetFramer は、パケットの途中で区切られた書き込みを、188 バイトのパケットに切り分ける (#1986)。
//
// 書き込みの終わりで途切れたパケットの先頭は carry に持ち越し、次の書き込みの先頭とつないで 1 つのパケットにする。
// 同期を失っている間は、sync byte に続く 188 バイト先も sync byte であることを確認してから合わせ直す。
// ペイロードの中の 0x47 を、パケットの先頭と取り違えないためである。188 バイト先がまだ届いていなければ、
// holdUnverified なら決めずに持ち越し、次の書き込みで確認する。こうすると、書き込みの切れ目によらず、同じ所で同期する。
// 持ち越した頭が、確認してパケットの先頭でないと分かったら、破棄せずに、その中の次の sync byte から探し直す。
type packetFramer struct {
	// carry は、前の書き込みの終わりで途切れたパケットの先頭。carryLen が 0 でなければ、sync byte から始まる。
	carry    [PacketSize]byte
	carryLen int
	// synced は、パケットの境界に合っているかどうか。
	synced bool
	// verifySynced は、同期している間も、次のパケットの先頭が届いていれば sync byte であることを確認するかどうか。
	verifySynced bool
	// holdUnverified は、同期を失っている間に、次のパケットの先頭がまだ届いていないパケットを、確認するまで持ち越すかどうか。
	// false なら、確認できないパケットはそのまま受け付ける。
	holdUnverified bool
	// syncLosses は、同期を失った回数。
	syncLosses int64
}

func newPacketFramer(verifySynced, holdUnverified bool) packetFramer {
	return packetFramer{
		carry: [PacketSize]byte{}, carryLen: 0, synced: true,
		verifySynced: verifySynced, holdUnverified: holdUnverified, syncLosses: 0,
	}
}

// loseSync は、同期を失ったことを記録する。
func (f *packetFramer) loseSync() {
	if f.synced {
		f.synced = false
		f.syncLosses++
	}
}

// accepts は、sync byte で始まり、その 188 バイト先が next (届いていなければ hasNext が false) であるパケットを、
// 今受け付けてよいかを返す。2 つ目の戻り値が false なら、まだ決められない。
func (f *packetFramer) accepts(next byte, hasNext bool) (bool, bool) {
	if f.synced && !f.verifySynced {
		return true, true
	}
	if hasNext {
		return next == syncByte, true
	}
	if f.synced || !f.holdUnverified {
		return true, true
	}

	return false, false
}

// feed は p を切り分け、パケットごとに emit を呼ぶ。base は、p の先頭の、ストリームの先頭からの位置。
// emit には、パケットと、その先頭の位置を渡す。emit が true を返したら、そこで止める。止めた後は、続けて読まない。
func (f *packetFramer) feed(p []byte, base int64, emit func(b []byte, offset int64) bool) {
	for f.carryLen > 0 {
		need := PacketSize - f.carryLen
		if len(p) < need {
			f.carryLen += copy(f.carry[f.carryLen:], p)

			return
		}
		ok, decided := f.accepts(byteAt(p, need))
		if !decided {
			f.carryLen += copy(f.carry[f.carryLen:], p[:need])

			return
		}
		if !ok {
			// 持ち越した頭はパケットの先頭でなかった。その中の次の sync byte から探し直す。
			f.loseSync()
			f.dropCarryHead()

			continue
		}
		copy(f.carry[f.carryLen:], p[:need])
		start := base - int64(f.carryLen)
		f.carryLen = 0
		f.synced = true
		p, base = p[need:], base+int64(need)
		if emit(f.carry[:], start) {
			return
		}
	}

	for len(p) > 0 {
		if p[0] != syncByte {
			f.loseSync()
			p, base = p[1:], base+1

			continue
		}
		if len(p) < PacketSize {
			// 足りない分は、次の書き込みで埋める。受け付けるかは、埋めたときに決める。
			f.carryLen = copy(f.carry[:], p)

			return
		}
		ok, decided := f.accepts(byteAt(p, PacketSize))
		if !decided {
			f.carryLen = copy(f.carry[:], p)

			return
		}
		if !ok {
			f.loseSync()
			p, base = p[1:], base+1

			continue
		}
		f.synced = true
		if emit(p[:PacketSize], base) {
			return
		}
		p, base = p[PacketSize:], base+PacketSize
	}
}

// dropCarryHead は、持ち越した頭の先頭のバイトを破棄し、次の sync byte までを詰める。sync byte がなければ空にする。
func (f *packetFramer) dropCarryHead() {
	for i := 1; i < f.carryLen; i++ {
		if f.carry[i] == syncByte {
			f.carryLen = copy(f.carry[:], f.carry[i:f.carryLen])

			return
		}
	}
	f.carryLen = 0
}

// byteAt は、p の i バイト目と、それが届いているかを返す。
func byteAt(p []byte, i int) (byte, bool) {
	if i < len(p) {
		return p[i], true
	}

	return 0, false
}
