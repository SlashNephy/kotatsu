import { describe, expect, test } from 'vitest';

import {
  dismissLineupAlerts,
  isLineupAlertDismissed,
  lineupAlertWindowMs,
  lineupDismissalsVersion,
  recentLineupAlerts,
  subscribeLineupDismissals,
} from './lineupAlerts';

describe('recentLineupAlerts', () => {
  const now = Date.parse('2026-11-14T00:00:00Z');
  const at = (ms: number) => ({ occurredAt: new Date(now - ms).toISOString() });

  test('出しておく長さの内のものだけを残す', () => {
    const inside = at(lineupAlertWindowMs);
    const fresh = at(0);
    const outside = at(lineupAlertWindowMs + 1);

    expect(recentLineupAlerts([fresh, outside, inside], now)).toEqual([fresh, inside]);
  });
});

describe('dismissLineupAlerts', () => {
  test('既読にした知らせを覚え、変わったことを知らせる', () => {
    let calls = 0;
    const unsubscribe = subscribeLineupDismissals(() => {
      calls += 1;
    });
    const before = lineupDismissalsVersion();

    dismissLineupAlerts(['a']);
    unsubscribe();
    dismissLineupAlerts(['b']);

    expect(isLineupAlertDismissed('a')).toBe(true);
    expect(isLineupAlertDismissed('b')).toBe(true);
    expect(isLineupAlertDismissed('c')).toBe(false);
    expect(lineupDismissalsVersion()).not.toBe(before);
    expect(calls).toBe(1);
  });
});
