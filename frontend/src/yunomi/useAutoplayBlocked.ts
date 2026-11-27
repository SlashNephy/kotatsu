import { useEffect, useState } from 'react';

import type { PlayerMedia } from './playerMedia';

export type AutoplayBlocked = {
  // blocked は、ブラウザに自動再生を拒否されて、止まったままかどうか。
  blocked: boolean;
  // play は、ユーザーの操作から再生を始める。操作の中なら、ブラウザは音ありの再生を許す。
  play: () => void;
};

// useAutoplayBlocked は、音ありの自動再生をブラウザに拒否されたかを見分ける。
// Chrome などは、ページを操作していないうちは、音ありの自動再生を拒否する (NotAllowedError)。拒否されると、映像は最初のコマで止まる。
// video は autoplay で再生を始めるが、拒否されても通知がないので、再生できるようになったら自分で play() を試して確認する。
// 一度再生が始まったら、それ以降は試さない。止めてからシークしたときに、勝手に再生し直さないようにする。
export function useAutoplayBlocked(media: PlayerMedia | null): AutoplayBlocked {
  const [blocked, setBlocked] = useState(false);

  useEffect(() => {
    if (media === null) {
      return;
    }
    let played = false;
    let disposed = false;
    const tryPlay = () => {
      if (played || !media.paused) {
        return;
      }
      media.play().catch((error: unknown) => {
        if (!disposed && error instanceof DOMException && error.name === 'NotAllowedError') {
          setBlocked(true);
        }
      });
    };
    const onPlay = () => {
      played = true;
      setBlocked(false);
    };
    media.addEventListener('canplay', tryPlay);
    media.addEventListener('play', onPlay);
    // HAVE_FUTURE_DATA (3) 以上なら、canplay はもう過ぎている。
    if (media.readyState >= 3) {
      tryPlay();
    }

    return () => {
      disposed = true;
      media.removeEventListener('canplay', tryPlay);
      media.removeEventListener('play', onPlay);
      setBlocked(false);
    };
  }, [media]);

  return {
    blocked,
    play: () => {
      void media?.play().catch(() => {});
    },
  };
}
