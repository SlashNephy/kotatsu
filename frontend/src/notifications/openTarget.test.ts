import { describe, expect, test, vi } from 'vitest';

import {
  openNotificationTarget,
  type NotificationClients,
  type NotificationWindow,
} from './openTarget';

// controlled は、Service Worker の制御下にあるタブ。navigate で移り、自分を返す。
function controlled(): NotificationWindow & { focus: ReturnType<typeof vi.fn> } {
  const window = {
    focus: vi.fn(() => Promise.resolve(window)),
    navigate: vi.fn(() => Promise.resolve(window)),
  };

  return window;
}

// uncontrolled は、制御下にないタブ。Chromium と同じく、navigate を TypeError で断る。
function uncontrolled(): NotificationWindow & { focus: ReturnType<typeof vi.fn> } {
  return {
    focus: vi.fn(() => Promise.resolve(null)),
    navigate: vi.fn(() =>
      Promise.reject(
        new TypeError("This service worker is not the client's active service worker."),
      ),
    ),
  };
}

function clientsOf(windows: NotificationWindow[]) {
  return {
    matchAll: vi.fn(() => Promise.resolve(windows)),
    openWindow: vi.fn(() => Promise.resolve(null)),
  } satisfies NotificationClients;
}

describe('openNotificationTarget', () => {
  test('制御下にないタブしかなければ、新しいタブで開く (#1431)', async () => {
    const tab = uncontrolled();
    const clients = clientsOf([tab]);

    await openNotificationTarget(clients, '/live/Service:1');

    expect(clients.openWindow).toHaveBeenCalledWith('/live/Service:1');
  });

  test('制御下にないタブの後に、制御下のタブがあれば、そのタブで移って前に出す (#1431)', async () => {
    const first = uncontrolled();
    const second = controlled();
    const clients = clientsOf([first, second]);

    await openNotificationTarget(clients, '/live/Service:1');

    expect(second.navigate).toHaveBeenCalledWith('/live/Service:1');
    expect(second.focus).toHaveBeenCalledOnce();
    expect(clients.openWindow).not.toHaveBeenCalled();
  });

  test('開いているタブがなければ、新しいタブで開く', async () => {
    const clients = clientsOf([]);

    await openNotificationTarget(clients, '/notifications');

    expect(clients.openWindow).toHaveBeenCalledWith('/notifications');
  });
});
