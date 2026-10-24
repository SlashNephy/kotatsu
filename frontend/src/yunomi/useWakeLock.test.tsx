import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { useWakeLock } from './useWakeLock';

// fakeSentinel は、手放したかを持つ WakeLockSentinel の代わり。
function fakeSentinel() {
  const sentinel = { released: false, release: vi.fn() };
  sentinel.release.mockImplementation(async () => {
    sentinel.released = true;
  });

  return sentinel;
}

let visibility: DocumentVisibilityState = 'visible';
let sentinels: ReturnType<typeof fakeSentinel>[] = [];
const request = vi.fn(async () => {
  const sentinel = fakeSentinel();
  sentinels.push(sentinel);

  return sentinel;
});

const setVisibility = async (next: DocumentVisibilityState) => {
  visibility = next;
  // タブが隠れると、ブラウザが Wake Lock を手放す。
  if (next === 'hidden') {
    for (const sentinel of sentinels) {
      sentinel.released = true;
    }
  }
  await act(async () => {
    document.dispatchEvent(new Event('visibilitychange'));
  });
};

beforeEach(() => {
  visibility = 'visible';
  sentinels = [];
  request.mockClear();
  vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => visibility);
  Object.defineProperty(navigator, 'wakeLock', { configurable: true, value: { request } });
});

afterEach(() => {
  vi.restoreAllMocks();
  Reflect.deleteProperty(navigator, 'wakeLock');
});

describe('useWakeLock', () => {
  test('再生している間だけ取り、止めたら手放す', async () => {
    const { rerender } = renderHook(({ active }) => useWakeLock(active), {
      initialProps: { active: false },
    });
    await act(async () => {});
    expect(request).not.toHaveBeenCalled();

    rerender({ active: true });
    await act(async () => {});
    expect(request).toHaveBeenCalledExactlyOnceWith('screen');

    rerender({ active: false });
    await act(async () => {});
    expect(sentinels[0]?.release).toHaveBeenCalledOnce();
  });

  test('タブが隠れて手放されたら、見えるようになったときに取り直す', async () => {
    const { unmount } = renderHook(() => useWakeLock(true));
    await act(async () => {});
    expect(request).toHaveBeenCalledOnce();

    await setVisibility('hidden');
    expect(request).toHaveBeenCalledOnce();
    await setVisibility('visible');
    expect(request).toHaveBeenCalledTimes(2);

    unmount();
    await act(async () => {});
    expect(sentinels[1]?.release).toHaveBeenCalledOnce();
  });

  test('対応していないブラウザでは何もしない', async () => {
    Reflect.deleteProperty(navigator, 'wakeLock');
    const { unmount } = renderHook(() => useWakeLock(true));
    await act(async () => {});
    unmount();
    expect(request).not.toHaveBeenCalled();
  });
});
