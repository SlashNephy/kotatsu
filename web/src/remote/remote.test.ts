import { describe, expect, test } from 'vitest';

import { RemoteKey } from '../yunomi/useDataBroadcast';
import { estimatedPosition, remoteKeyName } from './remote';

describe('estimatedPosition', () => {
  const lastSeenAt = '2026-10-18T12:00:00Z';
  const now = Date.parse(lastSeenAt) + 4000;

  test.each([
    {
      name: '再生中なら、送ってからの経過を足す',
      state: { position: 100, duration: 1800, paused: false },
      want: 104,
    },
    {
      name: '一時停止中なら、送った位置のまま',
      state: { position: 100, duration: 1800, paused: true },
      want: 100,
    },
    {
      name: '長さを超えない',
      state: { position: 1798, duration: 1800, paused: false },
      want: 1800,
    },
    {
      name: '長さが分からなければ、足すだけ',
      state: { position: 10, duration: null, paused: false },
      want: 14,
    },
    {
      name: 'ライブ視聴は位置がない',
      state: { position: null, duration: null, paused: false },
      want: null,
    },
  ])('$name', ({ state, want }) => {
    expect(estimatedPosition(state, lastSeenAt, now)).toBe(want);
  });
});

describe('remoteKeyName', () => {
  test('ボタンを、操作で送る名前にする', () => {
    expect(remoteKeyName(RemoteKey.d)).toBe('d');
    expect(remoteKeyName(RemoteKey.digit10)).toBe('digit10');
    expect(remoteKeyName(RemoteKey.blue)).toBe('blue');
  });
});
