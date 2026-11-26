import { act, cleanup, fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { emptyKeyword } from '../conditions/form';
import { createDemoEnvironment, renderRoute } from '../test/route';
import { Route } from './search';

afterEach(() => {
  cleanup();
});

// keywordConditions は、番組名のキーワード text だけの条件 (URL のクエリの形)。
function keywordConditions(text: string) {
  return { keywords: [{ ...emptyKeyword(), text }] };
}

// archiveSwitch は、「過去の放送を含める」のスイッチ。
function archiveSwitch(): HTMLInputElement {
  return screen.getByRole('switch', { name: '過去の放送を含める' });
}

describe('SearchPage', () => {
  test('ヘッダーの検索から開いたら、渡した言葉で、過去の放送を含めずに探す (#1627)', async () => {
    const { router } = await renderRoute(Route, '/search', '/', createDemoEnvironment());
    await act(() =>
      router.navigate({ to: '/search', search: { conditions: keywordConditions('ニュース') } }),
    );

    expect(await screen.findByDisplayValue('ニュース')).toBeTruthy();
    expect(archiveSwitch().checked).toBe(false);
  });

  test('番組の詳細から同じ名前で探すときは、過去の放送も含める (#1121)', async () => {
    const { router } = await renderRoute(Route, '/search', '/', createDemoEnvironment());
    await act(() =>
      router.navigate({
        to: '/search',
        search: { conditions: keywordConditions('ニュース'), archive: 30 },
      }),
    );

    expect(await screen.findByDisplayValue('ニュース')).toBeTruthy();
    expect(archiveSwitch().checked).toBe(true);
  });

  test('番組検索の画面のまま探し直したら、新しい条件で始め直す (#1627)', async () => {
    const { router } = await renderRoute(Route, '/search', '/', createDemoEnvironment());
    await act(() =>
      router.navigate({
        to: '/search',
        search: { conditions: keywordConditions('ニュース'), archive: 30 },
      }),
    );
    expect(await screen.findByDisplayValue('ニュース')).toBeTruthy();
    expect(archiveSwitch().checked).toBe(true);

    await act(() =>
      router.navigate({ to: '/search', search: { conditions: keywordConditions('アニメ') } }),
    );

    expect(await screen.findByDisplayValue('アニメ')).toBeTruthy();
    expect(screen.queryByDisplayValue('ニュース')).toBeNull();
    expect(archiveSwitch().checked).toBe(false);
  });

  test('打った条件で、URL を置き換える (#2146)', async () => {
    const { router } = await renderRoute(Route, '/search', '/search', createDemoEnvironment());
    const historyLength = router.history.length;

    fireEvent.change(await screen.findByRole('textbox', { name: '含めるキーワード' }), {
      target: { value: 'ドラマ' },
    });

    await waitFor(() =>
      expect(router.state.location.search).toEqual({ conditions: keywordConditions('ドラマ') }),
    );
    // 打つたびに履歴を積まない。
    expect(router.history.length).toBe(historyLength);
  });

  test('URL の条件と過去の放送の日数で、探し始める (#2146)', async () => {
    await renderRoute(
      Route,
      '/search',
      `/search?${new URLSearchParams({
        conditions: JSON.stringify(keywordConditions('ニュース')),
        archive: '7',
      })}`,
      createDemoEnvironment(),
    );

    expect(await screen.findByDisplayValue('ニュース')).toBeTruthy();
    expect(archiveSwitch().checked).toBe(true);
    expect(screen.getByRole('textbox', { name: '過去の放送を探す日数' })).toHaveProperty(
      'value',
      '直近7日',
    );
  });
});
