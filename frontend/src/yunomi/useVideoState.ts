import { useEffect, useState } from 'react';

import { browserStorage, loadVolume, saveVolume, type Volume } from './volume';
import {
  applyVolume,
  currentVolume,
  lowerBoost,
  releaseBoost,
  resumePendingBoost,
} from './volumeBoost';
import type { PlayerMedia } from './playerMedia';

export type VideoState = Volume & {
  paused: boolean;
  togglePlay: () => void;
  setVolume: (volume: number) => void;
  toggleMute: () => void;
};

// useVideoState は、media の再生と音量の状態を追い、操作バーから変える。
// media は、画質を変えたときに作り直されるので、変わるたびに付け直す。
// 音量は端末ごとに保存しておき、新しい要素にも当てる。
export function useVideoState(media: PlayerMedia | null): VideoState {
  const [paused, setPaused] = useState(true);
  const [volume, setVolumeState] = useState(() => loadVolume(browserStorage()));

  useEffect(() => {
    if (media === null) {
      return;
    }
    const stored = loadVolume(browserStorage());
    applyVolume(media, stored.volume);
    media.muted = stored.muted;
    const sync = () => {
      setPaused(media.paused);
      setVolumeState({ volume: currentVolume(media), muted: media.muted });
    };
    // キーボードの m や、OS のメディア操作で変えたときも、ここで拾って保存する。
    // ほかの経路で 1 より下げたときは、100% を超えて上げていた分を戻す。
    const onVolume = () => {
      lowerBoost(media);
      const next = { volume: currentVolume(media), muted: media.muted };
      setVolumeState(next);
      saveVolume(browserStorage(), next);
    };
    // 100% を超える音量を保存していても、ユーザーが操作するまでは WebAudio を動かせないので、最初の操作で上げる (#453)。
    const onActivate = () => resumePendingBoost(media);
    sync();
    media.addEventListener('play', sync);
    media.addEventListener('pause', sync);
    media.addEventListener('volumechange', onVolume);
    window.addEventListener('pointerdown', onActivate, { once: true });
    window.addEventListener('keydown', onActivate, { once: true });

    return () => {
      window.removeEventListener('pointerdown', onActivate);
      window.removeEventListener('keydown', onActivate);
      media.removeEventListener('play', sync);
      media.removeEventListener('pause', sync);
      media.removeEventListener('volumechange', onVolume);
      // 画質を変えたときなどに作り直された media は、AudioContext から外す。
      releaseBoost(media);
    };
  }, [media]);

  return {
    paused,
    ...volume,
    togglePlay: () => {
      if (media === null) {
        return;
      }
      if (media.paused) {
        void media.play().catch(() => {});
      } else {
        media.pause();
      }
    },
    setVolume: (next) => {
      if (media === null) {
        return;
      }
      applyVolume(media, next);
      // 音量を上げたら、ミュートを解除する。
      if (next > 0) {
        media.muted = false;
      }
      // 100% を超えたところで変えても、media.volume は 1 のままで volumechange が届かないので、ここで保存する。
      const state = { volume: currentVolume(media), muted: media.muted };
      setVolumeState(state);
      saveVolume(browserStorage(), state);
    },
    toggleMute: () => {
      if (media !== null) {
        media.muted = !media.muted;
      }
    },
  };
}
