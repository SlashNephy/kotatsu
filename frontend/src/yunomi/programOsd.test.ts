import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { usePlayerEvents, type PlayerSnapshot } from './playerState';
import { programOsdMs, useProgramOsd } from './programOsd';

const state: PlayerSnapshot = {
  kind: 'LIVE',
  target: 'U2VydmljZTozMjczNjAxMDI0',
  serviceName: 'こたつテレビ',
  program: { id: 'UHJvZ3JhbTox', name: 'こたつのニュース' },
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

const nextProgram = { id: 'UHJvZ3JhbToy', name: 'こたつの天気' };

type Props = { current: PlayerSnapshot; ready: boolean; enabled: boolean };

function render(initialProps: Props) {
  return renderHook(
    ({ current, ready, enabled }: Props) => {
      const events = usePlayerEvents(current);

      return useProgramOsd(events, ready, enabled);
    },
    { initialProps },
  );
}

beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe('useProgramOsd', () => {
  test('選局して映像が出たら、決まった間だけ出す', () => {
    const { result, rerender } = render({ current: state, ready: false, enabled: true });
    expect(result.current).toBe(false);

    rerender({ current: state, ready: true, enabled: true });
    expect(result.current).toBe(true);

    act(() => {
      vi.advanceTimersByTime(programOsdMs - 1);
    });
    expect(result.current).toBe(true);
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(result.current).toBe(false);

    // 読み直して映像が出直しても、選局したときの分は出し直さない。
    rerender({ current: state, ready: false, enabled: true });
    rerender({ current: state, ready: true, enabled: true });
    expect(result.current).toBe(false);
  });

  test('番組が変わったら出し直し、そこから決まった間だけ出す', () => {
    const { result, rerender } = render({ current: state, ready: true, enabled: true });
    act(() => {
      vi.advanceTimersByTime(programOsdMs - 1000);
    });

    const next = { ...state, program: nextProgram };
    rerender({ current: next, ready: true, enabled: true });
    act(() => {
      vi.advanceTimersByTime(programOsdMs - 1);
    });
    expect(result.current).toBe(true);
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(result.current).toBe(false);

    // 番組のほかが変わっただけなら、出さない。
    rerender({ current: { ...next, paused: true, muted: true }, ready: true, enabled: true });
    expect(result.current).toBe(false);
  });

  test('出さない間は、番組が変わっても出さず、出している分も隠す', () => {
    const { result, rerender } = render({ current: state, ready: true, enabled: true });
    expect(result.current).toBe(true);

    // バッファを再生し始めた。
    rerender({ current: state, ready: true, enabled: false });
    expect(result.current).toBe(false);
    rerender({ current: { ...state, program: nextProgram }, ready: true, enabled: false });
    expect(result.current).toBe(false);
  });
});
