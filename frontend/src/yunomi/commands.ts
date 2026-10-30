import type { RemoteKey } from './useDataBroadcast';

// プレイヤーの操作の登録簿 (設計書 7.5、#809)。キー、ショートカットの一覧、OS のメディア操作、ほかの端末からの操作、操作バーは、
// どれも、ここの ID で操作を呼ぶ。画面は、今使える操作だけを PlayerCommands に入れて渡す。

// PlayerCommandArgs は、操作ごとの引数。
export type PlayerCommandArgs = {
  play: [];
  pause: [];
  togglePlay: [];
  seekBackward: [];
  seekForward: [];
  seekBackwardLong: [];
  seekForwardLong: [];
  // seekTo は、録画の始まり (ライブのバッファでは、バッファの始まり) からの秒の位置へ移る。
  seekTo: [seconds: number];
  toggleCaptions: [];
  toggleJikkyo: [];
  toggleMute: [];
  setMuted: [muted: boolean];
  volumeUp: [];
  volumeDown: [];
  // setVolume は、音量を 0 から maxVolume の値にする。
  setVolume: [volume: number];
  selectAudio: [id: string];
  toggleFullscreen: [];
  toggleDataBroadcast: [];
  // pressKey は、データ放送のリモコンのボタンを押す。
  pressKey: [key: RemoteKey];
  previousChannel: [];
  nextChannel: [];
  // lastChannel は、直前に見ていた局へ戻る (#816)。
  lastChannel: [];
  pictureInPicture: [];
  capture: [];
  restart: [];
  toggleStats: [];
  showShortcuts: [];
  // stop は、再生をやめ、プレイヤーを閉じる。
  stop: [];
};

export type PlayerCommandId = keyof PlayerCommandArgs;

// PlayerCommands は、画面が今使える操作と、その処理。使えない操作は入れない。
export type PlayerCommands = {
  [Id in PlayerCommandId]?: (...args: PlayerCommandArgs[Id]) => void;
};

// SimpleCommandId は、引数のない操作。キーに割り当てられるのは、これだけ。
export type SimpleCommandId = {
  [Id in PlayerCommandId]: PlayerCommandArgs[Id] extends [] ? Id : never;
}[PlayerCommandId];

type PlayerCommandDefinition = {
  // repeatable は、キーを押し続けたときに、繰り返してよいかどうか。切り替える操作は、押すたびに 1 回だけにする。
  repeatable: boolean;
};

// playerCommands は、操作の定義。ショートカットの一覧も、この順に並べる。表示名は、翻訳の player.commands.<ID>。
export const playerCommands: Readonly<Record<PlayerCommandId, PlayerCommandDefinition>> = {
  togglePlay: { repeatable: false },
  play: { repeatable: false },
  pause: { repeatable: false },
  seekBackward: { repeatable: true },
  seekForward: { repeatable: true },
  seekBackwardLong: { repeatable: true },
  seekForwardLong: { repeatable: true },
  seekTo: { repeatable: false },
  toggleCaptions: { repeatable: false },
  toggleJikkyo: { repeatable: false },
  toggleMute: { repeatable: false },
  setMuted: { repeatable: false },
  toggleFullscreen: { repeatable: false },
  volumeUp: { repeatable: true },
  volumeDown: { repeatable: true },
  setVolume: { repeatable: false },
  selectAudio: { repeatable: false },
  previousChannel: { repeatable: false },
  nextChannel: { repeatable: false },
  lastChannel: { repeatable: false },
  pictureInPicture: { repeatable: false },
  capture: { repeatable: false },
  toggleDataBroadcast: { repeatable: false },
  pressKey: { repeatable: false },
  restart: { repeatable: false },
  toggleStats: { repeatable: false },
  showShortcuts: { repeatable: false },
  stop: { repeatable: false },
};

export const playerCommandIds = Object.keys(playerCommands) as PlayerCommandId[];

// runCommand は、操作を呼ぶ。今使えない操作なら、何もせず false を返す。
export function runCommand<Id extends PlayerCommandId>(
  commands: PlayerCommands,
  id: Id,
  ...args: PlayerCommandArgs[Id]
): boolean {
  const command = commands[id] as ((...args: PlayerCommandArgs[Id]) => void) | undefined;
  if (command === undefined) {
    return false;
  }
  command(...args);

  return true;
}
