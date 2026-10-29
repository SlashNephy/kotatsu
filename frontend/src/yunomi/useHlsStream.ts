import { useEffect, useRef, useState } from 'react';

// HlsStream は、kyusu の仮想 HLS (録画やタイムシフトのバッファ) の、読み始める位置と再生位置の追い方 (設計書 7.3)。
export type HlsStream = {
  // start は、読み始める位置 (秒)。HlsPlayer の startPosition に渡す。
  start: number;
  // loads は、読み直した回数。URL の fragment に入れて、hls.js を作り直させる。
  loads: number;
  // position は、再生位置 (秒)。仮想 HLS は起点からの時刻で並ぶので、video の時刻をそのまま使う。
  position: number;
  // positionRef は、描画を待たずに読む再生位置。
  positionRef: { current: number };
  // restartAt は、指定の位置から読み直す。
  restartAt: (seconds: number) => void;
  // currentPosition は、video の時刻から求め直した今の再生位置。
  currentPosition: () => number;
};

// useHlsStream は、仮想 HLS の再生位置を追う。enabled が false の間は、位置を動かさない。
// 読み始める位置へ移るまで、video の時刻は 0 のことがあるので、再生を始めてから位置を動かす。
export function useHlsStream(video: HTMLVideoElement | null, enabled: boolean): HlsStream {
  const [load, setLoad] = useState({ start: 0, loads: 0 });
  const [position, setPosition] = useState(0);
  const positionRef = useRef(0);
  const playing = useRef(false);
  useEffect(() => {
    if (video === null || !enabled) {
      return;
    }
    playing.current = false;
    const onPlaying = () => {
      playing.current = true;
    };
    const update = () => {
      if (playing.current) {
        positionRef.current = video.currentTime;
        setPosition(video.currentTime);
      }
    };
    video.addEventListener('playing', onPlaying);
    video.addEventListener('timeupdate', update);

    return () => {
      video.removeEventListener('playing', onPlaying);
      video.removeEventListener('timeupdate', update);
    };
  }, [video, enabled, load]);

  return {
    start: load.start,
    loads: load.loads,
    position,
    positionRef,
    restartAt: (seconds) => {
      positionRef.current = seconds;
      setPosition(seconds);
      setLoad((prev) => ({ start: seconds, loads: prev.loads + 1 }));
    },
    currentPosition: () =>
      video === null || !playing.current ? positionRef.current : video.currentTime,
  };
}
