import { useEffect, useRef } from 'react';

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
// 取りこぼしを防ぐため、timeupdate でも呼ぶ。video を作り直したら (画質の切り替え、別のウィンドウ、つなぎ直し)、付け直す。
export function useVideoFrames(
  video: HTMLVideoElement | null,
  onFrame: (frame: VideoFrame) => void,
): void {
  const handler = useRef(onFrame);
  handler.current = onFrame;
  useEffect(() => {
    if (video === null) {
      return;
    }
    let stopped = false;
    let cancel: () => void;
    if (typeof video.requestVideoFrameCallback === 'function') {
      let id = 0;
      const next = (_now: number, metadata: VideoFrameCallbackMetadata) => {
        if (stopped) {
          return;
        }
        id = video.requestVideoFrameCallback(next);
        handler.current({ mediaTime: metadata.mediaTime, presented: true });
      };
      id = video.requestVideoFrameCallback(next);
      cancel = () => video.cancelVideoFrameCallback(id);
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
    video.addEventListener('timeupdate', onTimeUpdate);

    return () => {
      stopped = true;
      cancel();
      video.removeEventListener('timeupdate', onTimeUpdate);
    };
  }, [video]);
}
