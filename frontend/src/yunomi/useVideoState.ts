import { useEffect, useState } from 'react';

import { browserStorage, loadVolume, saveVolume, type Volume } from './volume';
import {
  applyVolume,
  currentVolume,
  lowerBoost,
  releaseBoost,
  resumePendingBoost,
} from './volumeBoost';

export type VideoState = Volume & {
  paused: boolean;
  togglePlay: () => void;
  setVolume: (volume: number) => void;
  toggleMute: () => void;
};

// useVideoState は、video の再生と音量の状態を追い、操作バーから変える。
// video の要素は、画質を変えたときや、別のウィンドウに移したときに作り直されるので、要素が変わるたびに付け直す。
// 音量は端末ごとに覚えておき、新しい要素にも当てる。
export function useVideoState(video: HTMLVideoElement | null): VideoState {
  const [paused, setPaused] = useState(true);
  const [volume, setVolumeState] = useState(() => loadVolume(browserStorage()));

  useEffect(() => {
    if (video === null) {
      return;
    }
    const stored = loadVolume(browserStorage());
    applyVolume(video, stored.volume);
    video.muted = stored.muted;
    const sync = () => {
      setPaused(video.paused);
      setVolumeState({ volume: currentVolume(video), muted: video.muted });
    };
    // キーボードの m や、OS のメディア操作で変えたときも、ここで拾って覚える。
    // ほかの経路で 1 より下げたときは、100% を超えて上げていた分を戻す。
    const onVolume = () => {
      lowerBoost(video);
      const next = { volume: currentVolume(video), muted: video.muted };
      setVolumeState(next);
      saveVolume(browserStorage(), next);
    };
    // 100% を超える音量を覚えていても、ユーザーが操作するまでは WebAudio を動かせないので、最初の操作で上げる (#453)。
    const onActivate = () => resumePendingBoost(video);
    sync();
    video.addEventListener('play', sync);
    video.addEventListener('pause', sync);
    video.addEventListener('volumechange', onVolume);
    window.addEventListener('pointerdown', onActivate, { once: true });
    window.addEventListener('keydown', onActivate, { once: true });

    return () => {
      window.removeEventListener('pointerdown', onActivate);
      window.removeEventListener('keydown', onActivate);
      video.removeEventListener('play', sync);
      video.removeEventListener('pause', sync);
      video.removeEventListener('volumechange', onVolume);
      // 画質を変えたときなどに作り直された video は、AudioContext から外す。
      releaseBoost(video);
    };
  }, [video]);

  return {
    paused,
    ...volume,
    togglePlay: () => {
      if (video === null) {
        return;
      }
      if (video.paused) {
        void video.play().catch(() => {});
      } else {
        video.pause();
      }
    },
    setVolume: (next) => {
      if (video === null) {
        return;
      }
      applyVolume(video, next);
      // 音量を上げたら、ミュートを解除する。
      if (next > 0) {
        video.muted = false;
      }
      // 100% を超えたところで変えても、video.volume は 1 のままで volumechange が届かないので、ここで覚える。
      const state = { volume: currentVolume(video), muted: video.muted };
      setVolumeState(state);
      saveVolume(browserStorage(), state);
    },
    toggleMute: () => {
      if (video !== null) {
        video.muted = !video.muted;
      }
    },
  };
}
