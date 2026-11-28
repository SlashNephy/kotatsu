// ブラウザ内でデコードする経路 (#2276) の、2:3 プルダウンの見分けと組み直し。
// 24 コマの映像を 2:3 プルダウンで 60 フィールドに載せた放送 (アニメなど) は、フィールドを 3、2、3、2 枚ずつ同じコマに割り当てる。
// 3 枚のコマの 3 枚目のフィールドは、2 枚前 (1 つ前のコマの、同じ向きのフィールド) と同じ絵なので、フィールドどうしの差がほぼ 0 になる。
// 10 フィールド (5 コマ) を 1 周とし、繰り返すフィールドが周のどこに来るか (位相、5 通り) を、直近のフィールドの差から決める。
// 位相が決まれば、どのフィールドがどのコマに属するかが分かるので、同じコマの上と下のフィールドを組み合わせて 24p のコマに戻す。
// 位相の決め方は、録画の変換で出すコマ数を決める繰り返し度 (設計書 6.1) と同じ考え方で、yadif の実装には依らない。

// TelecineParams は、位相を決める閾値。
export type TelecineParams = {
  // window は、位相を決めるのに使う直近のフィールドの数。周 (10 フィールド) の倍数にする。
  window: number;
  // motion は、繰り返さないフィールドの差の中央値の下限 (0〜255 の輝度)。これより小さいと止まった絵とみなし、判定を変えない。
  motion: number;
  // lock は、繰り返すフィールドの差の中央値を、ほかのフィールドの差の中央値で割った値 (比) がこれ未満なら、その位相に決める。
  lock: number;
  // margin は、決める位相の比が、次に小さい位相の比のこれ倍未満であること。位相どうしの見分けがつかないときに決めない。
  margin: number;
  // unlock は、決めた位相の比がこれを超えたら、プルダウンでなくなったとみなす。lock より大きくし、行き来しないようにする。
  unlock: number;
};

// defaultTelecineParams は、録画から切り出したアニメ (2:3 プルダウン) と音楽番組 (60i) の各 600 コマで見て決めた既定。
// 中央値を使うのは、場面の切り替わりで 1 枚だけ差が大きくなったフィールドに、平均が引きずられるからである。
export const defaultTelecineParams: TelecineParams = {
  window: 40,
  motion: 1,
  lock: 0.25,
  margin: 0.5,
  unlock: 0.5,
};

// cycle は、1 周のフィールドの数。
const cycle = 10;

// filmIndex は、周の中の位置 q (0〜9) のフィールドが、周の中の何枚目のコマ (0〜3) に属するかを返す。
// コマは、3、2、3、2 枚のフィールドで並ぶ。
function filmIndex(q: number): number {
  return q < 3 ? 0 : q < 5 ? 1 : q < 8 ? 2 : 3;
}

// repeats は、周の中の位置 q のフィールドが、3 枚のコマの 3 枚目 (2 枚前と同じ絵) か。
function repeats(q: number): boolean {
  return q === 2 || q === 7;
}

// positionOf は、位相 phase のときの、通し番号 field のフィールドの周の中の位置を返す。
function positionOf(field: number, phase: number): number {
  return (((field - phase) % cycle) + cycle) % cycle;
}

function median(values: number[]): number {
  const sorted = values.toSorted((a, b) => a - b);
  const middle = sorted.length >> 1;

  return sorted.length === 0
    ? 0
    : sorted.length % 2 === 1
      ? sorted[middle]!
      : (sorted[middle - 1]! + sorted[middle]!) / 2;
}

// TelecineDetector は、フィールドの差を受け取り、2:3 プルダウンの位相を決める。
// 位相は 0〜4。位相 p では、通し番号が p (mod 10) のフィールドが周の始まりになる。
// (p と p + 5 は、繰り返すフィールドの位置とコマの区切りが同じなので、同じものとして扱う。)
export class TelecineDetector {
  // diffs は、直近のフィールドの差。通し番号 first から順に並ぶ。
  private readonly diffs: number[] = [];
  private first = 0;
  // phase は、決めた位相。決めていなければ null。
  phase: number | null = null;

  constructor(private readonly params: TelecineParams = defaultTelecineParams) {}

  // push は、通し番号 field のフィールドと、2 枚前のフィールドの差 (点ごとの差の絶対値の平均) を足し、決めた位相を返す。
  // 通し番号が飛んだら (コマが欠けた、B フレームを省いた)、それまでの差を破棄して数え直す。
  push(field: number, diff: number): number | null {
    if (this.diffs.length > 0 && field !== this.first + this.diffs.length) {
      this.reset();
    }
    if (this.diffs.length === 0) {
      this.first = field;
    }
    this.diffs.push(diff);
    if (this.diffs.length > this.params.window) {
      this.diffs.shift();
      this.first += 1;
    }
    if (this.diffs.length === this.params.window) {
      this.decide();
    }

    return this.phase;
  }

  // reset は、数えた差と決めた位相を破棄する。
  reset(): void {
    this.diffs.length = 0;
    this.phase = null;
  }

  // score は、位相 phase で、繰り返すフィールドの差の中央値と、ほかのフィールドの差の中央値と、その比を返す。
  private score(phase: number): { phase: number; others: number; ratio: number } {
    const repeated: number[] = [];
    const others: number[] = [];
    this.diffs.forEach((diff, index) => {
      (repeats(positionOf(this.first + index, phase)) ? repeated : others).push(diff);
    });
    const other = median(others);

    return {
      phase,
      others: other,
      ratio: other > 0 ? median(repeated) / other : Number.POSITIVE_INFINITY,
    };
  }

  private decide(): void {
    const { motion, lock, margin, unlock } = this.params;
    const scores = [0, 1, 2, 3, 4].map((phase) => this.score(phase));
    // 止まった絵では、どの位相でも差が小さく、見分けられないので、決めた位相をそのまま保つ。
    if (scores.every((score) => score.others < motion)) {
      return;
    }
    if (this.phase !== null) {
      if (scores[this.phase]!.ratio <= unlock) {
        return;
      }
      this.phase = null;
    }
    const [best, second] = scores.toSorted((a, b) => a.ratio - b.ratio);
    if (
      best !== undefined &&
      second !== undefined &&
      best.others >= motion &&
      best.ratio < lock &&
      best.ratio < second.ratio * margin
    ) {
      this.phase = best.phase;
    }
  }
}

// FieldFrame は、組み直しに渡すデコードしたコマ。source は、コマを上げたテクスチャなど、呼ぶ側の持ち物。
export type FieldFrame<Source> = {
  // index は、デコードした順の通し番号。欠けずに 1 ずつ増える (欠けたら、呼ぶ側が数え直す)。
  // 通し番号 index のコマのフィールドは、2 × index (先に映すフィールド) と 2 × index + 1。
  index: number;
  pts: number;
  topFieldFirst: boolean;
  source: Source;
};

// FilmFrame は、組み直した 24p のコマ。上のフィールドの行は top から、下のフィールドの行は bottom から取る。
export type FilmFrame<Source> = { pts: number; top: Source; bottom: Source };

// assembleFilm は、位相 phase で、コマ current が届いたときに出せる 24p のコマを返す。
// previous は、1 つ前のコマ (current.index − 1)。なければ null。duration は、コマ 1 つの長さ (PTS)。
// コマの 2 枚目のフィールドが届いたところで、そのコマの最初の 2 枚 (上と下が 1 枚ずつ) を組み合わせて出す。
// 3 枚のコマの 3 枚目は、1 枚目と同じ絵なので使わない。出す時刻は、周の始まりから、周の長さ (5 コマ) を 4 等分した間隔で置き、
// 24p の間隔をそろえる (フィールドの時刻のままだと、3 枚と 2 枚で間隔が交互に変わる)。
export function assembleFilm<Source>(
  phase: number,
  current: FieldFrame<Source>,
  previous: FieldFrame<Source> | null,
  duration: number,
): FilmFrame<Source>[] {
  const fieldDuration = duration / 2;
  const ptsOfField = (field: number) => current.pts + (field - 2 * current.index) * fieldDuration;
  const frameOf = (field: number) => (Math.floor(field / 2) === current.index ? current : previous);
  // 通し番号が偶数のフィールドは、そのコマで先に映すフィールド。topFieldFirst なら上のフィールド。
  const isTop = (field: number, frame: FieldFrame<Source>) =>
    field % 2 === 0 ? frame.topFieldFirst : !frame.topFieldFirst;
  const films: FilmFrame<Source>[] = [];
  for (const field of [2 * current.index, 2 * current.index + 1]) {
    const q = positionOf(field, phase);
    // 2 枚目のフィールドは、コマの始まり (周の中の 0、3、5、8) の次。
    if (q !== 1 && q !== 4 && q !== 6 && q !== 9) {
      continue;
    }
    const first = frameOf(field - 1);
    const second = frameOf(field);
    if (first === null || second === null) {
      continue;
    }
    const [top, bottom] = isTop(field - 1, first)
      ? [first.source, second.source]
      : [second.source, first.source];
    films.push({
      pts: ptsOfField(field - q) + (filmIndex(q) * 5 * duration) / 4,
      top,
      bottom,
    });
  }

  return films;
}

// sampleStep は、フィールドの差を出すときに間引く間隔 (横の画素と、フィールドの中の行)。
// 1440×1080 のコマで、フィールドごとに 360×135 点の差を出す。デコードの時間より十分に短い。
const sampleStep = 4;

// FieldSampler は、デコードしたコマの輝度から、フィールドごとに間引いた点を取り、1 つ前のコマの同じ向きのフィールドとの差を出す。
export class FieldSampler {
  // current と previous は、今と 1 つ前のコマの、上 (0) と下 (1) のフィールドの点。取るたびに入れ替えて使い回す。
  private current: [Uint8Array, Uint8Array] = [new Uint8Array(0), new Uint8Array(0)];
  private previous: [Uint8Array, Uint8Array] = [new Uint8Array(0), new Uint8Array(0)];
  private hasPrevious = false;
  private width = 0;
  private height = 0;

  // sample は、輝度の面 (heap の pointer から、1 行 stride バイト) の上と下のフィールドの差を返す。
  // 差は、点ごとの差の絶対値の平均 (0〜255)。1 つ前のコマがないか、大きさが違えば null を返す。
  sample(
    heap: Uint8Array,
    pointer: number,
    stride: number,
    width: number,
    height: number,
  ): { top: number; bottom: number } | null {
    const columns = Math.floor(width / sampleStep);
    const rows = Math.floor(height / 2 / sampleStep);
    const comparable = this.hasPrevious && this.width === width && this.height === height;
    [this.current, this.previous] = [this.previous, this.current];
    for (const parity of [0, 1] as const) {
      if (this.current[parity].length !== columns * rows) {
        this.current[parity] = new Uint8Array(columns * rows);
      }
      const out = this.current[parity];
      for (let row = 0; row < rows; row += 1) {
        // フィールドの (row × 間隔) 行目は、コマの (row × 間隔) × 2 + parity 行目。
        const line = pointer + (row * sampleStep * 2 + parity) * stride;
        for (let column = 0; column < columns; column += 1) {
          out[row * columns + column] = heap[line + column * sampleStep]!;
        }
      }
    }
    this.hasPrevious = true;
    this.width = width;
    this.height = height;
    if (!comparable) {
      return null;
    }

    return {
      top: meanDifference(this.current[0], this.previous[0]),
      bottom: meanDifference(this.current[1], this.previous[1]),
    };
  }

  // reset は、1 つ前のコマを忘れる。
  reset(): void {
    this.hasPrevious = false;
  }
}

function meanDifference(a: Uint8Array, b: Uint8Array): number {
  let sum = 0;
  for (let index = 0; index < a.length; index += 1) {
    sum += Math.abs(a[index]! - b[index]!);
  }

  return a.length === 0 ? 0 : sum / a.length;
}
