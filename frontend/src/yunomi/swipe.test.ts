import { describe, expect, test } from 'vitest';

import { stepBrightness, brightnessStyle } from './brightness';
import { swipeBandAt, swipeStep, swipeThreshold, SwipeTracker } from './swipe';

describe('swipeBandAt (#1598)', () => {
  test.each([
    { name: '左の帯は明るさ', x: 100, volume: true, want: 'brightness' },
    { name: '右の帯は音量', x: 900, volume: true, want: 'volume' },
    { name: '真ん中は受け付けない', x: 500, volume: true, want: null },
    // 画面の端からのジェスチャー (戻る) と取り合わない。
    { name: '左の端', x: 10, volume: true, want: null },
    { name: '右の端', x: 990, volume: true, want: null },
    // iOS では、音量を変えられないので、音量の帯を出さない。
    { name: '音量を変えられない端末の右の帯', x: 900, volume: false, want: null },
  ])('$name', ({ x, volume, want }) => {
    expect(swipeBandAt(x, 1000, volume)).toBe(want);
  });
});

describe('SwipeTracker (#1598)', () => {
  test('縦に動かした距離に応じて段を返し、上は正、下は負', () => {
    const tracker = new SwipeTracker('volume', 900, 500);
    // 閾値より短い動きは、まだ縦か横かを決めない。
    expect(tracker.move(900, 500 - swipeThreshold + 1)).toBe(0);
    expect(tracker.direction).toBe('pending');
    expect(tracker.move(900, 500 - swipeStep * 2)).toBe(2);
    expect(tracker.direction).toBe('vertical');
    expect(tracker.move(900, 500 - swipeStep * 3)).toBe(1);
    // 下へ戻すと、戻した分だけ負の段を返す。
    expect(tracker.move(900, 500 + swipeStep)).toBe(-4);
  });

  test('横に動かしたら、指を離すまで何も変えない', () => {
    const tracker = new SwipeTracker('brightness', 100, 500);
    expect(tracker.move(100 + swipeThreshold * 2, 500)).toBe(0);
    expect(tracker.direction).toBe('horizontal');
    expect(tracker.move(100 + swipeThreshold * 2, 500 - swipeStep * 5)).toBe(0);
  });
});

describe('明るさ (#1598)', () => {
  test.each([
    { name: '上げる', from: 1, steps: 2, want: 1.2 },
    { name: '下げる', from: 1, steps: -3, want: 0.7 },
    { name: '上限で止める', from: 1.4, steps: 5, want: 1.5 },
    { name: '下限で止める', from: 0.6, steps: -5, want: 0.5 },
  ])('$name', ({ from, steps, want }) => {
    expect(stepBrightness(from, steps)).toBe(want);
  });

  test('そのままの明るさなら、filter を掛けない', () => {
    expect(brightnessStyle(1)).toEqual({});
    expect(brightnessStyle(1.2)).toEqual({ filter: 'brightness(1.2)' });
  });
});
