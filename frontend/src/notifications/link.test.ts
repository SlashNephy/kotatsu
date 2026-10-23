import { describe, expect, test } from 'vitest';

import { notificationPath } from './link';

describe('notificationPath', () => {
  test.each([
    { name: 'アプリの中のパス', url: '/live/U2VydmljZToxMDI0', want: '/live/U2VydmljZToxMDI0' },
    { name: 'パスがない', url: null, want: null },
    { name: '空文字列', url: '', want: null },
    { name: 'ほかのオリジンの URL', url: 'https://example.com/live', want: null },
    { name: 'プロトコル相対の URL', url: '//example.com/live', want: null },
    { name: 'バックスラッシュで始まる URL', url: '/\\example.com', want: null },
    { name: '相対パス', url: 'live/1', want: null },
  ])('$name', ({ url, want }) => {
    expect(notificationPath(url)).toBe(want);
  });
});
