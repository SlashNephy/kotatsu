import { ptsTicksPerSecond } from './ptsTimeline';

// ブラウザ内でデコードする経路 (#1516) の、フレームを出す間合い。
// フィールド (59.94 回/秒) と画面の切り替わり (多くは 60 回/秒) はほぼ同じ間隔で並ぶので、出す時刻が少し揺れるだけで、
// 同じフィールドが 2 回出て次が飛ぶ。そこで、画面の切り替わりの間隔と位置を保持し、切り替わりごとの PTS を、
// 前の切り替わりの PTS + 間隔の分で進め、時計に少しずつ寄せる。
// 切り替わりの時刻は、メインスレッドの (映しているウィンドウの) requestAnimationFrame の時刻を受け取る。worker 自身の
// requestAnimationFrame は、描いた結果が画面に出る速さに左右されるため使わない。

const ticksPerMs = ptsTicksPerSecond / 1000;

export type FieldPacerParams = {
  // minInterval と maxInterval は、切り替わりの間隔として受け付ける範囲 (ミリ秒)。高リフレッシュの画面から
  // 低いものまで含め、外れた値は破棄する。
  minInterval: number;
  maxInterval: number;
  // intervalBlend は、新しい間隔を保持した間隔に混ぜる割合。
  intervalBlend: number;
  // hiddenGap は、これ以上空いたら (裏に回った)、間隔を保持せずに位置だけ保持し直す空き (ミリ秒)。
  hiddenGap: number;
  // remeasureCount は、2 回分以上と割れた回がこれだけ続いたら、保持した間隔が短すぎるとみなし、計測し直す回数。
  remeasureCount: number;
  // snapRange は、進めた PTS と時計の差がこれ以内なら寄せ、超えたら (飛んだ、貯め直した) 時計に合わせ直す範囲
  // (PTS の単位)。1 フィールドの長さより小さくする。
  snapRange: number;
  // ptsBlend は、寄せる割合。59.94 と 60 の差を吸収しすぎない程度に小さくする。
  ptsBlend: number;
};

export const defaultFieldPacerParams: FieldPacerParams = {
  minInterval: 4,
  maxInterval: 50,
  intervalBlend: 0.05,
  hiddenGap: 250,
  remeasureCount: 8,
  snapRange: 0.025 * ptsTicksPerSecond,
  ptsBlend: 0.02,
};

// FieldPacer は、画面の切り替わりごとに、次の切り替わりに映す PTS を決める。
export class FieldPacer {
  // interval は、保持した切り替わりの間隔 (ミリ秒)。
  interval: number | null = null;
  private last: number | null = null;
  private pts: number | null = null;
  // 2 回分以上と割れた回の空き (割る前)。続いたら、その中央値で計測し直す。
  private long: number[] = [];
  // resyncs は、PTS を時計に合わせ直した回数。
  resyncs = 0;

  constructor(private readonly params: FieldPacerParams = defaultFieldPacerParams) {}

  // onVsync は、画面が切り替わった時刻 time (performance の時刻、ミリ秒) を受け取り、次の切り替わりに映す PTS を返す。
  // clock は、performance の時刻の PTS を返す (まだ時計がなければ null)。間隔を保持するまでと、時計がないときは null を返す。
  onVsync(time: number, clock: (perf: number) => number | null): number | null {
    let steps = 1;
    if (this.last !== null) {
      const gap = time - this.last;
      if (gap >= this.params.hiddenGap) {
        // 裏に回っていた。間隔は保持せず、位置だけ保持し直す。
        this.pts = null;
      } else {
        steps = this.interval === null ? 1 : Math.max(1, Math.round(gap / this.interval));
        this.learn(gap, steps);
      }
    }
    this.last = time;
    if (this.interval === null) {
      return null;
    }
    const now = clock(time + this.interval);
    if (now === null) {
      return null;
    }
    if (this.pts === null) {
      this.pts = now;

      return now;
    }
    const stepped = this.pts + steps * this.interval * ticksPerMs;
    const diff = now - stepped;
    if (Math.abs(diff) <= this.params.snapRange) {
      this.pts = stepped + diff * this.params.ptsBlend;
    } else {
      this.pts = now;
      this.resyncs += 1;
    }

    return this.pts;
  }

  // reset は、保持した PTS を忘れる (間隔は画面のものなので残す)。選局したときと、飛んだときに呼ぶ。
  reset(): void {
    this.pts = null;
  }

  private learn(gap: number, steps: number): void {
    if (steps >= 2 && gap >= this.params.minInterval && gap <= this.params.maxInterval) {
      this.long.push(gap);
      if (this.long.length >= this.params.remeasureCount) {
        // 保持した間隔が短すぎる (画面のリフレッシュレートが下がったなど)。集めた値の中央値で計測し直す。
        const sorted = [...this.long].sort((a, b) => a - b);
        this.interval = sorted[Math.floor(sorted.length / 2)]!;
        this.long = [];
        // 今回の steps は古い間隔で数えたので、位置を保持し直す。
        this.pts = null;
      }

      return;
    }
    // 抜けた切り替わりの間隔は揺れが大きいので、1 回分の間隔だけで保持する。
    if (steps >= 2 || gap < this.params.minInterval || gap > this.params.maxInterval) {
      return;
    }
    this.long = [];
    this.interval =
      this.interval === null
        ? gap
        : this.interval + (gap - this.interval) * this.params.intervalBlend;
  }
}
