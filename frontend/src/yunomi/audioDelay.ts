import { useEffect, useSyncExternalStore } from 'react';

import type { PlayerCommands } from './commands';
import { browserStorage } from './volume';
import { setAudioDelay } from './volumeBoost';

// 音ずれ補正 (#818、設計書 7.5)。音声だけを WebAudio の DelayNode で遅らせ、映像より早く聞こえる音を合わせる。
// 遅らせることしかできないので、音が映像より遅い場合は補正できない。端末ごとに覚える。

const delayKey = 'kotatsu.player.audioDelayMs';
const resetKey = 'kotatsu.player.resetAudioDelayOnChannelChange';
// changeEvent は、同じタブの中で変えたことを知らせるイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:audio-delay';

// maxAudioDelayMs は、遅らせられる上限 (ミリ秒)。audioDelayStepMs は、キーとメニューで変える幅。
export const maxAudioDelayMs = 500;
export const audioDelayStepMs = 20;

// clampAudioDelay は、遅らせる長さを、0 から上限の、刻みにそろえた値にする。
function clampAudioDelay(ms: number): number {
  const stepped = Math.round(ms / audioDelayStepMs) * audioDelayStepMs;

  return Math.min(Math.max(stepped, 0), maxAudioDelayMs);
}

// loadAudioDelay は、覚えた遅らせる長さ (ミリ秒) を読む。読めなければ 0 を返す。
export function loadAudioDelay(storage: Pick<Storage, 'getItem'> | null): number {
  try {
    const ms = Number(storage?.getItem(delayKey) ?? 0);

    return Number.isFinite(ms) ? clampAudioDelay(ms) : 0;
  } catch {
    return 0;
  }
}

// loadResetAudioDelay は、選局したときに音ずれ補正を元に戻すかを読む。既定では戻さない。
export function loadResetAudioDelay(storage: Pick<Storage, 'getItem'> | null): boolean {
  try {
    return storage?.getItem(resetKey) === 'true';
  } catch {
    return false;
  }
}

// stepAudioDelay は、遅らせる長さ ms を、direction の向きに 1 刻み変えた値を返す。
export function stepAudioDelay(ms: number, direction: 1 | -1): number {
  return clampAudioDelay(
    Math.round(ms / audioDelayStepMs) * audioDelayStepMs + direction * audioDelayStepMs,
  );
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

// save は、設定を書き、同じタブの購読者に知らせる。
function save(key: string, value: string): void {
  try {
    browserStorage()?.setItem(key, value);
  } catch {
    // 書けなければ、次に開いたときは既定に戻るだけ。
  }
  window.dispatchEvent(new Event(changeEvent));
}

// useAudioDelay は、遅らせる長さ (ミリ秒) と、それを変える関数を返す。ほかのタブで変えても追う。
export function useAudioDelay(): [number, (ms: number) => void] {
  const ms = useSyncExternalStore(subscribe, () => loadAudioDelay(browserStorage()));

  return [ms, (next) => save(delayKey, String(clampAudioDelay(next)))];
}

// useResetAudioDelay は、選局したときに元に戻すかと、それを変える関数を返す。
export function useResetAudioDelay(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, () => loadResetAudioDelay(browserStorage()));

  return [enabled, (next) => save(resetKey, String(next))];
}

// useAudioDelayer は、設定に合わせて、video の音を遅らせる。
export function useAudioDelayer(video: HTMLVideoElement | null): void {
  const [ms] = useAudioDelay();
  useEffect(() => {
    if (video !== null) {
      setAudioDelay(video, ms / 1_000);
    }
  }, [video, ms]);
}

// useResetAudioDelayOnChannel は、選局したとき (serviceId が変わったとき、ライブの画面を開いたとき) に、
// 設定に従って音ずれ補正を元に戻す。
export function useResetAudioDelayOnChannel(serviceId: string): void {
  useEffect(() => {
    if (loadResetAudioDelay(browserStorage()) && loadAudioDelay(browserStorage()) !== 0) {
      save(delayKey, '0');
    }
  }, [serviceId]);
}

// useAudioDelayCommands は、音ずれ補正の操作を返す。変えたら、onChange に新しい長さ (ミリ秒) を渡す。
export function useAudioDelayCommands(
  onChange: (ms: number) => void,
): Pick<PlayerCommands, 'audioDelayIncrease' | 'audioDelayDecrease' | 'audioDelayReset'> {
  const [ms, setMs] = useAudioDelay();
  const change = (next: number) => {
    setMs(next);
    onChange(next);
  };

  return {
    audioDelayIncrease: () => change(stepAudioDelay(ms, 1)),
    audioDelayDecrease: () => change(stepAudioDelay(ms, -1)),
    audioDelayReset: () => change(0),
  };
}
