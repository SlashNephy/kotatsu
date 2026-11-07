import { useSyncExternalStore } from 'react';

import { browserStorage } from '../yunomi/volume';

// storageKey は、この端末で見たアニメを記録するかを覚えるキー (#1283)。ほかの端末とは同期しない。
const storageKey = 'kotatsu.animeRecords.recordOnDevice';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:anime-record-on-device';

// loadRecordOnDevice は、この端末で見たアニメを記録するかを読む。既定では記録する。
// 読めなければ、既定の true を返す。
export function loadRecordOnDevice(storage: Pick<Storage, 'getItem'> | null): boolean {
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
  return loadRecordOnDevice(browserStorage());
}

// useRecordOnDevice は、この端末で見たアニメを記録するかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useRecordOnDevice(): [boolean, (enabled: boolean) => void] {
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
