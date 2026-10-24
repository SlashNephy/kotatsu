// 番組表の時刻の表し方 (#486)。放送日は 4 時に始まるので、28 時間表記では、深夜の 0 時〜3 時を 24 時〜27 時と表す。

import { zoneOffset } from '../futon/timeZone';

const hour = 3_600_000;
// dayStartHour は、放送日が始まる時。
const dayStartHour = 4;

// guideHour は、時刻 time の「時」を返す。extended なら、放送日の 0 時 (その日の 4 時の 4 時間前) から数え、4〜27 にする。
// day は、time を含む放送日の始まり (layout.ts の broadcastDay の from)。
export function guideHour(time: number, day: number, extended: boolean): number {
  if (extended) {
    return Math.floor((time - (day - dayStartHour * hour)) / hour);
  }

  return new Date(time + zoneOffset(time)).getUTCHours();
}

// guideTime は、時刻 time を「HH:MM」で表す。extended なら、28 時間表記にする。
export function guideTime(time: number, day: number, extended: boolean): string {
  const minutes = new Date(time + zoneOffset(time)).getUTCMinutes();

  return `${String(guideHour(time, day, extended)).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`;
}

const storageKey = 'kotatsu.guide.extendedHours';

// loadExtendedHours は、28 時間表記にするかどうかを、端末から読む。読めなければ、使わない。
export function loadExtendedHours(): boolean {
  try {
    return localStorage.getItem(storageKey) === 'true';
  } catch {
    return false;
  }
}

// saveExtendedHours は、28 時間表記にするかどうかを、端末に覚える。覚えられなくても、今の画面には効かせる。
export function saveExtendedHours(extended: boolean): void {
  try {
    localStorage.setItem(storageKey, String(extended));
  } catch {
    // 端末に覚えられないだけで、表示は変える。
  }
}
