import { useSyncExternalStore } from 'react';

import {
  defaultSuperResolutionStrength,
  superResolutionMethods,
  superResolutionStrengths,
  type SuperResolutionMethod,
  type SuperResolutionStrength,
} from './pictureUpscaler';
import { browserStorage } from './volume';

// 映像の拡大 (#1654、設計書 7.2、7.5)。yadif が解除したコマを、映像を描く矩形の端末の画素の大きさへ、
// 縁を見て拡大し、鮮鋭化してから描く。ブラウザに引き伸ばさせるより、縁がぼやけない。

// storageKey は、映像を拡大するかを保存するキー。GPU の負荷は端末によるので、ほかの端末とは同期しない。
const storageKey = 'kotatsu.player.superResolution';
// changeEvent は、同じタブの中で変えたことを通知するイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:super-resolution';

// loadSuperResolution は、映像を拡大するかを読む。GPU の負荷が増え、鮮鋭化はノイズも目立たせるので、既定では拡大しない。
// 読めなければ、既定の false を返す。
export function loadSuperResolution(storage: Pick<Storage, 'getItem'> | null): boolean {
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
  return loadSuperResolution(browserStorage());
}

// useSuperResolution は、映像を拡大するかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useSuperResolution(): [boolean, (enabled: boolean) => void] {
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

// strengthKey は、鮮鋭化の強さを保存するキー。画面と映像の見え方は端末によるので、ほかの端末とは同期しない。
const strengthKey = 'kotatsu.player.superResolutionStrength';

// loadSuperResolutionStrength は、鮮鋭化の強さを読む。保存していないか、知らない値か、読めなければ既定の強さを返す。
export function loadSuperResolutionStrength(
  storage: Pick<Storage, 'getItem'> | null,
): SuperResolutionStrength {
  try {
    const stored = Number(storage?.getItem(strengthKey));

    return (
      superResolutionStrengths.find((strength) => strength === stored) ??
      defaultSuperResolutionStrength
    );
  } catch {
    return defaultSuperResolutionStrength;
  }
}

function getStrengthSnapshot(): SuperResolutionStrength {
  return loadSuperResolutionStrength(browserStorage());
}

// useSuperResolutionStrength は、鮮鋭化の強さと、それを変える関数を返す。ほかのタブで変えても追う。
export function useSuperResolutionStrength(): [
  SuperResolutionStrength,
  (strength: SuperResolutionStrength) => void,
] {
  const strength = useSyncExternalStore(subscribe, getStrengthSnapshot);

  return [
    strength,
    (next) => {
      try {
        browserStorage()?.setItem(strengthKey, String(next));
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}

// methodKey は、拡大の方式を保存するキー。GPU の負荷は端末によるので、ほかの端末とは同期しない。
const methodKey = 'kotatsu.player.superResolutionMethod';

// loadSuperResolutionMethod は、拡大の方式を読む。保存していないか、知らない値か、読めなければ、軽い fsr を返す。
export function loadSuperResolutionMethod(
  storage: Pick<Storage, 'getItem'> | null,
): SuperResolutionMethod {
  try {
    const stored = storage?.getItem(methodKey);

    return superResolutionMethods.find((method) => method === stored) ?? 'fsr';
  } catch {
    return 'fsr';
  }
}

function getMethodSnapshot(): SuperResolutionMethod {
  return loadSuperResolutionMethod(browserStorage());
}

// useSuperResolutionMethod は、拡大の方式と、それを変える関数を返す。ほかのタブで変えても追う。
export function useSuperResolutionMethod(): [
  SuperResolutionMethod,
  (method: SuperResolutionMethod) => void,
] {
  const method = useSyncExternalStore(subscribe, getMethodSnapshot);

  return [
    method,
    (next) => {
      try {
        browserStorage()?.setItem(methodKey, next);
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}
