import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { useEntryState } from './useEntryState';

afterEach(() => {
  cleanup();
});

// Counter は、押した回数を履歴の項目に残す画面。
function Counter() {
  const [count, setCount] = useEntryState('count', 0);

  return (
    <button type="button" onClick={() => setCount((value) => value + 1)}>
      {`count ${count}`}
    </button>
  );
}

// mount は、数える画面 (/captures) と、ほかの画面 (/notifications) のルーターを描く。
async function mount() {
  const root = createRootRoute({ component: Outlet });
  const router = createRouter({
    routeTree: root.addChildren([
      createRoute({ getParentRoute: () => root, path: '/captures', component: Counter }),
      createRoute({ getParentRoute: () => root, path: '/notifications', component: () => 'other' }),
    ]),
    history: createMemoryHistory({ initialEntries: ['/captures'] }),
  });
  await act(() => router.load());
  render(<RouterProvider router={router} />);
  await screen.findByRole('button');

  return router;
}

describe('useEntryState (#2146)', () => {
  test('ほかの画面からブラウザで戻ったら、その履歴の項目の状態から始める', async () => {
    const router = await mount();
    fireEvent.click(screen.getByRole('button'));
    fireEvent.click(screen.getByRole('button'));

    await act(() => router.navigate({ to: '/notifications' }));
    await act(async () => {
      router.history.back();
      await router.load();
    });

    expect((await screen.findByRole('button')).textContent).toBe('count 2');
  });

  test('新しく開いたときは、初めの値から始める', async () => {
    const router = await mount();
    fireEvent.click(screen.getByRole('button'));

    await act(() => router.navigate({ to: '/notifications' }));
    await act(() => router.navigate({ to: '/captures' }));

    expect((await screen.findByRole('button')).textContent).toBe('count 0');
  });

  test('URL を置き換えて履歴の項目が変わっても、戻ったときに今の状態から始める', async () => {
    const router = await mount();
    fireEvent.click(screen.getByRole('button'));
    await act(() => router.navigate({ to: '/captures', search: { q: 'a' }, replace: true }));
    fireEvent.click(screen.getByRole('button'));

    await act(() => router.navigate({ to: '/notifications' }));
    await act(async () => {
      router.history.back();
      await router.load();
    });

    expect((await screen.findByRole('button')).textContent).toBe('count 2');
  });
});
