import { useEffect, useRef } from 'react';

type Actions = Partial<{
  play: () => void;
  pause: () => void;
  seekBackward: () => void;
  seekForward: () => void;
  seekTo: (seconds: number) => void;
}>;

// useMediaSession は、OS のメディア操作 (ロック画面やメディアキー) に、番組名と操作を知らせる (設計書 7.5)。
export function useMediaSession(
  metadata: { title: string; artist: string; artwork: string | null },
  actions: Actions,
): void {
  const actionsRef = useRef(actions);
  actionsRef.current = actions;
  const { title, artist, artwork } = metadata;

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
