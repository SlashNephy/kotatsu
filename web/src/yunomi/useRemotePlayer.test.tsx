import { act, cleanup, renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { RelayEnvironmentProvider } from 'react-relay';
import { Environment, Network, RecordSource, Store } from 'relay-runtime';
import { afterEach, beforeEach, describe, expect, test, vi, type Mock } from 'vitest';

import { RemoteKey } from './useDataBroadcast';
import {
  announceDelayMs,
  dispatch,
  heartbeatMs,
  remoteKey,
  useRemotePlayer,
  type RemoteApi,
  type RemoteCommand,
  type RemoteHandlers,
  type RemoteState,
} from './useRemotePlayer';

const state: RemoteState = {
  kind: 'LIVE',
  target: 'U2VydmljZTozMjczNjAxMDI0',
  title: 'こたつのニュース',
  paused: false,
  position: null,
  duration: null,
  volume: 0.8,
  muted: false,
  audios: [],
  audio: null,
  dataBroadcast: false,
  keyGroups: [],
};

type MockHandlers = { [K in keyof RemoteHandlers]-?: Mock<NonNullable<RemoteHandlers[K]>> };

function handlers(): MockHandlers {
  return {
    play: vi.fn(),
    pause: vi.fn(),
    seek: vi.fn(),
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
      ({ current }) => useRemotePlayer(current, handlers(), fake.api),
      {
        wrapper,
        initialProps: { current: state },
      },
    );
    await act(async () => {});

    act(() => {
      vi.advanceTimersByTime(heartbeatMs);
    });
    expect(fake.updates).toEqual([{ state, announce: false }]);

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
    expect(fake.updates.at(-1)).toEqual({ state: paused, announce: true });
  });

  test('届いた操作を、届いた順に一度ずつ渡す。つなぎ直して同じ操作が届いても、もう一度は渡さない', async () => {
    const fake = fakeApi();
    const remote = handlers();
    renderHook(() => useRemotePlayer(state, remote, fake.api), { wrapper });
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
    const { unmount } = renderHook(() => useRemotePlayer(state, handlers(), fake.api), { wrapper });
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
    renderHook(() => useRemotePlayer(null, handlers(), fake.api), { wrapper });
    await act(async () => {});

    expect(register).not.toHaveBeenCalled();
  });
});

describe('dispatch', () => {
  test.each([
    {
      name: 'シーク',
      command: command(1, { type: 'SEEK', position: 120 }),
      call: 'seek',
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
    expect(remote.seek).not.toHaveBeenCalled();
    expect(remote.pressKey).not.toHaveBeenCalled();
  });
});

describe('remoteKey', () => {
  test('リモコンのボタンの名前だけを受け付ける', () => {
    expect(remoteKey('d')).toBe(RemoteKey.d);
    expect(remoteKey('digit12')).toBe(RemoteKey.digit12);
    expect(remoteKey('toString')).toBeNull();
  });
});
