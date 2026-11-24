// 番組表の時刻の表し方 (#486)。放送日は 4 時に始まるので、28 時間表記では、深夜の 0 時〜3 時を 24 時〜27 時と表す。

import { zoneOffset } from '../futon/timeZone';
import { useSyncExternalStore } from 'react';

import { preferenceChanged } from '../settings/syncedPreferences';

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
  return rawSnapshot() === 'true';
}

// saveExtendedHours は、28 時間表記にするかどうかを、端末に保存する。保存できなくても、今の画面には効かせる。
export function saveExtendedHours(extended: boolean): void {
  try {
    localStorage.setItem(storageKey, String(extended));
    preferenceChanged(storageKey);
  } catch {
    // 端末に保存できないだけで、表示は変える。
  }
  window.dispatchEvent(new Event(changeEvent));
}

// changeEvent は、同じタブの中で変えたことを通知するイベント。storage イベントは、ほかのタブと、ほかの端末から同期した値 (#1862) にしか届かない。
const changeEvent = 'kotatsu:extended-hours';

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

// useExtendedHours は、28 時間表記にするかどうかと、それを変える関数を返す。ほかのタブや、ほかの端末で変えても追う (#1862)。
// 読んだ文字列を手がかりにするので、値が変わらなければ、描き直さない。
export function useExtendedHours(): [boolean, (next: boolean) => void] {
  // 読んだ文字列から決める。引数なしで読み直すと、React Compiler が結果をメモ化して使い回すからである。
  return [useSyncExternalStore(subscribe, rawSnapshot) === 'true', saveExtendedHours];
}

function rawSnapshot(): string | null {
  try {
    return localStorage.getItem(storageKey);
  } catch {
    return null;
  }
}
