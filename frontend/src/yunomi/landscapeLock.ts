import { useSyncExternalStore } from 'react';

import { browserStorage } from './volume';

// 全画面にしたら、端末を横向きに固定するか (#1599、設計書 7.5)。縦向きのまま全画面にすると映像が小さいので、
// そのたびに端末を回すのを省く。端末ごとに覚える。既定では固定する。
// 固定は全画面の間しか効かず、iOS の Safari や PC のブラウザでは断られる。断られたら何もしない。

const storageKey = 'kotatsu.player.fullscreenLandscape';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:fullscreen-landscape';

// loadFullscreenLandscape は、覚えた設定を読む。覚えていないか、読めなければ、既定の true を返す。
export function loadFullscreenLandscape(storage: Pick<Storage, 'getItem'> | null): boolean {
  try {
    return storage?.getItem(storageKey) !== 'false';
  } catch {
    return true;
  }
}

// saveFullscreenLandscape は、設定を覚える。書けなくても、例外は投げない。
export function saveFullscreenLandscape(
  storage: Pick<Storage, 'setItem'> | null,
  enabled: boolean,
): void {
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

// useFullscreenLandscape は、全画面で横向きに固定するかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useFullscreenLandscape(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, () => loadFullscreenLandscape(browserStorage()));

  return [
    enabled,
    (next) => {
      saveFullscreenLandscape(browserStorage(), next);
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}

// LockableOrientation は、向きを固定できる screen.orientation。lock は、TypeScript の DOM の型にないことがある。
type LockableOrientation = ScreenOrientation & {
  lock?: (orientation: 'landscape') => Promise<void>;
};

function lockableOrientation(): LockableOrientation | null {
  const orientation = (window.screen as Screen | undefined)?.orientation as
    | LockableOrientation
    | undefined;

  return orientation !== undefined && typeof orientation.lock === 'function' ? orientation : null;
}

// fullscreenLandscapeSupported は、ブラウザが画面の向きの固定を持つかを返す。持っていても、全画面でないときや、
// PC のブラウザでは断られることがある。
export function fullscreenLandscapeSupported(): boolean {
  return lockableOrientation() !== null;
}

// lockLandscape は、画面を横向きに固定する。断られても、例外は投げない。
export function lockLandscape(): void {
  void lockableOrientation()
    ?.lock?.('landscape')
    .catch(() => {});
}

// unlockOrientation は、固定した向きを戻す。固定していなければ、何も起きない。
export function unlockOrientation(): void {
  try {
    lockableOrientation()?.unlock();
  } catch {
    // 固定していないときに外しても、困ることはない。
  }
}
