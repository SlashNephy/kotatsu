import { act, cleanup, fireEvent, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createDemoEnvironment, renderRoute } from '../test/route';
import { Route } from './reservations';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// rowOf は、予約の操作のボタンを含む、予約の行を返す。
function rowOf(button: HTMLElement): HTMLElement {
  return button.closest<HTMLElement>('[data-state]')!;
}

// firstRow は、操作できる先頭の予約の行を返す。
function firstRow(): HTMLElement {
  return rowOf(screen.getAllByRole('button', { name: '予約の操作' })[0]!);
}

describe('予約の一覧', () => {
  test('行を個別に無効にした後に、まとめて有効にすると、その行も有効の表示になる (#1425)', async () => {
    await renderRoute(Route, '/reservations', '/reservations', createDemoEnvironment());
    const actions = await screen.findAllByRole('button', { name: '予約の操作' });
    const row = rowOf(actions[0]!);
    expect(row.dataset.disabled).toBeUndefined();

    fireEvent.click(actions[0]!);
    fireEvent.click(await screen.findByRole('menuitem', { name: '無効にする' }));
    await waitFor(() => expect(firstRow().dataset.disabled).toBe('true'));

    fireEvent.click(screen.getByRole('button', { name: '選ぶ' }));
    fireEvent.click(within(firstRow()).getByRole('checkbox'));
    await act(async () =>
      fireEvent.click(screen.getByRole('button', { name: '選んだ1件を有効にする' })),
    );

    await waitFor(() => expect(firstRow().dataset.disabled).toBeUndefined());
  });
});
