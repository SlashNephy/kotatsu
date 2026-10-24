import { useEffect, useState } from 'react';

// waitingDelayMs は、再生の途中で止まって待つ (waiting) ときに、読み込み中の表示を出すまでのミリ秒。
// ライブでは短い待ちがよく起きるので、すぐに出すとちらつく。
export const waitingDelayMs = 1000;

// useVideoLoading は、映像を読み込んでいて、まだ映せていないかを返す (設計書 7.5)。
// 読み込みを始めてから (loadstart、録画のシークや画質の切り替えを含む) 映像が動き出す (playing) まで、
// 止めたままシークしたときは、シークし終える (seeked) まで、読み込み中とする。
// 再生の途中の待ち (waiting) は、waitingDelayMs 続いたときだけ読み込み中とする。
export function useVideoLoading(video: HTMLVideoElement | null): boolean {
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (video === null) {
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
      if (video.paused) {
        done();
      }
    };
    const onCanPlay = () => {
      // 止めたまま読み込み終えたら (自動再生を断られたときなど)、待っていることにはしない。
      if (video.paused) {
        done();
      }
    };
    // HAVE_FUTURE_DATA (3) 以上なら、もう映せる。
    setLoading(video.readyState < 3);
    video.addEventListener('loadstart', start);
    video.addEventListener('seeking', start);
    video.addEventListener('waiting', onWaiting);
    video.addEventListener('playing', done);
    video.addEventListener('seeked', onSeeked);
    // 止めたら、待っていることにはしない。自動再生を断られて止まったときは、再生ボタンを出す。
    video.addEventListener('pause', done);
    video.addEventListener('canplay', onCanPlay);
    // 読み込みに失敗したら、エラーの知らせに任せて、出し続けない。
    video.addEventListener('error', done);

    return () => {
      clearWaiting();
      video.removeEventListener('loadstart', start);
      video.removeEventListener('seeking', start);
      video.removeEventListener('waiting', onWaiting);
      video.removeEventListener('playing', done);
      video.removeEventListener('seeked', onSeeked);
      video.removeEventListener('pause', done);
      video.removeEventListener('canplay', onCanPlay);
      video.removeEventListener('error', done);
    };
  }, [video]);

  return loading;
}
