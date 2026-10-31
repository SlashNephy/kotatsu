import { useEffect, useRef, useState } from 'react';

import { toHalfWidth } from '../futon';
import { splitProgramMarks } from '../futon/programMarks';

// プレイヤーの状態とイベント (設計書 7.5、#811)。画面は、今の状態を PlayerSnapshot にまとめて、1 か所で作る。
// タブの題名、OS のメディア操作、ほかの端末に送る状態は、どれもこれを読む。状態の変わり目に何かするもの
// (ほかの端末に知らせる、番組が変わったら何かする) は、usePlayerEvents が配るイベントを購読する。

// PlayerProgram は、見ている番組。録画では、録画そのもの。
export type PlayerProgram = { id: string; name: string };

// PlayerSnapshot は、プレイヤーの今の状態。
export type PlayerSnapshot = {
  kind: 'LIVE' | 'RECORDING' | 'TIMESHIFT';
  // target は、見ているものの ID。ライブとタイムシフトではサービス、録画では録画。
  target: string;
  serviceName: string;
  // program は、見ている番組。ライブで番組表に番組がないときと、録画を読み込む前は null。
  program: PlayerProgram | null;
  // artwork は、OS のメディアの表示に出す画像の URL。
  artwork: string | null;
  paused: boolean;
  // position は再生位置、duration は長さ (秒)。シークできないライブでは、どちらも null。
  position: number | null;
  duration: number | null;
  playbackRate: number;
  volume: number;
  muted: boolean;
  audios: ReadonlyArray<{ id: string; label: string }>;
  audio: string | null;
  // dataBroadcast は、データ放送を出しているかどうか。keyGroups は、その文書が使うリモコンのボタンの組。
  dataBroadcast: boolean;
  keyGroups: ReadonlyArray<string>;
};

// PlayerEventMap は、イベントの種類と中身。再生位置と長さは、描くたびに変わるので、イベントにしない。
// 局を変えたときは、画面ごと作り直すので、イベントは出さない。見始めたときの処理は、購読する側が最初に描いたときに行う。
export type PlayerEventMap = {
  // programChanged は、見ている番組が変わったとき。ライブで、番組が終わって次の番組になったときなど。
  programChanged: { previous: PlayerProgram | null; current: PlayerProgram | null };
  pausedChanged: { paused: boolean };
  volumeChanged: { volume: number; muted: boolean };
  // audioChanged は、選べる音声か、選んでいる音声が変わったとき。
  audioChanged: { audios: PlayerSnapshot['audios']; audio: string | null };
  // dataBroadcastChanged は、データ放送を出し入れしたときか、文書の使うボタンの組が変わったとき。
  dataBroadcastChanged: { shown: boolean; keyGroups: ReadonlyArray<string> };
};

export type PlayerEventType = keyof PlayerEventMap;

export type PlayerEvent = {
  [Type in PlayerEventType]: { type: Type; payload: PlayerEventMap[Type] };
}[PlayerEventType];

export type PlayerEventListener<Type extends PlayerEventType> = (
  payload: PlayerEventMap[Type],
) => void;

// PlayerEvents は、イベントの購読の口。subscribe は、購読をやめる関数を返す。
export type PlayerEvents = {
  subscribe: <Type extends PlayerEventType>(
    type: Type,
    listener: PlayerEventListener<Type>,
  ) => () => void;
};

function sameList<T>(a: ReadonlyArray<T>, b: ReadonlyArray<T>, same: (x: T, y: T) => boolean) {
  return a.length === b.length && a.every((item, i) => same(item, b[i]!));
}

// playerEvents は、状態が previous から current に変わったときに出すイベントを返す。
export function playerEvents(previous: PlayerSnapshot, current: PlayerSnapshot): PlayerEvent[] {
  const events: PlayerEvent[] = [];
  if (
    previous.program?.id !== current.program?.id ||
    previous.program?.name !== current.program?.name
  ) {
    events.push({
      type: 'programChanged',
      payload: { previous: previous.program, current: current.program },
    });
  }
  if (previous.paused !== current.paused) {
    events.push({ type: 'pausedChanged', payload: { paused: current.paused } });
  }
  if (previous.volume !== current.volume || previous.muted !== current.muted) {
    events.push({
      type: 'volumeChanged',
      payload: { volume: current.volume, muted: current.muted },
    });
  }
  if (
    previous.audio !== current.audio ||
    !sameList(previous.audios, current.audios, (a, b) => a.id === b.id && a.label === b.label)
  ) {
    events.push({
      type: 'audioChanged',
      payload: { audios: current.audios, audio: current.audio },
    });
  }
  if (
    previous.dataBroadcast !== current.dataBroadcast ||
    !sameList(previous.keyGroups, current.keyGroups, (a, b) => a === b)
  ) {
    events.push({
      type: 'dataBroadcastChanged',
      payload: { shown: current.dataBroadcast, keyGroups: current.keyGroups },
    });
  }

  return events;
}

// usePlayerEvents は、描くたびに状態を前の状態と比べ、変わったところのイベントを配る。返す口は、画面を離れるまで変わらない。
export function usePlayerEvents(state: PlayerSnapshot): PlayerEvents {
  const [listeners] = useState(() => new Map<PlayerEventType, Set<(payload: never) => void>>());
  const [events] = useState<PlayerEvents>(() => ({
    subscribe: (type, listener) => {
      const set = listeners.get(type) ?? new Set();
      listeners.set(type, set);
      set.add(listener);

      return () => {
        set.delete(listener);
      };
    },
  }));
  const previous = useRef(state);

  useEffect(() => {
    const changes = playerEvents(previous.current, state);
    previous.current = state;
    for (const { type, payload } of changes) {
      for (const listener of listeners.get(type) ?? []) {
        (listener as (payload: PlayerEvent['payload']) => void)(payload);
      }
    }
  });

  return events;
}

// usePlayerEvent は、1 つか複数の種類のイベントを購読する。listener は描くたびに変わってよい。
export function usePlayerEvent<Type extends PlayerEventType>(
  events: PlayerEvents,
  types: Type | readonly Type[],
  listener: PlayerEventListener<Type>,
): void {
  const listenerRef = useRef(listener);
  listenerRef.current = listener;
  // 描くたびに配列を作り直しても、購読し直さないよう、種類の並びを文字列にして比べる。
  const key = (typeof types === 'string' ? [types] : types).join(',');

  useEffect(() => {
    const unsubscribes = (key.split(',') as Type[]).map((type) =>
      events.subscribe(type, (payload) => listenerRef.current(payload)),
    );

    return () => {
      for (const unsubscribe of unsubscribes) {
        unsubscribe();
      }
    };
  }, [events, key]);
}

// playerTitle は、タブの題名や OS のメディアの表示に出す番組名。印を描けないので囲み文字を外し、全角の英数字は半角にする。
export function playerTitle(state: PlayerSnapshot): string | null {
  return state.program === null ? null : toHalfWidth(splitProgramMarks(state.program.name).name);
}

// documentTitle は、タブの題名に並べるもの。ライブでは、局の名前も添える。
export function documentTitle(state: PlayerSnapshot): (string | null)[] {
  return [playerTitle(state), state.kind === 'LIVE' ? toHalfWidth(state.serviceName) : null];
}
