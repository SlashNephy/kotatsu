import { useEffect, useRef } from 'react';

import type { PlayerFrameMetadata, PlayerMedia } from './playerMedia';

// VideoFrame は、映ったコマ 1 つ分の知らせ。
export type VideoFrame = {
  // mediaTime は、そのコマの時刻 (requestVideoFrameCallback の mediaTime)。
  // requestVideoFrameCallback のないブラウザと、timeupdate で呼んだときは null で、そのときは currentTime を使う。
  mediaTime: number | null;
  // presented は、コマが映ったことの知らせかどうか。timeupdate で呼んだときは false で、映ったコマと結びつかない。
  presented: boolean;
};

// useVideoFrames は、video にコマが映るごとに onFrame を呼ぶ (#1512)。
// requestVideoFrameCallback があれば使い、なければ requestAnimationFrame で代える。一時停止の間は、requestVideoFrameCallback は来ない。
// 取りこぼしを防ぐため、timeupdate でも呼ぶ。video を作り直したら (画質の切り替え、つなぎ直し)、付け直す。
export function useVideoFrames(
  media: PlayerMedia | null,
  onFrame: (frame: VideoFrame) => void,
): void {
  const handler = useRef(onFrame);
  handler.current = onFrame;
  useEffect(() => {
    if (media === null) {
      return;
    }
    let stopped = false;
    let cancel: () => void;
    const request = media.requestVideoFrameCallback;
    const cancelFrame = media.cancelVideoFrameCallback;
    if (request !== undefined && cancelFrame !== undefined) {
      let id = 0;
      const next = (_now: number, metadata: PlayerFrameMetadata) => {
        if (stopped) {
          return;
        }
        id = request(next);
        handler.current({ mediaTime: metadata.mediaTime, presented: true });
      };
      id = request(next);
      cancel = () => cancelFrame(id);
    } else {
      let id = 0;
      const next = () => {
        if (stopped) {
          return;
        }
        id = requestAnimationFrame(next);
        handler.current({ mediaTime: null, presented: true });
      };
      id = requestAnimationFrame(next);
      cancel = () => cancelAnimationFrame(id);
    }
    const onTimeUpdate = () => handler.current({ mediaTime: null, presented: false });
    media.addEventListener('timeupdate', onTimeUpdate);

    return () => {
      stopped = true;
      cancel();
      media.removeEventListener('timeupdate', onTimeUpdate);
    };
  }, [media]);
}
