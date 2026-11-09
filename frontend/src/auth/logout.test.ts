import { afterEach, describe, expect, it, vi } from 'vitest';

import { unsubscribeDevicePush } from './logout';

function stubServiceWorker(
  subscription: { endpoint: string; unsubscribe: () => Promise<boolean> } | null,
) {
  vi.stubGlobal('navigator', {
    serviceWorker: {
      getRegistration: () =>
        Promise.resolve({ pushManager: { getSubscription: () => Promise.resolve(subscription) } }),
    },
  });
}

describe('unsubscribeDevicePush', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('サーバーの購読を消してから、ブラウザの購読を解除する', async () => {
    const calls: string[] = [];
    stubServiceWorker({
      endpoint: 'https://push.example/1',
      unsubscribe: () => {
        calls.push('browser');

        return Promise.resolve(true);
      },
    });

    await unsubscribeDevicePush((endpoint) => {
      calls.push(`server:${endpoint}`);

      return Promise.resolve();
    });

    expect(calls).toEqual(['server:https://push.example/1', 'browser']);
  });

  it('購読がなければ、サーバーに問い合わせない', async () => {
    stubServiceWorker(null);
    const server = vi.fn(() => Promise.resolve());

    await unsubscribeDevicePush(server);

    expect(server).not.toHaveBeenCalled();
  });

  it('サーバーが失敗しても、ブラウザの購読は解除する', async () => {
    const unsubscribe = vi.fn(() => Promise.resolve(true));
    stubServiceWorker({ endpoint: 'https://push.example/1', unsubscribe });

    await unsubscribeDevicePush(() => Promise.reject(new Error('offline')));

    expect(unsubscribe).toHaveBeenCalledOnce();
  });

  it('サーバーが応答しなくても、待ち続けない', async () => {
    vi.useFakeTimers();
    const unsubscribe = vi.fn(() => Promise.resolve(true));
    stubServiceWorker({ endpoint: 'https://push.example/1', unsubscribe });

    const done = unsubscribeDevicePush(() => new Promise(() => {}));
    await vi.advanceTimersByTimeAsync(3000);
    await done;

    expect(unsubscribe).toHaveBeenCalledOnce();
  });

  it('Service Worker を使えない端末では、何もしない', async () => {
    vi.stubGlobal('navigator', {});
    const server = vi.fn(() => Promise.resolve());

    await unsubscribeDevicePush(server);

    expect(server).not.toHaveBeenCalled();
  });
});
