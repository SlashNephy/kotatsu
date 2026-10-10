// 予約の一覧を、日ごとの時間軸に並べるための計算。

const minute = 60_000;
const day = 24 * 60 * minute;
const jst = 9 * 60 * minute;

// DayGroup は、1 日分の予約。from は、その日の 0 時 (日本時間)。
export type DayGroup<T> = { from: number; items: T[] };

// groupByDay は、予約を、開始した日 (日本時間) ごとにまとめる。日は早い順に、日の中の予約は渡された順に並べる。
export function groupByDay<T extends { startMs: number }>(items: readonly T[]): DayGroup<T>[] {
  const groups = new Map<number, DayGroup<T>>();
  for (const item of items) {
    const from = Math.floor((item.startMs + jst) / day) * day - jst;
    const group = groups.get(from);
    if (group === undefined) {
      groups.set(from, { from, items: [item] });
    } else {
      group.items.push(item);
    }
  }

  return [...groups.values()].sort((a, b) => a.from - b.from);
}

// Bar は、24 時間の軸の上の、予約の帯の位置と幅 (%)。
export type Bar = { left: number; width: number };

// minBarWidth は、短い予約でも見えるようにする、帯の最小の幅 (%)。
const minBarWidth = 0.5;

// timelineBar は、dayFrom から 24 時間の軸に、予約の帯を置く。日をまたぐ予約は、その日の終わりで切る。
export function timelineBar(dayFrom: number, startAt: number, endAt: number): Bar {
  const start = Math.max(startAt, dayFrom);
  const end = Math.min(endAt, dayFrom + day);
  const left = ((start - dayFrom) / day) * 100;
  const width = Math.max(((end - start) / day) * 100, minBarWidth);

  return { left, width: Math.min(width, 100 - left) };
}

// needsAttention は、録画が欠ける見込みで、利用者に知らせるべき状態かどうかを返す。
export function needsAttention(state: string, enabled: boolean): boolean {
  return enabled && (state === 'PARTIAL' || state === 'CONFLICT');
}
