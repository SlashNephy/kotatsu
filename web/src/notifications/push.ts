// この端末の Web Push の購読 (設計書 6.2)。

// base64urlToBytes は、VAPID の公開鍵 (base64url) を、applicationServerKey に渡すバイト列にする。
export function base64urlToBytes(value: string): Uint8Array<ArrayBuffer> {
  const base64 = value
    .replaceAll('-', '+')
    .replaceAll('_', '/')
    .padEnd(Math.ceil(value.length / 4) * 4, '=');
  const binary = atob(base64);
  const bytes = new Uint8Array(new ArrayBuffer(binary.length));
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }

  return bytes;
}

// pushSupported は、この端末で Web Push を使えるかどうかを返す。
export function pushSupported(): boolean {
  return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;
}

// currentSubscription は、この端末の購読を返す。購読していなければ null。
export async function currentSubscription(): Promise<PushSubscription | null> {
  const registration = await navigator.serviceWorker.ready;

  return registration.pushManager.getSubscription();
}

export type SubscriptionKeys = { endpoint: string; p256dh: string; auth: string };

// subscribe は、通知の許可を求めてから、この端末で購読する。許可されなければ、エラーにする。
export async function subscribe(publicKey: string): Promise<SubscriptionKeys> {
  if ((await Notification.requestPermission()) !== 'granted') {
    throw new Error('notification permission was not granted');
  }
  const registration = await navigator.serviceWorker.ready;
  const subscription = await registration.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: base64urlToBytes(publicKey),
  });
  const json = subscription.toJSON();
  const p256dh = json.keys?.p256dh;
  const auth = json.keys?.auth;
  if (json.endpoint === undefined || p256dh === undefined || auth === undefined) {
    throw new Error('the push subscription has no keys');
  }

  return { endpoint: json.endpoint, p256dh, auth };
}
