import { useEffect, useState, type RefObject } from 'react';

// useFullscreen は、プレイヤーを包む要素を全画面にする。映像だけでなく、操作バーや重ねたものも一緒に出す。
// 要素は、別のウィンドウに移ることがあるので、その時点で要素がある文書の全画面を使う。
export function useFullscreen(boxRef: RefObject<HTMLElement | null>): {
  active: boolean;
  toggle: () => void;
} {
  const [active, setActive] = useState(false);

  useEffect(() => {
    const update = () => setActive(document.fullscreenElement !== null);
    document.addEventListener('fullscreenchange', update);

    return () => document.removeEventListener('fullscreenchange', update);
  }, []);

  return {
    active,
    toggle: () => {
      const box = boxRef.current;
      const doc = box?.ownerDocument ?? document;
      if (doc.fullscreenElement === null) {
        void box?.requestFullscreen().catch(() => {});
      } else {
        void doc.exitFullscreen().catch(() => {});
      }
    },
  };
}
