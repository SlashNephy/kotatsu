import { useEffect, useState, type RefObject } from 'react';

import { loadFullscreenLandscape, lockLandscape, unlockOrientation } from './landscapeLock';
import { browserStorage } from './volume';

// useFullscreen は、プレイヤーを包む要素を全画面にする。映像だけでなく、操作バーや重ねたものも一緒に出す。
// 要素は、別のウィンドウに移ることがあるので、その時点で要素がある文書の全画面を使う。
// 端末の設定で選んでいれば、全画面にしたら横向きに固定し、全画面を抜けたら戻す (#1599)。
export function useFullscreen(boxRef: RefObject<HTMLElement | null>): {
  active: boolean;
  toggle: () => void;
} {
  const [active, setActive] = useState(false);

  useEffect(() => {
    const update = () => {
      const fullscreen = document.fullscreenElement !== null;
      setActive(fullscreen);
      // 戻るボタンや Esc で抜けたときも、固定した向きを戻す。
      if (!fullscreen) {
        unlockOrientation();
      }
    };
    document.addEventListener('fullscreenchange', update);

    return () => {
      document.removeEventListener('fullscreenchange', update);
      // 全画面のまま画面を離れても (スリープタイマーで一覧へ戻るなど)、固定した向きを残さない。
      unlockOrientation();
    };
  }, []);

  return {
    active,
    toggle: () => {
      const box = boxRef.current;
      const doc = box?.ownerDocument ?? document;
      if (doc.fullscreenElement === null) {
        void box
          ?.requestFullscreen()
          .then(() => {
            if (loadFullscreenLandscape(browserStorage())) {
              lockLandscape();
            }
          })
          .catch(() => {});
      } else {
        void doc.exitFullscreen().catch(() => {});
      }
    },
  };
}
