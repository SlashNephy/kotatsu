// ログアウトの前に、この端末に残るものを片付ける (設計書 6.2、7.8、#1365)。

// settleTimeout は、片付けを待つ時間の上限 (ミリ秒)。サーバーに届かなくても、ログアウトを止めない。
const settleTimeout = 3000;

function withTimeout(task: Promise<unknown>): Promise<unknown> {
  return Promise.race([
    task.catch(() => {}),
    new Promise((resolve) => setTimeout(resolve, settleTimeout)),
  ]);
}

// devicePushSubscription は、この端末の購読を返す。Service Worker がなければ待たずに null を返す
// (navigator.serviceWorker.ready は、登録がないと解決しない)。
async function devicePushSubscription(): Promise<PushSubscription | null> {
  if (!('serviceWorker' in navigator)) {
    return null;
  }
  const registration = await navigator.serviceWorker.getRegistration();

  return (await registration?.pushManager.getSubscription()) ?? null;
}

// unsubscribeDevicePush は、この端末の Web Push の購読を、サーバーとブラウザの両方で解除する。
// サーバーの購読を消すにはログイン中のセッションが要るので、ログアウトの前に呼ぶ。失敗してもログアウトを止めない。
export async function unsubscribeDevicePush(
  unsubscribeServer: (endpoint: string) => Promise<unknown>,
): Promise<void> {
  const subscription = await devicePushSubscription().catch(() => null);
  if (subscription === null) {
    return;
  }
  await withTimeout(unsubscribeServer(subscription.endpoint));
  await withTimeout(subscription.unsubscribe());
}
