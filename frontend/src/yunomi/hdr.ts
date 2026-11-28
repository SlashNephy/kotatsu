import { useSyncExternalStore } from 'react';

import { browserStorage } from './volume';

// 擬似 HDR の出力 (#1656、設計書 7.2、7.5)。yadif が解除したコマの明るい所を、SDR の白より明るく伸ばして描く。
// yadif の WebGL2 の描画先を RGBA16F にし、トーンマッピングを extended にすると、1 を超える値が白より明るく出る
// (WebGL の drawingBufferToneMapping。https://github.com/KhronosGroup/WebGL/pull/3668)。
// 持たないブラウザ (Chromium 158 でも既定では無効) と、HDR でない画面では、今までどおり SDR で描く。

// storageKey は、擬似 HDR で出すかを保存するキー。画面は端末によるので、ほかの端末とは同期しない。
const storageKey = 'kotatsu.player.hdrOutput';
// changeEvent は、同じタブの中で変えたことを通知するイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:hdr-output';
// hdrDisplayQuery は、HDR を出せる画面かを問うメディアクエリ。
const hdrDisplayQuery = '(dynamic-range: high)';

// loadHDROutput は、擬似 HDR で出すかを読む。見え方が元の映像と変わるので、既定では出さない。
// 読めなければ、既定の false を返す。
export function loadHDROutput(storage: Pick<Storage, 'getItem'> | null): boolean {
  try {
    return storage?.getItem(storageKey) === 'true';
  } catch {
    return false;
  }
}

function subscribeSetting(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

function getSettingSnapshot(): boolean {
  return loadHDROutput(browserStorage());
}

// useHDROutput は、擬似 HDR で出すかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useHDROutput(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribeSetting, getSettingSnapshot);

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

function subscribeDisplay(onChange: () => void): () => void {
  const query = window.matchMedia(hdrDisplayQuery);
  query.addEventListener('change', onChange);

  return () => {
    query.removeEventListener('change', onChange);
  };
}

function getDisplaySnapshot(): boolean {
  return window.matchMedia(hdrDisplayQuery).matches;
}

// useHDRDisplay は、今の画面が HDR を出せるかを返す。ウィンドウを別の画面へ移したときも追う。
export function useHDRDisplay(): boolean {
  return useSyncExternalStore(subscribeDisplay, getDisplaySnapshot, () => false);
}
