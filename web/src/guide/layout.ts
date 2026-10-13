// 番組表の配置を計算する。描画から切り離し、時刻と画素の対応と、見えている番組の切り出しを扱う。

// minuteHeight は、番組表の 1 分の高さ (px)。
export const minuteHeight = 3;
// minProgramHeight は、短い番組でも名前が読めるようにする、番組の枠の最小の高さ (px)。文字 1 行 (xs の 12px × 行の高さ 1.35) と上下の余白が収まる高さにする。
export const minProgramHeight = 19;
// undecidedFallbackMinutes は、終了時刻が未定で、次の番組もないときに見せる長さ (分)。
export const undecidedFallbackMinutes = 60;

const minute = 60_000;

// GuideWindow は、番組表に出す時間帯。
export type GuideWindow = { from: number; to: number };

// ProgramTime は、配置に使う番組の時刻 (エポックからのミリ秒)。
export type ProgramTime = { startAt: number; endAt: number; endUndecided: boolean };

export type Box = { top: number; height: number };

// timeToY は、時刻を、番組表の上端からの位置 (px) に直す。
export function timeToY(window: GuideWindow, time: number): number {
  return ((time - window.from) / minute) * minuteHeight;
}

// yToTime は、番組表の上端からの位置 (px) を、時刻に直す。
export function yToTime(window: GuideWindow, y: number): number {
  return window.from + (y / minuteHeight) * minute;
}

// gridHeight は、時間帯全体の高さ (px)。
export function gridHeight(window: GuideWindow): number {
  return timeToY(window, window.to);
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

// broadcastDay は、時刻を含む放送日 (4 時から翌 4 時まで、日本時間) の時間帯を返す。
export function broadcastDay(time: number): GuideWindow {
  const jst = 9 * 60 * minute;
  const dayStartHour = 4 * 60 * minute;
  const day = 24 * 60 * minute;
  const from = Math.floor((time + jst - dayStartHour) / day) * day - jst + dayStartHour;

  return { from, to: from + day };
}
