import { describe, expect, test } from 'vitest';

import { nowPlaying } from './nowPlaying';

const now = Date.parse('2026-10-11T10:00:00+09:00');
const program = (id: string, start: string, end: string) => ({
  id,
  name: `番組 ${id}`,
  startAt: `2026-10-11T${start}:00+09:00`,
  endAt: `2026-10-11T${end}:00+09:00`,
});

describe('nowPlaying', () => {
  test('サービスごとに放送中の番組を選び、放送中の番組がないサービスは出さない', () => {
    const rows = [
      {
        service: { id: 's1', name: 'NHK' },
        programs: [program('a', '09:00', '10:00'), program('b', '10:00', '10:30')],
      },
      { service: { id: 's2', name: 'ETV' }, programs: [program('c', '10:30', '11:00')] },
      { service: { id: 's3', name: 'NTV' }, programs: [program('d', '09:55', '10:55')] },
    ];

    expect(nowPlaying(rows, now, 10).map((item) => [item.serviceName, item.programName])).toEqual([
      ['NHK', '番組 b'],
      ['NTV', '番組 d'],
    ]);
  });

  test('limit 件までにする', () => {
    const rows = ['s1', 's2', 's3'].map((id) => ({
      service: { id, name: id },
      programs: [program(id, '09:00', '11:00')],
    }));

    expect(nowPlaying(rows, now, 2).map((item) => item.serviceId)).toEqual(['s1', 's2']);
  });
});
