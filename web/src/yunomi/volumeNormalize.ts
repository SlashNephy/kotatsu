import { useEffect, useSyncExternalStore } from 'react';

import { browserStorage } from './volume';
import { setNormalize } from './volumeBoost';

// 音量の補正 (#499、設計書 7.5)。番組と CM、局どうしの音量の差を縮める。
// WebAudio を通すので、iOS の Safari やバックグラウンドの再生で動きが変わるおそれがある。端末ごとに選び、既定では切る。

const storageKey = 'kotatsu.player.normalizeVolume';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:normalize-volume';

// loadNormalizeVolume は、設定を読む。読めなければ、既定の false を返す。
export function loadNormalizeVolume(storage: Pick<Storage, 'getItem'> | null): boolean {
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

// useNormalizeVolume は、音量を補正するかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useNormalizeVolume(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, () => loadNormalizeVolume(browserStorage()));

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

// useVolumeNormalizer は、設定に合わせて、video の音量の補正を入れるか切る。
export function useVolumeNormalizer(video: HTMLVideoElement | null): void {
  const [enabled] = useNormalizeVolume();
  useEffect(() => {
    if (video !== null) {
      setNormalize(video, enabled);
    }
  }, [video, enabled]);
}
