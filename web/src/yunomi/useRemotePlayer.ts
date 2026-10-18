import { useEffect, useRef } from 'react';
import { graphql, useRelayEnvironment } from 'react-relay';
import { requestSubscription, type Environment } from 'relay-runtime';

import { mutate } from '../settings/mutate';
import type { useRemotePlayerCommandsSubscription } from './__generated__/useRemotePlayerCommandsSubscription.graphql';
import type { useRemotePlayerRegisterMutation } from './__generated__/useRemotePlayerRegisterMutation.graphql';
import type { useRemotePlayerUnregisterMutation } from './__generated__/useRemotePlayerUnregisterMutation.graphql';
import type { useRemotePlayerUpdateMutation } from './__generated__/useRemotePlayerUpdateMutation.graphql';
import { RemoteKey } from './useDataBroadcast';

// heartbeatMs は、状態を送る間隔。サーバーは、3 回分送られてこなければ、閉じたとみなす (設計書 7.7)。
export const heartbeatMs = 10_000;
// announceDelayMs は、状態が変わってから、ほかの端末に知らせるまでの間。続けて変わったときに、まとめて送る。
export const announceDelayMs = 300;

// RemoteState は、ほかの端末に見せる、プレイヤーの状態 (GraphQL の PlayerStateInput)。
export type RemoteState = {
  kind: 'LIVE' | 'RECORDING';
  target: string;
  title: string | null;
  paused: boolean;
  position: number | null;
  duration: number | null;
  volume: number;
  muted: boolean;
  audios: ReadonlyArray<{ id: string; label: string }>;
  audio: string | null;
  dataBroadcast: boolean;
  keyGroups: ReadonlyArray<string>;
};

// RemoteCommand は、ほかの端末から届いた操作 (GraphQL の PlayerCommand)。
export type RemoteCommand = {
  id: string;
  type:
    | 'PLAY'
    | 'PAUSE'
    | 'SEEK'
    | 'SET_VOLUME'
    | 'SET_MUTED'
    | 'SELECT_AUDIO'
    | 'PRESS_KEY'
    | 'STOP';
  position: number | null;
  volume: number | null;
  muted: boolean | null;
  audio: string | null;
  key: string | null;
};

// RemoteHandlers は、届いた操作を、プレイヤーの操作に渡すもの。seek は、録画の再生だけで渡す。
export type RemoteHandlers = {
  play: () => void;
  pause: () => void;
  seek?: (seconds: number) => void;
  setVolume: (volume: number) => void;
  setMuted: (muted: boolean) => void;
  selectAudio: (id: string) => void;
  pressKey: (key: RemoteKey) => void;
  stop: () => void;
};

// RemoteApi は、サーバーとのやり取り。テストでは差し替える。
export type RemoteApi = {
  register: (state: RemoteState) => Promise<string>;
  update: (id: string, state: RemoteState, announce: boolean) => Promise<void>;
  unregister: (id: string) => void;
  // subscribe は、操作を購読し、やめる関数を返す。
  subscribe: (id: string, onCommand: (command: RemoteCommand) => void) => () => void;
};

// remoteKey は、操作で届いたキーの名前を、データ放送のリモコンのボタンにする。知らない名前なら null。
export function remoteKey(name: string): RemoteKey | null {
  return Object.hasOwn(RemoteKey, name) ? RemoteKey[name as keyof typeof RemoteKey] : null;
}

// announced は、ほかの端末に知らせる部分 (再生位置のほかすべて)。再生位置が進んだだけなら、知らせない。
function announced(state: RemoteState): string {
  return JSON.stringify({ ...state, position: null });
}

// dispatch は、届いた操作を、プレイヤーの操作に渡す。
export function dispatch(command: RemoteCommand, handlers: RemoteHandlers) {
  switch (command.type) {
    case 'PLAY':
      handlers.play();
      break;
    case 'PAUSE':
      handlers.pause();
      break;
    case 'SEEK':
      if (command.position !== null) {
        handlers.seek?.(command.position);
      }
      break;
    case 'SET_VOLUME':
      if (command.volume !== null) {
        handlers.setVolume(command.volume);
      }
      break;
    case 'SET_MUTED':
      if (command.muted !== null) {
        handlers.setMuted(command.muted);
      }
      break;
    case 'SELECT_AUDIO':
      if (command.audio !== null) {
        handlers.selectAudio(command.audio);
      }
      break;
    case 'PRESS_KEY': {
      const key = command.key === null ? null : remoteKey(command.key);
      if (key !== null) {
        handlers.pressKey(key);
      }
      break;
    }
    case 'STOP':
      handlers.stop();
      break;
  }
}

// useRemotePlayer は、このプレイヤーを「再生中のプレイヤー」として登録し、同じアカウントのほかの端末から操作できるようにする
// (#459、設計書 7.7)。state が null の間は登録しない。登録したら、状態を 10 秒ごとに送り、再生位置のほかが変わったら、
// すぐにほかの端末に知らせる。届いた操作は handlers に渡す。画面を離れたら、登録を取り消す。
export function useRemotePlayer(
  state: RemoteState | null,
  handlers: RemoteHandlers,
  api?: RemoteApi,
): void {
  const environment = useRelayEnvironment();
  const apiRef = useRef(api ?? relayRemoteApi(environment));
  const stateRef = useRef(state);
  stateRef.current = state;
  const handlersRef = useRef(handlers);
  handlersRef.current = handlers;
  const idRef = useRef<string | null>(null);
  // lastAnnounced は、最後にほかの端末に知らせた状態 (再生位置のほか)。
  const lastAnnounced = useRef<string | null>(null);
  const active = state !== null;

  // 登録、状態の送信、操作の購読、取り消し。
  useEffect(() => {
    if (!active) {
      return;
    }
    const remote = apiRef.current;
    let closed = false;
    let unsubscribe: (() => void) | null = null;
    let timer: ReturnType<typeof setInterval> | null = null;
    // handled は、渡し終えた操作の ID。つなぎ直したときに、同じ操作をもう一度渡さないようにする。
    let handled = 0;
    const initial = stateRef.current;
    if (initial === null) {
      return;
    }
    void remote
      .register(initial)
      .then((id) => {
        if (closed) {
          remote.unregister(id);

          return;
        }
        idRef.current = id;
        lastAnnounced.current = announced(initial);
        unsubscribe = remote.subscribe(id, (command) => {
          const commandId = Number(command.id);
          if (commandId <= handled) {
            return;
          }
          handled = commandId;
          dispatch(command, handlersRef.current);
        });
        timer = setInterval(() => {
          const current = stateRef.current;
          if (current !== null) {
            void remote.update(id, current, false).catch(() => {});
          }
        }, heartbeatMs);
      })
      .catch(() => {});

    return () => {
      closed = true;
      unsubscribe?.();
      if (timer !== null) {
        clearInterval(timer);
      }
      if (idRef.current !== null) {
        remote.unregister(idRef.current);
        idRef.current = null;
      }
    };
  }, [active]);

  // 再生位置のほかが変わったら、少し待ってから、ほかの端末に知らせる。
  const key = state === null ? null : announced(state);
  useEffect(() => {
    const id = idRef.current;
    if (key === null || id === null || key === lastAnnounced.current) {
      return;
    }
    const timer = setTimeout(() => {
      const current = stateRef.current;
      if (current === null) {
        return;
      }
      lastAnnounced.current = key;
      void apiRef.current.update(id, current, true).catch(() => {});
    }, announceDelayMs);

    return () => clearTimeout(timer);
  }, [key]);
}

// relayRemoteApi は、GraphQL でサーバーとやり取りする RemoteApi を返す。
function relayRemoteApi(environment: Environment): RemoteApi {
  return {
    register: async (state) => {
      const response = await mutate<useRemotePlayerRegisterMutation>(
        environment,
        graphql`
          mutation useRemotePlayerRegisterMutation($state: PlayerStateInput!) {
            registerPlayer(state: $state) {
              id
            }
          }
        `,
        { state: toInput(state) },
      );

      return response.registerPlayer.id;
    },
    update: async (id, state, announce) => {
      await mutate<useRemotePlayerUpdateMutation>(
        environment,
        graphql`
          mutation useRemotePlayerUpdateMutation(
            $id: ID!
            $state: PlayerStateInput!
            $announce: Boolean!
          ) {
            updatePlayer(id: $id, state: $state, announce: $announce) {
              id
            }
          }
        `,
        { id, state: toInput(state), announce },
      );
    },
    unregister: (id) => {
      void mutate<useRemotePlayerUnregisterMutation>(
        environment,
        graphql`
          mutation useRemotePlayerUnregisterMutation($id: ID!) {
            unregisterPlayer(id: $id)
          }
        `,
        { id },
      ).catch(() => {});
    },
    subscribe: (id, onCommand) => {
      // after に 0 を渡し、つなぎ直したときも、プレイヤーを登録した後に届いた操作をすべて受け取る。渡し終えたものは呼ぶ側で飛ばす。
      const disposable = requestSubscription<useRemotePlayerCommandsSubscription>(environment, {
        subscription: graphql`
          subscription useRemotePlayerCommandsSubscription($id: ID!, $after: ID) {
            playerCommands(id: $id, after: $after) {
              id
              type
              position
              volume
              muted
              audio
              key
            }
          }
        `,
        variables: { id, after: '0' },
        onNext: (response) => {
          const command = response?.playerCommands;
          if (command != null) {
            onCommand({
              id: command.id,
              type: command.type as RemoteCommand['type'],
              position: command.position ?? null,
              volume: command.volume ?? null,
              muted: command.muted ?? null,
              audio: command.audio ?? null,
              key: command.key ?? null,
            });
          }
        },
        onError: () => {},
      });

      return () => disposable.dispose();
    },
  };
}

function toInput(state: RemoteState) {
  return {
    ...state,
    audios: state.audios.map((audio) => ({ id: audio.id, label: audio.label })),
    keyGroups: [...state.keyGroups],
  };
}
