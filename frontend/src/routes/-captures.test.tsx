import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { createDemoEnvironment, renderRoute } from '../test/route';
import { Route } from './captures';

afterEach(() => {
  cleanup();
});

// searchInput は、キャプチャを探す欄。
function searchInput(): HTMLInputElement {
  return screen.getByRole('textbox', { name: '番組名、サービス名、字幕で探す' });
}

describe('CapturesPage', () => {
  test('URL の語で探し始める (#2146)', async () => {
    await renderRoute(Route, '/captures', '/captures?q=ニュース', createDemoEnvironment());

    expect(searchInput().value).toBe('ニュース');
  });

  test('打った語で URL を置き換え、語を消したら URL から外す (#2146)', async () => {
    const { router } = await renderRoute(Route, '/captures', '/captures', createDemoEnvironment());

    act(() => fireEvent.change(searchInput(), { target: { value: 'アニメ' } }));
    await act(() => router.load());
    expect(router.state.location.search).toEqual({ q: 'アニメ' });

    act(() => fireEvent.change(searchInput(), { target: { value: ' ' } }));
    await act(() => router.load());
    expect(router.state.location.search).toEqual({});
    // 空白を打った途中の値は、入力に残す。
    expect(searchInput().value).toBe(' ');
  });

  test('入力の外で URL の語が変わったら、入力を合わせる (#2146)', async () => {
    const { router } = await renderRoute(
      Route,
      '/captures',
      '/captures?q=ニュース',
      createDemoEnvironment(),
    );

    await act(() => router.navigate({ to: '/captures', search: {} }));

    expect(searchInput().value).toBe('');
  });
});
