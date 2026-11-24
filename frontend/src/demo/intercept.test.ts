import { afterEach, describe, expect, it, vi } from 'vitest';

import { interceptFetch } from '.';
import { createDemoCaptures } from './captures';
import type { Executor } from './executor';

function fakeExecutor(name: string): Executor {
  return {
    execute: () => Promise.resolve({ data: { name } }),
    subscribe: () => Promise.reject(new Error('not used')),
  };
}

async function graphql(): Promise<unknown> {
  const response = await window.fetch(`${window.location.origin}/internal/graphql`, {
    method: 'POST',
    body: JSON.stringify({ query: '{ name }' }),
  });

  return response.json();
}

// Storybook (#2056) は Story を同時にいくつも開き、開いた順と逆に閉じるとは限らない。
describe('interceptFetch', () => {
  const original = vi.fn(() => Promise.resolve(new Response('original')));
  window.fetch = original;

  afterEach(() => {
    window.fetch = original;
  });

  it('最後に入れた応答で応え、どの順で戻しても、すべて戻せば元の fetch に戻す', async () => {
    const restoreA = interceptFetch(fakeExecutor('a'), createDemoCaptures());
    const restoreB = interceptFetch(fakeExecutor('b'), createDemoCaptures());

    expect(await graphql()).toEqual({ data: { name: 'b' } });
    restoreB();
    expect(await graphql()).toEqual({ data: { name: 'a' } });
    restoreA();
    expect(window.fetch).toBe(original);
  });

  it('先に入れたものを先に戻しても、残ったものの応答で応える', async () => {
    const restoreA = interceptFetch(fakeExecutor('a'), createDemoCaptures());
    const restoreB = interceptFetch(fakeExecutor('b'), createDemoCaptures());

    restoreA();
    expect(await graphql()).toEqual({ data: { name: 'b' } });
    restoreB();
    expect(window.fetch).toBe(original);
  });
});
