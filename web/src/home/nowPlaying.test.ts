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

  test.each([
    {
      name: '元のサービスが一覧にあれば、イベント共有のサブチャンネルは出さない',
      services: ['s1', 's2', 's3'],
      want: ['s1', 's3'],
    },
    {
      name: '元のサービスが一覧になければ、サブチャンネルを出す',
      services: ['s2', 's3'],
      want: ['s2', 's3'],
    },
  ])('$name', ({ services, want }) => {
    const shared = program('a', '09:30', '10:30');
    const all = {
      s1: { service: { id: 's1', name: 'メイン' }, programs: [shared] },
      s2: {
        service: { id: 's2', name: 'サブ' },
        programs: [{ ...shared, sharedFrom: { id: 's1' } }],
      },
      s3: {
        service: { id: 's3', name: 'ほか' },
        programs: [{ ...program('b', '09:30', '10:30'), sharedFrom: null }],
      },
    } as const;
    const rows = services.map((id) => all[id as keyof typeof all]);

    expect(nowPlaying(rows, now, 10).map((item) => item.serviceId)).toEqual(want);
  });

  test('省いたサブチャンネルは、limit に数えない', () => {
    const shared = program('a', '09:30', '10:30');
    const rows = [
      { service: { id: 's1', name: 'メイン' }, programs: [shared] },
      { service: { id: 's2', name: 'サブ' }, programs: [{ ...shared, sharedFrom: { id: 's1' } }] },
      { service: { id: 's3', name: 'ほか' }, programs: [program('b', '09:30', '10:30')] },
    ];

    expect(nowPlaying(rows, now, 2).map((item) => item.serviceId)).toEqual(['s1', 's3']);
  });

  test('limit 件までにする', () => {
    const rows = ['s1', 's2', 's3'].map((id) => ({
      service: { id, name: id },
      programs: [program(id, '09:00', '11:00')],
    }));

    expect(nowPlaying(rows, now, 2).map((item) => item.serviceId)).toEqual(['s1', 's2']);
    expect(nowPlaying(rows, now, 0)).toEqual([]);
  });
});
