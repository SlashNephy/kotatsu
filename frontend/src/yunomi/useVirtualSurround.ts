import { useEffect, useSyncExternalStore } from 'react';

import { browserStorage } from './volume';
import { setSurround } from './volumeBoost';

// 仮想サラウンド (#454) を使うかを、端末ごとに覚える。ヘッドホンで聞くかは端末ごとに違うので、アカウントには保存しない。
// WebAudio を通すので、音量の補正と同じく、既定では切る。

const storageKey = 'kotatsu.player.virtualSurround';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:virtual-surround';

// surroundComponentType は、5.1ch (3/2+LFE) の音声の component_type。
const surroundComponentType = 9;

// isSurroundAudio は、番組の主音声 (音声の一覧の先頭) が 5.1ch かを返す。
export function isSurroundAudio(audios: readonly { componentType: number }[] | null): boolean {
  return audios?.[0]?.componentType === surroundComponentType;
}

// loadVirtualSurround は、設定を読む。読めなければ、既定の false を返す。
export function loadVirtualSurround(storage: Pick<Storage, 'getItem'> | null): boolean {
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

// useVirtualSurroundSetting は、仮想サラウンドを使うかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useVirtualSurroundSetting(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, () => loadVirtualSurround(browserStorage()));

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

// useVirtualSurround は、available の間、設定に合わせて video の仮想サラウンドを入れるか切る。
// available は、番組の音声が 5.1ch で、6ch のまま届く経路 (ブラウザの中での変換) で見ているとき。
// kyusu は 2ch にしてから送るので、仮想サラウンドにできない。
export function useVirtualSurround(video: HTMLVideoElement | null, available: boolean): void {
  const [enabled] = useVirtualSurroundSetting();
  const active = enabled && available;
  useEffect(() => {
    if (video !== null) {
      setSurround(video, active);
    }
  }, [video, active]);
}
