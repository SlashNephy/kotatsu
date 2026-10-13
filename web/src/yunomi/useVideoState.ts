import { useEffect, useState } from 'react';

import { browserStorage, loadVolume, saveVolume, type Volume } from './volume';

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
    video.volume = stored.volume;
    video.muted = stored.muted;
    const sync = () => {
      setPaused(video.paused);
      setVolumeState({ volume: video.volume, muted: video.muted });
    };
    // キーボードの m や、OS のメディア操作で変えたときも、ここで拾って覚える。
    const onVolume = () => {
      const next = { volume: video.volume, muted: video.muted };
      setVolumeState(next);
      saveVolume(browserStorage(), next);
    };
    sync();
    video.addEventListener('play', sync);
    video.addEventListener('pause', sync);
    video.addEventListener('volumechange', onVolume);

    return () => {
      video.removeEventListener('play', sync);
      video.removeEventListener('pause', sync);
      video.removeEventListener('volumechange', onVolume);
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
      video.volume = next;
      // 音量を上げたら、消音を解く。
      if (next > 0) {
        video.muted = false;
      }
    },
    toggleMute: () => {
      if (video !== null) {
        video.muted = !video.muted;
      }
    },
  };
}
