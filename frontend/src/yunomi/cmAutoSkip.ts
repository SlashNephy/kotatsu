import { useSyncExternalStore } from 'react';

import { browserStorage } from './volume';

// storageKey は、録画の再生で CM の区間を自動で飛ばすかを覚えるキー (#1282)。ほかの端末とは同期しない。
const storageKey = 'kotatsu.player.cmAutoSkip';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:cm-auto-skip';

// loadCmAutoSkip は、CM の区間を自動で飛ばすかを読む。CM の見分けは間違えることがあるので、既定では飛ばさない。
// 読めなければ、既定の false を返す。
export function loadCmAutoSkip(storage: Pick<Storage, 'getItem'> | null): boolean {
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
  return loadCmAutoSkip(browserStorage());
}

// useCmAutoSkip は、CM の区間を自動で飛ばすかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useCmAutoSkip(): [boolean, (enabled: boolean) => void] {
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
