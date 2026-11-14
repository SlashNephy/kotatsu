import { useSyncExternalStore } from 'react';

import { browserStorage } from './volume';

// コマの補間 (#1655、設計書 7.5)。24p や 30p の映像の間に、動きから推し量ったコマを挟んでなめらかに見せる。

// storageKey は、コマを補間するかを覚えるキー。GPU の負荷は端末によるので、ほかの端末とは同期しない。
const storageKey = 'kotatsu.player.interpolation';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:interpolation';

// loadInterpolation は、コマを補間するかを読む。作ったコマは崩れることがあり、映像も 1 コマ遅れるので、既定では補間しない。
// 読めなければ、既定の false を返す。
export function loadInterpolation(storage: Pick<Storage, 'getItem'> | null): boolean {
  try {
    return storage?.getItem(storageKey) === 'true';
  } catch {
    return false;
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
  return loadInterpolation(browserStorage());
}

// useInterpolation は、コマを補間するかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useInterpolation(): [boolean, (enabled: boolean) => void] {
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
