import { cleanup, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import type { PlayerFrameCallback, PlayerMedia } from './playerMedia';
import { useVideoFrames } from './useVideoFrames';

afterEach(() => {
  cleanup();
});

// FrameMedia は、コマごとの呼び返しを、this を使うメソッドで持つ経路の代わり。
// <video> 以外の経路 (#1516) は、こうした普通のメソッドで持ちうる。
class FrameMedia extends EventTarget {
  readonly callbacks = new Map<number, PlayerFrameCallback>();
  private next = 1;

  requestVideoFrameCallback(callback: PlayerFrameCallback): number {
    const id = this.next;
    this.next += 1;
    this.callbacks.set(id, callback);

    return id;
  }

  cancelVideoFrameCallback(id: number): void {
    this.callbacks.delete(id);
  }

  // present は、コマを 1 つ出したことにして、待っている呼び返しを呼ぶ。
  present(mediaTime: number): void {
    const waiting = [...this.callbacks.values()];
    this.callbacks.clear();
    for (const callback of waiting) {
      callback(performance.now(), { mediaTime });
    }
  }
}

describe('useVideoFrames', () => {
  test('呼び返しを this を保ったまま呼び、コマごとに知らせ、外したら止める', () => {
    const media = new FrameMedia();
    const onFrame = vi.fn();

    const { unmount } = renderHook(() => useVideoFrames(media as unknown as PlayerMedia, onFrame));
    media.present(1.5);
    media.present(1.6);

    expect(onFrame.mock.calls.map(([frame]) => frame)).toEqual([
      { mediaTime: 1.5, presented: true },
      { mediaTime: 1.6, presented: true },
    ]);

    unmount();
    expect(media.callbacks.size).toBe(0);
  });
});
