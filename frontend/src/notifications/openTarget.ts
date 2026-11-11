// 通知を押したときに、目的の画面を開く (設計書 6.2)。Service Worker (sw.ts) から呼ぶ。

// NotificationWindow は、Service Worker から見たブラウザのタブ (WindowClient) のうち、ここで使うもの。
export type NotificationWindow = {
  focus: () => Promise<unknown>;
  // navigate は、移ったタブを返す。別のオリジンへ移ったときは null を返す。
  navigate: (url: string) => Promise<NotificationWindow | null>;
};

// NotificationClients は、Service Worker の clients のうち、ここで使うもの。
export type NotificationClients = {
  matchAll: (options: {
    type: 'window';
    includeUncontrolled: boolean;
  }) => Promise<readonly NotificationWindow[]>;
  openWindow: (url: string) => Promise<unknown>;
};

// openNotificationTarget は、開いているアプリのタブで target へ移り、そのタブを前に出す。移れるタブがなければ、新しいタブで開く。
// Service Worker の制御下にないタブ (初めて開いて、まだ読み直していないタブ) では、navigate が TypeError で断られるので、
// ほかのタブを試し、どれも移れなければ新しいタブで開く (#1431)。
export async function openNotificationTarget(
  clients: NotificationClients,
  target: string,
): Promise<void> {
  const windows = await clients.matchAll({ type: 'window', includeUncontrolled: true });
  for (const window of windows) {
    let navigated: NotificationWindow | null;
    try {
      navigated = await window.navigate(target);
    } catch {
      // 制御下にないタブ。次のタブを試す。
      continue;
    }
    if (navigated !== null) {
      // 移れたら、前に出せなくても、ほかのタブでは開き直さない。
      await navigated.focus().catch(() => {});

      return;
    }
  }
  await clients.openWindow(target);
}
