import { describe, expect, test } from 'vitest';

import { lineupAlertWindowMs, recentLineupAlerts } from './lineupAlerts';

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
