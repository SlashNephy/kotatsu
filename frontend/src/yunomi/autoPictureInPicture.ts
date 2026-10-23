import { useSyncExternalStore } from 'react';

import { browserStorage } from './volume';

// タブを切り替えたら、自動で視聴画面を別のウィンドウ (Document PiP) に出すか (#492、設計書 7.5)。
// ブラウザの対応や使い方は端末ごとに違うので、端末ごとに覚える。既定では出さない。

const storageKey = 'kotatsu.player.autoPictureInPicture';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:auto-picture-in-picture';

// loadAutoPictureInPicture は、覚えた設定を読む。読めなければ、既定の false を返す。
export function loadAutoPictureInPicture(storage: Pick<Storage, 'getItem'> | null): boolean {
  try {
    return storage?.getItem(storageKey) === 'true';
  } catch {
    return false;
  }
}

// saveAutoPictureInPicture は、設定を覚える。書けなくても、例外は投げない。
export function saveAutoPictureInPicture(
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

// useAutoPictureInPicture は、自動で別のウィンドウに出すかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useAutoPictureInPicture(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, () => loadAutoPictureInPicture(browserStorage()));

  return [
    enabled,
    (next) => {
      saveAutoPictureInPicture(browserStorage(), next);
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}

// autoPictureInPictureSupported は、ブラウザが、タブを切り替えたときに自動で別のウィンドウに出せるかを返す。
// Chrome は、Media Session の enterpictureinpicture の操作を受け付けると、タブを切り替えたときにそれを呼び、
// その中でなら、ユーザーの操作なしに Document PiP を開ける。知らない操作を渡すと、setActionHandler は例外を投げる。
export function autoPictureInPictureSupported(): boolean {
  if (!('documentPictureInPicture' in window) || !('mediaSession' in navigator)) {
    return false;
  }
  try {
    navigator.mediaSession.setActionHandler('enterpictureinpicture' as MediaSessionAction, null);

    return true;
  } catch {
    return false;
  }
}
