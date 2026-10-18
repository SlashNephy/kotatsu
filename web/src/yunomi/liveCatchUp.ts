import { useEffect, useSyncExternalStore } from 'react';

import { browserStorage } from './volume';

// ライブの低遅延モード (#495、設計書 7.2、7.3)。放送からの遅れが広がったら、少し速く再生して追いつく。
// 回線が細いと、追いつこうとして止まりやすくなるので、端末ごとに選び、既定では切る。

// catchUpRate は、追いつくときの再生の速さ。音の高さが変わって聞き苦しくならない程度にとどめる。
export const catchUpRate = 1.1;
// catchUpStart は、追いつき始める遅れ (秒)。遅れは、読み込んだ映像の末尾と、今の再生位置との差で測る。
export const catchUpStart = 3;
// catchUpStop は、追いつくのをやめる遅れ (秒)。読み込みが止まらないよう、少しは溜めておく。
export const catchUpStop = 1;

// nextPlaybackRate は、遅れ lag (秒) と今の再生の速さ current から、次の再生の速さを返す。
// 追いつき始めたら、catchUpStop まで縮むまで続ける。間で速さを行き来しないよう、始めるときとやめるときの遅れを分ける。
export function nextPlaybackRate(lag: number, current: number): number {
  if (lag > catchUpStart) {
    return catchUpRate;
  }
  if (lag <= catchUpStop) {
    return 1;
  }

  return current === catchUpRate ? catchUpRate : 1;
}

// liveLag は、video の、読み込んだ映像の末尾と今の再生位置との差 (秒) を返す。まだ読み込んでいなければ 0。
export function liveLag(video: Pick<HTMLVideoElement, 'buffered' | 'currentTime'>): number {
  const { buffered } = video;
  if (buffered.length === 0) {
    return 0;
  }

  return Math.max(buffered.end(buffered.length - 1) - video.currentTime, 0);
}

// useLiveCatchUp は、enabled の間、1 秒ごとに遅れを測り、再生の速さを変えて追いつく。
// 一時停止している間は、変えない。止めた分は、再開したときの読み直し (useLiveLag) で取り戻す。
export function useLiveCatchUp(video: HTMLVideoElement | null, enabled: boolean): void {
  useEffect(() => {
    if (video === null || !enabled) {
      return;
    }
    const timer = window.setInterval(() => {
      if (video.paused) {
        return;
      }
      const next = nextPlaybackRate(liveLag(video), video.playbackRate);
      if (next !== video.playbackRate) {
        video.playbackRate = next;
      }
    }, 1_000);

    return () => {
      window.clearInterval(timer);
      video.playbackRate = 1;
    };
  }, [video, enabled]);
}

const storageKey = 'kotatsu.player.lowLatency';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:low-latency';

// loadLowLatency は、低遅延モードの設定を読む。読めなければ、既定の false を返す。
export function loadLowLatency(storage: Pick<Storage, 'getItem'> | null): boolean {
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

// useLowLatency は、低遅延モードを使うかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useLowLatency(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, () => loadLowLatency(browserStorage()));

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
