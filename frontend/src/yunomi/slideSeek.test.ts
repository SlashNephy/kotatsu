import { describe, expect, test } from 'vitest';

import { SlideSeekTracker, slideSeekSecondsPerWidth, slideSeekThreshold } from './slideSeek';

describe('SlideSeekTracker (#1601)', () => {
  test('滑らせ始めるまでは行き先を返さず、滑らせた距離に応じて動かす', () => {
    const tracker = new SlideSeekTracker(500, 600, 3600, 1000);
    expect(tracker.move(500 + slideSeekThreshold - 1)).toBeNull();
    // 滑らせ始めた所を起点にするので、閾値の分は動かさない。
    const startX = 500 + slideSeekThreshold;
    expect(tracker.move(startX)).toBe(600);
    expect(tracker.move(startX + 250)).toBeCloseTo(600 + slideSeekSecondsPerWidth / 4);
    expect(tracker.move(startX - 500)).toBeCloseTo(600 - slideSeekSecondsPerWidth / 2);
  });

  test('端で止まったら、溜まった移動量を切り詰め、戻す向きにすぐ効く', () => {
    const tracker = new SlideSeekTracker(500, 10, 3600, 1000);
    tracker.move(500 - slideSeekThreshold);
    // 左へ大きく滑らせると、0 秒で止まる。
    expect(tracker.move(0)).toBe(0);
    // 少し戻すと、溜まった分を待たずに、すぐ進む。
    expect(tracker.move(100)).toBeCloseTo(slideSeekSecondsPerWidth / 10);
  });

  test('録画の長さを超えて進めず、戻す向きにすぐ効く', () => {
    const tracker = new SlideSeekTracker(0, 3590, 3600, 1000);
    tracker.move(slideSeekThreshold);
    expect(tracker.move(900)).toBe(3600);
    expect(tracker.move(800)).toBeCloseTo(3600 - slideSeekSecondsPerWidth / 10);
  });
});
