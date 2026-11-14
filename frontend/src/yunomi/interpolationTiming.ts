// コマの補間 (#1655) で、画面を更新するたびに、どのコマの間の、どの時刻を出すかを決める。
// 中間のコマを作るには、後のコマが届いている必要がある。そこで、映像を 1 コマ分遅らせて出し、
// 届いた 2 コマの間を、画面の更新の時刻に合わせて埋める。

// TimedFrame は、受け取ったコマの、映像の時刻 (秒)。
export type TimedFrame = { mediaTime: number };

// FramePick は、出すコマ。from と to は frames の添字で、t は from から to への位置 (0〜1)。
export type FramePick = { from: number; to: number; t: number };

// pickFrames は、映像の時刻 shown に出すコマを選ぶ。frames は、時刻の早い順に並んでいるものとする。
// shown がどのコマより前なら最初のコマを、最後のコマより後なら最後のコマを、そのまま出す。
export function pickFrames(frames: readonly TimedFrame[], shown: number): FramePick | null {
  if (frames.length === 0) {
    return null;
  }
  const first = frames[0];
  if (first === undefined || shown <= first.mediaTime) {
    return { from: 0, to: 0, t: 0 };
  }
  for (let i = 0; i < frames.length - 1; i++) {
    const from = frames[i];
    const to = frames[i + 1];
    if (from === undefined || to === undefined) {
      break;
    }
    if (shown < to.mediaTime) {
      const span = to.mediaTime - from.mediaTime;

      return { from: i, to: i + 1, t: span > 0 ? (shown - from.mediaTime) / span : 0 };
    }
  }
  const last = frames.length - 1;

  return { from: last, to: last, t: 0 };
}

// framePeriod は、コマの時刻の並びから、コマの間隔 (秒) を求める。間隔の中央値を使う。
// コマを取りこぼすと間隔が倍になるので、平均でなく中央値にする。2 コマなければ null。
export function framePeriod(times: readonly number[]): number | null {
  const steps: number[] = [];
  for (let i = 1; i < times.length; i++) {
    const step = (times[i] ?? 0) - (times[i - 1] ?? 0);
    if (step > 0) {
      steps.push(step);
    }
  }
  if (steps.length === 0) {
    return null;
  }
  steps.sort((a, b) => a - b);

  return steps[Math.floor(steps.length / 2)] ?? null;
}

// MediaClock は、映像の時刻と画面に出る時刻の対応。最後に受け取ったコマで決める。
// displayTime は performance.now() の時計 (ミリ秒)、rate は再生の速さ。
export type MediaClock = { mediaTime: number; displayTime: number; rate: number };

// mediaTimeAt は、画面の時刻 displayTime に当たる映像の時刻を、clock から外挿する。
// 一時停止しているときは rate を 0 として渡す。
export function mediaTimeAt(clock: MediaClock, displayTime: number): number {
  return clock.mediaTime + ((displayTime - clock.displayTime) / 1_000) * clock.rate;
}

// maxFrameGap は、前のコマとの時刻の差がこれを超えたら、続きのコマでないとみなす秒数。
// シークや読み直しのあとは、前のコマと間を埋めない。
export const maxFrameGap = 0.5;

// continues は、時刻 next のコマが、時刻 previous のコマの続きかどうか。
export function continues(previous: number, next: number): boolean {
  return next > previous && next - previous <= maxFrameGap;
}
