import { cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import type { PlayerCommands } from './commands';
import type { PlayerSnapshot } from './playerState';
import { positionState, useMediaSession } from './useMediaSession';

describe('positionState', () => {
  test.each([
    { name: 'ライブ', position: null, want: null },
    {
      name: '長さが分からない',
      position: { duration: 0, position: 0, playbackRate: 1 },
      want: null,
    },
    {
      name: '秒に丸める',
      position: { duration: 1800.6, position: 754.5, playbackRate: 1.5 },
      want: { duration: 1800, position: 754, playbackRate: 1.5 },
    },
    {
      name: '長さを超えた位置は、長さにする',
      position: { duration: 1800, position: 1800.9, playbackRate: 1 },
      want: { duration: 1800, position: 1800, playbackRate: 1 },
    },
    {
      name: '位置が分からなければ、始まりにする',
      position: { duration: 1800, position: Number.NaN, playbackRate: 1 },
      want: { duration: 1800, position: 0, playbackRate: 1 },
    },
  ])('$name', ({ position, want }) => {
    expect(positionState(position)).toEqual(want);
  });
});

describe('useMediaSession', () => {
  // handlers は、OS に出している操作。jsdom には Media Session がないので、代わりを置く。
  let handlers: Map<string, MediaSessionActionHandler>;
  beforeEach(() => {
    handlers = new Map();
    vi.stubGlobal(
      'MediaMetadata',
      class {
        constructor(init: MediaMetadataInit) {
          Object.assign(this, init);
        }
      },
    );
    Object.defineProperty(navigator, 'mediaSession', {
      configurable: true,
      value: {
        metadata: null,
        playbackState: 'none',
        setActionHandler: (action: string, handler: MediaSessionActionHandler | null) => {
          if (handler === null) {
            handlers.delete(action);
          } else {
            handlers.set(action, handler);
          }
        },
      },
    });
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    Reflect.deleteProperty(navigator, 'mediaSession');
  });

  const player: PlayerSnapshot = {
    kind: 'LIVE',
    target: 'U2VydmljZTozMjczNjAxMDI0',
    serviceName: 'こたつテレビ',
    program: { id: 'UHJvZ3JhbTox', name: '🈑こたつのニュース' },
    artwork: null,
    paused: false,
    position: null,
    duration: null,
    playbackRate: 1,
    volume: 1,
    muted: false,
    audios: [],
    audio: null,
    dataBroadcast: false,
    keyGroups: [],
  };

  test('番組名の囲み文字を外して出す。番組が分からなければ、局の名前を出す', () => {
    const { rerender } = renderHook(({ current }) => useMediaSession(current, {}), {
      initialProps: { current: player },
    });
    expect(navigator.mediaSession.metadata).toMatchObject({
      title: 'こたつのニュース',
      artist: 'こたつテレビ',
    });

    rerender({ current: { ...player, program: null } });
    expect(navigator.mediaSession.metadata).toMatchObject({
      title: 'こたつテレビ',
      artist: 'こたつテレビ',
    });
  });

  test('今使える操作だけを OS に出し、OS の操作で登録簿の操作を呼ぶ', () => {
    const seekTo = vi.fn();
    const play = vi.fn();
    renderHook(() => useMediaSession(player, { play, seekTo }));

    expect([...handlers.keys()].sort()).toEqual(['play', 'seekto']);
    handlers.get('seekto')?.({ action: 'seekto', seekTime: 120 });
    handlers.get('play')?.({ action: 'play' });
    expect(seekTo).toHaveBeenCalledExactlyOnceWith(120);
    expect(play).toHaveBeenCalledOnce();
  });

  test('使える操作が変わったら、OS に出し直す', () => {
    // ライブで、局の一覧を読んで、次の局へ移れるようになったとき。
    const { rerender } = renderHook(({ commands }) => useMediaSession(player, commands), {
      initialProps: { commands: { play: vi.fn() } as PlayerCommands },
    });
    expect(handlers.has('nexttrack')).toBe(false);

    const nextChannel = vi.fn();
    rerender({ commands: { play: vi.fn(), nextChannel } });
    handlers.get('nexttrack')?.({ action: 'nexttrack' });
    expect(nextChannel).toHaveBeenCalledOnce();

    rerender({ commands: { play: vi.fn() } });
    expect(handlers.has('nexttrack')).toBe(false);
  });
});
