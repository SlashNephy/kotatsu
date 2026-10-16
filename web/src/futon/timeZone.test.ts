import { afterEach, describe, expect, test } from 'vitest';

import {
  defaultTimeZone,
  fromWallClock,
  setTimeZone,
  timeZone,
  zonedFormat,
  zoneOffset,
} from './timeZone';

const hour = 3_600_000;

afterEach(() => {
  setTimeZone(defaultTimeZone);
});

describe('zoneOffset', () => {
  test.each([
    { zone: 'Asia/Tokyo', at: '2026-10-16T12:00:00Z', want: 9 * hour },
    { zone: 'UTC', at: '2026-10-16T12:00:00Z', want: 0 },
    // ニューヨークは、夏時間の間は UTC-4、冬は UTC-5。
    { zone: 'America/New_York', at: '2026-07-01T12:00:00Z', want: -4 * hour },
    { zone: 'America/New_York', at: '2026-12-01T12:00:00Z', want: -5 * hour },
    { zone: 'Asia/Kolkata', at: '2026-10-16T12:00:00Z', want: 5.5 * hour },
  ])('$zone の $at は $want', ({ zone, at, want }) => {
    setTimeZone(zone);

    expect(zoneOffset(Date.parse(at))).toBe(want);
  });
});

describe('fromWallClock', () => {
  test.each([
    { zone: 'Asia/Tokyo', local: '2026-10-16T21:00:00Z', want: '2026-10-16T12:00:00.000Z' },
    { zone: 'America/New_York', local: '2026-07-01T08:00:00Z', want: '2026-07-01T12:00:00.000Z' },
    { zone: 'America/New_York', local: '2026-12-01T07:00:00Z', want: '2026-12-01T12:00:00.000Z' },
  ])('$zone の壁時計 $local は $want', ({ zone, local, want }) => {
    setTimeZone(zone);

    expect(new Date(fromWallClock(Date.parse(local))).toISOString()).toBe(want);
  });
});

describe('zonedFormat', () => {
  test('書くときのタイムゾーンで書く', () => {
    const format = zonedFormat({ hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });
    const at = Date.parse('2026-10-16T12:00:00Z');
    expect(format.format(at)).toBe('21:00');

    setTimeZone('Europe/London');

    expect(format.format(at)).toBe('13:00');
  });
});

describe('setTimeZone', () => {
  test('知らない名前では変えない', () => {
    expect(setTimeZone('Asia/Edo')).toBe(false);
    expect(timeZone()).toBe(defaultTimeZone);
  });
});
