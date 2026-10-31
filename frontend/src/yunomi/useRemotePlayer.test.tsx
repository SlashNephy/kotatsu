import { act, cleanup, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { RelayEnvironmentProvider } from 'react-relay';
import { Environment, Network, RecordSource, Store } from 'relay-runtime';
import { afterEach, beforeEach, describe, expect, test, vi, type Mock } from 'vitest';

import type { PlayerCommandArgs, PlayerCommands } from './commands';
import { usePlayerEvents } from './playerState';
import { RemoteKey } from './useDataBroadcast';
import {
  announceDelayMs,
  dispatch,
  heartbeatMs,
  remoteKey,
  remoteState,
  useRemotePlayer,
  type RemoteApi,
  type RemoteCommand,
  type RemotePlayerSnapshot,
  type RemoteState,
} from './useRemotePlayer';

const state: RemotePlayerSnapshot = {
  kind: 'LIVE',
  target: 'U2VydmljZTozMjczNjAxMDI0',
  serviceName: 'こたつテレビ',
  program: { id: 'UHJvZ3JhbTox', name: 'こたつのニュース' },
  artwork: null,
  paused: false,
  position: null,
  duration: null,
  playbackRate: 1,
  volume: 0.8,
  muted: false,
  audios: [],
  audio: null,
  dataBroadcast: false,
  keyGroups: [],
};

// useRemote は、画面と同じく、状態からイベントを作って、useRemotePlayer に渡す。
function useRemote(
  current: RemotePlayerSnapshot,
  active: boolean,
  commands: PlayerCommands,
  api: RemoteApi,
) {
  const events = usePlayerEvents(current);
  useRemotePlayer(active ? current : null, events, commands, api);
}

type RemoteCommandId =
  | 'play'
  | 'pause'
  | 'seekTo'
  | 'setVolume'
  | 'setMuted'
  | 'selectAudio'
  | 'pressKey'
  | 'stop';
type MockHandlers = {
  [Id in RemoteCommandId]: Mock<(...args: PlayerCommandArgs[Id]) => void>;
};

// handlers は、ほかの端末から呼べる操作を、すべて使える状態にした登録簿。
function handlers(): MockHandlers {
  return {
    play: vi.fn(),
    pause: vi.fn(),
    seekTo: vi.fn(),
    setVolume: vi.fn(),
    setMuted: vi.fn(),
    selectAudio: vi.fn(),
    pressKey: vi.fn(),
    stop: vi.fn(),
  };
}

// fakeApi は、サーバーの代わり。登録した ID を返し、送った状態と、購読の口を覚える。
function fakeApi() {
  const updates: Array<{ state: RemoteState; announce: boolean }> = [];
  const unregistered: string[] = [];
  let deliver: ((command: RemoteCommand) => void) | null = null;
  let unsubscribed = false;
  const api: RemoteApi = {
    register: () => Promise.resolve('player-1'),
    update: (_id, next, announce) => {
      updates.push({ state: next, announce });

      return Promise.resolve();
    },
    unregister: (id) => {
      unregistered.push(id);
    },
    subscribe: (_id, onCommand) => {
      deliver = onCommand;

      return () => {
        unsubscribed = true;
      };
    },
  };

  return {
    api,
    updates,
    unregistered,
    send: (command: RemoteCommand) => deliver?.(command),
    unsubscribed: () => unsubscribed,
  };
}

function command(id: number, patch: Partial<RemoteCommand>): RemoteCommand {
  return {
    id: String(id),
    type: 'PLAY',
    position: null,
    volume: null,
    muted: null,
    audio: null,
    key: null,
    ...patch,
  };
}

// environment は、使わない Relay の環境。サーバーとのやり取りは、fakeApi で差し替える。
const environment = new Environment({
  network: Network.create(() => Promise.reject(new Error('not used'))),
  store: new Store(new RecordSource()),
});

const wrapper = ({ children }: { children: ReactNode }) => (
  <RelayEnvironmentProvider environment={environment}>{children}</RelayEnvironmentProvider>
);

beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe('useRemotePlayer', () => {
  test('登録したら、10 秒ごとに状態を送り、再生位置のほかが変わったときだけ、ほかの端末に知らせる', async () => {
    const fake = fakeApi();
    const { rerender } = renderHook(
      ({ current }) => useRemote(current, true, handlers(), fake.api),
      {
        wrapper,
        initialProps: { current: state },
      },
    );
    await act(async () => {});

    act(() => {
      vi.advanceTimersByTime(heartbeatMs);
    });
    expect(fake.updates).toEqual([{ state: remoteState(state), announce: false }]);

    // 再生位置が進んだだけなら、知らせない。
    rerender({ current: { ...state, position: 12 } });
    act(() => {
      vi.advanceTimersByTime(announceDelayMs);
    });
    expect(fake.updates).toHaveLength(1);

    // 一時停止したら、少し待ってから知らせる。
    const paused = { ...state, position: 12, paused: true };
    rerender({ current: paused });
    act(() => {
      vi.advanceTimersByTime(announceDelayMs);
    });
    expect(fake.updates.at(-1)).toEqual({ state: remoteState(paused), announce: true });
  });

  test('続けて変わったら、最後の変化から少し待って、まとめて 1 回だけ知らせる', async () => {
    const fake = fakeApi();
    const { rerender } = renderHook(
      ({ current }) => useRemote(current, true, handlers(), fake.api),
      {
        wrapper,
        initialProps: { current: state },
      },
    );
    await act(async () => {});

    rerender({ current: { ...state, muted: true } });
    act(() => {
      vi.advanceTimersByTime(announceDelayMs / 2);
    });
    const last = { ...state, muted: true, paused: true };
    rerender({ current: last });
    act(() => {
      vi.advanceTimersByTime(announceDelayMs);
    });
    expect(fake.updates).toEqual([{ state: remoteState(last), announce: true }]);
  });

  test('登録の返事を待つ間に状態が変わったら、返事が来たところで知らせる', async () => {
    const fake = fakeApi();
    let resolve: (id: string) => void = () => {};
    fake.api.register = () =>
      new Promise((done) => {
        resolve = done;
      });
    const { rerender } = renderHook(
      ({ current }) => useRemote(current, true, handlers(), fake.api),
      {
        wrapper,
        initialProps: { current: state },
      },
    );
    // 音声の構成が分かった。
    const withAudios = {
      ...state,
      audios: [
        { id: '1', label: '主音声' },
        { id: '2', label: '副音声' },
      ],
    };
    rerender({ current: withAudios });
    await act(async () => {
      resolve('player-1');
    });
    act(() => {
      vi.advanceTimersByTime(announceDelayMs);
    });
    expect(fake.updates).toEqual([{ state: remoteState(withAudios), announce: true }]);
  });

  test('追っかけ再生で長さが延びただけなら、知らせない', async () => {
    const fake = fakeApi();
    const recording: RemotePlayerSnapshot = {
      ...state,
      kind: 'RECORDING',
      position: 10,
      duration: 100,
    };
    const { rerender } = renderHook(
      ({ current }) => useRemote(current, true, handlers(), fake.api),
      {
        wrapper,
        initialProps: { current: recording },
      },
    );
    await act(async () => {});
    for (const duration of [100.25, 100.5, 100.75]) {
      rerender({ current: { ...recording, position: duration - 90, duration } });
      act(() => {
        vi.advanceTimersByTime(announceDelayMs);
      });
    }
    expect(fake.updates).toEqual([]);
  });

  test('届いた操作を、届いた順に一度ずつ渡す。つなぎ直して同じ操作が届いても、もう一度は渡さない', async () => {
    const fake = fakeApi();
    const remote = handlers();
    renderHook(() => useRemote(state, true, remote, fake.api), { wrapper });
    await act(async () => {});

    act(() => {
      fake.send(command(1, { type: 'PAUSE' }));
      fake.send(command(2, { type: 'SET_VOLUME', volume: 0.3 }));
      // つなぎ直したときは、登録した後の操作が、もう一度届く。
      fake.send(command(1, { type: 'PAUSE' }));
      fake.send(command(2, { type: 'SET_VOLUME', volume: 0.3 }));
      fake.send(command(3, { type: 'PLAY' }));
    });
    expect(remote.pause).toHaveBeenCalledTimes(1);
    expect(remote.setVolume).toHaveBeenCalledExactlyOnceWith(0.3);
    expect(remote.play).toHaveBeenCalledTimes(1);
  });

  test('画面を離れたら、購読をやめて、登録を取り消す', async () => {
    const fake = fakeApi();
    const { unmount } = renderHook(() => useRemote(state, true, handlers(), fake.api), {
      wrapper,
    });
    await act(async () => {});

    unmount();
    expect(fake.unsubscribed()).toBe(true);
    expect(fake.unregistered).toEqual(['player-1']);
    // 取り消した後は、状態を送らない。
    act(() => {
      vi.advanceTimersByTime(heartbeatMs);
    });
    expect(fake.updates).toEqual([]);
  });

  test('状態がない間は、登録しない', async () => {
    const fake = fakeApi();
    const register = vi.spyOn(fake.api, 'register');
    renderHook(() => useRemote(state, false, handlers(), fake.api), { wrapper });
    await act(async () => {});

    expect(register).not.toHaveBeenCalled();
  });
});

describe('dispatch', () => {
  test.each([
    {
      name: 'シーク',
      command: command(1, { type: 'SEEK', position: 120 }),
      call: 'seekTo',
      args: [120],
    },
    {
      name: 'ミュート',
      command: command(1, { type: 'SET_MUTED', muted: true }),
      call: 'setMuted',
      args: [true],
    },
    {
      name: '音声',
      command: command(1, { type: 'SELECT_AUDIO', audio: '2' }),
      call: 'selectAudio',
      args: ['2'],
    },
    {
      name: 'データ放送のキー',
      command: command(1, { type: 'PRESS_KEY', key: 'blue' }),
      call: 'pressKey',
      args: [RemoteKey.blue],
    },
    { name: '閉じる', command: command(1, { type: 'STOP' }), call: 'stop', args: [] },
  ] as const)('$name', ({ command: received, call, args }) => {
    const remote = handlers();
    dispatch(received, remote);
    expect(remote[call]).toHaveBeenCalledExactlyOnceWith(...args);
  });

  test('値の足りない操作と、知らないキーは渡さない', () => {
    const remote = handlers();
    dispatch(command(1, { type: 'SEEK' }), remote);
    dispatch(command(2, { type: 'PRESS_KEY', key: 'power' }), remote);
    expect(remote.seekTo).not.toHaveBeenCalled();
    expect(remote.pressKey).not.toHaveBeenCalled();
  });

  test('今使えない操作は、捨てる', () => {
    // 録画の再生では、データ放送のキーを受け付けない。
    const { pressKey: _pressKey, ...recording } = handlers();
    expect(() => dispatch(command(1, { type: 'PRESS_KEY', key: 'blue' }), recording)).not.toThrow();
    dispatch(command(2, { type: 'PLAY' }), recording);
    expect(recording.play).toHaveBeenCalledOnce();
  });
});

describe('remoteKey', () => {
  test('リモコンのボタンの名前だけを受け付ける', () => {
    expect(remoteKey('d')).toBe(RemoteKey.d);
    expect(remoteKey('digit12')).toBe(RemoteKey.digit12);
    expect(remoteKey('toString')).toBeNull();
  });
});
