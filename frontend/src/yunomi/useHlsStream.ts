import { useEffect, useRef, useState } from 'react';
import type { PlayerMedia } from './playerMedia';

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
  // seekTo は、読み直さずに、video の時刻を移したときの位置を覚える。再生を始める前のシークも、位置と、次に読み始める位置に入れる。
  seekTo: (seconds: number) => void;
  // currentPosition は、video の時刻から求め直した今の再生位置。
  currentPosition: () => number;
  // framePosition は、映ったコマの時刻 t に当たる再生位置。再生を始めるまでは、位置を信じられないので null。
  framePosition: (t: number) => number | null;
};

// useHlsStream は、仮想 HLS の再生位置を追う。initial は、最初に読み始める位置。enabled が false の間は、位置を動かさない。
// 読み始める位置へ移るまで、video の時刻は 0 のことがあるので、再生を始めてから位置を動かす。
export function useHlsStream(media: PlayerMedia | null, enabled: boolean, initial = 0): HlsStream {
  const [load, setLoad] = useState({ start: initial, loads: 0 });
  const [position, setPosition] = useState(initial);
  const positionRef = useRef(initial);
  const playing = useRef(false);
  useEffect(() => {
    if (media === null || !enabled) {
      return;
    }
    playing.current = false;
    const onPlaying = () => {
      playing.current = true;
    };
    const update = () => {
      if (playing.current) {
        positionRef.current = media.currentTime;
        setPosition(media.currentTime);
      }
    };
    media.addEventListener('playing', onPlaying);
    media.addEventListener('timeupdate', update);

    return () => {
      media.removeEventListener('playing', onPlaying);
      media.removeEventListener('timeupdate', update);
    };
  }, [media, enabled, load]);

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
    seekTo: (seconds) => {
      positionRef.current = seconds;
      setPosition(seconds);
      if (!playing.current) {
        setLoad((prev) => ({ start: seconds, loads: prev.loads }));
      }
    },
    currentPosition: () =>
      media === null || !playing.current ? positionRef.current : media.currentTime,
    framePosition: (t) => (playing.current ? t : null),
  };
}
