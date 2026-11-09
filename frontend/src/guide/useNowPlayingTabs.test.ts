import { act, renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { nowPlayingTabs, useNowPlayingTabs, type NowPlayingRow } from './useNowPlayingTabs';

const channelTypes = [
  { type: 'GR', displayName: '地上波' },
  { type: 'BS', displayName: 'BS' },
];
const labels = { pinned: 'ピン留め', force: '人気の実況' };

function row(
  id: string,
  type: string,
  options: { force?: number; jikkyoId?: string; sharedFrom?: string } = {},
): NowPlayingRow {
  return {
    service: {
      id,
      name: id,
      channelType: { type },
      jikkyo:
        options.force === undefined
          ? null
          : { id: options.jikkyoId ?? `jk-${id}`, force: options.force },
    },
    programs: [
      {
        name: '番組',
        startAt: '2026-11-09T10:00:00+09:00',
        endAt: '2026-11-09T11:00:00+09:00',
        sharedFrom: options.sharedFrom === undefined ? null : { id: options.sharedFrom },
      },
    ],
  };
}

function build(rows: NowPlayingRow[], pinned: string[] = []) {
  return nowPlayingTabs(channelTypes, rows, pinned, labels);
}

const summary = (tabs: ReturnType<typeof build>) =>
  tabs.map((tab) => [tab.value, tab.rows.map((r) => r.service.id)]);

describe('nowPlayingTabs', () => {
  it.each([
    {
      title: 'ピン留め、人気の実況、種別の順に並べる',
      rows: [row('a', 'GR', { force: 3 }), row('b', 'BS')],
      pinned: ['b'],
      want: [
        ['pinned', ['b']],
        ['force', ['a']],
        ['GR', ['a']],
        ['BS', ['b']],
      ],
    },
    {
      title: 'ピン留めした局が番組表になければ、ピン留めのタブを作らない',
      rows: [row('a', 'GR')],
      pinned: ['missing'],
      want: [['GR', ['a']]],
    },
    {
      title: 'ピン留めのタブは、ピン留めした順に並べ、サブチャンネルも省かない',
      rows: [row('main', 'GR'), row('sub', 'GR', { sharedFrom: 'main' }), row('x', 'GR')],
      pinned: ['x', 'sub'],
      want: [
        ['pinned', ['x', 'sub']],
        ['GR', ['main', 'x']],
      ],
    },
    {
      title: '元の局が一覧にないサブチャンネルは省かない',
      rows: [row('sub', 'GR', { sharedFrom: 'hidden' })],
      pinned: [],
      want: [['GR', ['sub']]],
    },
    {
      title: '勢いのある局がなければ、人気の実況のタブを作らない',
      rows: [row('a', 'GR', { force: 0 })],
      pinned: [],
      want: [['GR', ['a']]],
    },
  ])('$title', ({ rows, pinned, want }) => {
    expect(summary(build(rows, pinned))).toEqual(want);
  });
});

describe('useNowPlayingTabs', () => {
  const hot = row('hot', 'GR', { force: 5 });
  const bs = row('bs', 'BS');

  it.each([
    { title: 'ピン留めのタブがあれば、それを選ぶ', pinned: ['bs'], want: 'pinned' },
    { title: 'なければ、人気の実況ではなく最初の種別を選ぶ', pinned: [], want: 'GR' },
  ])('$title', ({ pinned, want }) => {
    const { result } = renderHook(() => useNowPlayingTabs(build([hot, bs], pinned)));

    expect(result.current.value).toBe(want);
  });

  it('選んでいるタブがなくなったら最初の種別に戻り、タブが戻っても移らない', () => {
    const { result, rerender } = renderHook(
      ({ rows }: { rows: NowPlayingRow[] }) => useNowPlayingTabs(build(rows)),
      { initialProps: { rows: [hot, bs] } },
    );
    act(() => {
      result.current.onChange('force');
    });
    expect(result.current.rows.map((r) => r.service.id)).toEqual(['hot']);

    rerender({ rows: [row('hot', 'GR', { force: 0 }), bs] });
    expect(result.current.value).toBe('GR');

    rerender({ rows: [hot, bs] });
    expect(result.current.value).toBe('GR');
  });
});
