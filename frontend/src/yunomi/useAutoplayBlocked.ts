import { useEffect, useState } from 'react';

export type AutoplayBlocked = {
  // blocked は、ブラウザに自動再生を断られて、止まったままかどうか。
  blocked: boolean;
  // play は、ユーザーの操作から再生を始める。操作の中なら、ブラウザは音ありの再生を許す。
  play: () => void;
};

// useAutoplayBlocked は、音ありの自動再生をブラウザに断られたかを見分ける。
// Chrome などは、ページを操作していないうちは、音ありの自動再生を断る (NotAllowedError)。断られると、映像は最初のコマで止まる。
// video は autoplay で再生を始めるが、断られても知らせがないので、再生できるようになったら自分で play() を試して確かめる。
// 一度再生が始まったら、それ以降は試さない。止めてからシークしたときに、勝手に再生し直さないようにする。
export function useAutoplayBlocked(video: HTMLVideoElement | null): AutoplayBlocked {
  const [blocked, setBlocked] = useState(false);

  useEffect(() => {
    if (video === null) {
      return;
    }
    let played = false;
    let disposed = false;
    const tryPlay = () => {
      if (played || !video.paused) {
        return;
      }
      video.play().catch((error: unknown) => {
        if (!disposed && error instanceof DOMException && error.name === 'NotAllowedError') {
          setBlocked(true);
        }
      });
    };
    const onPlay = () => {
      played = true;
      setBlocked(false);
    };
    video.addEventListener('canplay', tryPlay);
    video.addEventListener('play', onPlay);
    // HAVE_FUTURE_DATA (3) 以上なら、canplay はもう過ぎている。
    if (video.readyState >= 3) {
      tryPlay();
    }

    return () => {
      disposed = true;
      video.removeEventListener('canplay', tryPlay);
      video.removeEventListener('play', onPlay);
      setBlocked(false);
    };
  }, [video]);

  return {
    blocked,
    play: () => {
      void video?.play().catch(() => {});
    },
  };
}
