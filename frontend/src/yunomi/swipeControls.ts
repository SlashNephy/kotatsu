import { useSyncExternalStore } from 'react';

import { browserStorage } from './volume';

// 全画面で、映像の左右を縦にスワイプして、明るさと音量を変えるか (#1598、設計書 7.5)。
// タッチの操作は端末によって合う合わないがあるので、端末ごとに覚える。既定では切っておく。

const storageKey = 'kotatsu.player.swipeControls';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:swipe-controls';

// loadSwipeControls は、覚えた設定を読む。覚えていないか、読めなければ、既定の false を返す。
export function loadSwipeControls(storage: Pick<Storage, 'getItem'> | null): boolean {
  try {
    return storage?.getItem(storageKey) === 'true';
  } catch {
    return false;
  }
}

// saveSwipeControls は、設定を覚える。書けなくても、例外は投げない。
export function saveSwipeControls(
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

// useSwipeControlsSetting は、スワイプで明るさと音量を変えるかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useSwipeControlsSetting(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, () => loadSwipeControls(browserStorage()));

  return [
    enabled,
    (next) => {
      saveSwipeControls(browserStorage(), next);
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}

// touchSupported は、端末がタッチで操作できるかを返す。マウスだけの環境では、設定の項目を出さない。
export function touchSupported(): boolean {
  return typeof navigator !== 'undefined' && navigator.maxTouchPoints > 0;
}

// volumeChangeable は、ページから video の音量を変えられるかを返す。iOS では、音量は本体のボタンでしか変えられず、
// volume に書いても 1 のままになるので、音量の帯を出さない。
export function volumeChangeable(): boolean {
  try {
    const video = document.createElement('video');
    video.volume = 0.5;

    return video.volume === 0.5;
  } catch {
    return false;
  }
}
