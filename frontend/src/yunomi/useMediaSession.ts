import { useEffect, useRef } from 'react';

import { splitProgramMarks } from '../futon/programMarks';
import { runCommand, type PlayerCommandId, type PlayerCommands } from './commands';

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

// mediaSessionCommands は、OS の操作の名前と、それで呼ぶ操作。前と次のボタンは、ライブで前と次の局へ移る (#493)。
const mediaSessionCommands: readonly (readonly [
  MediaSessionAction,
  PlayerCommandId,
  (commands: PlayerCommands, details: MediaSessionActionDetails) => void,
])[] = [
  ['play', 'play', (commands) => runCommand(commands, 'play')],
  ['pause', 'pause', (commands) => runCommand(commands, 'pause')],
  ['seekbackward', 'seekBackward', (commands) => runCommand(commands, 'seekBackward')],
  ['seekforward', 'seekForward', (commands) => runCommand(commands, 'seekForward')],
  [
    'seekto',
    'seekTo',
    (commands, details) => {
      if (details.seekTime !== undefined) {
        runCommand(commands, 'seekTo', details.seekTime);
      }
    },
  ],
  ['previoustrack', 'previousChannel', (commands) => runCommand(commands, 'previousChannel')],
  ['nexttrack', 'nextChannel', (commands) => runCommand(commands, 'nextChannel')],
];

// useMediaSession は、OS のメディア操作 (ロック画面やメディアキー) に、番組名、再生の状態と位置、操作を知らせる (設計書 7.5)。
// 番組名の囲み文字は、OS の表示で潰れないよう外す。autoPictureInPicture を入れると、タブを切り替えたときに、
// ブラウザが pictureInPicture の操作を呼び、視聴画面を別のウィンドウに出す (#492)。
export function useMediaSession(
  metadata: { title: string; artist: string; artwork: string | null },
  commands: PlayerCommands,
  playback: Playback,
  autoPictureInPicture = false,
): void {
  const commandsRef = useRef(commands);
  commandsRef.current = commands;
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

  // 使える操作だけを OS に出す。使える操作が変わったら (局の一覧を読んだ、データ放送を出したなど)、出し直す。
  const available = mediaSessionCommands
    .filter(([, id]) => commands[id] !== undefined)
    .map(([action]) => action)
    .join(',');
  useEffect(() => {
    if (!('mediaSession' in navigator)) {
      return;
    }
    const actions = mediaSessionCommands.filter(([action]) =>
      available.split(',').includes(action),
    );
    for (const [action, , run] of actions) {
      navigator.mediaSession.setActionHandler(action, (details) =>
        run(commandsRef.current, details),
      );
    }

    return () => {
      for (const [action] of actions) {
        navigator.mediaSession.setActionHandler(action, null);
      }
    };
  }, [available]);
  // 自動で別のウィンドウに出す設定は、ほかのタブで変えても追うので、変わったら登録し直す。
  // 知らない操作を渡すと例外を投げるブラウザがあるので、受け付けなければ何もしない。
  const pictureInPicture = autoPictureInPicture && commands.pictureInPicture !== undefined;
  useEffect(() => {
    if (!pictureInPicture || !('mediaSession' in navigator)) {
      return;
    }
    const action = 'enterpictureinpicture' as MediaSessionAction;
    try {
      navigator.mediaSession.setActionHandler(action, () =>
        runCommand(commandsRef.current, 'pictureInPicture'),
      );
    } catch {
      return;
    }

    return () => navigator.mediaSession.setActionHandler(action, null);
  }, [pictureInPicture]);
}
