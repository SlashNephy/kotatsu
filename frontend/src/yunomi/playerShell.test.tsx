import { act, renderHook } from '@testing-library/react';
import { describe, expect, test, vi } from 'vitest';

import { usePlayerShell } from './playerShell';

describe('seekCommands', () => {
  test.each([
    {
      name: '進む向きで CM を飛んだら、シークの結果を出さない',
      command: 'seekForward',
      skipped: true,
      flashed: false,
    },
    {
      name: '進む向きで CM を飛ばなければ、シークの結果を出す',
      command: 'seekForward',
      skipped: false,
      flashed: true,
    },
    {
      name: '30 秒進む向きでも、CM を飛んだら出さない',
      command: 'seekForwardLong',
      skipped: true,
      flashed: false,
    },
    {
      name: '15 秒進む向きでも、CM を飛んだら出さない',
      command: 'seekForward15',
      skipped: true,
      flashed: false,
    },
    {
      name: '戻る向きは、CM を飛んだと返されても、シークの結果を出す',
      command: 'seekBackward',
      skipped: true,
      flashed: true,
    },
    {
      name: 'CM を飛ばない再生元 (skip が何も返さない) でも、進む向きのシークの結果を出す',
      command: 'seekForward',
      skipped: undefined,
      flashed: true,
    },
  ] as const)('$name', ({ command, skipped, flashed }) => {
    const { result } = renderHook(() => usePlayerShell());
    const skip = vi.fn(() => skipped);
    const commands = result.current.seekCommands({
      skip,
      seekTo: vi.fn(),
      currentPosition: () => 0,
      retry: vi.fn(),
    });

    act(() => commands[command]?.());

    expect(skip).toHaveBeenCalledOnce();
    expect(result.current.flash !== null).toBe(flashed);
  });
});
