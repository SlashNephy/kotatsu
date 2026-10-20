import { describe, expect, test } from 'vitest';

import { nowPlaying, pinnedNowPlaying } from './nowPlaying';

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

    expect(nowPlaying(rows, now).map((item) => [item.serviceName, item.programName])).toEqual([
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

    expect(nowPlaying(rows, now).map((item) => item.serviceId)).toEqual(want);
  });
});

describe('pinnedNowPlaying', () => {
  const shared = program('a', '09:30', '10:30');
  const rows = [
    { service: { id: 's1', name: 'メイン' }, programs: [shared] },
    { service: { id: 's2', name: 'サブ' }, programs: [{ ...shared, sharedFrom: { id: 's1' } }] },
    { service: { id: 's3', name: '休止' }, programs: [program('b', '10:30', '11:00')] },
    { service: { id: 's4', name: 'ほか' }, programs: [program('c', '09:00', '11:00')] },
  ];

  test.each([
    {
      name: 'ピン留めした順に並べ、放送中の番組がないサービスは出さない',
      pinned: ['s4', 's3', 's1'],
      want: ['s4', 's1'],
    },
    {
      name: 'メインチャンネルと一緒にピン留めしたサブチャンネルも省かない',
      pinned: ['s2', 's1'],
      want: ['s2', 's1'],
    },
    {
      name: '番組表にないサービスは出さない',
      pinned: ['s9', 's4'],
      want: ['s4'],
    },
  ])('$name', ({ pinned, want }) => {
    expect(pinnedNowPlaying(rows, pinned, now).map((item) => item.serviceId)).toEqual(want);
  });
});
