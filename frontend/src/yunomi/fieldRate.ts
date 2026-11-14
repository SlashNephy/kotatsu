import { useSyncExternalStore } from 'react';

import { browserStorage } from './volume';

// フィールドごとの表示 (#1655、設計書 7.2)。1080i の 1 コマには、時刻の違う 2 つのフィールドが入っている。
// yadif の doubleRate で、フィールドごとに 1 コマを作って 59.94p で出すと、放送そのままのなめらかさになる。

// storageKey は、フィールドごとに表示するかを覚えるキー。表示が追いつくかは端末によるので、ほかの端末とは同期しない。
const storageKey = 'kotatsu.player.fieldRate';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:field-rate';

// loadFieldRate は、フィールドごとに表示するかを読む。既定では表示する。読めなければ、既定の true を返す。
export function loadFieldRate(storage: Pick<Storage, 'getItem'> | null): boolean {
  try {
    return storage?.getItem(storageKey) !== 'false';
  } catch {
    return true;
  }
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

function getSnapshot(): boolean {
  return loadFieldRate(browserStorage());
}

// useFieldRate は、フィールドごとに表示するかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useFieldRate(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, getSnapshot);

  return [
    enabled,
    (next) => {
      try {
        browserStorage()?.setItem(storageKey, String(next));
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}

// lateFieldsPerSecond は、1 秒あたりにこれ以上のコマが間に合わなければ、その秒は追いつかなかったとみなす数。
// 60 Hz の画面で 59.94 フィールドを出すと、十数秒に 1 コマはずれるので、数コマは見逃す。
export const lateFieldsPerSecond = 6;
// lateSeconds は、追いつかない秒がこれだけ続いたら、フィールドごとの表示をやめる秒数。
export const lateSeconds = 5;

// LateReport は、yadif が 1 秒ごとに知らせる統計のうち、追いつかないかを決めるのに使うもの。
// late は読み込みの始まりからの累計で、読み直すと 0 に戻る。
export type LateReport = {
  late: number;
  film: boolean;
};

// createLateWatch は、yadif の統計を受け取り、フィールドごとの表示が追いつかないかを見張るものを作る。
// observe は、追いつかない秒が lateSeconds 続いたら true を返す。
// 24p で出している間 (#1284) は、フィールドごとに表示していないので数えない。
export function createLateWatch(): { observe(report: LateReport): boolean; reset(): void } {
  let previous: number | null = null;
  let streak = 0;

  return {
    observe({ late, film }) {
      // 読み直すと累計が 0 に戻るので、減ったら、そこから数え直す。
      const delta = previous === null || late < previous ? 0 : late - previous;
      previous = late;
      if (film) {
        streak = 0;

        return false;
      }
      streak = delta >= lateFieldsPerSecond ? streak + 1 : 0;

      return streak >= lateSeconds;
    },
    reset() {
      previous = null;
      streak = 0;
    },
  };
}
