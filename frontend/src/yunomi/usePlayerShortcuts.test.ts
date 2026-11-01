import { cleanup, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { usePlayerShortcuts } from './usePlayerShortcuts';

afterEach(() => {
  cleanup();
});

function press(key: string): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { key, cancelable: true });
  window.dispatchEvent(event);

  return event;
}

describe('usePlayerShortcuts', () => {
  test('今使える操作は、キーで呼び、ブラウザの既定の動きを止める', () => {
    const togglePlay = vi.fn();
    renderHook(() => usePlayerShortcuts({ togglePlay }));

    const event = press(' ');
    expect(togglePlay).toHaveBeenCalledOnce();
    expect(event.defaultPrevented).toBe(true);
  });

  test('今使えない操作のキーは、ブラウザに任せる', () => {
    // データ放送を出している間のライブのように、シークを使えない画面。
    const togglePlay = vi.fn();
    renderHook(() => usePlayerShortcuts({ togglePlay }));

    const event = press('ArrowLeft');
    expect(togglePlay).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(false);
  });

  test('描き直して使える操作が変わったら、新しい操作を呼ぶ', () => {
    const seekBackward = vi.fn();
    const { rerender } = renderHook(({ commands }) => usePlayerShortcuts(commands), {
      initialProps: { commands: {} as Parameters<typeof usePlayerShortcuts>[0] },
    });
    rerender({ commands: { seekBackward } });

    press('ArrowLeft');
    expect(seekBackward).toHaveBeenCalledOnce();
  });
});
