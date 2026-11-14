import { useSyncExternalStore } from 'react';

import { browserStorage } from './volume';

// 再生中に視聴の画面から別の画面へ移ったら、画面の隅の小さなプレイヤーで再生を続けるか (#1602、設計書 7.5)。
// 画面の広さや使い方は端末ごとに違うので、端末ごとに覚える。既定では続ける。

const storageKey = 'kotatsu.player.miniPlayer';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:mini-player';

// loadMiniPlayer は、覚えた設定を読む。読めなければ、既定の true を返す。
export function loadMiniPlayer(storage: Pick<Storage, 'getItem'> | null): boolean {
  try {
    return storage?.getItem(storageKey) !== 'false';
  } catch {
    return true;
  }
}

// saveMiniPlayer は、設定を覚える。書けなくても、例外は投げない。
export function saveMiniPlayer(storage: Pick<Storage, 'setItem'> | null, enabled: boolean): void {
  try {
    storage?.setItem(storageKey, String(enabled));
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

// useMiniPlayer は、小さなプレイヤーで再生を続けるかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useMiniPlayer(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, () => loadMiniPlayer(browserStorage()));

  return [
    enabled,
    (next) => {
      saveMiniPlayer(browserStorage(), next);
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}
