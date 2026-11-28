// 録画型の再生元のデータ放送 (#2270) は、配信が等速より速く届くので、届いた順に web-bml へ渡すと、文書の更新や時刻が
// 映像より先に進む。TsPacer は、届いた TS を PCR で数え、再生位置が追いついた分だけを渡す。

// packetSize は、TS のパケットの大きさ。
const packetSize = 188;
// pcrWrap は、33 ビットの PCR の base が一周する秒数。
const pcrWrap = 2 ** 33 / 90_000;
// leadSeconds は、再生位置よりこれだけ先までを渡しておく秒数。渡す間隔の分だけ遅れて届かないようにする。
const leadSeconds = 0.5;

// pcrOf は、パケット packet の PCR (秒) を返す。PCR がなければ null。
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

  return base / 90_000;
}

// TsPacer は、届いた TS をためておき、release で渡した経過 (秒) に PCR が届くまでの分を push に渡す。
// 経過は、読み始めた位置からの放送時刻の経過で数える。PCR も、途切れた区間の分を含めて進むので、同じ物差しになる。
export class TsPacer {
  private pending = new Uint8Array(0);
  // origin は、最初に届いた PCR。まだ届いていなければ null。
  private origin: number | null = null;

  constructor(private readonly push: (bytes: Uint8Array) => void) {}

  // pendingBytes は、まだ渡していないバイト数。
  get pendingBytes(): number {
    return this.pending.length;
  }

  // feed は、届いた chunk をためる。
  feed(chunk: Uint8Array): void {
    const next = new Uint8Array(this.pending.length + chunk.length);
    next.set(this.pending);
    next.set(chunk, this.pending.length);
    this.pending = next;
  }

  // release は、経過 elapsed (秒) の少し先までに届いたパケットを渡す。PCR を持つパケットの時刻で区切り、
  // その先のパケットは、次に呼ぶまでためておく。
  release(elapsed: number): void {
    let end = 0;
    for (let p = 0; p + packetSize <= this.pending.length; p += packetSize) {
      const pcr = this.pending[p] === 0x47 ? pcrOf(this.pending.subarray(p, p + packetSize)) : null;
      if (pcr !== null) {
        this.origin ??= pcr;
        const at = (pcr - this.origin + pcrWrap) % pcrWrap;
        if (at > elapsed + leadSeconds) {
          break;
        }
      }
      end = p + packetSize;
    }
    if (end === 0) {
      return;
    }
    this.push(this.pending.slice(0, end));
    this.pending = this.pending.slice(end);
  }

  // flush は、ためているものをすべて渡す。配信を読み終えたときに使う。
  flush(): void {
    if (this.pending.length > 0) {
      this.push(this.pending);
      this.pending = new Uint8Array(0);
    }
  }
}
