import { useSyncExternalStore } from 'react';

import { browserStorage } from './volume';

// データ放送を裏で受け取っておくか (プリロード) を、端末ごとに覚えておく (設計書 7.4)。
// 受け取る分の転送量は回線によって重さが違うので、アカウントには保存しない。データ放送を出すことは多くないので、既定では受け取らない。

const storageKey = 'kotatsu.player.dataBroadcastPreload';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:data-broadcast-preload';

// loadDataBroadcastPreload は、覚えた設定を読む。読めなければ、既定の false を返す。
export function loadDataBroadcastPreload(storage: Pick<Storage, 'getItem'> | null): boolean {
  try {
    return storage?.getItem(storageKey) === 'true';
  } catch {
    return false;
  }
}

// saveDataBroadcastPreload は、設定を覚える。書けなくても、例外は投げない。
export function saveDataBroadcastPreload(
  storage: Pick<Storage, 'setItem'> | null,
  preload: boolean,
): void {
  try {
    storage?.setItem(storageKey, String(preload));
  } catch {
    // 書けなければ、次に開いたときは既定に戻るだけ。
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

// useDataBroadcastPreload は、データ放送をプリロードするかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useDataBroadcastPreload(): [boolean, (preload: boolean) => void] {
  const preload = useSyncExternalStore(subscribe, () => loadDataBroadcastPreload(browserStorage()));

  return [
    preload,
    (next) => {
      saveDataBroadcastPreload(browserStorage(), next);
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}
