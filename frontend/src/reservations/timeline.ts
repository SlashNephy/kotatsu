// 予約の一覧を、日ごとに並べるための計算。

import { fromWallClock, zoneOffset } from '../futon/timeZone';

const minute = 60_000;
const day = 24 * 60 * minute;

// DayGroup は、1 日分の予約。from は、その日の 0 時 (表示のタイムゾーン)。
export type DayGroup<T> = { from: number; items: T[] };

// groupByDay は、予約を、開始した日 (表示のタイムゾーン) ごとにまとめる。日は早い順に、日の中の予約は渡された順に並べる。
export function groupByDay<T extends { startMs: number }>(items: readonly T[]): DayGroup<T>[] {
  const groups = new Map<number, DayGroup<T>>();
  for (const item of items) {
    const from = fromWallClock(Math.floor((item.startMs + zoneOffset(item.startMs)) / day) * day);
    const group = groups.get(from);
    if (group === undefined) {
      groups.set(from, { from, items: [item] });
    } else {
      group.items.push(item);
    }
  }

  return [...groups.values()].sort((a, b) => a.from - b.from);
}

// needsAttention は、録画が欠ける見込みで、利用者に知らせるべき状態かどうかを返す。
export function needsAttention(state: string, enabled: boolean): boolean {
  return enabled && (state === 'PARTIAL' || state === 'CONFLICT');
}
