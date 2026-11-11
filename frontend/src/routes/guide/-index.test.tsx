import { cleanup, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createDemoEnvironment, renderRoute } from '../../test/route';
import { Route } from './index';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('GuidePage', () => {
  test.each(['2024-13-45', '2024-02-30', '2023-02-29', '2024-04-31', 'abc'])(
    '存在しない日付や形の違う値 %s の URL は、日付を捨てて今日の番組表を出す (#1430)',
    async (date) => {
      vi.spyOn(console, 'error').mockImplementation(() => undefined);
      await renderRoute(Route, '/guide/', `/guide?date=${date}`, createDemoEnvironment());

      // 2 月 30 日のような日付も、次の月に繰り越さず、今日の放送日 (4 時に替わる) を出す。
      const today = new Intl.DateTimeFormat('ja-JP', {
        timeZone: 'Asia/Tokyo',
        month: 'numeric',
        day: 'numeric',
        weekday: 'short',
      }).format(Date.now() - 4 * 60 * 60_000);
      expect(await screen.findByText(today)).toBeTruthy();
      expect(screen.queryByText('読み込めませんでした。時間をおいてやり直してください')).toBeNull();
    },
  );

  test('存在する日付の URL は、そのまま使う', async () => {
    const { router } = await renderRoute(
      Route,
      '/guide/',
      '/guide?date=2024-02-29',
      createDemoEnvironment(),
    );

    expect(router.state.matches.at(-1)?.search).toHaveProperty('date', '2024-02-29');
  });
});
