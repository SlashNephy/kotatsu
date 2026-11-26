import { cleanup, fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { createDemoEnvironment, renderRoute } from '../../test/route';
import { Route } from './logs';

afterEach(() => {
  cleanup();
});

describe('LogsPage (#2146)', () => {
  test('URL の絞り方で始める', async () => {
    await renderRoute(
      Route,
      '/admin/logs',
      `/admin/logs?${new URLSearchParams({ q: 'error', problems: 'true' })}`,
      createDemoEnvironment(),
    );

    expect(
      (await screen.findByRole<HTMLInputElement>('textbox', { name: 'ログを検索' })).value,
    ).toBe('error');
    expect(screen.getByRole<HTMLInputElement>('switch', { name: '警告とエラーだけ' }).checked).toBe(
      true,
    );
  });

  test('絞り方を変えたら、URL を置き換える', async () => {
    const { router } = await renderRoute(
      Route,
      '/admin/logs',
      '/admin/logs',
      createDemoEnvironment(),
    );
    const historyLength = router.history.length;

    fireEvent.click(await screen.findByRole('switch', { name: '警告とエラーだけ' }));
    await waitFor(() => expect(router.state.location.search).toEqual({ problems: true }));

    fireEvent.change(screen.getByRole('textbox', { name: 'ログを検索' }), {
      target: { value: 'tuner' },
    });
    await waitFor(() =>
      expect(router.state.location.search).toEqual({ problems: true, q: 'tuner' }),
    );
    expect(router.history.length).toBe(historyLength);
  });
});
