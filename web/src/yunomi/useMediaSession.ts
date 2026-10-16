import { useEffect, useRef } from 'react';

import { splitProgramMarks } from '../futon/programMarks';

type Actions = Partial<{
  play: () => void;
  pause: () => void;
  seekBackward: () => void;
  seekForward: () => void;
  seekTo: (seconds: number) => void;
  // previousTrack と nextTrack は、ライブで、前と次の局へ移る (#493)。
  previousTrack: () => void;
  nextTrack: () => void;
}>;

// Playback は、OS に知らせる再生の状態。position は、録画のようにシークできるときの再生位置 (秒)。ライブでは null。
export type Playback = {
  playing: boolean;
  position: { duration: number; position: number; playbackRate: number } | null;
};

// positionState は、OS に渡す再生位置を返す。渡せない値 (長さが分からない、位置が範囲の外) は直すか、null にする。
// 再生位置は秒の単位に丸める。OS は再生速度から位置を進めるので、細かく知らせ直さなくてよい。
export function positionState(position: Playback['position']): MediaPositionState | null {
  if (position === null || !Number.isFinite(position.duration) || position.duration <= 0) {
    return null;
  }
  const duration = Math.floor(position.duration);

  return {
    duration,
    position: Number.isFinite(position.position)
      ? Math.min(Math.max(Math.floor(position.position), 0), duration)
      : 0,
    playbackRate: position.playbackRate > 0 ? position.playbackRate : 1,
  };
}

// useMediaSession は、OS のメディア操作 (ロック画面やメディアキー) に、番組名、再生の状態と位置、操作を知らせる (設計書 7.5)。
// 番組名の囲み文字は、OS の表示で潰れないよう外す。
export function useMediaSession(
  metadata: { title: string; artist: string; artwork: string | null },
  actions: Actions,
  playback: Playback,
): void {
  const actionsRef = useRef(actions);
  actionsRef.current = actions;
  const { artist, artwork } = metadata;
  const title = splitProgramMarks(metadata.title).name;
  const state = positionState(playback.position);
  const { playing } = playback;
  const duration = state?.duration;
  const position = state?.position;
  const playbackRate = state?.playbackRate;

  useEffect(() => {
    if (!('mediaSession' in navigator)) {
      return;
    }
    navigator.mediaSession.playbackState = playing ? 'playing' : 'paused';
  }, [playing]);

  // 止めたり再開したりするたびに none を挟むと、OS のメディアの操作が出たり消えたりするので、離れたときだけ none にする。
  useEffect(() => {
    if (!('mediaSession' in navigator)) {
      return;
    }

    return () => {
      navigator.mediaSession.playbackState = 'none';
    };
  }, []);

  useEffect(() => {
    if (!('mediaSession' in navigator) || !('setPositionState' in navigator.mediaSession)) {
      return;
    }
    navigator.mediaSession.setPositionState(
      duration === undefined || position === undefined || playbackRate === undefined
        ? undefined
        : { duration, position, playbackRate },
    );
  }, [duration, position, playbackRate]);

  useEffect(() => {
    if (!('mediaSession' in navigator) || !('setPositionState' in navigator.mediaSession)) {
      return;
    }

    return () => navigator.mediaSession.setPositionState(undefined);
  }, []);

  useEffect(() => {
    if (!('mediaSession' in navigator)) {
      return;
    }
    navigator.mediaSession.metadata = new MediaMetadata({
      title,
      artist,
      // 録画の代表画像は JPEG、サービスのロゴは PNG なので、種類は書かずにブラウザに見分けさせる。
      artwork: artwork === null ? [] : [{ src: artwork }],
    });

    return () => {
      navigator.mediaSession.metadata = null;
    };
  }, [title, artist, artwork]);

  useEffect(() => {
    if (!('mediaSession' in navigator)) {
      return;
    }
    // OS の操作の名前と、割り当てる操作の名前の組。割り当てていない操作は、OS に出さない。
    const handlers: [MediaSessionAction, keyof Actions, MediaSessionActionHandler][] = [
      ['play', 'play', () => actionsRef.current.play?.()],
      ['pause', 'pause', () => actionsRef.current.pause?.()],
      ['seekbackward', 'seekBackward', () => actionsRef.current.seekBackward?.()],
      ['seekforward', 'seekForward', () => actionsRef.current.seekForward?.()],
      ['previoustrack', 'previousTrack', () => actionsRef.current.previousTrack?.()],
      ['nexttrack', 'nextTrack', () => actionsRef.current.nextTrack?.()],
      [
        'seekto',
        'seekTo',
        (details) => {
          if (details.seekTime !== undefined) {
            actionsRef.current.seekTo?.(details.seekTime);
          }
        },
      ],
    ];
    for (const [action, name, handler] of handlers) {
      if (actionsRef.current[name] !== undefined) {
        navigator.mediaSession.setActionHandler(action, handler);
      }
    }

    return () => {
      for (const [action] of handlers) {
        navigator.mediaSession.setActionHandler(action, null);
      }
    };
  }, []);
}
