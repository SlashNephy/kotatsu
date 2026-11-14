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

// interpolationRates は、補間したコマを出す回数の上限として選べる値 (1 秒あたり)。null は、画面の書き換えに合わせる。
// ブラウザは画面の書き換えの速さを教えないので、書き換えの間隔を測って合わせる。
export const interpolationRates = [null, 120, 60] as const;
export type InterpolationRate = (typeof interpolationRates)[number];

// rateKey は、補間したコマを出す回数の上限を覚えるキー。
const rateKey = 'kotatsu.player.interpolationRate';
const rateChangeEvent = 'kotatsu:interpolation-rate';

// loadInterpolationRate は、補間したコマを出す回数の上限を読む。覚えていないか、知らない値なら、画面の書き換えに合わせる。
export function loadInterpolationRate(storage: Pick<Storage, 'getItem'> | null): InterpolationRate {
  try {
    const stored = Number(storage?.getItem(rateKey));

    return interpolationRates.find((rate) => rate !== null && rate === stored) ?? null;
  } catch {
    return null;
  }
}

function subscribeRate(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(rateChangeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(rateChangeEvent, onChange);
  };
}

function getRateSnapshot(): InterpolationRate {
  return loadInterpolationRate(browserStorage());
}

// useInterpolationRate は、補間したコマを出す回数の上限と、それを変える関数を返す。ほかのタブで変えても追う。
export function useInterpolationRate(): [InterpolationRate, (rate: InterpolationRate) => void] {
  const rate = useSyncExternalStore(subscribeRate, getRateSnapshot);

  return [
    rate,
    (next) => {
      try {
        const storage = browserStorage();
        if (next === null) {
          storage?.removeItem(rateKey);
        } else {
          storage?.setItem(rateKey, String(next));
        }
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(rateChangeEvent));
    },
  ];
}
