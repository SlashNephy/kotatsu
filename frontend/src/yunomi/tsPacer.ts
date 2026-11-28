// 録画型の再生元のデータ放送 (#2270) は、配信が等速より速く届くので、届いた順に web-bml へ渡すと、文書の更新や時刻が
// 映像より先に進む。TsPacer は、届いた TS を PCR で数え、再生位置が追いついた分だけを渡す。

// packetSize は、TS のパケットの大きさ。
const packetSize = 188;
// pcrWrap は、33 ビットの PCR の base が一周する値。PCR は、足していくと誤差が出ないよう、90 kHz の整数のまま数える。
const pcrWrap = 2 ** 33;
// pcrPerSecond は、PCR の base の 1 秒の値。
const pcrPerSecond = 90_000;
// leadSeconds は、再生位置よりこれだけ先までを渡しておく秒数。渡す間隔の分だけ遅れて届かないようにする。
const leadSeconds = 0.5;

// pcrOf は、パケット packet の PCR の base (90 kHz) を返す。PCR がなければ null。
function pcrOf(packet: Uint8Array): number | null {
  const adaptation = ((packet[3] ?? 0) >> 4) & 0b11;
  if ((adaptation & 0b10) === 0 || (packet[4] ?? 0) < 7 || ((packet[5] ?? 0) & 0x10) === 0) {
    return null;
  }
  const base =
    (packet[6] ?? 0) * 2 ** 25 +
    (packet[7] ?? 0) * 2 ** 17 +
    (packet[8] ?? 0) * 2 ** 9 +
    (packet[9] ?? 0) * 2 +
    ((packet[10] ?? 0) >> 7);

  return base;
}

// clock は、PCR から経過を数える状態。last は最後に読んだ PCR (まだ読んでいなければ null)、at はその経過 (90 kHz)。
type Clock = { last: number | null; at: number };

// advance は、clock から PCR pcr まで進めた状態を返す。PCR が戻ったとき (編集したファイルのつなぎ目など) は、
// 進まなかったものとして数え、止まらないようにする。途切れた区間で先へ飛んだ分は、そのまま数える。
function advance(clock: Clock, pcr: number): Clock {
  if (clock.last === null) {
    return { last: pcr, at: clock.at };
  }
  const delta = (pcr - clock.last + pcrWrap) % pcrWrap;

  return { last: pcr, at: clock.at + (delta > pcrWrap / 2 ? 0 : delta) };
}

// TsPacer は、届いた TS をためておき、release で渡した経過 (秒) に PCR が届くまでの分を push に渡す。
// 経過は、読み始めた位置からの放送時刻の経過で数える。PCR も、途切れた区間の分を含めて進むので、同じ物差しになる。
export class TsPacer {
  // buffer の start から end までが、まだ渡していないバイト。届くたびに写し直さないよう、余裕を持って確保する。
  private buffer = new Uint8Array(64 * 1024);
  private start = 0;
  private end = 0;
  // clock は、渡し終えたパケットまでで数えた経過。
  private clock: Clock = { last: null, at: 0 };

  constructor(private readonly push: (bytes: Uint8Array) => void) {}

  // pendingBytes は、まだ渡していないバイト数。
  get pendingBytes(): number {
    return this.end - this.start;
  }

  // feed は、届いた chunk をためる。足りなければ、渡し終えた分を詰め、それでも足りなければ広げる。
  feed(chunk: Uint8Array): void {
    if (this.end + chunk.length > this.buffer.length) {
      const pending = this.end - this.start;
      if (pending + chunk.length <= this.buffer.length) {
        this.buffer.copyWithin(0, this.start, this.end);
      } else {
        const next = new Uint8Array((pending + chunk.length) * 2);
        next.set(this.buffer.subarray(this.start, this.end));
        this.buffer = next;
      }
      this.start = 0;
      this.end = pending;
    }
    this.buffer.set(chunk, this.end);
    this.end += chunk.length;
  }

  // release は、経過 elapsed (秒) の少し先までに届いたパケットを渡す。PCR を持つパケットの時刻で区切り、
  // その先のパケットは、次に呼ぶまでためておく。
  release(elapsed: number): void {
    let clock = this.clock;
    let until = this.start;
    for (let p = this.start; p + packetSize <= this.end; p += packetSize) {
      const pcr = this.buffer[p] === 0x47 ? pcrOf(this.buffer.subarray(p, p + packetSize)) : null;
      if (pcr !== null) {
        const next = advance(clock, pcr);
        if (next.at > (elapsed + leadSeconds) * pcrPerSecond) {
          break;
        }
        clock = next;
      }
      until = p + packetSize;
    }
    if (until === this.start) {
      return;
    }
    this.clock = clock;
    this.push(this.buffer.slice(this.start, until));
    this.start = until;
  }

  // flush は、ためているものをすべて渡す。配信を読み終えたときに使う。
  flush(): void {
    if (this.end > this.start) {
      this.push(this.buffer.slice(this.start, this.end));
      this.start = this.end;
    }
  }
}
