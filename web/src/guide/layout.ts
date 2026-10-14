// 番組表の配置を計算する。描画から切り離し、時刻と画素の対応と、見えている番組の切り出しを扱う。

// minuteHeight は、番組表の 1 分の高さ (px)。
export const minuteHeight = 3;
// minProgramHeight は、短い番組でも名前が読めるようにする、番組の枠の最小の高さ (px)。文字 1 行 (xs の 12px × 行の高さ 1.35) と上下の余白が収まる高さにする。
export const minProgramHeight = 19;
// undecidedFallbackMinutes は、終了時刻が未定で、次の番組もないときに見せる長さ (分)。
export const undecidedFallbackMinutes = 60;

const minute = 60_000;

// GuideWindow は、番組表に出す時間帯。
// ys を持つときは、分ごとの高さが違う目盛り (timeScale) で、ys[i] は、時間帯の始まりから i 分の位置 (px)。
// 持たないときは、1 分を minuteHeight とする一定の目盛り。
export type GuideWindow = { from: number; to: number; ys?: readonly number[] };

// ProgramTime は、配置に使う番組の時刻 (エポックからのミリ秒)。
export type ProgramTime = { startAt: number; endAt: number; endUndecided: boolean };

export type Box = { top: number; height: number };

// timeToY は、時刻を、番組表の上端からの位置 (px) に直す。
export function timeToY(window: GuideWindow, time: number): number {
  const minutes = (time - window.from) / minute;
  const ys = window.ys;
  if (ys === undefined) {
    return minutes * minuteHeight;
  }
  const index = Math.min(Math.max(Math.floor(minutes), 0), ys.length - 2);
  if (index < 0) {
    return minutes * minuteHeight;
  }

  return ys[index]! + (minutes - index) * (ys[index + 1]! - ys[index]!);
}

// yToTime は、番組表の上端からの位置 (px) を、時刻に直す。
export function yToTime(window: GuideWindow, y: number): number {
  const ys = window.ys;
  if (ys === undefined || ys.length < 2) {
    return window.from + (y / minuteHeight) * minute;
  }
  // y を含む 1 分を、二分探索で探す。
  let lo = 0;
  let hi = ys.length - 2;
  while (lo < hi) {
    const mid = (lo + hi + 1) >>> 1;
    if (ys[mid]! <= y) {
      lo = mid;
    } else {
      hi = mid - 1;
    }
  }
  const height = ys[lo + 1]! - ys[lo]!;

  return window.from + (lo + (y - ys[lo]!) / height) * minute;
}

// gridHeight は、時間帯全体の高さ (px)。
export function gridHeight(window: GuideWindow): number {
  return timeToY(window, window.to);
}

// timeScale は、短い番組がある時間だけ 1 分の高さを増やした目盛りを返す。短い番組 (一定の目盛りで minProgramHeight に
// 満たないもの) がかかる分は、その番組が minProgramHeight に届くまで高くする。目盛りは、渡したすべての列で共通にするので、
// 正時の線や、同じ時刻に始まる番組の位置は、列をまたいでも揃う。
export function timeScale(
  window: GuideWindow,
  columns: readonly { programs: readonly ProgramTime[]; layout: ColumnLayout }[],
): GuideWindow {
  const count = Math.max(Math.round((window.to - window.from) / minute), 0);
  const heights = Array.from({ length: count }, () => minuteHeight);
  for (const { programs, layout } of columns) {
    programs.forEach((program, i) => {
      const start = Math.max(program.startAt, window.from);
      const end = Math.min(layout.ends[i]!, window.to);
      const minutes = (end - start) / minute;
      if (minutes <= 0 || minutes * minuteHeight >= minProgramHeight) {
        return;
      }
      const perMinute = minProgramHeight / minutes;
      const first = Math.floor((start - window.from) / minute);
      const last = Math.min(Math.ceil((end - window.from) / minute), count);
      for (let m = first; m < last; m++) {
        heights[m] = Math.max(heights[m]!, perMinute);
      }
    });
  }
  const ys = [0];
  for (const height of heights) {
    ys.push(ys[ys.length - 1]! + height);
  }

  return { ...window, ys };
}

// effectiveEnd は、番組の終わりとして描く時刻を返す。
// 終了時刻が未定の番組は、endAt が開始の直後になっているので、次の番組の開始まで (なければ一定の長さ) 伸ばす。
export function effectiveEnd(program: ProgramTime, nextStartAt: number | undefined): number {
  if (!program.endUndecided) {
    return program.endAt;
  }

  return nextStartAt ?? program.startAt + undecidedFallbackMinutes * minute;
}

// programBox は、番組の枠の位置と高さを返す。時間帯からはみ出す部分は切り詰める。
// end は、描く終わりの時刻 (ColumnLayout の ends)。
export function programBox(window: GuideWindow, program: ProgramTime, end: number): Box {
  const start = Math.max(program.startAt, window.from);
  const top = timeToY(window, start);
  const bottom = timeToY(window, Math.min(end, window.to));

  return { top, height: Math.max(bottom - top, minProgramHeight) };
}

// ColumnLayout は、1 つの列の番組の配置。添字は、列の番組の添字と同じ。
export type ColumnLayout = {
  // ends は、描く終わりの時刻。終了時刻が未定の番組は、次に始まる番組の開始まで伸ばす。
  ends: number[];
  // maxEnds は、先頭からその番組までの ends の最大値。見えている番組を二分探索で探すのに使う。
  maxEnds: number[];
  // lanes は、重なる番組を横に並べるときの、番組の位置 (0 から)。
  lanes: number[];
  // laneCounts は、その番組と重なり合う番組の集まりの、横に並べる数。
  laneCounts: number[];
};

// layoutColumn は、列の番組 (開始時刻の順) の配置を計算する。
// 同じサービスの番組は、ふつうは重ならない。ただし、スポーツ中継と、中止のときに放送する番組のように、
// 放送局が重なる番組を送ることがあるので、重なる番組は横に並べる。
export function layoutColumn(programs: readonly ProgramTime[]): ColumnLayout {
  const ends: number[] = [];
  const maxEnds: number[] = [];
  const lanes: number[] = [];
  const laneCounts: number[] = [];
  let nextIndex = 0;
  let clusterStart = 0;
  let clusterEnd = -Infinity;
  let laneEnds: number[] = [];
  const closeCluster = (until: number) => {
    for (let i = clusterStart; i < until; i++) {
      laneCounts[i] = laneEnds.length;
    }
  };
  programs.forEach((program, i) => {
    // 次に始まる番組は、同じ時刻に始まる番組を飛ばして探す。
    nextIndex = Math.max(nextIndex, i + 1);
    while (nextIndex < programs.length && programs[nextIndex]!.startAt <= program.startAt) {
      nextIndex++;
    }
    const end = effectiveEnd(program, programs[nextIndex]?.startAt);
    ends.push(end);
    maxEnds.push(Math.max(end, maxEnds[i - 1] ?? -Infinity));
    if (program.startAt >= clusterEnd) {
      closeCluster(i);
      clusterStart = i;
      laneEnds = [];
    }
    const lane = laneEnds.findIndex((laneEnd) => laneEnd <= program.startAt);
    if (lane === -1) {
      lanes.push(laneEnds.length);
      laneEnds.push(end);
    } else {
      lanes.push(lane);
      laneEnds[lane] = end;
    }
    clusterEnd = Math.max(clusterEnd, end);
  });
  closeCluster(programs.length);

  return { ends, maxEnds, lanes, laneCounts };
}

// stackedBoxes は、列の番組ごとの枠を返す。短い番組は、名前が読めるよう最小の高さまで伸ばす。
// 伸ばすときは、終わりの位置を保って上へ伸ばし、その分、同じレーンの前の番組の終わりを削る。正時の直前の短い番組
// (気象情報など) が、正時の線をまたがないようにするため。番組表の上端で上へ伸ばせないときだけ、後ろの番組を下げる。
// 1 分の番組が続くところでも、どの番組も読めて押せるようにする。
export function stackedBoxes(
  window: GuideWindow,
  programs: readonly ProgramTime[],
  layout: ColumnLayout,
): Box[] {
  const natural = programs.map((program, i) => ({
    top: timeToY(window, Math.max(program.startAt, window.from)),
    bottom: timeToY(window, Math.min(layout.ends[i]!, window.to)),
  }));
  // 後ろから見て、終わりを後ろの番組の上端までに抑え、短ければ上へ伸ばす。
  const nextTop = new Map<number, number>();
  const raised = natural.map(() => ({ top: 0, bottom: 0 }));
  for (let i = programs.length - 1; i >= 0; i--) {
    const lane = layout.lanes[i]!;
    const bottom = Math.min(natural[i]!.bottom, nextTop.get(lane) ?? Infinity);
    const top = Math.min(natural[i]!.top, bottom - minProgramHeight);
    raised[i] = { top, bottom };
    nextTop.set(lane, top);
  }
  // 前から見て、上端より上にはみ出したものと、前の番組と重なるものを下げる。
  const previousBottom = new Map<number, number>();

  return raised.map((box, i) => {
    const lane = layout.lanes[i]!;
    const top = Math.max(box.top, previousBottom.get(lane) ?? 0);
    const bottom = Math.max(box.bottom, top + minProgramHeight);
    previousBottom.set(lane, bottom);

    return { top, height: bottom - top };
  });
}

// visibleRange は、列の番組のうち、時刻 [from, to) に重なりうる番組の添字の範囲 [start, end) を返す。
// 列の番組の数によらず速く求めるため、二分探索で探す。範囲の中には、重なる番組の都合で、見えない番組も含みうる。
export function visibleRange(
  programs: readonly ProgramTime[],
  layout: ColumnLayout,
  from: number,
  to: number,
): { start: number; end: number } {
  let lo = 0;
  let hi = programs.length;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if (layout.maxEnds[mid]! <= from) {
      lo = mid + 1;
    } else {
      hi = mid;
    }
  }
  const start = lo;
  hi = programs.length;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if (programs[mid]!.startAt < to) {
      lo = mid + 1;
    } else {
      hi = mid;
    }
  }

  return { start, end: Math.max(start, lo) };
}

// dateOf は、放送日の始まりの時刻を YYYY-MM-DD (日本時間) にする。
export function dateOf(time: number): string {
  return new Date(time + 9 * 3_600_000).toISOString().slice(0, 10);
}

// broadcastDay は、時刻を含む放送日 (4 時から翌 4 時まで、日本時間) の時間帯を返す。
export function broadcastDay(time: number): GuideWindow {
  const jst = 9 * 60 * minute;
  const dayStartHour = 4 * 60 * minute;
  const day = 24 * 60 * minute;
  const from = Math.floor((time + jst - dayStartHour) / day) * day - jst + dayStartHour;

  return { from, to: from + day };
}

// receivedUntil は、番組表が届いている時刻 (いちばん遅く終わる番組の終わり)。ends は、その日に出す番組の終わりの時刻。
// 先の日の番組表は、放送から届いた分しかないので、その先は空になる。1 つも届いていなければ、0 を返し、日の全体を届いていないとする。
// 深夜に放送を休むサービスもあるので、日の終わり (to) の 1 時間前までに届いていれば、すべて届いたとみなし、undefined を返す。
export function receivedUntil(ends: readonly number[], to: number): number | undefined {
  const until = Math.max(0, ...ends);

  return until < to - 3_600_000 ? until : undefined;
}
