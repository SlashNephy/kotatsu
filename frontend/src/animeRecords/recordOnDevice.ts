import { useSyncExternalStore } from 'react';

import { browserStorage } from '../yunomi/volume';

// RecordKind は、この端末で記録するかを分けて保存する記録の種類。見たアニメ (#1283) と、見たドラマ (#1649)。
export type RecordKind = 'anime' | 'drama';

// storageKeys は、この端末で記録するかを保存するキー。ほかの端末とは同期しない。アニメのキーは、分ける前のものをそのまま使う。
const storageKeys: Record<RecordKind, string> = {
  anime: 'kotatsu.animeRecords.recordOnDevice',
  drama: 'kotatsu.dramaRecords.recordOnDevice',
};
// changeEvent は、同じタブの中で変えたことを通知するイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:anime-record-on-device';

// loadRecordOnDevice は、この端末で見たアニメ (kind が drama なら、見たドラマ) を記録するかを読む。既定では記録する。
// 読めなければ、既定の true を返す。
export function loadRecordOnDevice(
  storage: Pick<Storage, 'getItem'> | null,
  kind: RecordKind = 'anime',
): boolean {
  try {
    return storage?.getItem(storageKeys[kind]) !== 'false';
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

// useRecordOnDevice は、この端末で見たアニメ (kind が drama なら、見たドラマ) を記録するかと、それを変える関数を返す。
// ほかのタブで変えても追う。
export function useRecordOnDevice(
  kind: RecordKind = 'anime',
): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, () => loadRecordOnDevice(browserStorage(), kind));

  return [
    enabled,
    (next) => {
      try {
        browserStorage()?.setItem(storageKeys[kind], String(next));
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}
