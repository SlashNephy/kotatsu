import { useEffect, useState } from 'react';

import type { PlayerMedia } from './playerMedia';

// waitingDelayMs は、再生の途中で止まって待つ (waiting) ときに、読み込み中の表示を出すまでのミリ秒。
// ライブでは短い待ちがよく起きるので、すぐに出すとちらつく。
export const waitingDelayMs = 1000;

// useVideoLoading は、映像を読み込んでいて、まだ映せていないかを返す (設計書 7.5)。
// 読み込みを始めてから (loadstart、録画のシークや画質の切り替えを含む) 映像が動き出す (playing) まで、
// 止めたままシークしたときは、シークし終える (seeked) まで、読み込み中とする。
// 再生の途中の待ち (waiting) は、waitingDelayMs 続いたときだけ読み込み中とする。
export function useVideoLoading(media: PlayerMedia | null): boolean {
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (media === null) {
      return;
    }
    let timer: ReturnType<typeof setTimeout> | undefined;
    const clearWaiting = () => {
      clearTimeout(timer);
      timer = undefined;
    };
    const start = () => {
      clearWaiting();
      setLoading(true);
    };
    const done = () => {
      clearWaiting();
      setLoading(false);
    };
    const onWaiting = () => {
      clearWaiting();
      timer = setTimeout(() => setLoading(true), waitingDelayMs);
    };
    const onSeeked = () => {
      if (media.paused) {
        done();
      }
    };
    const onCanPlay = () => {
      // 止めたまま読み込み終えたら (自動再生を拒否されたときなど)、待っていることにはしない。
      if (media.paused) {
        done();
      }
    };
    // HAVE_FUTURE_DATA (3) 以上なら、もう映せる。
    setLoading(media.readyState < 3);
    media.addEventListener('loadstart', start);
    media.addEventListener('seeking', start);
    media.addEventListener('waiting', onWaiting);
    media.addEventListener('playing', done);
    media.addEventListener('seeked', onSeeked);
    // 止めたら、待っていることにはしない。自動再生を拒否されて止まったときは、再生ボタンを出す。
    media.addEventListener('pause', done);
    media.addEventListener('canplay', onCanPlay);
    // 読み込みに失敗したら、エラーの通知に任せて、出し続けない。
    media.addEventListener('error', done);

    return () => {
      clearWaiting();
      media.removeEventListener('loadstart', start);
      media.removeEventListener('seeking', start);
      media.removeEventListener('waiting', onWaiting);
      media.removeEventListener('playing', done);
      media.removeEventListener('seeked', onSeeked);
      media.removeEventListener('pause', done);
      media.removeEventListener('canplay', onCanPlay);
      media.removeEventListener('error', done);
    };
  }, [media]);

  return loading;
}
