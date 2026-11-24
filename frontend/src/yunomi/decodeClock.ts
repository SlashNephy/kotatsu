import { ptsTicksPerSecond } from './ptsTimeline';

// ブラウザ内でデコードする経路 (#1516) の、worker の時計。
// メインスレッドは、いま耳に届いている音の PTS (伸ばしたもの) と、その時の performance の時刻の組を送る。
// worker は、届いた組 (少し揺れる) を均して、performance の時刻から PTS を出す。

// ticksPerMs は、PTS の 1 ミリ秒。
const ticksPerMs = ptsTicksPerSecond / 1000;

export type DecodeClockParams = {
  // resync は、予測との差がこれを超えたら合わせ直す差 (PTS の単位)。届く組の揺れより大きく、1 コマの長さより小さくする。
  resync: number;
  // blend は、差がそれ以下のときに寄せる割合。1 より十分小さくし、揺れを均す。
  blend: number;
};

// defaultDecodeClockParams は、計測した揺れから決めた既定 (#1516 の測定)。
export const defaultDecodeClockParams: DecodeClockParams = {
  resync: 0.02 * ptsTicksPerSecond,
  blend: 0.1,
};

// DecodeClock は、届いた組で合わせた時計。
export class DecodeClock {
  // offset は、PTS − performance の時刻 (PTS の単位)。まだ組が届いていなければ null。
  private offset: number | null = null;
  // resyncs は、合わせ直した回数。
  resyncs = 0;

  constructor(private readonly params: DecodeClockParams = defaultDecodeClockParams) {}

  // observe は、メインスレッドから届いた組で合わせる。
  observe(pts: number, perf: number): void {
    const offset = pts - perf * ticksPerMs;
    if (this.offset === null) {
      this.offset = offset;

      return;
    }
    const diff = offset - this.offset;
    if (Math.abs(diff) > this.params.resync) {
      this.offset = offset;
      this.resyncs += 1;

      return;
    }
    this.offset += diff * this.params.blend;
  }

  // at は、performance の時刻 perf の PTS を返す。まだ組が届いていなければ null。
  at(perf: number): number | null {
    return this.offset === null ? null : this.offset + perf * ticksPerMs;
  }

  // reset は、合わせた時計を忘れる。選局したときと、止めたときに呼ぶ。
  reset(): void {
    this.offset = null;
  }
}
