import { act, cleanup, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createFailingEnvironment, renderRoute } from '../../../test/route';
import { Route } from './$watchRuleId';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('WatchRulePage', () => {
  test('エラーの後に別の視聴ルールへ移ると、エラーの表示を消して読み直す (#1453)', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    // 1 つ目は読み込みに失敗する。2 つ目は、応答を返さず読み込み中のままにする。
    const { environment, requested } = createFailingEnvironment('WatchRule:1');
    const { router } = await renderRoute(
      Route,
      '/rules/watch/$watchRuleId',
      '/rules/watch/WatchRule:1',
      environment,
    );
    expect(
      await screen.findByText('読み込めませんでした。時間をおいてやり直してください'),
    ).toBeTruthy();

    await act(() =>
      router.navigate({ to: '/rules/watch/$watchRuleId', params: { watchRuleId: 'WatchRule:2' } }),
    );

    expect(screen.queryByText('読み込めませんでした。時間をおいてやり直してください')).toBeNull();
    expect(requested.some((variables) => variables.includes('WatchRule:2'))).toBe(true);
  });
});
