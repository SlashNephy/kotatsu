import { act, cleanup, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { emptyConditions, emptyKeyword, type ConditionsForm } from '../conditions/form';
import { createDemoEnvironment, renderRoute } from '../test/route';
import { Route } from './search';

afterEach(() => {
  cleanup();
});

// keywordConditions は、番組名のキーワード text だけの条件。
function keywordConditions(text: string): ConditionsForm {
  return { ...emptyConditions(), keywords: [{ ...emptyKeyword(), text }] };
}

// archiveSwitch は、「過去の放送を含める」のスイッチ。
function archiveSwitch(): HTMLInputElement {
  return screen.getByRole('switch', { name: '過去の放送を含める' });
}

describe('SearchPage', () => {
  test('ヘッダーの検索から開いたら、渡した言葉で、過去の放送を含めずに探す (#1627)', async () => {
    const { router } = await renderRoute(Route, '/search', '/', createDemoEnvironment());
    await act(() =>
      router.navigate({
        to: '/search',
        state: { searchConditions: keywordConditions('ニュース'), searchArchive: false },
      }),
    );

    expect(await screen.findByDisplayValue('ニュース')).toBeTruthy();
    expect(archiveSwitch().checked).toBe(false);
  });

  test('番組の詳細から同じ名前で探すときは、過去の放送も含める (#1121)', async () => {
    const { router } = await renderRoute(Route, '/search', '/', createDemoEnvironment());
    await act(() =>
      router.navigate({
        to: '/search',
        state: { searchConditions: keywordConditions('ニュース') },
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
        state: { searchConditions: keywordConditions('ニュース') },
      }),
    );
    expect(await screen.findByDisplayValue('ニュース')).toBeTruthy();
    expect(archiveSwitch().checked).toBe(true);

    await act(() =>
      router.navigate({
        to: '/search',
        state: { searchConditions: keywordConditions('アニメ'), searchArchive: false },
      }),
    );

    expect(await screen.findByDisplayValue('アニメ')).toBeTruthy();
    expect(screen.queryByDisplayValue('ニュース')).toBeNull();
    expect(archiveSwitch().checked).toBe(false);
  });
});
